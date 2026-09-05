//go:build integration

package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"uuid"

	"github.com/meridian-labs/meridian/internal/database"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/task"
)

const integrationTokenPepper = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestIdentityHTTPWorkflow(t *testing.T) {
	databaseURL := os.Getenv("MERIDIAN_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MERIDIAN_TEST_DATABASE_URL is not set")
	}

	db, err := database.Open(t.Context(), databaseURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE users, tenants CASCADE`); err != nil {
		t.Fatalf("reset identity fixtures: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset identity River fixtures: %v", err)
	}

	store := repository.NewIdentityStore(db.Pool)
	digester, err := service.NewTokenDigester(integrationTokenPepper)
	if err != nil {
		t.Fatalf("construct token digester: %v", err)
	}
	identity := service.NewIdentity(store, digester)
	keyring, err := service.NewCredentialKeyring(
		base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32)),
		1,
		base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32)),
	)
	if err != nil {
		t.Fatalf("construct credential keyring: %v", err)
	}
	if _, err := identity.BootstrapPlatformAdmin(t.Context(), service.CreateUserInput{
		Username: "padmin", Password: "correct horse battery staple", DisplayName: "Platform Admin",
	}); err != nil {
		t.Fatalf("bootstrap platform administrator: %v", err)
	}
	repositoryStore := repository.NewRepositoryStore(db.Pool)
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("configure identity River runtime: %v", err)
	}
	credentials := service.NewCredentials(repository.NewCredentialStoreWithRiver(db.Pool, runtime.Client()), store, keyring)
	repositories := service.NewRepositories(repositoryStore, store)
	jobs := service.NewJobs(repository.NewJobControlStore(db.Pool, runtime.Client()), store)
	audits := service.NewAudits(repositoryStore, store)
	httpHandler := NewWithRuntimeServices(Dependencies{
		Identity: identity, Credentials: credentials, Repositories: repositories, Jobs: jobs, Audits: audits,
	}, false).Handler()

	adminLogin := requestJSON(t, httpHandler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "padmin", "password": "correct horse battery staple",
	}, nil)
	assertStatus(t, adminLogin, http.StatusOK)
	adminCookie := responseCookie(t, adminLogin, sessionCookieName)
	adminCSRF := responseString(t, adminLogin, "csrfToken")

	missingCSRF := requestJSON(t, httpHandler, http.MethodPost, "/api/v1/admin/users", map[string]any{
		"username": "alice", "displayName": "Alice", "password": "alice secure password",
	}, []*http.Cookie{adminCookie})
	assertError(t, missingCSRF, http.StatusForbidden, "csrf_invalid")

	rotated := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/auth/csrf", nil, []*http.Cookie{adminCookie})
	assertStatus(t, rotated, http.StatusOK)
	rotatedCSRF := responseString(t, rotated, "csrfToken")
	if rotatedCSRF == adminCSRF {
		t.Fatal("rotated CSRF token equals the login token")
	}

	staleCSRF := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/admin/users", map[string]any{
		"username": "alice", "displayName": "Alice", "password": "alice secure password",
	}, []*http.Cookie{adminCookie}, map[string]string{csrfHeaderName: adminCSRF})
	assertError(t, staleCSRF, http.StatusForbidden, "csrf_invalid")

	createdUser := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/admin/users", map[string]any{
		"username": "alice", "displayName": "Alice", "password": "alice secure password",
	}, []*http.Cookie{adminCookie}, map[string]string{csrfHeaderName: rotatedCSRF})
	assertStatus(t, createdUser, http.StatusCreated)
	userID, err := uuid.Parse(responseString(t, createdUser, "id"))
	if err != nil {
		t.Fatalf("parse created user ID: %v", err)
	}

	createdTenant := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/admin/tenants", map[string]any{
		"slug": "acme", "displayName": "Acme",
	}, []*http.Cookie{adminCookie}, map[string]string{csrfHeaderName: rotatedCSRF})
	assertStatus(t, createdTenant, http.StatusCreated)
	listedUsers := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/admin/users?q=alice", nil, []*http.Cookie{adminCookie})
	assertStatus(t, listedUsers, http.StatusOK)
	if !strings.Contains(listedUsers.Body.String(), `"username":"alice"`) || strings.Contains(listedUsers.Body.String(), "password_hash") {
		t.Fatalf("platform user directory = %s", listedUsers.Body.String())
	}
	listedTenants := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/admin/tenants", nil, []*http.Cookie{adminCookie})
	assertStatus(t, listedTenants, http.StatusOK)
	if !strings.Contains(listedTenants.Body.String(), `"slug":"acme"`) || strings.Contains(listedTenants.Body.String(), `"settings"`) {
		t.Fatalf("platform tenant directory leaked settings or omitted tenant: %s", listedTenants.Body.String())
	}
	duplicateTenant := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/admin/tenants", map[string]any{
		"slug": "acme", "displayName": "Duplicate Acme",
	}, []*http.Cookie{adminCookie}, map[string]string{csrfHeaderName: rotatedCSRF})
	assertError(t, duplicateTenant, http.StatusConflict, "duplicate")

	membershipPath := "/api/v1/admin/tenants/acme/members/" + userID.String()
	member := requestJSONWithHeaders(t, httpHandler, http.MethodPut, membershipPath, map[string]any{
		"role": "tenant_admin",
	}, []*http.Cookie{adminCookie}, map[string]string{csrfHeaderName: rotatedCSRF})
	assertStatus(t, member, http.StatusOK)

	aliceLogin := requestJSON(t, httpHandler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "alice", "password": "alice secure password",
	}, nil)
	assertStatus(t, aliceLogin, http.StatusOK)
	aliceCookie := responseCookie(t, aliceLogin, sessionCookieName)
	aliceCSRF := responseString(t, aliceLogin, "csrfToken")
	assertTenantMembership(t, aliceLogin, "acme", "tenant_admin")
	forbiddenPlatformUsers := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/admin/users", nil, []*http.Cookie{aliceCookie})
	assertError(t, forbiddenPlatformUsers, http.StatusNotFound, "not_found")

	me := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/auth/me", nil, []*http.Cookie{aliceCookie})
	assertStatus(t, me, http.StatusOK)
	assertTenantMembership(t, me, "acme", "tenant_admin")

	var tenantID uuid.UUID
	if err := db.Pool.QueryRow(t.Context(), `SELECT id FROM tenants WHERE slug = 'acme'`).Scan(&tenantID); err != nil {
		t.Fatalf("resolve audit tenant fixture: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `
		INSERT INTO audit_logs (id, tenant_id, actor_id, actor_type, action, target_type, target_id, detail, request_id)
		VALUES
		  ($1, $2, $3, 'user', 'repository.created', 'repository', $4, '{"source":"api"}'::jsonb, $5),
		  ($6, NULL, NULL, 'system', 'runtime.started', 'runtime', NULL, '{"workers":4}'::jsonb, NULL)
	`, uuid.NewV7(), tenantID, userID, uuid.NewV7(), uuid.NewV7(), uuid.NewV7()); err != nil {
		t.Fatalf("create audit fixtures: %v", err)
	}
	tenantAudits := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/audit-logs", nil, []*http.Cookie{aliceCookie})
	assertStatus(t, tenantAudits, http.StatusOK)
	assertAuditPage(t, tenantAudits, 1, "acme", "repository.created")
	platformAudits := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/admin/audit-logs?filter%5BtenantSlug%5D=acme", nil, []*http.Cookie{adminCookie})
	assertStatus(t, platformAudits, http.StatusOK)
	assertAuditPage(t, platformAudits, 1, "acme", "repository.created")
	forbiddenPlatformAudits := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/admin/audit-logs", nil, []*http.Cookie{aliceCookie})
	assertError(t, forbiddenPlatformAudits, http.StatusNotFound, "not_found")

	invalidBearer := requestJSONWithHeaders(t, httpHandler, http.MethodGet, "/api/v1/auth/me", nil, []*http.Cookie{aliceCookie}, map[string]string{
		"Authorization": "Bearer invalid",
	})
	assertError(t, invalidBearer, http.StatusUnauthorized, "unauthenticated")

	createdToken := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/tokens", map[string]any{
		"name": "release automation", "scopes": []string{"asset:read", "asset:push"},
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF})
	assertStatus(t, createdToken, http.StatusCreated)
	plaintextPAT := responseString(t, createdToken, "token")
	if !strings.HasPrefix(plaintextPAT, "pat_") {
		t.Fatalf("created token prefix = %q, want pat_", plaintextPAT)
	}
	tokenID := responseString(t, createdToken, "id")
	if _, err := identity.AuthenticatePAT(t.Context(), plaintextPAT); err != nil {
		t.Fatalf("authenticate new PAT: %v", err)
	}

	listedTokens := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/tokens", nil, []*http.Cookie{aliceCookie})
	assertStatus(t, listedTokens, http.StatusOK)
	if strings.Contains(listedTokens.Body.String(), plaintextPAT) || strings.Contains(listedTokens.Body.String(), `"token"`) {
		t.Fatal("token list disclosed PAT bearer material or a token field")
	}

	revokePath := "/api/v1/t/acme/tokens/" + tokenID
	for attempt := range 2 {
		revoked := requestJSONWithHeaders(t, httpHandler, http.MethodDelete, revokePath, nil, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF})
		assertStatus(t, revoked, http.StatusNoContent)
		if attempt == 0 && !errors.Is(authenticatePAT(identity, t, plaintextPAT), service.ErrUnauthenticated) {
			t.Fatal("revoked PAT remained authenticated")
		}
	}
	revokedRequest := requestJSONWithHeaders(t, httpHandler, http.MethodGet, "/api/v1/t/acme/tokens", nil, nil, map[string]string{
		"Authorization": "Bearer " + plaintextPAT,
	})
	assertError(t, revokedRequest, http.StatusUnauthorized, "unauthenticated")

	createdCredential := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/credentials", map[string]any{
		"name": "integration credential", "kind": "http_token", "httpToken": map[string]any{"username": "bot", "token": "first-secret-token"},
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF})
	assertStatus(t, createdCredential, http.StatusCreated)
	if strings.Contains(createdCredential.Body.String(), "first-secret-token") || strings.Contains(createdCredential.Body.String(), "\"token\"") {
		t.Fatal("credential create response disclosed secret material")
	}
	credentialID, err := uuid.Parse(responseString(t, createdCredential, "id"))
	if err != nil {
		t.Fatalf("parse credential ID: %v", err)
	}
	credentialETag := responseString(t, createdCredential, "etag")
	testedCredential := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/credentials/"+credentialID.String()+":test", map[string]any{
		"repositoryUrl": "https://127.0.0.1:1/repository.git",
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF})
	assertStatus(t, testedCredential, http.StatusOK)
	var connectionResult struct {
		OK          bool   `json:"ok"`
		ErrorClass  string `json:"errorClass"`
		Message     string `json:"message"`
		HostKeyData any    `json:"hostKeyCandidate"`
	}
	if err := json.Unmarshal(testedCredential.Body.Bytes(), &connectionResult); err != nil {
		t.Fatalf("decode credential connection result: %v", err)
	}
	if connectionResult.OK || connectionResult.ErrorClass == "" || connectionResult.Message == "" {
		t.Fatalf("credential connection result = %#v, want structured unsuccessful result", connectionResult)
	}
	if strings.Contains(testedCredential.Body.String(), "first-secret-token") {
		t.Fatal("credential connection response disclosed secret material")
	}
	rotationHeaders := map[string]string{
		csrfHeaderName:    aliceCSRF,
		"If-Match":        credentialETag,
		"Idempotency-Key": uuid.NewV7().String(),
	}
	rotatedCredential := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/credentials/"+credentialID.String()+":rotate", map[string]any{
		"secret": map[string]any{"username": "bot", "token": "second-secret-token"}, "resyncRepositories": true,
	}, []*http.Cookie{aliceCookie}, rotationHeaders)
	assertStatus(t, rotatedCredential, http.StatusOK)
	if strings.Contains(rotatedCredential.Body.String(), "second-secret-token") || strings.Contains(rotatedCredential.Body.String(), "first-secret-token") {
		t.Fatal("credential rotation response disclosed secret material")
	}
	replay := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/credentials/"+credentialID.String()+":rotate", map[string]any{
		"secret": map[string]any{"username": "bot", "token": "second-secret-token"}, "resyncRepositories": true,
	}, []*http.Cookie{aliceCookie}, rotationHeaders)
	assertStatus(t, replay, http.StatusOK)
	if replay.Body.String() != rotatedCredential.Body.String() {
		t.Fatalf("idempotent rotation body differs\nfirst: %s\nreplay: %s", rotatedCredential.Body.String(), replay.Body.String())
	}
	conflictHeaders := map[string]string{csrfHeaderName: aliceCSRF, "If-Match": credentialETag, "Idempotency-Key": rotationHeaders["Idempotency-Key"]}
	conflict := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/credentials/"+credentialID.String()+":rotate", map[string]any{
		"secret": map[string]any{"username": "bot", "token": "third-secret-token"}, "resyncRepositories": true,
	}, []*http.Cookie{aliceCookie}, conflictHeaders)
	assertError(t, conflict, http.StatusConflict, "idempotency_conflict")

	createdGlobal := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/admin/global-credentials", map[string]any{
		"name": "integration global credential", "kind": "http_token", "httpToken": map[string]any{"username": "global-bot", "token": "global-first-token"},
	}, []*http.Cookie{adminCookie}, map[string]string{csrfHeaderName: rotatedCSRF})
	assertStatus(t, createdGlobal, http.StatusCreated)
	if strings.Contains(createdGlobal.Body.String(), "global-first-token") || strings.Contains(createdGlobal.Body.String(), "\"token\"") {
		t.Fatal("global credential create response disclosed secret material")
	}
	globalID, err := uuid.Parse(responseString(t, createdGlobal, "id"))
	if err != nil {
		t.Fatalf("parse global credential ID: %v", err)
	}
	globalETag := responseString(t, createdGlobal, "etag")
	testedGlobal := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/admin/global-credentials/"+globalID.String()+":test", map[string]any{
		"repositoryUrl": "https://127.0.0.1:1/repository.git",
	}, []*http.Cookie{adminCookie}, map[string]string{csrfHeaderName: rotatedCSRF})
	assertStatus(t, testedGlobal, http.StatusOK)
	if strings.Contains(testedGlobal.Body.String(), "global-first-token") {
		t.Fatal("global credential connection response disclosed secret material")
	}
	checkedRepository := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/repositories:check-connection", map[string]any{
		"url": "https://127.0.0.1:1/repository.git", "credentialId": nil,
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF})
	assertStatus(t, checkedRepository, http.StatusOK)
	if strings.Contains(checkedRepository.Body.String(), "first-secret-token") {
		t.Fatal("repository connection response disclosed secret material")
	}
	if _, err := db.Pool.Exec(t.Context(), `UPDATE tenants SET quota = jsonb_set(quota, '{maxRepositories}', '1'::jsonb) WHERE slug = 'acme'`); err != nil {
		t.Fatalf("set repository quota: %v", err)
	}
	createdRepository := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/repositories", map[string]any{
		"url": "https://git.example.com:443/Team/Repo.git/", "credentialId": credentialID.String(), "defaultBranch": "main",
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF})
	assertStatus(t, createdRepository, http.StatusCreated)
	repositoryID, err := uuid.Parse(responseString(t, createdRepository, "id"))
	if err != nil {
		t.Fatalf("parse repository ID: %v", err)
	}
	repositoryETag := responseString(t, createdRepository, "etag")
	if !strings.Contains(createdRepository.Body.String(), "https://git.example.com:443/Team/Repo.git/") || !strings.Contains(createdRepository.Body.String(), "main") {
		t.Fatalf("repository response lost display/default values: %s", createdRepository.Body.String())
	}
	listedRepositories := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/repositories", nil, []*http.Cookie{aliceCookie})
	assertStatus(t, listedRepositories, http.StatusOK)
	var repositoryPage struct {
		Total int `json:"total"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listedRepositories.Body.Bytes(), &repositoryPage); err != nil {
		t.Fatalf("decode repository page: %v", err)
	}
	if repositoryPage.Total != 1 || len(repositoryPage.Items) != 1 || repositoryPage.Items[0].ID != repositoryID.String() {
		t.Fatalf("repository page = %#v", repositoryPage)
	}
	updatedRepository := requestJSONWithHeaders(t, httpHandler, http.MethodPatch, "/api/v1/t/acme/repositories/"+repositoryID.String(), map[string]any{
		"defaultBranch": "develop", "note": "managed by integration",
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF, "If-Match": repositoryETag})
	assertStatus(t, updatedRepository, http.StatusOK)
	repositoryETag = responseString(t, updatedRepository, "etag")
	if !strings.Contains(updatedRepository.Body.String(), "develop") || !strings.Contains(updatedRepository.Body.String(), "managed by integration") {
		t.Fatalf("repository patch response = %s", updatedRepository.Body.String())
	}
	staleRepositoryUpdate := requestJSONWithHeaders(t, httpHandler, http.MethodPatch, "/api/v1/t/acme/repositories/"+repositoryID.String(), map[string]any{
		"note": nil,
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF, "If-Match": `"repository:` + repositoryID.String() + `:1"`})
	assertError(t, staleRepositoryUpdate, http.StatusPreconditionFailed, "precondition_failed")
	quotaRejected := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/repositories", map[string]any{
		"url": "https://git.example.com/another.git", "defaultBranch": "main",
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF})
	assertError(t, quotaRejected, http.StatusConflict, "quota_exceeded")
	var quotaPayload struct {
		Details struct {
			Quota   string `json:"quota"`
			Current int64  `json:"current"`
			Limit   int64  `json:"limit"`
		} `json:"details"`
	}
	if err := json.Unmarshal(quotaRejected.Body.Bytes(), &quotaPayload); err != nil {
		t.Fatalf("decode quota error: %v", err)
	}
	if quotaPayload.Details.Quota != "repositories" || quotaPayload.Details.Current != 1 || quotaPayload.Details.Limit != 1 {
		t.Fatalf("quota details = %#v", quotaPayload.Details)
	}
	listedAfterQuota := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/repositories", nil, []*http.Cookie{aliceCookie})
	assertStatus(t, listedAfterQuota, http.StatusOK)
	if !strings.Contains(listedAfterQuota.Body.String(), `"total":1`) {
		t.Fatalf("repository count changed after rejected create: %s", listedAfterQuota.Body.String())
	}
	boundGlobal := requestJSONWithHeaders(t, httpHandler, http.MethodPatch, "/api/v1/t/acme/repositories/"+repositoryID.String(), map[string]any{
		"credentialId": globalID.String(),
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF, "If-Match": repositoryETag})
	assertStatus(t, boundGlobal, http.StatusOK)
	repositoryETag = responseString(t, boundGlobal, "etag")
	if !strings.Contains(boundGlobal.Body.String(), globalID.String()) {
		t.Fatalf("global credential binding missing from repository: %s", boundGlobal.Body.String())
	}
	tenantCredentials := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/credentials", nil, []*http.Cookie{aliceCookie})
	assertStatus(t, tenantCredentials, http.StatusOK)
	if !strings.Contains(tenantCredentials.Body.String(), "integration global credential") || !strings.Contains(tenantCredentials.Body.String(), `"isGlobal":true`) {
		t.Fatalf("tenant credential list did not expose selectable global credential: %s", tenantCredentials.Body.String())
	}
	globalRotationHeaders := map[string]string{
		csrfHeaderName:    rotatedCSRF,
		"If-Match":        globalETag,
		"Idempotency-Key": uuid.NewV7().String(),
	}
	rotatedGlobal := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/admin/global-credentials/"+globalID.String()+":rotate", map[string]any{
		"secret": map[string]any{"username": "global-bot", "token": "global-second-token"}, "resyncRepositories": true,
	}, []*http.Cookie{adminCookie}, globalRotationHeaders)
	assertStatus(t, rotatedGlobal, http.StatusOK)
	if strings.Contains(rotatedGlobal.Body.String(), "global-second-token") || strings.Contains(rotatedGlobal.Body.String(), "global-first-token") {
		t.Fatal("global credential rotation response disclosed secret material")
	}
	var globalRotationBody struct {
		SyncJobs []struct {
			JobID uuid.UUID `json:"jobId"`
		} `json:"syncJobs"`
	}
	if err := json.Unmarshal(rotatedGlobal.Body.Bytes(), &globalRotationBody); err != nil {
		t.Fatalf("decode global rotation jobs: %v", err)
	}
	if len(globalRotationBody.SyncJobs) != 1 || globalRotationBody.SyncJobs[0].JobID == uuid.Nil() {
		t.Fatalf("global rotation jobs = %#v", globalRotationBody.SyncJobs)
	}
	platformJobs := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/admin/jobs", nil, []*http.Cookie{adminCookie})
	assertStatus(t, platformJobs, http.StatusOK)
	if !strings.Contains(platformJobs.Body.String(), globalRotationBody.SyncJobs[0].JobID.String()) || strings.Contains(platformJobs.Body.String(), "global-second-token") || strings.Contains(platformJobs.Body.String(), "credentialId") {
		t.Fatalf("platform job projection leaked or omitted data: %s", platformJobs.Body.String())
	}
	platformJob := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/admin/jobs/"+globalRotationBody.SyncJobs[0].JobID.String(), nil, []*http.Cookie{adminCookie})
	assertStatus(t, platformJob, http.StatusOK)
	if strings.Contains(platformJob.Body.String(), "input") || strings.Contains(platformJob.Body.String(), "result") || strings.Contains(platformJob.Body.String(), "error") {
		t.Fatalf("platform job detail contains redacted fields: %s", platformJob.Body.String())
	}
	platformJobAsTenant := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/admin/jobs/"+globalRotationBody.SyncJobs[0].JobID.String(), nil, []*http.Cookie{aliceCookie})
	assertError(t, platformJobAsTenant, http.StatusNotFound, "not_found")
	tenantJobs := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/jobs", nil, []*http.Cookie{aliceCookie})
	assertStatus(t, tenantJobs, http.StatusOK)
	if !strings.Contains(tenantJobs.Body.String(), globalRotationBody.SyncJobs[0].JobID.String()) || strings.Contains(tenantJobs.Body.String(), `"input"`) || strings.Contains(tenantJobs.Body.String(), `"riverJobId"`) {
		t.Fatalf("tenant job page omitted job or leaked internals: %s", tenantJobs.Body.String())
	}
	tenantJob := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/jobs/"+globalRotationBody.SyncJobs[0].JobID.String(), nil, []*http.Cookie{aliceCookie})
	assertStatus(t, tenantJob, http.StatusOK)
	if !strings.Contains(tenantJob.Body.String(), `"status":"pending"`) || !strings.Contains(tenantJob.Body.String(), `"capabilities":["job:read","job:run"]`) || strings.Contains(tenantJob.Body.String(), `"input"`) {
		t.Fatalf("tenant job detail has invalid projection: %s", tenantJob.Body.String())
	}
	crossTenantJob := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/other/jobs/"+globalRotationBody.SyncJobs[0].JobID.String(), nil, []*http.Cookie{aliceCookie})
	assertError(t, crossTenantJob, http.StatusNotFound, "not_found")
	jobPAT := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/tokens", map[string]any{
		"name": "job automation", "scopes": []string{"job:run"},
	}, []*http.Cookie{aliceCookie}, map[string]string{csrfHeaderName: aliceCSRF})
	assertStatus(t, jobPAT, http.StatusCreated)
	jobPATValue := responseString(t, jobPAT, "token")
	jobViaPAT := requestJSONWithHeaders(t, httpHandler, http.MethodGet, "/api/v1/t/acme/jobs/"+globalRotationBody.SyncJobs[0].JobID.String(), nil, nil, map[string]string{
		"Authorization": "Bearer " + jobPATValue,
	})
	assertStatus(t, jobViaPAT, http.StatusOK)

	cancelledJob := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/jobs/"+globalRotationBody.SyncJobs[0].JobID.String()+":cancel", nil, []*http.Cookie{aliceCookie}, map[string]string{
		csrfHeaderName: aliceCSRF,
	})
	assertStatus(t, cancelledJob, http.StatusAccepted)
	if responseString(t, cancelledJob, "jobId") != globalRotationBody.SyncJobs[0].JobID.String() {
		t.Fatalf("cancelled job response = %s", cancelledJob.Body.String())
	}
	cancelledAgain := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/jobs/"+globalRotationBody.SyncJobs[0].JobID.String()+":cancel", nil, []*http.Cookie{aliceCookie}, map[string]string{
		csrfHeaderName: aliceCSRF,
	})
	assertError(t, cancelledAgain, http.StatusConflict, "job_not_cancellable")
	terminalStream := requestJSONWithHeaders(t, httpHandler, http.MethodGet, "/api/v1/t/acme/jobs/"+globalRotationBody.SyncJobs[0].JobID.String()+"/logs", nil, []*http.Cookie{aliceCookie}, map[string]string{
		"Last-Event-ID": "0",
	})
	assertStatus(t, terminalStream, http.StatusOK)
	if terminalStream.Header().Get("Cache-Control") != "no-cache" || terminalStream.Header().Get("X-Accel-Buffering") != "no" || !strings.Contains(terminalStream.Body.String(), "event: state") || !strings.Contains(terminalStream.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("terminal job stream headers/body = %#v/%s", terminalStream.Header(), terminalStream.Body.String())
	}

	retryKey := uuid.NewV7().String()
	retryHeaders := map[string]string{csrfHeaderName: aliceCSRF, "Idempotency-Key": retryKey}
	retriedJob := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/jobs/"+globalRotationBody.SyncJobs[0].JobID.String()+":retry", nil, []*http.Cookie{aliceCookie}, retryHeaders)
	assertStatus(t, retriedJob, http.StatusAccepted)
	retriedJobID := responseString(t, retriedJob, "jobId")
	if retriedJobID == globalRotationBody.SyncJobs[0].JobID.String() {
		t.Fatal("manual retry reused the terminal source job")
	}
	retryReplay := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/jobs/"+globalRotationBody.SyncJobs[0].JobID.String()+":retry", nil, []*http.Cookie{aliceCookie}, retryHeaders)
	assertStatus(t, retryReplay, http.StatusAccepted)
	if retryReplay.Body.String() != retriedJob.Body.String() {
		t.Fatalf("retry replay differs\nfirst: %s\nreplay: %s", retriedJob.Body.String(), retryReplay.Body.String())
	}
	retriedDetail := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/jobs/"+retriedJobID, nil, []*http.Cookie{aliceCookie})
	assertStatus(t, retriedDetail, http.StatusOK)
	if !strings.Contains(retriedDetail.Body.String(), `"trigger":"retry"`) || !strings.Contains(retriedDetail.Body.String(), `"retryOfJobId":"`+globalRotationBody.SyncJobs[0].JobID.String()+`"`) || !strings.Contains(retriedDetail.Body.String(), `"attempts":[]`) {
		t.Fatalf("retried job detail = %s", retriedDetail.Body.String())
	}
	retryPending := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/t/acme/jobs/"+retriedJobID+":retry", nil, []*http.Cookie{aliceCookie}, map[string]string{
		csrfHeaderName: aliceCSRF, "Idempotency-Key": uuid.NewV7().String(),
	})
	assertError(t, retryPending, http.StatusConflict, "invalid_state")
	globalReplay := requestJSONWithHeaders(t, httpHandler, http.MethodPost, "/api/v1/admin/global-credentials/"+globalID.String()+":rotate", map[string]any{
		"secret": map[string]any{"username": "global-bot", "token": "global-second-token"}, "resyncRepositories": true,
	}, []*http.Cookie{adminCookie}, globalRotationHeaders)
	assertStatus(t, globalReplay, http.StatusOK)
	if globalReplay.Body.String() != rotatedGlobal.Body.String() {
		t.Fatalf("global idempotent rotation body differs\nfirst: %s\nreplay: %s", rotatedGlobal.Body.String(), globalReplay.Body.String())
	}
	globalETag = responseString(t, rotatedGlobal, "etag")
	deletedGlobal := requestJSONWithHeaders(t, httpHandler, http.MethodDelete, "/api/v1/admin/global-credentials/"+globalID.String()+"?force=true", nil, []*http.Cookie{adminCookie}, map[string]string{
		csrfHeaderName: rotatedCSRF, "If-Match": globalETag,
	})
	assertStatus(t, deletedGlobal, http.StatusNoContent)
	unboundGlobal := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/repositories/"+repositoryID.String(), nil, []*http.Cookie{aliceCookie})
	assertStatus(t, unboundGlobal, http.StatusOK)
	if !strings.Contains(unboundGlobal.Body.String(), `"credentialId":null`) || !strings.Contains(unboundGlobal.Body.String(), `"class":"auth"`) {
		t.Fatalf("forced global deletion did not expose unbound health state: %s", unboundGlobal.Body.String())
	}
	repositoryETag = responseString(t, unboundGlobal, "etag")
	deletedRepository := requestJSONWithHeaders(t, httpHandler, http.MethodDelete, "/api/v1/t/acme/repositories/"+repositoryID.String(), nil, []*http.Cookie{aliceCookie}, map[string]string{
		csrfHeaderName: aliceCSRF, "If-Match": repositoryETag,
	})
	assertStatus(t, deletedRepository, http.StatusNoContent)
	missingRepository := requestJSON(t, httpHandler, http.MethodGet, "/api/v1/t/acme/repositories/"+repositoryID.String(), nil, []*http.Cookie{aliceCookie})
	assertError(t, missingRepository, http.StatusNotFound, "not_found")

	if _, err := db.Pool.Exec(t.Context(), `UPDATE tenants SET status = 'disabled' WHERE slug = 'acme'`); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	disabledTenantLogin := requestJSON(t, httpHandler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "alice", "password": "alice secure password",
	}, nil)
	assertStatus(t, disabledTenantLogin, http.StatusOK)
	assertNoTenantMemberships(t, disabledTenantLogin)
}

func assertAuditPage(t *testing.T, response *httptest.ResponseRecorder, total int, tenantSlug, action string) {
	t.Helper()
	var page struct {
		Total int `json:"total"`
		Items []struct {
			TenantSlug string         `json:"tenantSlug"`
			Action     string         `json:"action"`
			Metadata   map[string]any `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode audit page: %v", err)
	}
	if page.Total != total || len(page.Items) != total || page.Items[0].TenantSlug != tenantSlug || page.Items[0].Action != action || len(page.Items[0].Metadata) == 0 {
		t.Fatalf("audit page = %#v, want one redacted %s/%s fact", page, tenantSlug, action)
	}
}

func authenticatePAT(identity *service.Identity, t *testing.T, token string) error {
	t.Helper()
	_, err := identity.AuthenticatePAT(t.Context(), token)
	return err
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return requestJSONWithHeaders(t, handler, method, path, body, cookies, nil)
}

func requestJSONWithHeaders(t *testing.T, handler http.Handler, method, path string, body any, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request body: %v", err)
		}
		input = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, input)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, want, response.Body.String())
	}
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	assertStatus(t, response, status)
	if got := responseString(t, response, "code"); got != code {
		t.Fatalf("error code = %q, want %q", got, code)
	}
}

func responseCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response has no %s cookie", name)
	return nil
}

func responseString(t *testing.T, response *httptest.ResponseRecorder, field string) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	value, ok := payload[field].(string)
	if !ok || value == "" {
		t.Fatalf("response field %s is not a non-empty string: %#v", field, payload[field])
	}
	return value
}

func assertTenantMembership(t *testing.T, response *httptest.ResponseRecorder, slug, role string) {
	t.Helper()
	var payload struct {
		Me *struct {
			Tenants []struct {
				Slug string `json:"slug"`
				Role string `json:"role"`
			} `json:"tenants"`
		} `json:"me"`
		Tenants []struct {
			Slug string `json:"slug"`
			Role string `json:"role"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode memberships: %v", err)
	}
	tenants := payload.Tenants
	if payload.Me != nil {
		tenants = payload.Me.Tenants
	}
	for _, tenant := range tenants {
		if tenant.Slug == slug && tenant.Role == role {
			return
		}
	}
	t.Fatalf("membership %s/%s absent from response: %s", slug, role, response.Body.String())
}

func assertNoTenantMemberships(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var payload struct {
		Me struct {
			Tenants []any `json:"tenants"`
		} `json:"me"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode disabled-tenant login: %v", err)
	}
	if len(payload.Me.Tenants) != 0 {
		t.Fatalf("disabled tenant remains in login memberships: %s", response.Body.String())
	}
}
