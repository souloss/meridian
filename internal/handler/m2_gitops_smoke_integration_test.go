//go:build integration

package handler

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/database"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/storage"
	"github.com/meridian-labs/meridian/internal/task"
)

// TestM2GitopsSmoke exercises the M2 gitops configuration import preview/apply
// gates (SMK-028 config-import-preview-apply and SMK-038 root-dot normalization).
func TestM2GitopsSmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M2 gitops smoke requires MERIDIAN_TEST_DATABASE_URL; use make smoke-m2-gitops")
	}
	t.Run("SMK-028", smokeM2ConfigImportPreviewApply)
	t.Run("SMK-038", smokeM2ConfigImportRootDotNormalization)
}

// m2GitopsFixture wires the full M2 runtime with the gitops config import service.
type m2GitopsFixture struct {
	db        *database.Database
	handler   http.Handler
	identity  *service.Identity
	tenantID  uuid.UUID
	alice     m1SmokeSession
	platform  m1SmokeSession
	workspace string
}

func newM2GitopsFixture(t *testing.T) *m2GitopsFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M2 gitops database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M2 gitops database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M2 gitops database: %v", err)
	}
	for _, table := range []string{"config_import_previews", "recent_services", "asset_items", "asset_versions", "asset_ref_tracks", "layer_heads", "layer_revisions", "layers", "assets", "tenant_kind_overrides", "source_bindings", "source_specs", "discovery_candidates", "services", "producer_profiles", "repositories", "users", "tenants"} {
		if _, err := db.Pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("reset M2 gitops table %s: %v", table, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M2 gitops queue: %v", err)
	}

	identityStore := repository.NewIdentityStore(db.Pool)
	digester, err := service.NewTokenDigester(integrationTokenPepper)
	if err != nil {
		t.Fatalf("create token digester: %v", err)
	}
	jwtIssuer, err := service.NewJWTIssuer(integrationTokenPepper)
	if err != nil {
		t.Fatalf("construct JWT issuer: %v", err)
	}
	identity := service.NewIdentity(identityStore, digester, jwtIssuer)
	keyring, err := service.NewCredentialKeyring(base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")), 1,
		base64.RawURLEncoding.EncodeToString([]byte("12345678901234567890123456789012")))
	if err != nil {
		t.Fatalf("create credential keyring: %v", err)
	}
	admin, err := identity.BootstrapPlatformAdmin(t.Context(), service.CreateUserInput{
		Username: "padmin", DisplayName: "Platform Admin", Password: "smoke secure password",
	})
	if err != nil {
		t.Fatalf("bootstrap administrator: %v", err)
	}
	actor := service.Principal{Kind: service.PrincipalJWT, User: admin}

	repositoryStore := repository.NewRepositoryStore(db.Pool)
	discoveryStore := repository.NewDiscoveryStoreWithRiver(db.Pool, nil)
	assetStore := repository.NewAssetStore(db.Pool)
	layerStore := repository.NewLayerStore(db.Pool)
	serviceLifecycleStore := repository.NewServiceLifecycleStoreWithRiver(db.Pool, nil)
	workspace := t.TempDir()
	blobs, err := storage.NewLocalStore(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatalf("create blob store: %v", err)
	}
	layerEdit := service.NewLayerEdit(layerStore, blobs, identityStore)
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions:     repositoryStore,
		SyncRunner:     service.NewPipelineRunner(assetStore, blobs, workspace),
		DiscoverRunner: service.NewDiscoveryRunner(discoveryStore, workspace),
		MergeRunner:    layerEdit,
	}, logger)
	if err != nil {
		t.Fatalf("create M2 gitops queue runtime: %v", err)
	}
	discoveryStore.BindRiver(runtime.Client())
	serviceLifecycleStore.BindRiver(runtime.Client())
	layerStore.BindRiver(runtime.Client())

	assets := service.NewAssets(assetStore, identityStore)
	views := service.NewViews(assetStore, identityStore)
	discovery := service.NewDiscovery(discoveryStore, identityStore).WithAssets(assets)
	serviceLifecycle := service.NewServiceLifecycle(serviceLifecycleStore, assets, identityStore)
	configImport := service.NewConfigImport(discoveryStore, identityStore, workspace)

	handler := NewWithRuntimeServices(Dependencies{
		Identity:         identity,
		Credentials:      service.NewCredentials(repository.NewCredentialStoreWithRiver(db.Pool, runtime.Client()), identityStore, keyring),
		Repositories:     service.NewRepositories(repositoryStore, identityStore),
		Jobs:             service.NewJobs(repository.NewJobControlStore(db.Pool, runtime.Client()), identityStore),
		Producers:        service.NewProducers(discoveryStore, identityStore),
		Discovery:        discovery,
		Assets:           assets,
		Views:            views,
		ServiceLifecycle: serviceLifecycle,
		LayerEdit:        layerEdit,
		ConfigImport:     configImport,
	}, false).Handler()

	runtime.Start(t.Context())
	t.Cleanup(func() {
		stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runtime.Stop(stopContext)
	})

	tenant, err := identity.CreateTenant(t.Context(), actor, service.CreateTenantInput{
		Slug: "acme", DisplayName: "Acme", Quota: new(service.Quota{MaxRepositories: 20, MaxServices: 20, MaxStorageBytes: 1073741824, MaxCollectConcurrency: 2}),
	})
	if err != nil {
		t.Fatalf("create smoke tenant: %v", err)
	}
	user, err := identity.CreateUser(t.Context(), actor, service.CreateUserInput{
		Username: "alice", DisplayName: "Alice", Password: "smoke secure password",
	})
	if err != nil {
		t.Fatalf("create smoke user: %v", err)
	}
	if _, _, err := identity.PutTenantMembership(t.Context(), actor, "acme", user.ID, "tenant_admin"); err != nil {
		t.Fatalf("grant smoke tenant membership: %v", err)
	}

	f := &m2GitopsFixture{db: db, handler: handler, identity: identity, tenantID: tenant.ID, workspace: workspace}
	login := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "padmin", "password": "smoke secure password"}, nil)
	assertStatus(t, login, http.StatusOK)
	f.platform = m1SmokeSession{token: responseString(t, login, "accessToken")}
	aliceLogin := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "alice", "password": "smoke secure password"}, nil)
	assertStatus(t, aliceLogin, http.StatusOK)
	f.alice = m1SmokeSession{token: responseString(t, aliceLogin, "accessToken")}
	return f
}

func (f *m2GitopsFixture) request(t *testing.T, session *m1SmokeSession, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	requestHeaders := make(map[string]string, len(headers)+1)
	for name, value := range headers {
		requestHeaders[name] = value
	}
	if session != nil {
		requestHeaders["Authorization"] = "Bearer " + session.token
	}
	return requestJSONWithHeaders(t, f.handler, method, path, body, nil, requestHeaders)
}

// setupRepository registers the seeded repository (now carrying a config file)
// and returns its id.
func (f *m2GitopsFixture) setupRepository(t *testing.T) string {
	t.Helper()
	bare, _ := seedDiscoveryRepository(t)
	server := discoveryGitHTTPServer(t, bare)
	certificatePath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatalf("write local Git CA: %v", err)
	}
	t.Setenv("GIT_SSL_CAINFO", certificatePath)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	remote := server.URL + "/repository.git"

	check := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories:check-connection", map[string]any{"url": remote}, nil)
	assertStatus(t, check, http.StatusOK)
	created := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories", map[string]any{"url": remote, "defaultBranch": "main"}, nil)
	assertStatus(t, created, http.StatusCreated)
	return responseString(t, created, "id")
}

// smokeM2ConfigImportPreviewApply covers SMK-028: preview then apply with the
// same commit and config digest, DB remaining authoritative.
func smokeM2ConfigImportPreviewApply(t *testing.T) {
	f := newM2GitopsFixture(t)
	repositoryID := f.setupRepository(t)

	preview := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/config-imports", map[string]any{
		"refType": "branch", "ref": "main",
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, preview, http.StatusCreated)
	var previewBody struct {
		PreviewID    string `json:"previewId"`
		Commit       string `json:"commit"`
		ConfigDigest string `json:"configDigest"`
		Services     []struct {
			Slug string `json:"slug"`
		} `json:"services"`
		Sources []struct {
			Kind string `json:"kind"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &previewBody); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if previewBody.PreviewID == "" || previewBody.Commit == "" || previewBody.ConfigDigest == "" {
		t.Fatalf("preview missing required fields: %s", preview.Body.String())
	}
	if len(previewBody.Services) != 2 {
		t.Fatalf("preview services = %d, want 2", len(previewBody.Services))
	}

	apply := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/config-imports/"+previewBody.PreviewID+":apply", map[string]any{
		"configDigest": previewBody.ConfigDigest,
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, apply, http.StatusOK)
	var applyBody struct {
		Commit            string   `json:"commit"`
		ConfigDigest      string   `json:"configDigest"`
		CreatedServiceIds []string `json:"createdServiceIds"`
		SourceSpecIds     []string `json:"sourceSpecIds"`
	}
	if err := json.Unmarshal(apply.Body.Bytes(), &applyBody); err != nil {
		t.Fatalf("decode apply: %v", err)
	}
	if applyBody.Commit != previewBody.Commit || applyBody.ConfigDigest != previewBody.ConfigDigest {
		t.Fatalf("apply commit/digest mismatch with preview")
	}
	if len(applyBody.CreatedServiceIds) != 2 {
		t.Fatalf("created services = %d, want 2", len(applyBody.CreatedServiceIds))
	}
	if len(applyBody.SourceSpecIds) != 2 {
		t.Fatalf("source specs = %d, want 2", len(applyBody.SourceSpecIds))
	}
}

// smokeM2ConfigImportRootDotNormalization covers SMK-038: a config service root
// of `.` is normalized to the empty string before preview and apply.
func smokeM2ConfigImportRootDotNormalization(t *testing.T) {
	f := newM2GitopsFixture(t)
	repositoryID := f.setupRootDotRepository(t)

	preview := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/config-imports", map[string]any{
		"refType": "branch", "ref": "main",
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, preview, http.StatusCreated)
	var previewBody struct {
		Services []struct {
			Slug    string `json:"slug"`
			RootDir string `json:"rootDir"`
		} `json:"services"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &previewBody); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if len(previewBody.Services) != 1 || previewBody.Services[0].Slug != "root-service" {
		t.Fatalf("preview services = %+v, want one root-service", previewBody.Services)
	}
	if previewBody.Services[0].RootDir != "" {
		t.Fatalf("root-dot service rootDir = %q, want empty string after normalization", previewBody.Services[0].RootDir)
	}

	// Applying the normalized preview persists the root service with an empty rootDir.
	apply := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/config-imports/"+responseString(t, preview, "previewId")+":apply", map[string]any{
		"configDigest": responseString(t, preview, "configDigest"),
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, apply, http.StatusOK)
	var service struct {
		RootDir string `json:"rootDir"`
	}
	detail := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/root-service", nil, nil)
	assertStatus(t, detail, http.StatusOK)
	if err := json.Unmarshal(detail.Body.Bytes(), &service); err != nil {
		t.Fatalf("decode root service: %v", err)
	}
	if service.RootDir != "" {
		t.Fatalf("applied root service rootDir = %q, want empty string", service.RootDir)
	}
}

// setupRootDotRepository registers a repository whose configuration declares a
// single service rooted at the `.` sentinel.
func (f *m2GitopsFixture) setupRootDotRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "seed")
	bare := filepath.Join(root, "repository.git")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir root-dot repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".asset-platform.yaml"), []byte("version: 1\nservices:\n  - name: root-service\n    root: .\n    assets:\n      - kind: openapi\n        base: {mode: builtin, path: openapi.yaml}\n"), 0o644); err != nil {
		t.Fatalf("write root-dot config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "openapi.yaml"), []byte("openapi: 3.1.0\ninfo: {title: root, version: 1.0.0}\npaths: {}\n"), 0o644); err != nil {
		t.Fatalf("write root-dot openapi: %v", err)
	}
	runGit := func(arguments ...string) string {
		command := exec.Command("git", arguments...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	runGit("-C", repo, "init", "-q", "-b", "main")
	runGit("-C", repo, "-c", "user.name=smoke", "-c", "user.email=smoke@example.com", "commit", "-q", "--allow-empty", "-m", "seed")
	runGit("-C", repo, "add", "-A")
	runGit("-C", repo, "-c", "user.name=smoke", "-c", "user.email=smoke@example.com", "commit", "-q", "-m", "root-dot config")
	runGit("clone", "-q", "--bare", repo, bare)

	server := discoveryGitHTTPServer(t, bare)
	certificatePath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatalf("write local Git CA: %v", err)
	}
	t.Setenv("GIT_SSL_CAINFO", certificatePath)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	remote := server.URL + "/repository.git"

	check := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories:check-connection", map[string]any{"url": remote}, nil)
	assertStatus(t, check, http.StatusOK)
	created := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories", map[string]any{"url": remote, "defaultBranch": "main"}, nil)
	assertStatus(t, created, http.StatusCreated)
	return responseString(t, created, "id")
}

var _ = context.Background
