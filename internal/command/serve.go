package command

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/meridian-labs/meridian/internal/handler"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/storage"
	"github.com/meridian-labs/meridian/internal/task"
	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	var addr string
	var databaseURL string
	var insecureCookies bool

	command := &cobra.Command{
		Use:   "serve",
		Short: "Start the Meridian HTTP server",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runServer(command.Context(), addr, databaseURL, os.Getenv("MERIDIAN_TOKEN_PEPPER"), !insecureCookies, command.ErrOrStderr())
		},
	}
	command.Flags().StringVar(&addr, "addr", ":8080", "HTTP listen address")
	command.Flags().StringVar(&databaseURL, "database-url", os.Getenv("MERIDIAN_DATABASE_URL"), "PostgreSQL connection URL (or MERIDIAN_DATABASE_URL)")
	command.Flags().BoolVar(&insecureCookies, "insecure-cookies", false, "allow refresh cookies over HTTP for loopback development")
	return command
}

func runServer(ctx context.Context, addr, databaseURL, encodedPepper string, secureCookies bool, output io.Writer) (err error) {
	logger := slog.New(slog.NewJSONHandler(output, nil))
	if err := handler.ConfigureTracer(); err != nil {
		return err
	}
	if !secureCookies && !isLoopbackAddress(addr) {
		return errors.New("--insecure-cookies requires an explicit loopback --addr")
	}
	digester, err := service.NewTokenDigester(encodedPepper)
	if err != nil {
		return err
	}
	jwtIssuer, err := service.NewJWTIssuer(os.Getenv("MERIDIAN_JWT_SIGNING_KEY"))
	if err != nil {
		return err
	}
	keyring, err := credentialKeyringFromEnvironment()
	if err != nil {
		return err
	}
	db, err := openDatabase(ctx, databaseURL, output)
	if err != nil {
		return err
	}
	defer closeDatabase(db, &err)
	if err := db.MigrateUp(ctx); err != nil {
		return err
	}

	identityStore := repository.NewIdentityStore(db.Pool)
	identity := service.NewIdentity(identityStore, digester, jwtIssuer)
	repositoryStore := repository.NewRepositoryStore(db.Pool)
	discoveryStore := repository.NewDiscoveryStoreWithRiver(db.Pool, nil)
	serviceLifecycleStore := repository.NewServiceLifecycleStoreWithRiver(db.Pool, nil)
	workspaceRoot := os.Getenv("MERIDIAN_WORKSPACE_ROOT")
	if workspaceRoot == "" {
		workspaceRoot = filepath.Join(os.TempDir(), "meridian-workspace")
	}
	blobRoot := os.Getenv("MERIDIAN_BLOB_ROOT")
	if blobRoot == "" {
		blobRoot = filepath.Join(os.TempDir(), "meridian-blobs")
	}
	blobStore, err := storage.NewLocalStore(blobRoot)
	if err != nil {
		return fmt.Errorf("configure content blob store: %w", err)
	}
	assetStore := repository.NewAssetStore(db.Pool)
	syncRunner := service.NewPipelineRunner(assetStore, blobStore, workspaceRoot)
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions:     repositoryStore,
		SyncRunner:     syncRunner,
		DiscoverRunner: service.NewDiscoveryRunner(discoveryStore, workspaceRoot),
		Outbox:         repositoryStore,
	}, logger)
	if err != nil {
		return fmt.Errorf("configure River runtime: %w", err)
	}
	discoveryStore.BindRiver(runtime.Client())
	serviceLifecycleStore.BindRiver(runtime.Client())
	credentials := service.NewCredentials(repository.NewCredentialStoreWithRiver(db.Pool, runtime.Client()), identityStore, keyring)
	repositories := service.NewRepositories(repositoryStore, identityStore)
	jobs := service.NewJobs(repository.NewJobControlStore(db.Pool, runtime.Client()), identityStore)
	audits := service.NewAudits(repositoryStore, identityStore)
	producers := service.NewProducers(discoveryStore, identityStore)
	discovery := service.NewDiscovery(discoveryStore, identityStore)
	assets := service.NewAssets(assetStore, identityStore)
	views := service.NewViews(assetStore, identityStore)
	discovery.WithAssets(assets)
	serviceLifecycle := service.NewServiceLifecycle(serviceLifecycleStore, assets, identityStore)
	server := &http.Server{
		Addr: addr,
		Handler: handler.NewWithRuntimeServices(handler.Dependencies{
			Identity: identity, Credentials: credentials, Repositories: repositories, Jobs: jobs, Audits: audits,
			Producers: producers, Discovery: discovery, Assets: assets, Views: views, ServiceLifecycle: serviceLifecycle,
		}, secureCookies).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0, // SSE responses manage their lifetime through request cancellation.
		IdleTimeout:       60 * time.Second,
	}
	if err := runtime.Start(ctx); err != nil {
		return fmt.Errorf("start River runtime: %w", err)
	}
	defer func() {
		stopContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if stopErr := runtime.Stop(stopContext); stopErr != nil && err == nil {
			err = fmt.Errorf("River runtime shutdown failed: %w", stopErr)
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("meridian listening", "addr", addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server stopped: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http server shutdown failed: %w", err)
		}
		return nil
	}
}

// credentialKeyringFromEnvironment parses the immutable credential encryption
// configuration used by the HTTP process. Secrets are never included in errors.
func credentialKeyringFromEnvironment() (service.CredentialKeyring, error) {
	activeVersionText := strings.TrimSpace(os.Getenv("MERIDIAN_MASTER_KEY_VERSION"))
	activeVersion, err := strconv.ParseInt(activeVersionText, 10, 32)
	if err != nil || activeVersion < 1 {
		return service.CredentialKeyring{}, errors.New("MERIDIAN_MASTER_KEY_VERSION must be a positive integer")
	}
	previous := make(map[int32]string)
	previousFile := strings.TrimSpace(os.Getenv("MERIDIAN_MASTER_KEY_PREVIOUS_FILE"))
	if previousFile != "" {
		encoded, err := os.ReadFile(previousFile)
		if err != nil {
			return service.CredentialKeyring{}, fmt.Errorf("read MERIDIAN_MASTER_KEY_PREVIOUS_FILE: %w", err)
		}
		var values map[string]string
		if err := json.Unmarshal(encoded, &values); err != nil {
			return service.CredentialKeyring{}, fmt.Errorf("decode MERIDIAN_MASTER_KEY_PREVIOUS_FILE: %w", err)
		}
		for versionText, key := range values {
			version, err := strconv.ParseInt(versionText, 10, 32)
			if err != nil || version < 1 {
				return service.CredentialKeyring{}, errors.New("MERIDIAN_MASTER_KEY_PREVIOUS_FILE contains an invalid version")
			}
			previous[int32(version)] = key
		}
	}
	return service.NewCredentialKeyringWithPrevious(
		os.Getenv("MERIDIAN_MASTER_KEY"), int32(activeVersion), os.Getenv("MERIDIAN_CREDENTIAL_FINGERPRINT_KEY"), previous,
	)
}

func isLoopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
