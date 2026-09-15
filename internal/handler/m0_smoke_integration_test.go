//go:build integration

package handler

import (
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"uuid"

	"github.com/meridian-labs/meridian/internal/database"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

func TestM0Smoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M0 smoke requires MERIDIAN_TEST_DATABASE_URL; use make smoke-all MILESTONE=M0")
	}

	t.Run("SMK-001", smokeM0MigrationHealth)
	t.Run("SMK-002", smokeM0LoginTenantSwitch)
	t.Run("SMK-003", smokeM0RepositoryIsolation)
	t.Run("SMK-004", smokeM0PATLifecycle)
	t.Run("SMK-029", smokeM0RepositoryQuota)
}

type m0SmokeFixture struct {
	db       *database.Database
	identity *service.Identity
	handler  http.Handler
	admin    service.Principal
	platform m0SmokeSession
}

type m0SmokeSession struct {
	token string
}

func newM0SmokeFixture(t *testing.T) *m0SmokeFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M0 smoke database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M0 smoke database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M0 smoke database: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE users, tenants CASCADE`); err != nil {
		t.Fatalf("reset M0 smoke identity fixtures: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M0 smoke queue fixtures: %v", err)
	}

	identityStore := repository.NewIdentityStore(db.Pool)
	digester, err := service.NewTokenDigester(integrationTokenPepper)
	if err != nil {
		t.Fatalf("construct M0 smoke token digester: %v", err)
	}
	jwtIssuer, err := service.NewJWTIssuer(integrationTokenPepper)
	if err != nil {
		t.Fatalf("construct JWT issuer: %v", err)
	}
	identity := service.NewIdentity(identityStore, digester, jwtIssuer)
	administrator, err := identity.BootstrapPlatformAdmin(t.Context(), service.CreateUserInput{
		Username: "padmin", DisplayName: "Platform Admin", Password: "smoke platform password",
	})
	if err != nil {
		t.Fatalf("bootstrap M0 smoke administrator: %v", err)
	}
	f := &m0SmokeFixture{
		db: db, identity: identity,
		admin: service.Principal{Kind: service.PrincipalJWT, User: administrator},
	}
	f.handler = NewWithAllServices(
		identity,
		nil,
		service.NewRepositories(repository.NewRepositoryStore(db.Pool), identityStore),
		false,
	).Handler()
	f.platform = f.login(t, "padmin", "smoke platform password")
	return f
}

func (f *m0SmokeFixture) login(t *testing.T, username, password string) m0SmokeSession {
	t.Helper()
	response := requestJSON(t, f.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": username, "password": password,
	}, nil)
	assertStatus(t, response, http.StatusOK)
	return m0SmokeSession{token: responseString(t, response, "accessToken")}
}

func (f *m0SmokeFixture) request(t *testing.T, session *m0SmokeSession, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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

func (f *m0SmokeFixture) provisionTenantUser(t *testing.T, tenantSlug, username, role string) m0SmokeSession {
	t.Helper()
	if _, err := f.identity.CreateTenant(t.Context(), f.admin, service.CreateTenantInput{Slug: tenantSlug, DisplayName: strings.ToUpper(tenantSlug)}); err != nil {
		t.Fatalf("create %s tenant: %v", tenantSlug, err)
	}
	user, err := f.identity.CreateUser(t.Context(), f.admin, service.CreateUserInput{
		Username: username, DisplayName: strings.ToUpper(username), Password: "smoke user password",
	})
	if err != nil {
		t.Fatalf("create %s user: %v", username, err)
	}
	if _, _, err := f.identity.PutTenantMembership(t.Context(), f.admin, tenantSlug, user.ID, role); err != nil {
		t.Fatalf("assign %s to %s: %v", username, tenantSlug, err)
	}
	return f.login(t, username, "smoke user password")
}

func (f *m0SmokeFixture) addTenantUser(t *testing.T, tenantSlug, username, role string) m0SmokeSession {
	t.Helper()
	user, err := f.identity.CreateUser(t.Context(), f.admin, service.CreateUserInput{
		Username: username, DisplayName: strings.ToUpper(username), Password: "smoke user password",
	})
	if err != nil {
		t.Fatalf("create %s user: %v", username, err)
	}
	if _, _, err := f.identity.PutTenantMembership(t.Context(), f.admin, tenantSlug, user.ID, role); err != nil {
		t.Fatalf("assign %s to %s: %v", username, tenantSlug, err)
	}
	return f.login(t, username, "smoke user password")
}

func smokeM0MigrationHealth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open migration smoke database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close migration smoke database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("operator migration up: %v", err)
	}
	initialSchema := m0SchemaSnapshot(t, db.SQL)
	initialStatus, err := db.MigrationStatus(t.Context())
	if err != nil {
		t.Fatalf("read initial migration status: %v", err)
	}
	if initialStatus.ApplicationVersion < 1 || len(initialStatus.RiverVersions) == 0 {
		t.Fatalf("initial migration status is incomplete: %#v", initialStatus)
	}

	server := New()
	assertM0Health(t, requestJSON(t, server.Handler(), http.MethodGet, "/healthz", nil, nil))
	assertM0Health(t, requestJSON(t, server.Handler(), http.MethodGet, "/readyz", nil, nil))
	server.ready.Store(false)
	assertM0Health(t, requestJSON(t, server.Handler(), http.MethodGet, "/healthz", nil, nil))
	assertStatus(t, requestJSON(t, server.Handler(), http.MethodGet, "/readyz", nil, nil), http.StatusServiceUnavailable)

	if err := db.MigrateDown(t.Context(), int(initialStatus.ApplicationVersion)); err != nil {
		t.Fatalf("operator migration down: %v", err)
	}
	downStatus, err := db.MigrationStatus(t.Context())
	if err != nil {
		t.Fatalf("read fully rolled back status: %v", err)
	}
	if downStatus.ApplicationVersion != 0 || len(downStatus.RiverVersions) != 0 {
		t.Fatalf("operator down did not fully roll back: %#v", downStatus)
	}
	assertStatus(t, requestJSON(t, server.Handler(), http.MethodGet, "/readyz", nil, nil), http.StatusServiceUnavailable)

	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("operator second migration up: %v", err)
	}
	if finalSchema := m0SchemaSnapshot(t, db.SQL); !reflect.DeepEqual(initialSchema, finalSchema) {
		t.Fatalf("schema differs after operator up/down/up\ninitial: %v\nfinal: %v", initialSchema, finalSchema)
	}
	server.ready.Store(true)
	assertM0Health(t, requestJSON(t, server.Handler(), http.MethodGet, "/readyz", nil, nil))
}

func assertM0Health(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	assertStatus(t, response, http.StatusOK)
	if status := responseString(t, response, "status"); status != "ok" {
		t.Fatalf("health status = %q, want ok", status)
	}
}

func m0SchemaSnapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `
		SELECT c.relname || '.' || a.attname || ':' || format_type(a.atttypid, a.atttypmod) || ':' || a.attnotnull::text || ':' || coalesce(col_description(c.oid, a.attnum), '')
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname <> 'goose_db_version'
		  AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY c.relname, a.attnum
	`)
	if err != nil {
		t.Fatalf("snapshot M0 schema: %v", err)
	}
	defer rows.Close()
	var snapshot []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan M0 schema snapshot: %v", err)
		}
		snapshot = append(snapshot, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate M0 schema snapshot: %v", err)
	}
	return snapshot
}

func smokeM0LoginTenantSwitch(t *testing.T) {
	f := newM0SmokeFixture(t)
	createdUser := f.request(t, &f.platform, http.MethodPost, "/api/v1/admin/users", map[string]any{
		"username": "alice", "displayName": "Alice", "password": "alice smoke password",
	}, nil)
	assertStatus(t, createdUser, http.StatusCreated)
	userID := responseString(t, createdUser, "id")
	createdTenant := f.request(t, &f.platform, http.MethodPost, "/api/v1/admin/tenants", map[string]any{
		"slug": "acme", "displayName": "Acme",
	}, nil)
	assertStatus(t, createdTenant, http.StatusCreated)
	duplicate := f.request(t, &f.platform, http.MethodPost, "/api/v1/admin/tenants", map[string]any{
		"slug": "acme", "displayName": "Duplicate Acme",
	}, nil)
	assertError(t, duplicate, http.StatusConflict, "duplicate")
	member := f.request(t, &f.platform, http.MethodPut, "/api/v1/admin/tenants/acme/members/"+userID, map[string]any{"role": "tenant_admin"}, nil)
	assertStatus(t, member, http.StatusOK)

	disabledTenant := f.request(t, &f.platform, http.MethodPost, "/api/v1/admin/tenants", map[string]any{
		"slug": "rival", "displayName": "Rival",
	}, nil)
	assertStatus(t, disabledTenant, http.StatusCreated)
	disabledMember := f.request(t, &f.platform, http.MethodPut, "/api/v1/admin/tenants/rival/members/"+userID, map[string]any{"role": "tenant_admin"}, nil)
	assertStatus(t, disabledMember, http.StatusOK)
	disabled := f.request(t, &f.platform, http.MethodPatch, "/api/v1/admin/tenants/rival", map[string]any{"status": "disabled"}, map[string]string{
		"If-Match": responseString(t, disabledTenant, "etag"),
	})
	assertStatus(t, disabled, http.StatusOK)

	alice := f.login(t, "alice", "alice smoke password")
	loginAgain := requestJSON(t, f.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "alice", "password": "alice smoke password",
	}, nil)
	assertStatus(t, loginAgain, http.StatusOK)
	assertTenantMembership(t, loginAgain, "acme", "tenant_admin")
	if strings.Contains(loginAgain.Body.String(), `"slug":"rival"`) {
		t.Fatalf("disabled rival tenant leaked into login projection: %s", loginAgain.Body.String())
	}
	me := f.request(t, &alice, http.MethodGet, "/api/v1/auth/me", nil, nil)
	assertStatus(t, me, http.StatusOK)
	assertTenantMembership(t, me, "acme", "tenant_admin")
	if strings.Contains(me.Body.String(), `"slug":"rival"`) {
		t.Fatalf("disabled rival tenant leaked into current identity: %s", me.Body.String())
	}
	platformTenantRead := f.request(t, &f.platform, http.MethodGet, "/api/v1/t/acme/repositories", nil, nil)
	assertError(t, platformTenantRead, http.StatusNotFound, "not_found")
}

func smokeM0RepositoryIsolation(t *testing.T) {
	f := newM0SmokeFixture(t)
	acme := f.provisionTenantUser(t, "acme", "alice", "tenant_admin")
	rival := f.provisionTenantUser(t, "rival", "robin", "tenant_admin")
	viewer := f.addTenantUser(t, "rival", "victor", "viewer")
	created := f.request(t, &rival, http.MethodPost, "/api/v1/t/rival/repositories", map[string]any{
		"url": "https://git.example.com/rival.git", "defaultBranch": "main",
	}, nil)
	assertStatus(t, created, http.StatusCreated)
	repositoryID := responseString(t, created, "id")
	repositoryETag := created.Header().Get("ETag")
	if repositoryETag == "" {
		t.Fatal("repository create response has no ETag header")
	}
	missingID := uuid.NewV7().String()

	operations := []struct {
		name    string
		method  string
		body    any
		headers func(string, string) map[string]string
	}{
		{name: "getRepository", method: http.MethodGet, headers: func(_, _ string) map[string]string { return nil }},
		{name: "updateRepository", method: http.MethodPatch, body: map[string]any{"note": "isolation probe"}, headers: func(id, etag string) map[string]string {
			if etag == "" {
				etag = fmt.Sprintf(`"repository:%s:1"`, id)
			}
			return map[string]string{"If-Match": etag}
		}},
		{name: "deleteRepository", method: http.MethodDelete, headers: func(id, etag string) map[string]string {
			if etag == "" {
				etag = fmt.Sprintf(`"repository:%s:1"`, id)
			}
			return map[string]string{"If-Match": etag}
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			crossTenant := f.request(t, &acme, operation.method, "/api/v1/t/acme/repositories/"+repositoryID, operation.body, operation.headers(repositoryID, repositoryETag))
			assertError(t, crossTenant, http.StatusNotFound, "not_found")
			missing := f.request(t, &acme, operation.method, "/api/v1/t/acme/repositories/"+missingID, operation.body, operation.headers(missingID, ""))
			assertError(t, missing, http.StatusNotFound, "not_found")
			if crossBody, missingBody := normalizedM0Error(t, crossTenant), normalizedM0Error(t, missing); !reflect.DeepEqual(crossBody, missingBody) {
				t.Fatalf("cross-tenant and missing errors differ: cross=%#v missing=%#v", crossBody, missingBody)
			}
			if strings.Contains(crossTenant.Body.String(), "rival") || strings.Contains(crossTenant.Body.String(), repositoryID) {
				t.Fatalf("cross-tenant error leaked resource fields: %s", crossTenant.Body.String())
			}
			unauthenticated := f.request(t, nil, operation.method, "/api/v1/t/rival/repositories/"+repositoryID, operation.body, operation.headers(repositoryID, repositoryETag))
			assertError(t, unauthenticated, http.StatusUnauthorized, "unauthenticated")
		})
	}

	for _, mutation := range operations[1:] {
		denied := f.request(t, &viewer, mutation.method, "/api/v1/t/rival/repositories/"+repositoryID, mutation.body, mutation.headers(repositoryID, repositoryETag))
		assertError(t, denied, http.StatusNotFound, "not_found")
	}
	assertStatus(t, f.request(t, &rival, http.MethodGet, "/api/v1/t/rival/repositories/"+repositoryID, nil, nil), http.StatusOK)
}

func normalizedM0Error(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode M0 error response: %v", err)
	}
	delete(payload, "requestId")
	return payload
}

func smokeM0PATLifecycle(t *testing.T) {
	f := newM0SmokeFixture(t)
	alice := f.provisionTenantUser(t, "acme", "alice", "tenant_admin")
	created := f.request(t, &alice, http.MethodPost, "/api/v1/t/acme/tokens", map[string]any{
		"name": "smoke automation", "scopes": []string{"asset:read"},
	}, nil)
	assertStatus(t, created, http.StatusCreated)
	plaintext := responseString(t, created, "token")
	if !strings.HasPrefix(plaintext, "pat_") {
		t.Fatalf("created PAT prefix = %q, want pat_", plaintext)
	}
	tokenID := responseString(t, created, "id")
	listed := f.request(t, &alice, http.MethodGet, "/api/v1/t/acme/tokens", nil, nil)
	assertStatus(t, listed, http.StatusOK)
	if strings.Contains(listed.Body.String(), plaintext) || strings.Contains(listed.Body.String(), `"token"`) {
		t.Fatalf("token list disclosed bearer material: %s", listed.Body.String())
	}
	revoked := f.request(t, &alice, http.MethodDelete, "/api/v1/t/acme/tokens/"+tokenID, nil, nil)
	assertStatus(t, revoked, http.StatusNoContent)
	revokedRequest := f.request(t, nil, http.MethodGet, "/api/v1/t/acme/repositories", nil, map[string]string{
		"Authorization": "Bearer " + plaintext,
	})
	assertError(t, revokedRequest, http.StatusUnauthorized, "unauthenticated")
}

func smokeM0RepositoryQuota(t *testing.T) {
	f := newM0SmokeFixture(t)
	alice := f.provisionTenantUser(t, "acme", "alice", "tenant_admin")
	if _, err := f.db.Pool.Exec(t.Context(), `UPDATE tenants SET quota = jsonb_set(quota, '{maxRepositories}', '1'::jsonb) WHERE slug = 'acme'`); err != nil {
		t.Fatalf("set repository smoke quota: %v", err)
	}
	first := f.request(t, &alice, http.MethodPost, "/api/v1/t/acme/repositories", map[string]any{
		"url": "https://git.example.com/first.git", "defaultBranch": "main",
	}, nil)
	assertStatus(t, first, http.StatusCreated)
	rejected := f.request(t, &alice, http.MethodPost, "/api/v1/t/acme/repositories", map[string]any{
		"url": "https://git.example.com/second.git", "defaultBranch": "main",
	}, nil)
	assertError(t, rejected, http.StatusConflict, "quota_exceeded")
	var payload struct {
		Details struct {
			Quota   string `json:"quota"`
			Current int64  `json:"current"`
			Limit   int64  `json:"limit"`
		} `json:"details"`
	}
	if err := json.Unmarshal(rejected.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode quota response: %v", err)
	}
	if payload.Details.Quota != "repositories" || payload.Details.Current != 1 || payload.Details.Limit != 1 {
		t.Fatalf("quota details = %#v", payload.Details)
	}
	var count int
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM repositories r JOIN tenants t ON t.id = r.tenant_id WHERE t.slug = 'acme' AND r.deleted_at IS NULL`).Scan(&count); err != nil {
		t.Fatalf("count repositories after quota rejection: %v", err)
	}
	if count != 1 {
		t.Fatalf("repository count after quota rejection = %d, want 1", count)
	}
}
