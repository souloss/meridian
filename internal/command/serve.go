package command

import (
	"context"
	"encoding/base64"
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

// HTTP 服务运行时配置常量。
const (
	// defaultHTTPAddress 是未指定 --addr 时的默认监听地址。
	defaultHTTPAddress = ":8080"
	// defaultWorkspaceSuffix 是未配置 MERIDIAN_WORKSPACE_ROOT 时临时目录下的工作区子目录名。
	defaultWorkspaceSuffix = "meridian-workspace"
	// defaultBlobSuffix 是未配置 MERIDIAN_BLOB_ROOT 时临时目录下的 blob 子目录名。
	defaultBlobSuffix = "meridian-blobs"
	// defaultShareSigningKey 是未配置 MERIDIAN_SHARE_SIGNING_KEY 时仅用于本地开发的固定签名密钥。
	defaultShareSigningKey = "meridian-dev-share-signing-key-fixed-32"
	// serverReadHeaderTimeout 是 HTTP 读取请求头的超时。
	serverReadHeaderTimeout = 5 * time.Second
	// serverReadTimeout 是 HTTP 读取整个请求的超时。
	serverReadTimeout = 15 * time.Second
	// serverIdleTimeout 是 HTTP 连接空闲超时。
	serverIdleTimeout = 60 * time.Second
	// runtimeShutdownTimeout 是停止 River 运行时与服务端的宽限时长。
	runtimeShutdownTimeout = 10 * time.Second
)

// 命令与环境变量配置常量。
const (
	// envTokenPepper 是令牌摘要加盐环境变量名。
	envTokenPepper = "MERIDIAN_TOKEN_PEPPER"
	// envDatabaseURL 是默认数据库连接串环境变量名。
	envDatabaseURL = "MERIDIAN_DATABASE_URL"
	// envJWTSigningKey 是 JWT 签名密钥环境变量名。
	envJWTSigningKey = "MERIDIAN_JWT_SIGNING_KEY"
	// envWorkspaceRoot 是工作区根目录环境变量名。
	envWorkspaceRoot = "MERIDIAN_WORKSPACE_ROOT"
	// envBlobRoot 是 blob 存储根目录环境变量名。
	envBlobRoot = "MERIDIAN_BLOB_ROOT"
	// envShareSigningKey 是分享链接签名密钥环境变量名。
	envShareSigningKey = "MERIDIAN_SHARE_SIGNING_KEY"
	// envMasterKeyVersion 是主密钥当前版本环境变量名。
	envMasterKeyVersion = "MERIDIAN_MASTER_KEY_VERSION"
	// envMasterKeyPreviousFile 是历史主密钥文件路径环境变量名。
	envMasterKeyPreviousFile = "MERIDIAN_MASTER_KEY_PREVIOUS_FILE"
	// envMasterKey 是当前主密钥环境变量名。
	envMasterKey = "MERIDIAN_MASTER_KEY"
	// envCredentialFingerprintKey 是凭据指纹密钥环境变量名。
	envCredentialFingerprintKey = "MERIDIAN_CREDENTIAL_FINGERPRINT_KEY"
)

// minimumMasterKeyVersion 是主密钥版本允许的最小正整数。
const minimumMasterKeyVersion = 1

// 命令包内哨兵错误。
var (
	// errInsecureCookiesRequireLoopback 表示 --insecure-cookies 必须搭配显式回环 --addr。
	errInsecureCookiesRequireLoopback = errors.New("--insecure-cookies requires an explicit loopback --addr")
	// errMasterKeyVersionInvalid 表示主密钥版本环境变量不是正整数。
	errMasterKeyVersionInvalid = errors.New("MERIDIAN_MASTER_KEY_VERSION must be a positive integer")
	// errMasterKeyPreviousVersionInvalid 表示历史密钥文件中包含非法版本号。
	errMasterKeyPreviousVersionInvalid = errors.New("MERIDIAN_MASTER_KEY_PREVIOUS_FILE contains an invalid version")
)

// newServeCommand 构造启动 Meridian HTTP 服务的子命令。
func newServeCommand() *cobra.Command {
	var addr string
	var databaseURL string
	var insecureCookies bool

	command := &cobra.Command{
		Use:   "serve",
		Short: "Start the Meridian HTTP server",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runServer(command.Context(), addr, databaseURL, os.Getenv(envTokenPepper), !insecureCookies, command.ErrOrStderr())
		},
	}
	command.Flags().StringVar(&addr, "addr", defaultHTTPAddress, "HTTP listen address")
	command.Flags().StringVar(&databaseURL, "database-url", os.Getenv(envDatabaseURL), "PostgreSQL connection URL (or MERIDIAN_DATABASE_URL)")
	command.Flags().BoolVar(&insecureCookies, "insecure-cookies", false, "allow refresh cookies over HTTP for loopback development")
	return command
}

// runServer 装配全部依赖并运行 HTTP 服务直至上下文取消或服务退出。
func runServer(ctx context.Context, addr, databaseURL, encodedPepper string, secureCookies bool, output io.Writer) (err error) {
	logger := slog.New(slog.NewJSONHandler(output, nil))
	if err := handler.ConfigureTracer(); err != nil {
		return err
	}
	if !secureCookies && !isLoopbackAddress(addr) {
		return errInsecureCookiesRequireLoopback
	}
	digester, err := service.NewTokenDigester(encodedPepper)
	if err != nil {
		return err
	}
	jwtIssuer, err := service.NewJWTIssuer(os.Getenv(envJWTSigningKey))
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
	workspaceRoot := os.Getenv(envWorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = filepath.Join(os.TempDir(), defaultWorkspaceSuffix)
	}
	blobRoot := os.Getenv(envBlobRoot)
	if blobRoot == "" {
		blobRoot = filepath.Join(os.TempDir(), defaultBlobSuffix)
	}
	blobStore, err := storage.NewLocalStore(blobRoot)
	if err != nil {
		return fmt.Errorf("configure content blob store: %w", err)
	}
	assetStore := repository.NewAssetStore(db.Pool)
	layerStore := repository.NewLayerStore(db.Pool)
	aiStore := repository.NewAIStore(db.Pool)
	diffStore := repository.NewDiffStore(db.Pool)
	systemGroupStore := repository.NewSystemGroupStore(db.Pool)
	notificationStore := repository.NewNotificationStore(db.Pool)
	syncRunner := service.NewPipelineRunner(assetStore, blobStore, workspaceRoot)
	layerEdit := service.NewLayerEdit(layerStore, blobStore, identityStore)
	aiWorkflow := service.NewAiWorkflow(aiStore, blobStore, identityStore)
	shareKey, _ := base64.RawURLEncoding.DecodeString(strings.TrimSpace(os.Getenv(envShareSigningKey)))
	if len(shareKey) == 0 {
		shareKey = []byte(defaultShareSigningKey)
	}
	diffService := service.NewDiffService(diffStore, blobStore, identityStore, shareKey)
	searchService := service.NewSearch(assetStore, systemGroupStore, identityStore)
	systemGroups := service.NewSystemGroups(systemGroupStore, identityStore)
	notifications := service.NewNotifications(notificationStore, identityStore, keyring)
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions:       repositoryStore,
		SyncRunner:       syncRunner,
		DiscoverRunner:   service.NewDiscoveryRunner(discoveryStore, workspaceRoot),
		MergeRunner:      layerEdit,
		AiGenerateRunner: aiWorkflow,
		Outbox:           repositoryStore,
		OutboxDeliverer:  service.NewWebhookDeliverer(keyring),
	}, logger)
	if err != nil {
		return fmt.Errorf("configure River runtime: %w", err)
	}
	discoveryStore.BindRiver(runtime.Client())
	serviceLifecycleStore.BindRiver(runtime.Client())
	layerStore.BindRiver(runtime.Client())
	aiStore.BindRiver(runtime.Client())
	diffStore.BindRiver(runtime.Client())
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
	configImport := service.NewConfigImport(discoveryStore, identityStore, workspaceRoot)
	server := &http.Server{
		Addr: addr,
		Handler: handler.NewWithRuntimeServices(handler.Dependencies{
			Identity: identity, Credentials: credentials, Repositories: repositories, Jobs: jobs, Audits: audits,
			Producers: producers, Discovery: discovery, Assets: assets, Views: views, ServiceLifecycle: serviceLifecycle,
			LayerEdit: layerEdit, ConfigImport: configImport, AiWorkflow: aiWorkflow, DiffService: diffService,
			Search: searchService, SystemGroups: systemGroups, Notifications: notifications,
		}, secureCookies).Handler(),
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      0, // SSE 响应自行管理生命周期，不设写超时。
		IdleTimeout:       serverIdleTimeout,
	}
	if err := runtime.Start(ctx); err != nil {
		return fmt.Errorf("start River runtime: %w", err)
	}
	defer func() {
		stopContext, cancel := context.WithTimeout(context.Background(), runtimeShutdownTimeout)
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), runtimeShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http server shutdown failed: %w", err)
		}
		return nil
	}
}

// credentialKeyringFromEnvironment 解析 HTTP 进程使用的不可变凭据加密配置。
// 机密信息永不包含在错误中。
func credentialKeyringFromEnvironment() (service.CredentialKeyring, error) {
	activeVersionText := strings.TrimSpace(os.Getenv(envMasterKeyVersion))
	activeVersion, err := strconv.ParseInt(activeVersionText, 10, 32)
	if err != nil || activeVersion < minimumMasterKeyVersion {
		return service.CredentialKeyring{}, errMasterKeyVersionInvalid
	}
	previous := make(map[int32]string)
	previousFile := strings.TrimSpace(os.Getenv(envMasterKeyPreviousFile))
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
			if err != nil || version < minimumMasterKeyVersion {
				return service.CredentialKeyring{}, errMasterKeyPreviousVersionInvalid
			}
			previous[int32(version)] = key
		}
	}
	return service.NewCredentialKeyringWithPrevious(
		os.Getenv(envMasterKey), int32(activeVersion), os.Getenv(envCredentialFingerprintKey), previous,
	)
}

// isLoopbackAddress 判断监听地址是否仅绑定到本机回环接口。
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
