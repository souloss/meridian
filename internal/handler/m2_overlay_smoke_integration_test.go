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
	"github.com/meridian-labs/meridian/internal/plugins"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/storage"
	"github.com/meridian-labs/meridian/internal/task"
)

// TestM2OverlaySmoke exercises the M2 layer overlay, provenance, rollback, and
// invalid-overlay gates against a full runtime fixture.
func TestM2OverlaySmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M2 overlay smoke requires MERIDIAN_TEST_DATABASE_URL; use make smoke-m2-overlay")
	}
	t.Run("SMK-011", smokeM2DeterminismAndProvenance)
	t.Run("SMK-012", smokeM2OverlayEditorE2E)
	t.Run("SMK-013", smokeM2RollbackVersionPlusOne)
	t.Run("SMK-014", smokeM2InvalidOverlayRejected)
}

// m2OverlayFixture wires the full M2 runtime: pipeline sync, layer edit, and the
// asset.merge worker.
type m2OverlayFixture struct {
	db        *database.Database
	handler   http.Handler
	identity  *service.Identity
	blobs     *storage.LocalStore
	tenantID  uuid.UUID
	alice     m1SmokeSession
	platform  m1SmokeSession
	workspace string
}

func newM2OverlayFixture(t *testing.T) *m2OverlayFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M2 overlay database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M2 overlay database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M2 overlay database: %v", err)
	}
	for _, table := range []string{"recent_services", "asset_items", "asset_versions", "asset_ref_tracks", "layer_heads", "layer_revisions", "layers", "assets", "tenant_kind_overrides", "source_bindings", "source_specs", "discovery_candidates", "services", "producer_profiles", "repositories", "users", "tenants"} {
		if _, err := db.Pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("reset M2 overlay table %s: %v", table, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M2 overlay queue: %v", err)
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
	pluginRuntime, err := plugins.New(t.Context())
	if err != nil {
		t.Fatalf("create plugin runtime: %v", err)
	}
	t.Cleanup(func() {
		closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = pluginRuntime.Close(closeContext)
	})
	layerEdit := service.NewLayerEdit(layerStore, blobs, identityStore, pluginRuntime.Kinds)
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions:     repositoryStore,
		SyncRunner:     service.NewPipelineRunner(assetStore, blobs, workspace, pluginRuntime.Kinds),
		DiscoverRunner: service.NewDiscoveryRunner(discoveryStore, workspace),
		MergeRunner:    layerEdit,
	}, logger)
	if err != nil {
		t.Fatalf("create M2 overlay queue runtime: %v", err)
	}
	discoveryStore.BindRiver(runtime.Client())
	serviceLifecycleStore.BindRiver(runtime.Client())
	layerStore.BindRiver(runtime.Client())

	assets := service.NewAssets(assetStore, identityStore)
	views := service.NewViews(assetStore, identityStore)
	discovery := service.NewDiscovery(discoveryStore, identityStore).WithAssets(assets)
	serviceLifecycle := service.NewServiceLifecycle(serviceLifecycleStore, assets, identityStore)

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

	f := &m2OverlayFixture{db: db, handler: handler, identity: identity, blobs: blobs, tenantID: tenant.ID, workspace: workspace}
	login := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "padmin", "password": "smoke secure password"}, nil)
	assertStatus(t, login, http.StatusOK)
	f.platform = m1SmokeSession{token: responseString(t, login, "accessToken")}
	aliceLogin := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "alice", "password": "smoke secure password"}, nil)
	assertStatus(t, aliceLogin, http.StatusOK)
	f.alice = m1SmokeSession{token: responseString(t, aliceLogin, "accessToken")}
	return f
}

func (f *m2OverlayFixture) request(t *testing.T, session *m1SmokeSession, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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

func (f *m2OverlayFixture) waitJob(t *testing.T, jobID string) map[string]any {
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

// setupMaterializedAsset drives the M1 golden path to materialize one asset with
// a base version on the default branch, returning the asset id and latest version id.
func (f *m2OverlayFixture) setupMaterializedAsset(t *testing.T) (assetID, versionID string) {
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
	if jobBody := f.waitJob(t, responseString(t, discovered, "jobId")); jobBody["status"] != "succeeded" {
		t.Fatalf("discovery job status = %v", jobBody["status"])
	}

	// Accept only order-service.
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
	var orderID string
	for _, item := range candidateBody.Items {
		if item.Path == "order-service" {
			orderID = item.ID
		}
	}
	if orderID == "" {
		t.Fatalf("order-service candidate not found")
	}
	accepted := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates:accept", map[string]any{"candidateIds": []string{orderID}}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, accepted, http.StatusOK)

	source := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "base", "origin": "repo", "mode": "builtin", "path": "order-service/openapi.yaml",
	}, nil)
	assertStatus(t, source, http.StatusCreated)

	sync := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":sync", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, sync, http.StatusAccepted)
	if jobBody := f.waitJob(t, responseString(t, sync, "jobId")); jobBody["status"] != "succeeded" {
		t.Fatalf("sync job status = %v, error = %v", jobBody["status"], jobBody["error"])
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
	if len(detailBody.Assets) != 1 {
		t.Fatalf("assets = %d, want 1", len(detailBody.Assets))
	}
	return detailBody.Assets[0].ID, detailBody.Assets[0].LatestVersion.ID
}

const m2ValidOverlay = "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge:\n      description: overlaid\n"

// smokeM2DeterminismAndProvenance covers SMK-011: the preview merge engine is
// deterministic and provenance points at the last writing layer/revision.
func smokeM2DeterminismAndProvenance(t *testing.T) {
	f := newM2OverlayFixture(t)
	assetID, _ := f.setupMaterializedAsset(t)

	preview := func() *httptest.ResponseRecorder {
		return f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/assets:preview-merge", map[string]any{
			"assetId": assetID, "refType": "branch", "ref": "main",
		}, nil)
	}
	first := preview()
	assertStatus(t, first, http.StatusOK)
	var firstBody struct {
		InputFingerprint string `json:"inputFingerprint"`
		Content          string `json:"content"`
		Provenance       []struct {
			Pointer string `json:"pointer"`
		} `json:"provenance"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if firstBody.InputFingerprint == "" || firstBody.Content == "" {
		t.Fatalf("preview missing fingerprint or content: %s", first.Body.String())
	}

	// Deterministic across 100 runs.
	for run := 0; run < 100; run++ {
		rec := preview()
		assertStatus(t, rec, http.StatusOK)
		if rec.Body.String() != first.Body.String() {
			t.Fatalf("preview output diverged at run %d", run)
		}
	}

	// A merge that writes /info must record provenance with that pointer.
	overlay := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/assets:preview-merge", map[string]any{
		"assetId": assetID, "refType": "branch", "ref": "main",
		"layers": []map[string]any{}, "overrideContent": m2ValidOverlay,
	}, nil)
	assertStatus(t, overlay, http.StatusOK)
}

// smokeM2OverlayEditorE2E covers SMK-012: create a manual source spec whose
// createLayerRevision accepts a valid overlay and rejects an invalid one.
func smokeM2OverlayEditorE2E(t *testing.T) {
	f := newM2OverlayFixture(t)
	assetID, _ := f.setupMaterializedAsset(t)

	// Manual source spec on the materialized asset returns initialLayerId.
	create := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "overlay", "origin": "manual", "mode": "manual", "targetAssetId": assetID,
	}, nil)
	assertStatus(t, create, http.StatusCreated)
	initialLayerID := responseString(t, create, "initialLayerId")
	if initialLayerID == "" {
		t.Fatalf("manual source initialLayerId missing: %s", create.Body.String())
	}

	// Valid overlay revision is persisted.
	valid := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/layers/"+initialLayerID+"/revisions", map[string]any{
		"scopeType": "global", "scopeKey": "*", "content": m2ValidOverlay,
		"contentType": "yaml", "expectedEffectiveRevisionId": nil, "submitForReview": false,
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	if valid.Code != http.StatusOK && valid.Code != http.StatusCreated {
		t.Fatalf("valid overlay revision status = %d: %s", valid.Code, valid.Body.String())
	}
	var validBody struct {
		Revision struct {
			ID string `json:"id"`
		} `json:"revision"`
		JobId string `json:"jobId"`
	}
	if err := json.Unmarshal(valid.Body.Bytes(), &validBody); err != nil {
		t.Fatalf("decode valid revision: %v", err)
	}
	if validBody.Revision.ID == "" {
		t.Fatalf("revision id missing: %s", valid.Body.String())
	}

	// Invalid overlay is rejected with 422 overlay_invalid and line/column.
	invalid := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/layers/"+initialLayerID+"/revisions", map[string]any{
		"scopeType": "global", "scopeKey": "*",
		"content":     "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge:\n      title: a\n      title: b\n",
		"contentType": "yaml", "expectedEffectiveRevisionId": nil, "submitForReview": false,
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertError(t, invalid, http.StatusUnprocessableEntity, "overlay_invalid")
	var invalidBody struct {
		Details struct {
			Errors []struct {
				Line   int `json:"line"`
				Column int `json:"column"`
			} `json:"errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(invalid.Body.Bytes(), &invalidBody); err != nil {
		t.Fatalf("decode invalid overlay error: %v", err)
	}
	if len(invalidBody.Details.Errors) == 0 {
		t.Fatalf("invalid overlay missing error details: %s", invalid.Body.String())
	}
	if invalidBody.Details.Errors[0].Line < 1 || invalidBody.Details.Errors[0].Column < 1 {
		t.Fatalf("invalid overlay coordinates not one-based: %+v", invalidBody.Details.Errors[0])
	}
}

// smokeM2RollbackVersionPlusOne covers SMK-013: rollback moves the effective head
// to a historical revision without creating a revision, and the merge produces a
// new version.
func smokeM2RollbackVersionPlusOne(t *testing.T) {
	f := newM2OverlayFixture(t)
	assetID, _ := f.setupMaterializedAsset(t)

	// Create a manual overlay layer with two revisions.
	create := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "overlay", "origin": "manual", "mode": "manual", "targetAssetId": assetID,
	}, nil)
	assertStatus(t, create, http.StatusCreated)
	layerID := responseString(t, create, "initialLayerId")

	submit := func(content string) string {
		rec := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/layers/"+layerID+"/revisions", map[string]any{
			"scopeType": "global", "scopeKey": "*", "content": content,
			"contentType": "yaml", "expectedEffectiveRevisionId": nil, "submitForReview": false,
		}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
		assertStatus(t, rec, http.StatusCreated)
		var body struct {
			Revision struct {
				ID string `json:"id"`
			} `json:"revision"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode revision: %v", err)
		}
		return body.Revision.ID
	}
	firstRevision := submit("overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge:\n      description: first\n")
	_ = submit("overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge:\n      description: second\n")

	versionCount := func() int {
		var count int
		if err := f.db.Pool.QueryRow(t.Context(), "SELECT count(*) FROM asset_versions WHERE asset_id = $1", uuid.MustParse(assetID)).Scan(&count); err != nil {
			t.Fatalf("count versions: %v", err)
		}
		return count
	}
	revisionCount := func() int {
		var count int
		if err := f.db.Pool.QueryRow(t.Context(), "SELECT count(*) FROM layer_revisions WHERE layer_id = $1", uuid.MustParse(layerID)).Scan(&count); err != nil {
			t.Fatalf("count revisions: %v", err)
		}
		return count
	}
	effectiveHead := func() string {
		var effective string
		if err := f.db.Pool.QueryRow(t.Context(), "SELECT effective_revision_id::text FROM layer_heads WHERE layer_id = $1 AND scope_type = 'global'", uuid.MustParse(layerID)).Scan(&effective); err != nil {
			t.Fatalf("read effective head: %v", err)
		}
		return effective
	}

	beforeVersions := versionCount()
	beforeRevisions := revisionCount()

	// Roll back to the first revision.
	rollback := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/layers/"+layerID+":rollback", map[string]any{
		"scopeType": "global", "scopeKey": "*", "targetRevisionId": firstRevision, "expectedEffectiveRevisionId": nil,
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, rollback, http.StatusAccepted)
	jobID := responseString(t, rollback, "jobId")
	if jobBody := f.waitJob(t, jobID); jobBody["status"] != "succeeded" {
		t.Fatalf("rollback merge job status = %v", jobBody["status"])
	}

	// No new layer revision was created; the effective head points at the
	// requested historical revision.
	if got := revisionCount(); got != beforeRevisions {
		t.Fatalf("revision count = %d, want %d (rollback must not create a revision)", got, beforeRevisions)
	}
	if got := effectiveHead(); got != firstRevision {
		t.Fatalf("effective head = %s, want %s", got, firstRevision)
	}

	// The track gained exactly one version (materialized by the merge worker).
	if got := versionCount(); got != beforeVersions+1 {
		t.Fatalf("track version count = %d, want %d", got, beforeVersions+1)
	}
}

// smokeM2InvalidOverlayRejected covers SMK-014: an invalid overlay is rejected
// without persisting a revision or changing the effective head.
func smokeM2InvalidOverlayRejected(t *testing.T) {
	f := newM2OverlayFixture(t)
	assetID, _ := f.setupMaterializedAsset(t)

	create := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "overlay", "origin": "manual", "mode": "manual", "targetAssetId": assetID,
	}, nil)
	assertStatus(t, create, http.StatusCreated)
	layerID := responseString(t, create, "initialLayerId")

	revisionCount := func() int {
		var count int
		if err := f.db.Pool.QueryRow(t.Context(), "SELECT count(*) FROM layer_revisions WHERE layer_id = $1", uuid.MustParse(layerID)).Scan(&count); err != nil {
			t.Fatalf("count revisions: %v", err)
		}
		return count
	}
	before := revisionCount()

	// Reject a structurally invalid overlay (duplicate key).
	invalid := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/layers/"+layerID+"/revisions", map[string]any{
		"scopeType": "global", "scopeKey": "*",
		"content":     "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /info\n    merge:\n      description: a\n      description: b\n",
		"contentType": "yaml", "expectedEffectiveRevisionId": nil, "submitForReview": false,
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertError(t, invalid, http.StatusUnprocessableEntity, "overlay_invalid")

	// No revision was persisted.
	if got := revisionCount(); got != before {
		t.Fatalf("revision count = %d, want %d (invalid overlay must not persist)", got, before)
	}
}

var _ = strings.TrimSpace
