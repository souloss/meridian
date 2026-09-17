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

// TestM1PipelineSmoke exercises the M1 golden path: repository sync materializes
// builtin OpenAPI sources into assets, layers, revisions, versions, items, and
// source bindings, then serves the asset/viewer read endpoints.
func TestM1PipelineSmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M1 pipeline smoke requires MERIDIAN_TEST_DATABASE_URL; use make smoke-m1-golden-path")
	}
	t.Run("SMK-006", smokeM1GoldenPath)
	t.Run("SMK-007", smokeM1ImmediateNoop)
	t.Run("SMK-008", smokeM1PathSelfHealing)
	t.Run("SMK-009", smokeM1GlobThreeBindings)
	t.Run("SMK-010", smokeM1ResolveViewModes)
}

// m1PipelineFixture wires the full M1 runtime: discovery + asset read + the
// real PipelineRunner as the sync runner, plus a local blob store.
type m1PipelineFixture struct {
	db        *database.Database
	handler   http.Handler
	identity  *service.Identity
	platform  m1SmokeSession
	alice     m1SmokeSession
	tenantID  uuid.UUID
	workspace string
	blobs     *storage.LocalStore
}

func newM1PipelineFixture(t *testing.T) *m1PipelineFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M1 pipeline database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M1 pipeline database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M1 pipeline database: %v", err)
	}
	for _, table := range []string{"recent_services", "asset_items", "asset_versions", "asset_ref_tracks", "layer_heads", "layer_revisions", "layers", "assets", "tenant_kind_overrides", "source_bindings", "source_specs", "discovery_candidates", "services", "producer_profiles", "repositories", "users", "tenants"} {
		if _, err := db.Pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("reset M1 pipeline table %s: %v", table, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M1 pipeline queue: %v", err)
	}

	store := repository.NewIdentityStore(db.Pool)
	digester, err := service.NewTokenDigester(integrationTokenPepper)
	if err != nil {
		t.Fatalf("create token digester: %v", err)
	}
	jwtIssuer, err := service.NewJWTIssuer(integrationTokenPepper)
	if err != nil {
		t.Fatalf("construct JWT issuer: %v", err)
	}
	identity := service.NewIdentity(store, digester, jwtIssuer)
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
	serviceLifecycleStore := repository.NewServiceLifecycleStoreWithRiver(db.Pool, nil)
	workspace := t.TempDir()
	blobs, err := storage.NewLocalStore(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatalf("create blob store: %v", err)
	}
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions:     repositoryStore,
		SyncRunner:     service.NewPipelineRunner(assetStore, blobs, workspace),
		DiscoverRunner: service.NewDiscoveryRunner(discoveryStore, workspace),
	}, logger)
	if err != nil {
		t.Fatalf("create M1 pipeline queue runtime: %v", err)
	}
	discoveryStore.BindRiver(runtime.Client())
	serviceLifecycleStore.BindRiver(runtime.Client())

	assets := service.NewAssets(assetStore, store)
	views := service.NewViews(assetStore, store)
	discovery := service.NewDiscovery(discoveryStore, store).WithAssets(assets)
	serviceLifecycle := service.NewServiceLifecycle(serviceLifecycleStore, assets, store)

	handler := NewWithRuntimeServices(Dependencies{
		Identity:     identity,
		Credentials:  service.NewCredentials(repository.NewCredentialStoreWithRiver(db.Pool, runtime.Client()), store, keyring),
		Repositories: service.NewRepositories(repositoryStore, store),
		Jobs:         service.NewJobs(repository.NewJobControlStore(db.Pool, runtime.Client()), store),
		Producers:    service.NewProducers(discoveryStore, store),
		Discovery:    discovery,
		Assets:       assets,
		Views:        views,
		ServiceLifecycle: serviceLifecycle,
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

	f := &m1PipelineFixture{db: db, handler: handler, identity: identity, workspace: workspace, blobs: blobs, tenantID: tenant.ID}
	login := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "padmin", "password": "smoke secure password"}, nil)
	assertStatus(t, login, http.StatusOK)
	f.platform = m1SmokeSession{token: responseString(t, login, "accessToken")}
	aliceLogin := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "alice", "password": "smoke secure password"}, nil)
	assertStatus(t, aliceLogin, http.StatusOK)
	f.alice = m1SmokeSession{token: responseString(t, aliceLogin, "accessToken")}
	return f
}

func (f *m1PipelineFixture) request(t *testing.T, session *m1SmokeSession, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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

// waitJob polls one tenant job to a terminal state and returns its JSON body.
func (f *m1PipelineFixture) waitJob(t *testing.T, jobID string) map[string]any {
	t.Helper()
	var body map[string]any
	deadline := time.Now().Add(30 * time.Second)
	for {
		job := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/jobs/"+jobID, nil, nil)
		assertStatus(t, job, http.StatusOK)
		if err := json.Unmarshal(job.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode job: %v", err)
		}
		status, _ := body["status"].(string)
		if status == "succeeded" || status == "failed" || status == "cancelled" {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish: %s", job.Body.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// setupRepository clones the seeded repository over the Git HTTP fixture,
// checks the connection, registers it, discovers candidates, and returns the
// repository id and the resolved discovery commit.
func (f *m1PipelineFixture) setupRepository(t *testing.T) (string, string) {
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
	repositoryID := responseString(t, created, "id")

	discovered := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":discover", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, discovered, http.StatusAccepted)
	jobBody := f.waitJob(t, responseString(t, discovered, "jobId"))
	if jobBody["status"] != "succeeded" {
		t.Fatalf("discovery job status = %v", jobBody["status"])
	}
	commit := ""
	if result, ok := jobBody["result"].(map[string]any); ok {
		commit, _ = result["resolvedCommit"].(string)
	}
	return repositoryID, commit
}

// acceptServices accepts the order-service and pay-service candidates.
func (f *m1PipelineFixture) acceptServices(t *testing.T, repositoryID string) {
	t.Helper()
	candidates := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates", nil, nil)
	assertStatus(t, candidates, http.StatusOK)
	var candidateBody struct {
		Items []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal(candidates.Body.Bytes(), &candidateBody); err != nil {
		t.Fatalf("decode candidates: %v", err)
	}
	ids := make([]string, 0, 2)
	for _, item := range candidateBody.Items {
		if item.Path == "order-service" || item.Path == "pay-service" {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) != 2 {
		t.Fatalf("expected order-service and pay-service candidates, got %d", len(ids))
	}
	accepted := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates:accept", map[string]any{"candidateIds": ids}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, accepted, http.StatusOK)
}

// createOpenAPISource binds the builtin openapi source for a service.
func (f *m1PipelineFixture) createOpenAPISource(t *testing.T, serviceSlug, path string) string {
	t.Helper()
	body := map[string]any{"kind": "openapi", "role": "base", "origin": "repo", "mode": "builtin", "path": path}
	create := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/services/"+serviceSlug+"/sources", body, nil)
	assertStatus(t, create, http.StatusCreated)
	return responseString(t, create, "id")
}

// syncAndWait triggers a repository sync and returns the terminal job body.
func (f *m1PipelineFixture) syncAndWait(t *testing.T, repositoryID string) map[string]any {
	t.Helper()
	sync := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":sync", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, sync, http.StatusAccepted)
	return f.waitJob(t, responseString(t, sync, "jobId"))
}

func smokeM1GoldenPath(t *testing.T) {
	f := newM1PipelineFixture(t)
	repositoryID, commit := f.setupRepository(t)
	f.acceptServices(t, repositoryID)
	f.createOpenAPISource(t, "order-service", "order-service/openapi.yaml")

	jobBody := f.syncAndWait(t, repositoryID)
	if jobBody["status"] != "succeeded" {
		t.Fatalf("sync job status = %v, error = %v", jobBody["status"], jobBody["error"])
	}
	if result, ok := jobBody["result"].(map[string]any); ok {
		if got, _ := result["resolvedCommit"].(string); got != "" && commit != "" && got != commit {
			t.Fatalf("sync resolvedCommit = %q, want %q", got, commit)
		}
	}

	// order-service detail carries the openapi asset; pay-service reports openapi missing.
	orderDetail := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/order-service", nil, nil)
	assertStatus(t, orderDetail, http.StatusOK)
	var orderBody struct {
		Assets []struct {
			ID string `json:"id"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(orderDetail.Body.Bytes(), &orderBody); err != nil {
		t.Fatalf("decode order service detail: %v", err)
	}
	if len(orderBody.Assets) != 1 {
		t.Fatalf("order-service assets = %d, want 1", len(orderBody.Assets))
	}
	assetID := orderBody.Assets[0].ID

	payDetail := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/pay-service", nil, nil)
	assertStatus(t, payDetail, http.StatusOK)
	var payBody struct {
		MissingKinds []struct {
			Kind string `json:"kind"`
		} `json:"missingKinds"`
	}
	if err := json.Unmarshal(payDetail.Body.Bytes(), &payBody); err != nil {
		t.Fatalf("decode pay service detail: %v", err)
	}
	hasOpenAPI := false
	for _, kind := range payBody.MissingKinds {
		if kind.Kind == "openapi" {
			hasOpenAPI = true
		}
	}
	if !hasOpenAPI {
		t.Fatalf("pay-service missingKinds missing openapi: %+v", payBody.MissingKinds)
	}

	// getAsset resolves onto the default branch track.
	asset := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/assets/"+assetID, nil, nil)
	assertStatus(t, asset, http.StatusOK)
	var assetBody struct {
		LatestVersion struct {
			ID string `json:"id"`
		} `json:"latestVersion"`
	}
	if err := json.Unmarshal(asset.Body.Bytes(), &assetBody); err != nil {
		t.Fatalf("decode asset: %v", err)
	}
	if assetBody.LatestVersion.ID == "" {
		t.Fatalf("asset latestVersion missing: %s", asset.Body.String())
	}

	// getAssetVersion exposes the layer manifest.
	version := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/asset-versions/"+assetBody.LatestVersion.ID, nil, nil)
	assertStatus(t, version, http.StatusOK)
	var versionBody struct {
		LayerManifest []struct {
			Origin       string `json:"origin"`
			Role         string `json:"role"`
			ReviewStatus string `json:"reviewStatus"`
			ScopeType    string `json:"scopeType"`
			ScopeKey     string `json:"scopeKey"`
		} `json:"layerManifest"`
	}
	if err := json.Unmarshal(version.Body.Bytes(), &versionBody); err != nil {
		t.Fatalf("decode asset version: %v", err)
	}
	if len(versionBody.LayerManifest) != 1 {
		t.Fatalf("layerManifest length = %d, want 1", len(versionBody.LayerManifest))
	}
	entry := versionBody.LayerManifest[0]
	if entry.Origin != "repo" || entry.Role != "base" || entry.ReviewStatus != "not_required" || entry.ScopeType != "ref" || entry.ScopeKey != "branch:main" {
		t.Fatalf("layerManifest entry = %+v", entry)
	}

	// listAssetVersionItems exposes the three indexed operations.
	items := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/asset-versions/"+assetBody.LatestVersion.ID+"/items", nil, nil)
	assertStatus(t, items, http.StatusOK)
	var itemsBody struct {
		Total int `json:"total"`
		Items []struct {
			ItemType string `json:"itemType"`
		} `json:"items"`
	}
	if err := json.Unmarshal(items.Body.Bytes(), &itemsBody); err != nil {
		t.Fatalf("decode asset items: %v", err)
	}
	if itemsBody.Total != 3 {
		t.Fatalf("asset items total = %d, want 3", itemsBody.Total)
	}
	for _, item := range itemsBody.Items {
		if item.ItemType != "operation" {
			t.Fatalf("asset item type = %q, want operation", item.ItemType)
		}
	}

	// listRecentServices returns the order-service read above.
	recent := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services:recent", nil, nil)
	assertStatus(t, recent, http.StatusOK)
	var recentBody struct {
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recent.Body.Bytes(), &recentBody); err != nil {
		t.Fatalf("decode recent services: %v", err)
	}
	recentSlugs := make([]string, 0, len(recentBody.Items))
	for _, item := range recentBody.Items {
		recentSlugs = append(recentSlugs, item.Slug)
	}
	if !containsString(recentSlugs, "order-service") {
		t.Fatalf("recent services missing order-service: %v", recentSlugs)
	}
}

func smokeM1ImmediateNoop(t *testing.T) {
	f := newM1PipelineFixture(t)
	repositoryID, _ := f.setupRepository(t)
	f.acceptServices(t, repositoryID)
	f.createOpenAPISource(t, "order-service", "order-service/openapi.yaml")

	first := f.syncAndWait(t, repositoryID)
	if first["status"] != "succeeded" {
		t.Fatalf("first sync status = %v", first["status"])
	}
	// Capture the track latest version and the revision/version counts after the first sync.
	detail := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/order-service", nil, nil)
	assertStatus(t, detail, http.StatusOK)
	var detailBody struct {
		Assets []struct {
			ID            string `json:"id"`
			LatestVersion struct {
				ID string `json:"id"`
			} `json:"latestVersion"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("decode service detail: %v", err)
	}
	if len(detailBody.Assets) != 1 {
		t.Fatalf("assets = %d, want 1", len(detailBody.Assets))
	}
	beforeLatest := detailBody.Assets[0].LatestVersion.ID
	beforeFingerprint := ""
	version := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/asset-versions/"+beforeLatest, nil, nil)
	assertStatus(t, version, http.StatusOK)
	var versionBody struct {
		InputFingerprint string `json:"inputFingerprint"`
	}
	if err := json.Unmarshal(version.Body.Bytes(), &versionBody); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	beforeFingerprint = versionBody.InputFingerprint

	// Repeat an identical sync on the same ref.
	second := f.syncAndWait(t, repositoryID)
	if second["status"] != "succeeded" {
		t.Fatalf("second sync status = %v", second["status"])
	}

	after := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/order-service", nil, nil)
	assertStatus(t, after, http.StatusOK)
	if err := json.Unmarshal(after.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("decode service detail after second sync: %v", err)
	}
	afterLatest := detailBody.Assets[0].LatestVersion.ID
	if afterLatest != beforeLatest {
		t.Fatalf("track latest changed after no-op sync: %s -> %s", beforeLatest, afterLatest)
	}
	versionAfter := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/asset-versions/"+afterLatest, nil, nil)
	assertStatus(t, versionAfter, http.StatusOK)
	if err := json.Unmarshal(versionAfter.Body.Bytes(), &versionBody); err != nil {
		t.Fatalf("decode version after second sync: %v", err)
	}
	if versionBody.InputFingerprint != beforeFingerprint {
		t.Fatalf("input fingerprint changed after no-op sync")
	}
}

func smokeM1PathSelfHealing(t *testing.T) {
	f := newM1PipelineFixture(t)
	repositoryID, _ := f.setupRepository(t)
	f.acceptServices(t, repositoryID)
	sourceID := f.createOpenAPISource(t, "order-service", "order-service/openapi.yaml")

	first := f.syncAndWait(t, repositoryID)
	if first["status"] != "succeeded" {
		t.Fatalf("first sync status = %v", first["status"])
	}
	// Capture the last valid current version via the asset.
	detail := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/order-service", nil, nil)
	assertStatus(t, detail, http.StatusOK)
	var detailBody struct {
		Assets []struct {
			ID            string `json:"id"`
			LatestVersion struct {
				ID string `json:"id"`
			} `json:"latestVersion"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("decode service detail: %v", err)
	}
	assetID := detailBody.Assets[0].ID
	lastValidVersion := detailBody.Assets[0].LatestVersion.ID

	// The ETag is not exposed on listSourceBindings; fetch the source spec via
	// the service source list to obtain the revision for the patch.
	sources := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/order-service/sources", nil, nil)
	assertStatus(t, sources, http.StatusOK)
	var sourcesBody struct {
		Items []struct {
			ID   string `json:"id"`
			Etag string `json:"etag"`
		} `json:"items"`
	}
	if err := json.Unmarshal(sources.Body.Bytes(), &sourcesBody); err != nil {
		t.Fatalf("decode source list: %v", err)
	}
	etag := ""
	for _, item := range sourcesBody.Items {
		if item.ID == sourceID {
			etag = item.Etag
		}
	}
	if etag == "" {
		t.Fatalf("source %s not found in list", sourceID)
	}

	patch := f.request(t, &f.alice, http.MethodPatch, "/api/v1/t/acme/sources/"+sourceID, map[string]any{"path": "order-service/missing.yaml"}, map[string]string{"If-Match": etag})
	assertStatus(t, patch, http.StatusOK)

	failed := f.syncAndWait(t, repositoryID)
	if failed["status"] != "failed" {
		t.Fatalf("broken sync status = %v, want failed", failed["status"])
	}

	// The asset is now stale and the source carries the error.
	asset := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/assets/"+assetID, nil, nil)
	assertStatus(t, asset, http.StatusOK)
	var assetBody struct {
		Health string `json:"health"`
	}
	if err := json.Unmarshal(asset.Body.Bytes(), &assetBody); err != nil {
		t.Fatalf("decode stale asset: %v", err)
	}
	if assetBody.Health != "stale" {
		t.Fatalf("asset health = %q, want stale", assetBody.Health)
	}

	// The last valid version remains readable.
	lastVersion := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/asset-versions/"+lastValidVersion, nil, nil)
	assertStatus(t, lastVersion, http.StatusOK)

	// Restore the valid path and re-sync: error clears and health returns to ok.
	sourcesAfter := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/order-service/sources", nil, nil)
	assertStatus(t, sourcesAfter, http.StatusOK)
	if err := json.Unmarshal(sourcesAfter.Body.Bytes(), &sourcesBody); err != nil {
		t.Fatalf("decode source list after failure: %v", err)
	}
	for _, item := range sourcesBody.Items {
		if item.ID == sourceID {
			etag = item.Etag
		}
	}
	recover := f.request(t, &f.alice, http.MethodPatch, "/api/v1/t/acme/sources/"+sourceID, map[string]any{"path": "order-service/openapi.yaml"}, map[string]string{"If-Match": etag})
	assertStatus(t, recover, http.StatusOK)

	recovered := f.syncAndWait(t, repositoryID)
	if recovered["status"] != "succeeded" {
		t.Fatalf("recovery sync status = %v", recovered["status"])
	}
	assetAfter := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/assets/"+assetID, nil, nil)
	assertStatus(t, assetAfter, http.StatusOK)
	if err := json.Unmarshal(assetAfter.Body.Bytes(), &assetBody); err != nil {
		t.Fatalf("decode recovered asset: %v", err)
	}
	if assetBody.Health != "ok" {
		t.Fatalf("asset health after recovery = %q, want ok", assetBody.Health)
	}
}

func smokeM1GlobThreeBindings(t *testing.T) {
	f := newM1PipelineFixture(t)
	repositoryID, _ := f.setupRepository(t)
	f.acceptServices(t, repositoryID)

	// A glob that matches three openapi files under distinct parent directories,
	// with asset names derived from the parent directory placeholder.
	create := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "base", "origin": "repo", "mode": "builtin",
		"path": "order-service/apis/*/openapi.yaml", "assetNameTemplate": "{parent_dir}",
	}, nil)
	assertStatus(t, create, http.StatusCreated)
	sourceID := responseString(t, create, "id")
	sync := f.syncAndWait(t, repositoryID)
	if sync["status"] != "succeeded" {
		t.Fatalf("glob sync status = %v", sync["status"])
	}

	// Three matched files materialize into three bindings, one per expansion key.
	bindings := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/sources/"+sourceID+"/bindings", nil, nil)
	assertStatus(t, bindings, http.StatusOK)
	var bindingsBody struct {
		Items []struct {
			ExpansionKey string `json:"expansionKey"`
			ScopeType    string `json:"scopeType"`
			ScopeKey     string `json:"scopeKey"`
		} `json:"items"`
	}
	if err := json.Unmarshal(bindings.Body.Bytes(), &bindingsBody); err != nil {
		t.Fatalf("decode glob bindings: %v", err)
	}
	if len(bindingsBody.Items) != 3 {
		t.Fatalf("glob bindings = %d, want 3", len(bindingsBody.Items))
	}
	seenKeys := map[string]bool{}
	for _, item := range bindingsBody.Items {
		if item.ScopeType != "ref" || item.ScopeKey != "branch:main" {
			t.Fatalf("binding scope = %s:%s", item.ScopeType, item.ScopeKey)
		}
		if seenKeys[item.ExpansionKey] {
			t.Fatalf("duplicate expansion key %q", item.ExpansionKey)
		}
		seenKeys[item.ExpansionKey] = true
	}
}

func smokeM1ResolveViewModes(t *testing.T) {
	f := newM1PipelineFixture(t)
	repositoryID, _ := f.setupRepository(t)
	f.acceptServices(t, repositoryID)
	f.createOpenAPISource(t, "order-service", "order-service/openapi.yaml")
	sync := f.syncAndWait(t, repositoryID)
	if sync["status"] != "succeeded" {
		t.Fatalf("sync status = %v", sync["status"])
	}

	detail := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/order-service", nil, nil)
	assertStatus(t, detail, http.StatusOK)
	var detailBody struct {
		Assets []struct {
			ID            string `json:"id"`
			LatestVersion struct {
				ID string `json:"id"`
			} `json:"latestVersion"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("decode service detail: %v", err)
	}
	versionID := detailBody.Assets[0].LatestVersion.ID

	// single → document
	single := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/views:resolve", map[string]any{
		"viewId": "source", "inputs": []map[string]any{{"type": "version", "versionId": versionID}},
	}, nil)
	assertStatus(t, single, http.StatusOK)
	if !strings.Contains(single.Body.String(), `"document"`) {
		t.Fatalf("single resolution kind not document: %s", single.Body.String())
	}

	// collection → items
	collection := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/views:resolve", map[string]any{
		"viewId": "operations", "inputs": []map[string]any{{"type": "version", "versionId": versionID}},
	}, nil)
	assertStatus(t, collection, http.StatusOK)

	// invalid input arity → 422 input_spec_mismatch
	invalid := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/views:resolve", map[string]any{
		"viewId": "source", "inputs": []map[string]any{{"type": "version", "versionId": versionID}, {"type": "version", "versionId": versionID}},
	}, nil)
	assertError(t, invalid, http.StatusUnprocessableEntity, "input_spec_mismatch")

	// unknown view → 404
	missing := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/views:resolve", map[string]any{
		"viewId": "no-such-view", "inputs": []map[string]any{{"type": "version", "versionId": versionID}},
	}, nil)
	assertStatus(t, missing, http.StatusNotFound)
}
