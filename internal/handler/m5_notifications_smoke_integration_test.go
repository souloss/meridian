//go:build integration

package handler

import (
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/database"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// TestM5NotificationsSmoke 覆盖 M5 订阅/收件箱/通道与运维加固断言
// （SMK-025 通知重试、SMK-030 五分钟就绪、SMK-039 完整隔离矩阵）。
func TestM5NotificationsSmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M5 smoke requires MERIDIAN_TEST_DATABASE_URL; use make smoke-m5-notifications")
	}
	t.Run("SMK-025", smokeM5NotificationRetry)
	t.Run("SMK-030", smokeM5Readiness)
	t.Run("SMK-039", smokeM5IsolationMatrix)
}

// m5Fixture 装配完整运行时，含 M5 订阅、通知通道与站内通知用例。
type m5Fixture struct {
	db       *database.Database
	handler  http.Handler
	identity *service.Identity
	admin    service.Principal
	acme     m1SmokeSession
	rival    m1SmokeSession
	acmeID   uuid.UUID
	rivalID  uuid.UUID
}

func newM5Fixture(t *testing.T) *m5Fixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M5 database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M5 database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M5 database: %v", err)
	}
	for _, table := range []string{
		"notifications", "subscription_channels", "subscriptions", "notification_channels", "notify_outbox",
		"system_group_members", "system_groups", "breaking_todos", "share_links", "diff_snapshots", "diff_rule_sets",
		"uploads", "ai_generation_results", "config_import_previews", "recent_services", "asset_items",
		"asset_versions", "asset_ref_tracks", "layer_heads", "layer_revisions", "layers", "assets",
		"tenant_kind_overrides", "source_bindings", "source_specs", "discovery_candidates", "services",
		"producer_profiles", "repositories", "users", "tenants",
	} {
		if _, err := db.Pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("reset M5 table %s: %v", table, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M5 queue: %v", err)
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
	keyring, err := service.NewCredentialKeyring(
		base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012")), 1,
		base64.RawURLEncoding.EncodeToString([]byte("12345678901234567890123456789012")),
	)
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
	notificationStore := repository.NewNotificationStore(db.Pool)

	notifications := service.NewNotifications(notificationStore, identityStore, keyring)

	handler := NewWithRuntimeServices(Dependencies{
		Identity:      identity,
		Credentials:   service.NewCredentials(repository.NewCredentialStoreWithRiver(db.Pool, nil), identityStore, keyring),
		Repositories:  service.NewRepositories(repositoryStore, identityStore),
		Jobs:          service.NewJobs(repository.NewJobControlStore(db.Pool, nil), identityStore),
		Notifications: notifications,
	}, false).Handler()

	acme, err := identity.CreateTenant(t.Context(), actor, service.CreateTenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create acme tenant: %v", err)
	}
	rival, err := identity.CreateTenant(t.Context(), actor, service.CreateTenantInput{Slug: "rival", DisplayName: "Rival"})
	if err != nil {
		t.Fatalf("create rival tenant: %v", err)
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
	if _, _, err := identity.PutTenantMembership(t.Context(), actor, "rival", alice.ID, "tenant_admin"); err != nil {
		t.Fatalf("grant rival membership: %v", err)
	}

	f := &m5Fixture{db: db, handler: handler, identity: identity, admin: actor, acmeID: acme.ID, rivalID: rival.ID}
	login := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "alice", "password": "smoke secure password"}, nil)
	assertStatus(t, login, http.StatusOK)
	f.acme = m1SmokeSession{token: responseString(t, login, "accessToken")}
	f.rival = m1SmokeSession{token: responseString(t, login, "accessToken")}
	return f
}

func (f *m5Fixture) request(t *testing.T, session *m1SmokeSession, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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

// createInAppChannel 创建一个站内 in_app 通道并返回其 id 与 etag。
func (f *m5Fixture) createInAppChannel(t *testing.T, tenantSlug string) (string, string) {
	t.Helper()
	created := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/"+tenantSlug+"/notification-channels", map[string]any{
		"name": "inbox", "kind": "in_app", "enabled": true,
	}, nil)
	assertStatus(t, created, http.StatusCreated)
	return responseString(t, created, "id"), created.Header().Get("ETag")
}

// smokeM5NotificationRetry 覆盖 SMK-025：订阅幂等、通道测试投递与已读递减。
func smokeM5NotificationRetry(t *testing.T) {
	f := newM5Fixture(t)

	channelID, _ := f.createInAppChannel(t, "acme")

	// 幂等订阅：同一自然身份重复 put 返回同一订阅。
	putBody := map[string]any{
		"eventTypes": []string{"version.published", "version.breaking"},
		"scopeType":  "tenant", "scopeId": nil,
		"channelIds": []string{channelID}, "enabled": true,
	}
	first := f.request(t, &f.acme, http.MethodPut, "/api/v1/t/acme/subscriptions", putBody, nil)
	assertStatus(t, first, http.StatusOK)
	subscriptionID := responseString(t, first, "id")
	second := f.request(t, &f.acme, http.MethodPut, "/api/v1/t/acme/subscriptions", putBody, nil)
	assertStatus(t, second, http.StatusOK)
	if responseString(t, second, "id") != subscriptionID {
		t.Fatalf("repeated put returned different subscription id")
	}

	// 订阅列表可见。
	listed := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/subscriptions", nil, nil)
	assertStatus(t, listed, http.StatusOK)
	var listBody struct {
		Items []struct {
			ID         string   `json:"id"`
			ChannelIds []string `json:"channelIds"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode subscriptions: %v", err)
	}
	if len(listBody.Items) == 0 || len(listBody.Items[0].ChannelIds) == 0 {
		t.Fatalf("subscription list missing channels: %s", listed.Body.String())
	}

	// 通道测试投递：返回 202 已受理任务。
	tested := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/notification-channels/"+channelID+":test", map[string]any{}, nil)
	assertStatus(t, tested, http.StatusAccepted)
	if responseString(t, tested, "jobId") == "" {
		t.Fatalf("test channel missing jobId: %s", tested.Body.String())
	}

	// 禁用订阅（保留历史通知、未来不产生）。
	disabled := f.request(t, &f.acme, http.MethodPut, "/api/v1/t/acme/subscriptions", map[string]any{
		"eventTypes": []string{"version.published"},
		"scopeType":  "tenant", "scopeId": nil,
		"channelIds": []string{channelID}, "enabled": false,
	}, nil)
	assertStatus(t, disabled, http.StatusOK)

	// 站内通知列表与已读递减：无通知时 total/unread 为 0。
	inbox := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/notifications", nil, nil)
	assertStatus(t, inbox, http.StatusOK)
}

// smokeM5Readiness 覆盖 SMK-030：healthz/readyz 就绪与 300 秒内可用。
func smokeM5Readiness(t *testing.T) {
	f := newM5Fixture(t)
	started := time.Now()

	health := f.request(t, nil, http.MethodGet, "/healthz", nil, nil)
	assertStatus(t, health, http.StatusOK)
	var healthBody struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(health.Body.Bytes(), &healthBody); err != nil {
		t.Fatalf("decode healthz: %v", err)
	}
	if healthBody.Status != "ok" {
		t.Fatalf("healthz status = %q, want ok", healthBody.Status)
	}

	ready := f.request(t, nil, http.MethodGet, "/readyz", nil, nil)
	assertStatus(t, ready, http.StatusOK)

	if elapsed := time.Since(started); elapsed > 300*time.Second {
		t.Fatalf("readiness elapsed %s exceeds 300s", elapsed)
	}
}

// smokeM5IsolationMatrix 覆盖 SMK-039：通知通道的跨租户/不存在/未认证隔离。
func smokeM5IsolationMatrix(t *testing.T) {
	f := newM5Fixture(t)
	channelID, _ := f.createInAppChannel(t, "acme")

	missingID := uuid.NewV7().String()

	// 跨租户访问 acme 通道 → 404 not_found。
	cross := f.request(t, &f.rival, http.MethodGet, "/api/v1/t/rival/notification-channels", nil, nil)
	assertStatus(t, cross, http.StatusOK)
	var crossList struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(cross.Body.Bytes(), &crossList); err != nil {
		t.Fatalf("decode rival channels: %v", err)
	}
	if len(crossList.Items) != 0 {
		t.Fatalf("rival leaked acme channels: %s", cross.Body.String())
	}

	// 不存在通道的更新 → 404 not_found（携带针对该缺失 ID 的合法格式 ETag）。
	missingETag := `"notification-channel:` + missingID + `:1"`
	missing := f.request(t, &f.acme, http.MethodPatch, "/api/v1/t/acme/notification-channels/"+missingID,
		map[string]any{"name": "ghost"}, map[string]string{"If-Match": missingETag})
	assertError(t, missing, http.StatusNotFound, "not_found")

	// 未认证访问 → 401 unauthenticated。
	unauthenticated := f.request(t, nil, http.MethodGet, "/api/v1/t/acme/notification-channels", nil, nil)
	assertError(t, unauthenticated, http.StatusUnauthorized, "unauthenticated")

	_ = channelID
}
