//go:build integration

package handler

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/database"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/storage"
	"github.com/meridian-labs/meridian/internal/task"
)

// TestM4GlobalViewsSmoke exercises the M4 dbschema generic-kind path, the
// system-group dependency graph, and cross-kind search isolation
// (SMK-022, SMK-023, SMK-024).
func TestM4GlobalViewsSmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M4 smoke requires MERIDIAN_TEST_DATABASE_URL; use make smoke-m4-global-views")
	}
	t.Run("SMK-022", smokeM4DBSchemaGenericKind)
	t.Run("SMK-023", smokeM4GroupDependencyGraph)
	t.Run("SMK-024", smokeM4SearchFacetsAndIsolation)
}

// m4Fixture wires the full runtime including M4 search and system groups.
type m4Fixture struct {
	db        *database.Database
	handler   http.Handler
	identity  *service.Identity
	tenantID  uuid.UUID
	alice     m1SmokeSession
	rival     m1SmokeSession
	platform  m1SmokeSession
	workspace string
}

func newM4Fixture(t *testing.T) *m4Fixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M4 database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M4 database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M4 database: %v", err)
	}
	for _, table := range []string{"system_group_members", "system_groups", "breaking_todos", "share_links", "diff_snapshots", "diff_rule_sets", "uploads", "ai_generation_results", "config_import_previews", "recent_services", "asset_items", "asset_versions", "asset_ref_tracks", "layer_heads", "layer_revisions", "layers", "assets", "tenant_kind_overrides", "source_bindings", "source_specs", "discovery_candidates", "services", "producer_profiles", "repositories", "users", "tenants"} {
		if _, err := db.Pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("reset M4 table %s: %v", table, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M4 queue: %v", err)
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
	systemGroupStore := repository.NewSystemGroupStore(db.Pool)
	serviceLifecycleStore := repository.NewServiceLifecycleStoreWithRiver(db.Pool, nil)
	workspace := t.TempDir()
	blobs, err := storage.NewLocalStore(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatalf("create blob store: %v", err)
	}
	layerEdit := service.NewLayerEdit(layerStore, blobs, identityStore)
	aiWorkflow := service.NewAiWorkflow(aiStore, blobs, identityStore)
	diffService := service.NewDiffService(diffStore, blobs, identityStore, []byte("smoke-share-signing-key-fixed-32-bytes"))
	searchService := service.NewSearch(assetStore, systemGroupStore, identityStore)
	systemGroups := service.NewSystemGroups(systemGroupStore, identityStore)
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions:       repositoryStore,
		SyncRunner:       service.NewPipelineRunner(assetStore, blobs, workspace),
		DiscoverRunner:   service.NewDiscoveryRunner(discoveryStore, workspace),
		MergeRunner:      layerEdit,
		AiGenerateRunner: aiWorkflow,
	}, logger)
	if err != nil {
		t.Fatalf("create M4 queue runtime: %v", err)
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
		Search:           searchService,
		SystemGroups:     systemGroups,
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
	rival, err := identity.CreateTenant(t.Context(), actor, service.CreateTenantInput{
		Slug: "rival", DisplayName: "Rival", Quota: new(service.Quota{MaxRepositories: 20, MaxServices: 20, MaxStorageBytes: 1073741824, MaxCollectConcurrency: 2}),
	})
	if err != nil {
		t.Fatalf("create rival tenant: %v", err)
	}
	user, err := identity.CreateUser(t.Context(), actor, service.CreateUserInput{
		Username: "alice", DisplayName: "Alice", Password: "smoke secure password",
	})
	if err != nil {
		t.Fatalf("create smoke user: %v", err)
	}
	if _, _, err := identity.PutTenantMembership(t.Context(), actor, "acme", user.ID, "tenant_admin"); err != nil {
		t.Fatalf("grant acme membership: %v", err)
	}
	if _, _, err := identity.PutTenantMembership(t.Context(), actor, "rival", user.ID, "tenant_admin"); err != nil {
		t.Fatalf("grant rival membership: %v", err)
	}

	f := &m4Fixture{db: db, handler: handler, identity: identity, tenantID: tenant.ID, workspace: workspace}
	login := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "padmin", "password": "smoke secure password"}, nil)
	assertStatus(t, login, http.StatusOK)
	f.platform = m1SmokeSession{token: responseString(t, login, "accessToken")}
	aliceLogin := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "alice", "password": "smoke secure password"}, nil)
	assertStatus(t, aliceLogin, http.StatusOK)
	f.alice = m1SmokeSession{token: responseString(t, aliceLogin, "accessToken")}
	f.rival = m1SmokeSession{token: responseString(t, aliceLogin, "accessToken")}
	_ = rival
	return f
}

func (f *m4Fixture) request(t *testing.T, session *m1SmokeSession, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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

func (f *m4Fixture) waitJob(t *testing.T, jobID string) map[string]any {
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

// smokeM4DBSchemaGenericKind covers SMK-022: pushing a dbschema document is
// accepted, requires review, and indexes stable table/column items after merge.
func smokeM4DBSchemaGenericKind(t *testing.T) {
	f := newM4Fixture(t)
	// A service must exist for the push identity to resolve against.
	f.setupOrderServiceForSearch(t)

	// Publish a minimal dbschema document through the push path.
	content := "schemaVersion: meridian-dbschema-1\ndatabase:\n  name: catalog\n  engine: postgresql\ntables:\n  - name: orders\n    description: order rows\n    columns:\n      - {name: id, dataType: bigint, nullable: false}\n      - {name: amount, dataType: numeric, nullable: true}\n"
	pushed := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/assets:push", map[string]any{
		"serviceSlug": "order-service", "kind": "dbschema", "name": "catalog", "ref": "main",
		"sourceSystem": "smoke-ci", "createIfMissing": true, "role": "base",
		"contentType": "yaml", "content": content,
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	if pushed.Code != http.StatusOK && pushed.Code != http.StatusCreated {
		t.Fatalf("push dbschema status = %d; body = %s", pushed.Code, pushed.Body.String())
	}
	var pushBody struct {
		RevisionID string `json:"revisionId"`
		JobID      string `json:"jobId"`
	}
	if err := json.Unmarshal(pushed.Body.Bytes(), &pushBody); err != nil {
		t.Fatalf("decode push: %v", err)
	}
	if pushBody.RevisionID == "" {
		t.Fatalf("push missing revisionId: %s", pushed.Body.String())
	}

	// The pushed third-party revision is pending review before it takes effect.
	var reviewStatus string
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT review_status FROM layer_revisions WHERE tenant_id = $1 AND id = $2`, f.tenantID, pushBody.RevisionID).Scan(&reviewStatus); err != nil {
		t.Fatalf("query revision: %v", err)
	}
	if reviewStatus != "pending_review" {
		t.Fatalf("dbschema revision status = %q, want pending_review", reviewStatus)
	}

	// Approve the candidate; the merge job materializes a version with items.
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

	// The merged version indexes table and column items with stable keys.
	var versionID string
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT av.id FROM asset_versions av JOIN assets a ON a.tenant_id = av.tenant_id AND a.id = av.asset_id WHERE av.tenant_id = $1 AND a.kind = 'dbschema' ORDER BY av.sequence_no DESC LIMIT 1`, f.tenantID).Scan(&versionID); err != nil {
		t.Fatalf("query dbschema version: %v", err)
	}
	items := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/asset-versions/"+versionID+"/items", nil, nil)
	assertStatus(t, items, http.StatusOK)
	var itemPage struct {
		Items []struct {
			ItemType string `json:"itemType"`
			Key      string `json:"key"`
		} `json:"items"`
	}
	if err := json.Unmarshal(items.Body.Bytes(), &itemPage); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	types := map[string]bool{}
	for _, item := range itemPage.Items {
		types[item.ItemType] = true
	}
	if !types["table"] || !types["column"] {
		t.Fatalf("dbschema items missing table/column: %+v", itemPage.Items)
	}
	if len(itemPage.Items) != 3 { // 1 table + 2 columns
		t.Fatalf("dbschema item count = %d, want 3: %+v", len(itemPage.Items), itemPage.Items)
	}
	for _, item := range itemPage.Items {
		if item.Key == "orders" && item.ItemType != "table" {
			t.Fatalf("orders key must be a table item")
		}
		if item.Key == "orders.id" && item.ItemType != "column" {
			t.Fatalf("orders.id key must be a column item")
		}
	}
}

// smokeM4GroupDependencyGraph covers SMK-023: a system group with two members
// resolves a dep_graph view over the group's dependency edges.
func smokeM4GroupDependencyGraph(t *testing.T) {
	f := newM4Fixture(t)

	// Materialize order-service and pay-service assets via the repository pipeline.
	serviceIDs := f.setupTwoServices(t)

	// Create a system group and put both services as members.
	group := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/system-groups", map[string]any{
		"slug": "fixture-group", "displayName": "Fixture Group", "serviceIds": serviceIDs,
	}, nil)
	assertStatus(t, group, http.StatusCreated)
	groupID := responseString(t, group, "id")
	groupEtag := responseString(t, group, "etag")

	// Resolve the dep-graph scope view for the group.
	resolved := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/views:resolve", map[string]any{
		"viewId": "dep-graph", "scope": map[string]any{"type": "system_group", "id": groupID},
	}, nil)
	assertStatus(t, resolved, http.StatusOK)
	var resolution struct {
		Kind  string `json:"kind"`
		Nodes []struct {
			ID        string `json:"id"`
			Label     string `json:"label"`
			ServiceID string `json:"serviceId"`
		} `json:"nodes"`
		Edges     []map[string]any `json:"edges"`
		Truncated bool             `json:"truncated"`
	}
	if err := json.Unmarshal(resolved.Body.Bytes(), &resolution); err != nil {
		t.Fatalf("decode graph resolution: %v", err)
	}
	if resolution.Kind != "dep_graph" {
		t.Fatalf("graph kind = %q, want dep_graph", resolution.Kind)
	}
	if resolution.Truncated {
		t.Fatalf("graph should not be truncated")
	}
	nodeSlugs := map[string]bool{}
	for _, node := range resolution.Nodes {
		nodeSlugs[node.ID] = true
	}
	if !nodeSlugs["order-service"] || !nodeSlugs["pay-service"] {
		t.Fatalf("graph nodes missing [order-service, pay-service]: %+v", resolution.Nodes)
	}
	_ = groupEtag
}

// smokeM4SearchFacetsAndIsolation covers SMK-024: search returns an item hit
// with facets, and the rival tenant sees nothing.
func smokeM4SearchFacetsAndIsolation(t *testing.T) {
	f := newM4Fixture(t)
	f.setupOrderServiceForSearch(t)

	search := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/search?q=orders", nil, nil)
	assertStatus(t, search, http.StatusOK)
	var result struct {
		Total int `json:"total"`
		Items []struct {
			Type     string `json:"type"`
			ItemType string `json:"-"`
			Kind     any    `json:"kind"`
			DeepLink struct {
				AssetID   string `json:"assetId"`
				VersionID string `json:"versionId"`
				ItemKey   string `json:"itemKey"`
			} `json:"deepLink"`
			Highlights map[string]any `json:"highlights"`
		} `json:"items"`
		Facets struct {
			Kinds []map[string]any `json:"kinds"`
		} `json:"facets"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode search: %v", err)
	}
	if result.Total == 0 || len(result.Items) == 0 {
		t.Fatalf("acme search returned no hits: %s", search.Body.String())
	}
	hit := result.Items[0]
	if hit.Type != "item" {
		t.Fatalf("search hit type = %q, want item", hit.Type)
	}
	if hit.DeepLink.AssetID == "" || hit.DeepLink.VersionID == "" {
		t.Fatalf("search hit deep link missing: %+v", hit.DeepLink)
	}
	if len(hit.Highlights) == 0 {
		t.Fatalf("search hit highlights empty: %+v", hit.Highlights)
	}
	if len(result.Facets.Kinds) == 0 {
		t.Fatalf("search facets missing kinds: %s", search.Body.String())
	}

	// Rival tenant isolation: no data visible.
	rival := f.request(t, &f.rival, http.MethodGet, "/api/v1/t/rival/search?q=orders", nil, nil)
	assertStatus(t, rival, http.StatusOK)
	var rivalResult struct {
		Total int              `json:"total"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rival.Body.Bytes(), &rivalResult); err != nil {
		t.Fatalf("decode rival search: %v", err)
	}
	if rivalResult.Total != 0 || len(rivalResult.Items) != 0 {
		t.Fatalf("rival search leaked data: %s", rival.Body.String())
	}
}

// setupOrderServiceForSearch materializes an openapi asset so the search index
// has operation items to match.
func (f *m4Fixture) setupOrderServiceForSearch(t *testing.T) {
	t.Helper()
	bare, _ := seedDiscoveryRepository(t)
	server := discoveryGitHTTPServer(t, bare)
	certificatePath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(certificatePath, pemEncode(server.Certificate().Raw), 0o600); err != nil {
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
		t.Fatalf("discovery job = %v", jobBody["status"])
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
		t.Fatalf("order-service candidate not found")
	}
	accepted := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates:accept", map[string]any{
		"candidateIds": []string{orderID},
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, accepted, http.StatusOK)

	createSource := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "base", "origin": "repo", "mode": "builtin", "path": "order-service/openapi.yaml",
	}, nil)
	assertStatus(t, createSource, http.StatusCreated)

	synced := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":sync", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, synced, http.StatusAccepted)
	if jobBody := f.waitJob(t, responseString(t, synced, "jobId")); jobBody["status"] != "succeeded" {
		t.Fatalf("sync job = %v", jobBody["status"])
	}
}

// setupTwoServices materializes order-service and pay-service assets and
// returns their service ids.
func (f *m4Fixture) setupTwoServices(t *testing.T) []string {
	t.Helper()
	bare, _ := seedDiscoveryRepository(t)
	server := discoveryGitHTTPServer(t, bare)
	certificatePath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(certificatePath, pemEncode(server.Certificate().Raw), 0o600); err != nil {
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
		t.Fatalf("discovery job = %v", jobBody["status"])
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
	ids := make([]string, 0, 2)
	for _, candidate := range candidateBody.Items {
		if candidate.Slug == "order-service" || candidate.Slug == "pay-service" {
			ids = append(ids, candidate.ID)
		}
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %s", len(ids), candidates.Body.String())
	}
	accepted := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates:accept", map[string]any{
		"candidateIds": ids,
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
	serviceIDs := make([]string, 0, len(serviceList.Items))
	for _, item := range serviceList.Items {
		serviceIDs = append(serviceIDs, item.ID)
	}
	if len(serviceIDs) != 2 {
		t.Fatalf("accepted service count = %d, want 2", len(serviceIDs))
	}
	return serviceIDs
}

func pemEncode(raw []byte) []byte {
	return []byte("-----BEGIN CERTIFICATE-----\n" + base64.StdEncoding.EncodeToString(raw) + "\n-----END CERTIFICATE-----\n")
}
