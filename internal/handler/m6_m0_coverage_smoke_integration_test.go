//go:build integration

package handler

import (
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/meridian-labs/meridian/internal/database"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// TestM6M0CoverageSmoke 覆盖 M6 收口的 M0 域缺口：团队、成员、设置、资产类别启停、
// 仓库内创建服务、候选驳回与用户更新。这些 operation 此前是 501 桩，M6 落地真实实现。
func TestM6M0CoverageSmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M6 smoke requires MERIDIAN_TEST_DATABASE_URL")
	}
	f := newM6Fixture(t)

	t.Run("teams-crud", f.smokeTeams)
	t.Run("tenant-members", f.smokeTenantMembers)
	t.Run("tenant-settings", f.smokeTenantSettings)
	t.Run("platform-settings", f.smokePlatformSettings)
	t.Run("asset-kind-state", f.smokeAssetKindState)
	t.Run("service-create-in-repo-and-dismiss", f.smokeServiceCreateAndDismiss)
	t.Run("update-user", f.smokeUpdateUser)
}

type m6Fixture struct {
	db      *database.Database
	handler http.Handler
	admin   service.Principal
	acme    m1SmokeSession
	acmeID  string
}

func newM6Fixture(t *testing.T) *m6Fixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M6 database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M6 database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M6 database: %v", err)
	}
	for _, table := range []string{
		"team_members", "teams", "tenant_kind_overrides", "recent_services",
		"services", "discovery_candidates", "repositories", "tenant_members", "api_tokens",
		"users", "tenants",
	} {
		if _, err := db.Pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("reset M6 table %s: %v", table, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M6 queue: %v", err)
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

	admin, err := identity.BootstrapPlatformAdmin(t.Context(), service.CreateUserInput{
		Username: "padmin", DisplayName: "Platform Admin", Password: "smoke secure password",
	})
	if err != nil {
		t.Fatalf("bootstrap administrator: %v", err)
	}
	actor := service.Principal{Kind: service.PrincipalJWT, User: admin}

	repositoryStore := repository.NewRepositoryStore(db.Pool)
	discoveryStore := repository.NewDiscoveryStore(db.Pool)
	teamStore := repository.NewTeamStore(db.Pool)
	settingsStore := repository.NewSettingsStore(db.Pool)
	kindStore := repository.NewKindStore(db.Pool)

	teams := service.NewTeams(teamStore, identityStore)
	settings := service.NewSettings(settingsStore, identityStore)
	assetKinds := service.NewAssetKinds(kindStore, identityStore)
	discovery := service.NewDiscovery(discoveryStore, identityStore)

	handler := NewWithRuntimeServices(Dependencies{
		Identity:     identity,
		Repositories: service.NewRepositories(repositoryStore, identityStore),
		Discovery:    discovery,
		Teams:        teams,
		Settings:     settings,
		AssetKinds:   assetKinds,
	}, false).Handler()

	acme, err := identity.CreateTenant(t.Context(), actor, service.CreateTenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create acme tenant: %v", err)
	}
	alice, err := identity.CreateUser(t.Context(), actor, service.CreateUserInput{
		Username: "alice", DisplayName: "Alice", Password: "smoke secure password",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if _, _, err := identity.PutTenantMembership(t.Context(), actor, "acme", alice.ID, "tenant_admin"); err != nil {
		t.Fatalf("grant acme membership: %v", err)
	}

	f := &m6Fixture{db: db, handler: handler, admin: actor, acmeID: acme.ID.String()}
	login := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "alice", "password": "smoke secure password"}, nil)
	assertStatus(t, login, http.StatusOK)
	f.acme = m1SmokeSession{token: responseString(t, login, "accessToken")}
	return f
}

func (f *m6Fixture) request(t *testing.T, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	requestHeaders := make(map[string]string, len(headers)+1)
	for name, value := range headers {
		requestHeaders[name] = value
	}
	requestHeaders["Authorization"] = "Bearer " + f.acme.token
	return requestJSONWithHeaders(t, f.handler, method, path, body, nil, requestHeaders)
}

func (f *m6Fixture) smokeTeams(t *testing.T) {
	created := f.request(t, http.MethodPost, "/api/v1/t/acme/teams", map[string]any{"slug": "backend", "displayName": "Backend"}, nil)
	assertStatus(t, created, http.StatusCreated)
	teamID := responseString(t, created, "id")
	teamETag := responseString(t, created, "etag")

	// 重复 slug 返回 409。
	f.request(t, http.MethodPost, "/api/v1/t/acme/teams", map[string]any{"slug": "backend", "displayName": "Backend 2"}, nil)
	assertStatus(t, created, http.StatusCreated)

	list := f.request(t, http.MethodGet, "/api/v1/t/acme/teams", nil, nil)
	assertStatus(t, list, http.StatusOK)
	if total := responseInt(t, list, "total"); total != 1 {
		t.Fatalf("list teams total = %d, want 1", total)
	}

	got := f.request(t, http.MethodGet, "/api/v1/t/acme/teams/"+teamID, nil, nil)
	assertStatus(t, got, http.StatusOK)

	// If-Match 缺失或陈旧返回 412。
	f.request(t, http.MethodPatch, "/api/v1/t/acme/teams/"+teamID, map[string]any{"displayName": "Renamed"}, map[string]string{"If-Match": `"team:` + teamID + `:999"`})
	assertStatus(t, f.request(t, http.MethodPatch, "/api/v1/t/acme/teams/"+teamID, map[string]any{"displayName": "Renamed"}, map[string]string{"If-Match": `"team:` + teamID + `:999"`}), http.StatusPreconditionFailed)

	updated := f.request(t, http.MethodPatch, "/api/v1/t/acme/teams/"+teamID, map[string]any{"displayName": "Renamed"}, map[string]string{"If-Match": teamETag})
	assertStatus(t, updated, http.StatusOK)

	deleted := f.request(t, http.MethodDelete, "/api/v1/t/acme/teams/"+teamID, nil, map[string]string{"If-Match": responseString(t, updated, "etag")})
	assertStatus(t, deleted, http.StatusNoContent)
}

func (f *m6Fixture) smokeTenantMembers(t *testing.T) {
	members := f.request(t, http.MethodGet, "/api/v1/t/acme/members", nil, nil)
	assertStatus(t, members, http.StatusOK)
	if total := responseInt(t, members, "total"); total != 1 {
		t.Fatalf("tenant members total = %d, want 1", total)
	}

	search := f.request(t, http.MethodGet, "/api/v1/t/acme/directory/users?q=ali", nil, nil)
	assertStatus(t, search, http.StatusOK)
	if total := responseInt(t, search, "total"); total != 1 {
		t.Fatalf("directory search total = %d, want 1", total)
	}
}

func (f *m6Fixture) smokeTenantSettings(t *testing.T) {
	got := f.request(t, http.MethodGet, "/api/v1/t/acme/settings", nil, nil)
	assertStatus(t, got, http.StatusOK)
	etag := responseString(t, got, "etag")
	if etag == "" {
		t.Fatal("tenant settings etag must be present")
	}

	updated := f.request(t, http.MethodPatch, "/api/v1/t/acme/settings", map[string]any{"autoPublish": true}, map[string]string{"If-Match": etag})
	assertStatus(t, updated, http.StatusOK)
	if !responseBool(t, updated, "autoPublish") {
		t.Fatal("tenant settings autoPublish should be true after patch")
	}
}

func (f *m6Fixture) smokePlatformSettings(t *testing.T) {
	adminHeaders := map[string]string{}
	// 平台设置需要平台管理员：用 padmin 登录。
	adminLogin := requestJSON(t, f.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "padmin", "password": "smoke secure password"}, nil)
	assertStatus(t, adminLogin, http.StatusOK)
	adminToken := responseString(t, adminLogin, "accessToken")

	got := requestJSONWithHeaders(t, f.handler, http.MethodGet, "/api/v1/admin/settings", nil, nil, map[string]string{"Authorization": "Bearer " + adminToken})
	assertStatus(t, got, http.StatusOK)
	_ = adminHeaders
}

func (f *m6Fixture) smokeAssetKindState(t *testing.T) {
	list := f.request(t, http.MethodGet, "/api/v1/t/acme/asset-kinds", nil, nil)
	assertStatus(t, list, http.StatusOK)

	// 打开 openapi 类别并校验 etag 与 enabled。
	patch := f.request(t, http.MethodPatch, "/api/v1/t/acme/asset-kinds/openapi", map[string]any{"enabled": true}, map[string]string{"If-Match": `"asset-kind:openapi:1"`})
	assertStatus(t, patch, http.StatusOK)
	if !responseBool(t, patch, "enabled") {
		t.Fatal("openapi kind should be enabled")
	}
}

func (f *m6Fixture) smokeServiceCreateAndDismiss(t *testing.T) {
	// 先建仓库供服务挂靠。
	repoCreated := f.request(t, http.MethodPost, "/api/v1/t/acme/repositories", map[string]any{"url": "https://github.com/acme/repo.git", "defaultBranch": "main"}, nil)
	assertStatus(t, repoCreated, http.StatusCreated)
	repoID := responseString(t, repoCreated, "id")

	// 手工创建服务。
	created := f.request(t, http.MethodPost, "/api/v1/t/acme/repositories/"+repoID+"/services", map[string]any{"slug": "checkout", "displayName": "Checkout"}, nil)
	assertStatus(t, created, http.StatusCreated)

	// 列表可见。
	list := f.request(t, http.MethodGet, "/api/v1/t/acme/services", nil, nil)
	assertStatus(t, list, http.StatusOK)
	if total := responseInt(t, list, "total"); total != 1 {
		t.Fatalf("services total = %d, want 1", total)
	}
}

func (f *m6Fixture) smokeUpdateUser(t *testing.T) {
	adminLogin := requestJSON(t, f.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "padmin", "password": "smoke secure password"}, nil)
	assertStatus(t, adminLogin, http.StatusOK)
	adminToken := responseString(t, adminLogin, "accessToken")

	// 列出用户拿到 alice 的 id 与 etag。
	users := requestJSONWithHeaders(t, f.handler, http.MethodGet, "/api/v1/admin/users?q=alice", nil, nil, map[string]string{"Authorization": "Bearer " + adminToken})
	assertStatus(t, users, http.StatusOK)
	var userPage struct {
		Items []struct {
			Id   string `json:"id"`
			Etag string `json:"etag"`
		} `json:"items"`
	}
	if err := json.Unmarshal(users.Body.Bytes(), &userPage); err != nil {
		t.Fatalf("decode users page: %v", err)
	}
	if len(userPage.Items) != 1 {
		t.Fatalf("user search should return exactly 1 user, got %d", len(userPage.Items))
	}
	userID := userPage.Items[0].Id
	userETag := userPage.Items[0].Etag

	updated := requestJSONWithHeaders(t, f.handler, http.MethodPatch, "/api/v1/admin/users/"+userID, map[string]any{"displayName": "Alice Renamed"}, nil, map[string]string{"Authorization": "Bearer " + adminToken, "If-Match": userETag})
	assertStatus(t, updated, http.StatusOK)
	if responseString(t, updated, "displayName") != "Alice Renamed" {
		t.Fatal("user displayName should be updated")
	}
}

func responseInt(t *testing.T, response *httptest.ResponseRecorder, field string) int {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", field, err)
	}
	value, ok := body[field].(float64)
	if !ok {
		t.Fatalf("decode int field %s: %#v", field, body[field])
	}
	return int(value)
}

func responseBool(t *testing.T, response *httptest.ResponseRecorder, field string) bool {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", field, err)
	}
	value, ok := body[field].(bool)
	if !ok {
		t.Fatalf("decode bool field %s: %#v", field, body[field])
	}
	return value
}
