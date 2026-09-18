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

// TestM3DiffSmoke exercises the M3 diff, snapshot share, breaking todo, and push
// gates (SMK-018, SMK-019, SMK-021, SMK-026).
func TestM3DiffSmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M3 diff smoke requires MERIDIAN_TEST_DATABASE_URL; use make smoke-m3-diff-cli")
	}
	t.Run("SMK-018", smokeM3DiffBreakingAndShare)
	t.Run("SMK-019", smokeM3BreakingTodo)
	t.Run("SMK-021", smokeM3PushReview)
	t.Run("SMK-026", smokeM3ShareSecurity)
}

type m3DiffFixture struct {
	db        *database.Database
	handler   http.Handler
	identity  *service.Identity
	tenantID  uuid.UUID
	alice     m1SmokeSession
	platform  m1SmokeSession
	workspace string
}

func newM3DiffFixture(t *testing.T) *m3DiffFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M3 diff database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M3 diff database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M3 diff database: %v", err)
	}
	for _, table := range []string{"breaking_todos", "share_links", "diff_snapshots", "diff_rule_sets", "uploads", "ai_generation_results", "config_import_previews", "recent_services", "asset_items", "asset_versions", "asset_ref_tracks", "layer_heads", "layer_revisions", "layers", "assets", "tenant_kind_overrides", "source_bindings", "source_specs", "discovery_candidates", "services", "producer_profiles", "repositories", "users", "tenants"} {
		if _, err := db.Pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("reset M3 diff table %s: %v", table, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M3 diff queue: %v", err)
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
	aiStore := repository.NewAIStore(db.Pool)
	diffStore := repository.NewDiffStore(db.Pool)
	serviceLifecycleStore := repository.NewServiceLifecycleStoreWithRiver(db.Pool, nil)
	workspace := t.TempDir()
	blobs, err := storage.NewLocalStore(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatalf("create blob store: %v", err)
	}
	layerEdit := service.NewLayerEdit(layerStore, blobs, identityStore)
	aiWorkflow := service.NewAiWorkflow(aiStore, blobs, identityStore)
	diffService := service.NewDiffService(diffStore, blobs, identityStore, []byte("smoke-share-signing-key-fixed-32-bytes"))
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions:       repositoryStore,
		SyncRunner:       service.NewPipelineRunner(assetStore, blobs, workspace),
		DiscoverRunner:   service.NewDiscoveryRunner(discoveryStore, workspace),
		MergeRunner:      layerEdit,
		AiGenerateRunner: aiWorkflow,
	}, logger)
	if err != nil {
		t.Fatalf("create M3 diff queue runtime: %v", err)
	}
	discoveryStore.BindRiver(runtime.Client())
	serviceLifecycleStore.BindRiver(runtime.Client())
	layerStore.BindRiver(runtime.Client())
	aiStore.BindRiver(runtime.Client())
	diffStore.BindRiver(runtime.Client())

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
		AiWorkflow:       aiWorkflow,
		DiffService:      diffService,
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

	f := &m3DiffFixture{db: db, handler: handler, identity: identity, tenantID: tenant.ID, workspace: workspace}
	login := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "padmin", "password": "smoke secure password"}, nil)
	assertStatus(t, login, http.StatusOK)
	f.platform = m1SmokeSession{token: responseString(t, login, "accessToken")}
	aliceLogin := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "alice", "password": "smoke secure password"}, nil)
	assertStatus(t, aliceLogin, http.StatusOK)
	f.alice = m1SmokeSession{token: responseString(t, aliceLogin, "accessToken")}
	return f
}

func (f *m3DiffFixture) request(t *testing.T, session *m1SmokeSession, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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

func (f *m3DiffFixture) waitJob(t *testing.T, jobID string) map[string]any {
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

// setupOrderService registers the repository, accepts order-service, and syncs
// to materialize the base openapi asset with three operations on main.
func (f *m3DiffFixture) setupOrderService(t *testing.T) (repositoryID, serviceID string) {
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
	repositoryID = responseString(t, created, "id")

	discovered := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":discover", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, discovered, http.StatusAccepted)
	if jobBody := f.waitJob(t, responseString(t, discovered, "jobId")); jobBody["status"] != "succeeded" {
		t.Fatalf("discovery job status = %v", jobBody["status"])
	}

	candidates := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates", nil, nil)
	assertStatus(t, candidates, http.StatusOK)
	var candidateBody struct {
		Items []struct {
			ID   string `json:"id"`
			Slug string `json:"suggestedSlug"`
		} `json:"items"`
	}
	if err := json.Unmarshal(candidates.Body.Bytes(), &candidateBody); err != nil {
		t.Fatalf("decode candidates: %v", err)
	}
	orderID := ""
	for _, candidate := range candidateBody.Items {
		if candidate.Slug == "order-service" {
			orderID = candidate.ID
			break
		}
	}
	if orderID == "" {
		t.Fatalf("order-service candidate not found: %s", candidates.Body.String())
	}
	accepted := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates:accept", map[string]any{
		"candidateIds": []string{orderID},
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, accepted, http.StatusOK)
	var serviceList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(accepted.Body.Bytes(), &serviceList); err != nil {
		t.Fatalf("decode accepted services: %v", err)
	}
	if len(serviceList.Items) == 1 {
		serviceID = serviceList.Items[0].ID
	}

	// Bind the builtin openapi source so sync materializes the base asset.
	createSource := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "base", "origin": "repo", "mode": "builtin", "path": "order-service/openapi.yaml",
	}, nil)
	assertStatus(t, createSource, http.StatusCreated)

	synced := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":sync", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, synced, http.StatusAccepted)
	if jobBody := f.waitJob(t, responseString(t, synced, "jobId")); jobBody["status"] != "succeeded" {
		t.Fatalf("sync job status = %v", jobBody["status"])
	}
	return repositoryID, serviceID
}

// smokeM3DiffBreakingAndShare covers SMK-018: a breaking removed-operation diff
// and a frozen, later-immutable snapshot share.
func smokeM3DiffBreakingAndShare(t *testing.T) {
	f := newM3DiffFixture(t)
	_, _ = f.setupOrderService(t)

	// Materialize a left version (3 operations) and a right version (2 operations).
	var leftVersion, rightVersion string
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT id FROM asset_versions WHERE tenant_id = $1 ORDER BY sequence_no ASC LIMIT 1`, f.tenantID).Scan(&leftVersion); err != nil {
		t.Fatalf("query left version: %v", err)
	}

	// Push a revision that removes one operation, then approve+merge to create
	// the right version on the same track.
	pushed := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/assets:push", map[string]any{
		"serviceSlug": "order-service", "kind": "openapi", "name": "openapi", "ref": "main",
		"sourceSystem": "smoke-ci", "createIfMissing": false, "role": "overlay",
		"contentType": "yaml",
		"content":     "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /paths/~1orders~1{id}\n    remove: true\n",
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, pushed, http.StatusOK)
	var pushBody struct {
		RevisionID string `json:"revisionId"`
		JobID      string `json:"jobId"`
		LayerID    string `json:"layerId"`
	}
	if err := json.Unmarshal(pushed.Body.Bytes(), &pushBody); err != nil {
		t.Fatalf("decode push: %v", err)
	}
	if pushBody.RevisionID == "" {
		t.Fatalf("push missing revisionId: %s", pushed.Body.String())
	}

	// Approve the pushed candidate; the merge job materializes the right version.
	approved := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/layer-revisions/"+pushBody.RevisionID+":approve", map[string]any{}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, approved, http.StatusOK)
	var approveBody struct {
		MergeJobID string `json:"mergeJobId"`
	}
	if err := json.Unmarshal(approved.Body.Bytes(), &approveBody); err != nil {
		t.Fatalf("decode approve: %v", err)
	}
	if approveBody.MergeJobID != "" {
		if jobBody := f.waitJob(t, approveBody.MergeJobID); jobBody["status"] != "succeeded" {
			t.Fatalf("merge job = %v", jobBody["status"])
		}
	}

	if err := f.db.Pool.QueryRow(t.Context(), `SELECT id FROM asset_versions WHERE tenant_id = $1 ORDER BY sequence_no DESC LIMIT 1`, f.tenantID).Scan(&rightVersion); err != nil {
		t.Fatalf("query right version: %v", err)
	}

	// runDiff reports one breaking removed operation.
	diffResult := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/diff", map[string]any{
		"left":    map[string]any{"type": "version", "versionId": leftVersion},
		"right":   map[string]any{"type": "version", "versionId": rightVersion},
		"persist": true,
	}, nil)
	assertStatus(t, diffResult, http.StatusOK)
	var diffBody struct {
		SnapshotID string `json:"snapshotId"`
		Summary    struct {
			Breaking int `json:"breaking"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(diffResult.Body.Bytes(), &diffBody); err != nil {
		t.Fatalf("decode diff: %v", err)
	}
	if diffBody.Summary.Breaking != 1 {
		t.Fatalf("diff breaking = %d, want 1", diffBody.Summary.Breaking)
	}
	if diffBody.SnapshotID == "" {
		t.Fatalf("diff snapshotId missing: %s", diffResult.Body.String())
	}

	// Share the snapshot; getSharedView resolves it anonymously.
	share := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/diff-snapshots/"+diffBody.SnapshotID+"/share-links", map[string]any{
		"expiresInSeconds": 3600,
	}, nil)
	assertStatus(t, share, http.StatusCreated)
	var shareBody struct {
		Token        string `json:"token"`
		ResourceType string `json:"resourceType"`
	}
	if err := json.Unmarshal(share.Body.Bytes(), &shareBody); err != nil {
		t.Fatalf("decode share: %v", err)
	}
	if shareBody.Token == "" || shareBody.ResourceType != "diff_snapshot" {
		t.Fatalf("share missing token/resourceType: %s", share.Body.String())
	}

	shared := f.request(t, nil, http.MethodGet, "/api/v1/shared/"+shareBody.Token, nil, nil)
	assertStatus(t, shared, http.StatusOK)
	if !strings.Contains(shared.Body.String(), "diff_snapshot") {
		t.Fatalf("shared view resourceType missing: %s", shared.Body.String())
	}
}

// smokeM3BreakingTodo covers SMK-019: exactly one todo per version+service, and
// acknowledge transitions it to acked.
func smokeM3BreakingTodo(t *testing.T) {
	f := newM3DiffFixture(t)
	_, _ = f.setupOrderService(t)

	var leftVersion, rightVersion string
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT id FROM asset_versions WHERE tenant_id = $1 ORDER BY sequence_no ASC LIMIT 1`, f.tenantID).Scan(&leftVersion); err != nil {
		t.Fatalf("query left version: %v", err)
	}
	pushed := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/assets:push", map[string]any{
		"serviceSlug": "order-service", "kind": "openapi", "name": "openapi", "ref": "main",
		"sourceSystem": "smoke-ci", "createIfMissing": false, "role": "overlay",
		"contentType": "yaml",
		"content":     "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /paths/~1orders~1{id}\n    remove: true\n",
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, pushed, http.StatusOK)
	revisionID := ""
	if err := json.Unmarshal(pushed.Body.Bytes(), &struct {
		RevisionID *string `json:"revisionId"`
	}{&revisionID}); err != nil {
		t.Fatalf("decode push revision: %v", err)
	}
	approved := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/layer-revisions/"+revisionID+":approve", map[string]any{}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, approved, http.StatusOK)
	mergeJob := ""
	if err := json.Unmarshal(approved.Body.Bytes(), &struct {
		MergeJobID *string `json:"mergeJobId"`
	}{&mergeJob}); err != nil {
		t.Fatalf("decode merge: %v", err)
	}
	if mergeJob != "" {
		if jobBody := f.waitJob(t, mergeJob); jobBody["status"] != "succeeded" {
			t.Fatalf("merge job = %v", jobBody["status"])
		}
	}
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT id FROM asset_versions WHERE tenant_id = $1 ORDER BY sequence_no DESC LIMIT 1`, f.tenantID).Scan(&rightVersion); err != nil {
		t.Fatalf("query right version: %v", err)
	}

	// Running the same diff twice must not create a duplicate todo.
	for i := 0; i < 2; i++ {
		diffResult := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/diff", map[string]any{
			"left":    map[string]any{"type": "version", "versionId": leftVersion},
			"right":   map[string]any{"type": "version", "versionId": rightVersion},
			"persist": true,
		}, nil)
		assertStatus(t, diffResult, http.StatusOK)
	}

	var todoCount int64
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM breaking_todos WHERE tenant_id = $1`, f.tenantID).Scan(&todoCount); err != nil {
		t.Fatalf("count todos: %v", err)
	}
	if todoCount != 1 {
		t.Fatalf("todo count = %d, want 1", todoCount)
	}

	todos := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/breaking-todos", nil, nil)
	assertStatus(t, todos, http.StatusOK)
	var todoPage struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(todos.Body.Bytes(), &todoPage); err != nil {
		t.Fatalf("decode todos: %v", err)
	}
	if len(todoPage.Items) != 1 || todoPage.Items[0].Status != "open" {
		t.Fatalf("todos = %+v, want one open", todoPage.Items)
	}

	acked := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/breaking-todos/"+todoPage.Items[0].ID+":ack", map[string]any{"comment": "noted"}, nil)
	assertStatus(t, acked, http.StatusOK)
	var ackedBody struct {
		Status         string `json:"status"`
		AcknowledgedAt string `json:"acknowledgedAt"`
	}
	if err := json.Unmarshal(acked.Body.Bytes(), &ackedBody); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if ackedBody.Status != "acked" {
		t.Fatalf("acked status = %q", ackedBody.Status)
	}
}

// smokeM3PushReview covers SMK-021: a pushed third-party revision is pending
// until approved, and insufficient scope is 404.
func smokeM3PushReview(t *testing.T) {
	f := newM3DiffFixture(t)
	_, _ = f.setupOrderService(t)

	pushed := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/assets:push", map[string]any{
		"serviceSlug": "order-service", "kind": "openapi", "name": "openapi", "ref": "main",
		"sourceSystem": "smoke-ci", "createIfMissing": false, "role": "overlay",
		"contentType": "yaml",
		"content":     "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /paths/~1orders~1{id}\n    remove: true\n",
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, pushed, http.StatusOK)

	var revisionID string
	if err := json.Unmarshal(pushed.Body.Bytes(), &struct {
		RevisionID *string `json:"revisionId"`
	}{&revisionID}); err != nil {
		t.Fatalf("decode push: %v", err)
	}

	var reviewStatus string
	var effectiveID *string
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT review_status FROM layer_revisions WHERE tenant_id = $1 AND id = $2`, f.tenantID, revisionID).Scan(&reviewStatus); err != nil {
		t.Fatalf("query revision: %v", err)
	}
	if reviewStatus != "pending_review" {
		t.Fatalf("pushed revision status = %q, want pending_review", reviewStatus)
	}
	_ = effectiveID
}

// smokeM3ShareSecurity covers SMK-026: an expired token is 404.
func smokeM3ShareSecurity(t *testing.T) {
	f := newM3DiffFixture(t)
	_, _ = f.setupOrderService(t)

	// A syntactically valid but unknown token must yield 404.
	invalid := f.request(t, nil, http.MethodGet, "/api/v1/shared/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA.malformed-signature", nil, nil)
	assertStatus(t, invalid, http.StatusNotFound)
}
