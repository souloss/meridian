//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
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

// TestM6CoverageSmoke 覆盖 M6 收口里程碑在 M1/M2/M3/M4/M5 域的 53 个缺口操作，
// 每个 operation 此前均为 501 桩，M6 落地真实实现。按 work-item 顺序分块断言：
//   - M6-AGENT-002：生产者配置、源配置删除/物化、资产版本生命周期、公开资产、
//     用户偏好、视图覆盖、视图清单、服务收藏。
//   - M6-AGENT-003：层读取/更新、层修订、资产层内容、合并预览、评审列表。
//   - M6-AGENT-004：差异规则集 CRUD、快照列表/读取/删除/导出、分享链接、差异上传、
//     资产 AI 生成。
//   - M6-AGENT-005：标签 CRUD、系统分组 CRUD。
//   - M6-AGENT-006：通知通道 CRUD、服务评论、租户导出、租户删除、订阅/通知/指标。
func TestM6CoverageSmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M6 coverage smoke requires MERIDIAN_TEST_DATABASE_URL")
	}
	t.Run("M6-AGENT-002/producers", smokeM6ProducerProfiles)
	t.Run("M6-AGENT-002/source-specs", smokeM6SourceSpecs)
	t.Run("M6-AGENT-002/asset-versions-and-public", smokeM6AssetVersionsAndPublic)
	t.Run("M6-AGENT-002/preferences-view-overrides-views", smokeM6PreferencesViewOverridesViews)
	t.Run("M6-AGENT-002/service-star", smokeM6ServiceStar)
	t.Run("M6-AGENT-003/layers-revisions-contents-preview", smokeM6LayersRevisionsContentsPreview)
	t.Run("M6-AGENT-004/diff-rule-sets-snapshots-share-upload", smokeM6DiffRuleSetsSnapshotsShareUpload)
	t.Run("M6-AGENT-004/ai-generate", smokeM6AiGenerate)
	t.Run("M6-AGENT-005/tags", smokeM6Tags)
	t.Run("M6-AGENT-005/system-groups", smokeM6SystemGroups)
	t.Run("M6-AGENT-006/channels-comments-export", smokeM6ChannelsCommentsExport)
	t.Run("M6-AGENT-006/tenant-delete-and-metrics", smokeM6TenantDeleteAndMetrics)
}

// m6CoverageFixture 装配完整运行时，含发现、资产、层、AI、差异、分组、通知与目录协作。
type m6CoverageFixture struct {
	db        *database.Database
	handler   http.Handler
	identity  *service.Identity
	admin     service.Principal
	acme      m1SmokeSession
	platform  m1SmokeSession
	acmeID    uuid.UUID
	workspace string
}

func newM6CoverageFixture(t *testing.T) *m6CoverageFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M6 coverage database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M6 coverage database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M6 coverage database: %v", err)
	}
	for _, table := range []string{
		"tenant_exports", "view_overrides", "service_comments", "service_tags", "tag_definitions", "service_stars",
		"notifications", "subscription_channels", "subscriptions", "notification_channels", "notify_outbox",
		"team_members", "teams", "system_group_members", "system_groups", "breaking_todos", "share_links",
		"diff_snapshots", "diff_rule_sets", "uploads", "ai_generation_results", "config_import_previews",
		"recent_services", "asset_items", "asset_versions", "asset_ref_tracks", "layer_heads", "layer_revisions",
		"layers", "assets", "tenant_kind_overrides", "source_bindings", "source_specs", "discovery_candidates",
		"services", "producer_profiles", "repositories", "api_tokens", "tenant_members", "users", "tenants",
	} {
		if _, err := db.Pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("reset M6 coverage table %s: %v", table, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M6 coverage queue: %v", err)
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
	discoveryStore := repository.NewDiscoveryStoreWithRiver(db.Pool, nil)
	assetStore := repository.NewAssetStore(db.Pool)
	layerStore := repository.NewLayerStore(db.Pool)
	aiStore := repository.NewAIStore(db.Pool)
	diffStore := repository.NewDiffStore(db.Pool)
	systemGroupStore := repository.NewSystemGroupStore(db.Pool)
	notificationStore := repository.NewNotificationStore(db.Pool)
	serviceLifecycleStore := repository.NewServiceLifecycleStoreWithRiver(db.Pool, nil)
	teamStore := repository.NewTeamStore(db.Pool)
	settingsStore := repository.NewSettingsStore(db.Pool)
	kindStore := repository.NewKindStore(db.Pool)
	coverageStore := repository.NewCoverageStore(db.Pool)
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
	aiWorkflow := service.NewAiWorkflow(aiStore, blobs, identityStore, pluginRuntime.AI)
	if err := pluginRuntime.RegisterAIProvider(t.Context(), aiWorkflow.Provider()); err != nil {
		t.Fatalf("install builtin AI provider: %v", err)
	}
	diffService := service.NewDiffService(diffStore, blobs, identityStore, []byte("smoke-share-signing-key-fixed-32-bytes"), pluginRuntime.Kinds)
	searchService := service.NewSearch(assetStore, systemGroupStore, identityStore)
	systemGroups := service.NewSystemGroups(systemGroupStore, identityStore)
	notifications := service.NewNotifications(notificationStore, identityStore, keyring)
	teams := service.NewTeams(teamStore, identityStore)
	settings := service.NewSettings(settingsStore, identityStore)
	assetKinds := service.NewAssetKinds(kindStore, identityStore)
	coverage := service.NewCoverage(coverageStore, identityStore)
	webhooks := service.NewWebhooks(discoveryStore)
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions:       repositoryStore,
		SyncRunner:       service.NewPipelineRunner(assetStore, blobs, workspace, pluginRuntime.Kinds),
		DiscoverRunner:   service.NewDiscoveryRunner(discoveryStore, workspace),
		MergeRunner:      layerEdit,
		AiGenerateRunner: aiWorkflow,
	}, logger)
	if err != nil {
		t.Fatalf("create M6 coverage queue runtime: %v", err)
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
		Audits:           service.NewAudits(repositoryStore, identityStore),
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
		Notifications:    notifications,
		Teams:            teams,
		Settings:         settings,
		AssetKinds:       assetKinds,
		Coverage:         coverage,
		Webhooks:         webhooks,
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
		t.Fatalf("create acme tenant: %v", err)
	}
	rival, err := identity.CreateTenant(t.Context(), actor, service.CreateTenantInput{
		Slug: "rival", DisplayName: "Rival", Quota: new(service.Quota{MaxRepositories: 20, MaxServices: 20, MaxStorageBytes: 1073741824, MaxCollectConcurrency: 2}),
	})
	if err != nil {
		t.Fatalf("create rival tenant: %v", err)
	}
	_ = rival
	alice, err := identity.CreateUser(t.Context(), actor, service.CreateUserInput{
		Username: "alice", DisplayName: "Alice", Password: "smoke secure password",
	})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if _, _, err := identity.PutTenantMembership(t.Context(), actor, "acme", alice.ID, "tenant_admin"); err != nil {
		t.Fatalf("grant acme membership: %v", err)
	}

	f := &m6CoverageFixture{db: db, handler: handler, identity: identity, admin: actor, acmeID: tenant.ID, workspace: workspace}
	login := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "padmin", "password": "smoke secure password"}, nil)
	assertStatus(t, login, http.StatusOK)
	f.platform = m1SmokeSession{token: responseString(t, login, "accessToken")}
	aliceLogin := requestJSON(t, handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "alice", "password": "smoke secure password"}, nil)
	assertStatus(t, aliceLogin, http.StatusOK)
	f.acme = m1SmokeSession{token: responseString(t, aliceLogin, "accessToken")}
	return f
}

func (f *m6CoverageFixture) request(t *testing.T, session *m1SmokeSession, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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

func (f *m6CoverageFixture) waitJob(t *testing.T, jobID string) map[string]any {
	t.Helper()
	var body map[string]any
	deadline := time.Now().Add(30 * time.Second)
	for {
		job := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/jobs/"+jobID, nil, nil)
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

// setupOrderService 通过内置 openapi 源同步 order-service，返回 (repositoryID, serviceID, assetID, versionID)。
func (f *m6CoverageFixture) setupOrderService(t *testing.T) (repositoryID, serviceID, assetID, versionID string) {
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

	check := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/repositories:check-connection", map[string]any{"url": remote}, nil)
	assertStatus(t, check, http.StatusOK)
	created := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/repositories", map[string]any{"url": remote, "defaultBranch": "main"}, nil)
	assertStatus(t, created, http.StatusCreated)
	repositoryID = responseString(t, created, "id")

	discovered := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":discover", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, discovered, http.StatusAccepted)
	if jobBody := f.waitJob(t, responseString(t, discovered, "jobId")); jobBody["status"] != "succeeded" {
		t.Fatalf("discovery job status = %v", jobBody["status"])
	}
	candidates := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates", nil, nil)
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
	accepted := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates:accept", map[string]any{
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

	createSource := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "base", "origin": "repo", "mode": "builtin", "path": "order-service/openapi.yaml",
	}, nil)
	assertStatus(t, createSource, http.StatusCreated)

	synced := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":sync", map[string]any{"refType": "branch", "ref": "main"}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, synced, http.StatusAccepted)
	if jobBody := f.waitJob(t, responseString(t, synced, "jobId")); jobBody["status"] != "succeeded" {
		t.Fatalf("sync job status = %v", jobBody["status"])
	}

	asset := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/services/order-service", nil, nil)
	assertStatus(t, asset, http.StatusOK)
	var serviceDetail struct {
		Assets []struct {
			ID            string `json:"id"`
			LatestVersion struct {
				ID string `json:"id"`
			} `json:"latestVersion"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(asset.Body.Bytes(), &serviceDetail); err != nil {
		t.Fatalf("decode service detail: %v", err)
	}
	if len(serviceDetail.Assets) == 0 {
		t.Fatalf("order-service has no assets: %s", asset.Body.String())
	}
	assetID = serviceDetail.Assets[0].ID
	versionID = serviceDetail.Assets[0].LatestVersion.ID
	return repositoryID, serviceID, assetID, versionID
}

// publishVersion 发布一个已物化版本并返回其新的 etag。
func (f *m6CoverageFixture) publishVersion(t *testing.T, versionID string) string {
	t.Helper()
	var versionRow struct {
		Revision int64
	}
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT revision FROM asset_versions WHERE id = $1`, versionID).Scan(&versionRow.Revision); err != nil {
		t.Fatalf("query version revision: %v", err)
	}
	etag := `"asset-version:` + versionID + `:` + itoa64(versionRow.Revision) + `"`
	published := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/asset-versions/"+versionID+":publish", map[string]any{}, map[string]string{
		"If-Match": etag, "Idempotency-Key": uuid.NewV7().String(),
	})
	assertStatus(t, published, http.StatusOK)
	return responseString(t, published, "etag")
}

// makePublicService 将 order-service 置为 public + published 以触发公开可见性门控。
func (f *m6CoverageFixture) makePublicService(t *testing.T) string {
	t.Helper()
	got := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/services/order-service", nil, nil)
	assertStatus(t, got, http.StatusOK)
	etag := responseString(t, got, "etag")
	updated := f.request(t, &f.acme, http.MethodPatch, "/api/v1/t/acme/services/order-service", map[string]any{"visibility": "public"}, map[string]string{"If-Match": etag})
	assertStatus(t, updated, http.StatusOK)
	// 推进 lifecycle draft → published（公开读取需要 published/deprecated）。
	etag = responseString(t, updated, "etag")
	published := f.request(t, &f.acme, http.MethodPatch, "/api/v1/t/acme/services/order-service", map[string]any{"lifecycle": "published"}, map[string]string{"If-Match": etag})
	assertStatus(t, published, http.StatusOK)
	return responseString(t, published, "etag")
}

// createProducerProfile 以平台管理员创建一条 command 生产者配置并返回 (id, etag)。
func (f *m6CoverageFixture) createProducerProfile(t *testing.T, name string) (string, string) {
	t.Helper()
	executable := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write producer executable: %v", err)
	}
	profile := f.request(t, &f.platform, http.MethodPost, "/api/v1/admin/producer-profiles", map[string]any{
		"name": name, "kind": "command", "executable": executable,
		"args": []string{}, "envAllowlist": []string{}, "supportedKinds": []string{"openapi"},
		"replaySafe": true, "network": "none", "timeoutSec": 30, "memoryMiB": 1024, "cpuSeconds": 600, "pids": 128,
		"enabled": true,
	}, nil)
	assertStatus(t, profile, http.StatusCreated)
	return responseString(t, profile, "id"), responseString(t, profile, "etag")
}

func smokeM6ProducerProfiles(t *testing.T) {
	f := newM6CoverageFixture(t)
	profileID, etag := f.createProducerProfile(t, "smoke-producer")

	list := f.request(t, &f.platform, http.MethodGet, "/api/v1/admin/producer-profiles", nil, nil)
	assertStatus(t, list, http.StatusOK)
	if total := responseInt(t, list, "total"); total != 1 {
		t.Fatalf("producer profiles total = %d, want 1", total)
	}

	got := f.request(t, &f.platform, http.MethodGet, "/api/v1/admin/producer-profiles/"+profileID, nil, nil)
	assertStatus(t, got, http.StatusOK)
	if responseString(t, got, "name") != "smoke-producer" {
		t.Fatal("producer profile name mismatch")
	}

	updated := f.request(t, &f.platform, http.MethodPatch, "/api/v1/admin/producer-profiles/"+profileID, map[string]any{"name": "smoke-producer-renamed"}, map[string]string{"If-Match": etag})
	assertStatus(t, updated, http.StatusOK)
	if responseString(t, updated, "name") != "smoke-producer-renamed" {
		t.Fatal("producer profile rename failed")
	}

	deleted := f.request(t, &f.platform, http.MethodDelete, "/api/v1/admin/producer-profiles/"+profileID, nil, map[string]string{"If-Match": responseString(t, updated, "etag")})
	assertStatus(t, deleted, http.StatusNoContent)

	after := f.request(t, &f.platform, http.MethodGet, "/api/v1/admin/producer-profiles", nil, nil)
	assertStatus(t, after, http.StatusOK)
	if total := responseInt(t, after, "total"); total != 0 {
		t.Fatalf("producer profiles total after delete = %d, want 0", total)
	}
}

func smokeM6SourceSpecs(t *testing.T) {
	f := newM6CoverageFixture(t)
	_, _, _, _ = f.setupOrderService(t)

	listed := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/services/order-service/sources", nil, nil)
	assertStatus(t, listed, http.StatusOK)
	var specList struct {
		Items []struct {
			ID   string `json:"id"`
			Etag string `json:"etag"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &specList); err != nil {
		t.Fatalf("decode source specs: %v", err)
	}
	if len(specList.Items) != 1 {
		t.Fatalf("source specs count = %d, want 1", len(specList.Items))
	}
	sourceID := specList.Items[0].ID
	sourceEtag := specList.Items[0].Etag

	// produceSource 入队一条 asset.produce 任务。
	produced := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/sources/"+sourceID+":produce", map[string]any{}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, produced, http.StatusAccepted)
	if responseString(t, produced, "jobId") == "" {
		t.Fatal("produceSource missing jobId")
	}

	// deleteSourceSpec 软删除。
	deleted := f.request(t, &f.acme, http.MethodDelete, "/api/v1/t/acme/sources/"+sourceID, nil, map[string]string{"If-Match": sourceEtag})
	assertStatus(t, deleted, http.StatusNoContent)

	after := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/services/order-service/sources", nil, nil)
	assertStatus(t, after, http.StatusOK)
	if err := json.Unmarshal(after.Body.Bytes(), &specList); err != nil {
		t.Fatalf("decode source specs after delete: %v", err)
	}
	if len(specList.Items) != 0 {
		t.Fatalf("source specs after delete = %d, want 0", len(specList.Items))
	}
}

func smokeM6AssetVersionsAndPublic(t *testing.T) {
	f := newM6CoverageFixture(t)
	_, _, assetID, versionID := f.setupOrderService(t)
	_ = assetID

	// listAssetVersions：物化后恰有一个版本。
	versions := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/assets/"+assetID+"/versions", nil, nil)
	assertStatus(t, versions, http.StatusOK)
	if total := responseInt(t, versions, "total"); total != 1 {
		t.Fatalf("asset versions total = %d, want 1", total)
	}

	// 发布版本以启用公开读取与 deprecate/retire 迁移。
	publishedEtag := f.publishVersion(t, versionID)
	f.makePublicService(t)

	// getPublicAsset 匿名读取公开资产。
	public := f.request(t, nil, http.MethodGet, "/api/v1/public/t/acme/services/order-service/assets/openapi/openapi", nil, nil)
	assertStatus(t, public, http.StatusOK)
	if responseString(t, public, "kind") != "openapi" {
		t.Fatal("public asset kind mismatch")
	}

	// resolvePublicView 解析匿名公开文档视图。
	resolved := f.request(t, nil, http.MethodPost, "/api/v1/public/t/acme/views:resolve", map[string]any{
		"viewId": "source", "kind": "openapi", "assetName": "openapi",
	}, nil)
	assertStatus(t, resolved, http.StatusOK)

	// deprecateAssetVersion：published → deprecated。
	deprecated := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/asset-versions/"+versionID+":deprecate", map[string]any{}, map[string]string{"If-Match": publishedEtag, "Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, deprecated, http.StatusOK)
	if responseString(t, deprecated, "lifecycle") != "deprecated" {
		t.Fatalf("deprecated lifecycle = %q", responseString(t, deprecated, "lifecycle"))
	}

	// retireAssetVersion：deprecated 版本不可再 retire（仅 published 可），返回 409。
	retire := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/asset-versions/"+versionID+":retire", map[string]any{}, map[string]string{"If-Match": responseString(t, deprecated, "etag"), "Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, retire, http.StatusConflict)
}

func smokeM6PreferencesViewOverridesViews(t *testing.T) {
	f := newM6CoverageFixture(t)

	// getMyPreferences：与用户一同原子创建的默认偏好。
	prefs := f.request(t, &f.acme, http.MethodGet, "/api/v1/auth/me/preferences", nil, nil)
	assertStatus(t, prefs, http.StatusOK)
	etag := responseString(t, prefs, "etag")

	// updateMyPreferences：If-Match 下更新 locale/theme。
	updated := f.request(t, &f.acme, http.MethodPatch, "/api/v1/auth/me/preferences", map[string]any{"locale": "en", "theme": "dark"}, map[string]string{"If-Match": etag})
	assertStatus(t, updated, http.StatusOK)
	if responseString(t, updated, "locale") != "en" || responseString(t, updated, "theme") != "dark" {
		t.Fatalf("preferences not updated: %s", updated.Body.String())
	}

	// listViewOverrides：初始为空。
	overrides := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/view-overrides", nil, nil)
	assertStatus(t, overrides, http.StatusOK)

	// putViewOverride：首次创建无需 If-Match 值，但契约强制要求 If-Match 头。
	put := f.request(t, &f.acme, http.MethodPut, "/api/v1/t/acme/view-overrides/swagger-ui", map[string]any{"enabled": false, "defaultOptions": map[string]any{"tryItOut": true}}, map[string]string{"If-Match": `"view-override:swagger-ui:1"`})
	assertStatus(t, put, http.StatusOK)
	overrideEtag := responseString(t, put, "etag")

	overrides = f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/view-overrides", nil, nil)
	assertStatus(t, overrides, http.StatusOK)

	// deleteViewOverride：If-Match 下删除。
	deleted := f.request(t, &f.acme, http.MethodDelete, "/api/v1/t/acme/view-overrides/swagger-ui", nil, map[string]string{"If-Match": overrideEtag})
	assertStatus(t, deleted, http.StatusNoContent)

	// listViews：返回内置视图注册表。
	views := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/views", nil, nil)
	assertStatus(t, views, http.StatusOK)
}

func smokeM6ServiceStar(t *testing.T) {
	f := newM6CoverageFixture(t)
	_, _, _, _ = f.setupOrderService(t)

	star := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/services/order-service:star", nil, nil)
	assertStatus(t, star, http.StatusOK)
	if !responseBool(t, star, "starred") {
		t.Fatal("starService should set starred=true")
	}

	unstar := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/services/order-service:unstar", nil, nil)
	assertStatus(t, unstar, http.StatusOK)
	if responseBool(t, unstar, "starred") {
		t.Fatal("unstarService should set starred=false")
	}
}

func smokeM6LayersRevisionsContentsPreview(t *testing.T) {
	f := newM6CoverageFixture(t)
	_, _, assetID, versionID := f.setupOrderService(t)
	_ = versionID

	// getAsset 返回资产元数据；层 ID 直接从 DB 取（getAsset 的 layers 摘要当前恒为空）。
	got := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/assets/"+assetID, nil, nil)
	assertStatus(t, got, http.StatusOK)
	var layerID string
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT id FROM layers WHERE tenant_id = $1 AND asset_id = $2 ORDER BY ord, id LIMIT 1`, f.acmeID, assetID).Scan(&layerID); err != nil {
		t.Fatalf("query asset layer: %v", err)
	}

	// getLayer 返回层及其头指针。
	layer := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/layers/"+layerID, nil, nil)
	assertStatus(t, layer, http.StatusOK)
	layerEtag := responseString(t, layer, "etag")

	// updateLayer：base 层的 role 不可改为 overlay（仅校验值域），此处只更新 enabled。
	updated := f.request(t, &f.acme, http.MethodPatch, "/api/v1/t/acme/layers/"+layerID, map[string]any{"enabled": true}, map[string]string{"If-Match": layerEtag})
	assertStatus(t, updated, http.StatusOK)

	// getLayerRevision：按 revision id 读取不可变修订。
	var revisionID string
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT id FROM layer_revisions WHERE tenant_id = $1 AND layer_id = $2 ORDER BY created_at DESC LIMIT 1`, f.acmeID, layerID).Scan(&revisionID); err != nil {
		t.Fatalf("query layer revision: %v", err)
	}
	revision := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/layer-revisions/"+revisionID, nil, nil)
	assertStatus(t, revision, http.StatusOK)

	// listLayerRevisions：仓库同步修订作用域为 branch:main。
	revisions := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/layers/"+layerID+"/revisions?refType=branch&ref=main", nil, nil)
	assertStatus(t, revisions, http.StatusOK)
	if total := responseInt(t, revisions, "total"); total != 1 {
		t.Fatalf("layer revisions total = %d, want 1", total)
	}

	// listAssetLayerContents：一次性返回每层生效内容。
	contents := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/assets/"+assetID+"/layer-contents", nil, nil)
	assertStatus(t, contents, http.StatusOK)

	// previewAssetView：使用内联草稿层做不落库合并。
	preview := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/assets/"+assetID+"/views:preview", map[string]any{
		"enabledLayerIds": []string{layerID},
		"contentType":     "yaml",
		"draftLayers": []map[string]any{{
			"role": "overlay", "origin": "manual", "ord": 1, "contentType": "yaml",
			"content": "overlay: platform/v1\ntarget_kind: openapi\nactions:\n  - target: /paths/~1added\n    merge:\n      get: {summary: added}\n",
		}},
	}, nil)
	assertStatus(t, preview, http.StatusOK)
	if responseString(t, preview, "engineVersion") == "" {
		t.Fatal("preview missing engineVersion")
	}

	// listReviews：仓库同步为 not_required，无待审核修订。
	reviews := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/reviews", nil, nil)
	assertStatus(t, reviews, http.StatusOK)
	if total := responseInt(t, reviews, "total"); total != 0 {
		t.Fatalf("reviews total = %d, want 0", total)
	}
}

func smokeM6DiffRuleSetsSnapshotsShareUpload(t *testing.T) {
	f := newM6CoverageFixture(t)
	_, _, _, versionID := f.setupOrderService(t)

	// createDiffRuleSet。
	created := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/diff-rule-sets", map[string]any{
		"name": "breaking-guard", "kind": "openapi",
		"rules": []map[string]any{{"code": "operation-removed", "level": "breaking", "enabled": true}},
	}, nil)
	assertStatus(t, created, http.StatusCreated)
	ruleSetID := responseString(t, created, "id")
	ruleSetEtag := responseString(t, created, "etag")

	// listDiffRuleSets。
	listRuleSets := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/diff-rule-sets", nil, nil)
	assertStatus(t, listRuleSets, http.StatusOK)

	// updateDiffRuleSet。
	updatedRuleSet := f.request(t, &f.acme, http.MethodPatch, "/api/v1/t/acme/diff-rule-sets/"+ruleSetID, map[string]any{"name": "breaking-guard-v2"}, map[string]string{"If-Match": ruleSetEtag})
	assertStatus(t, updatedRuleSet, http.StatusOK)

	// runDiff 持久化一条快照。
	diffResult := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/diff", map[string]any{
		"left":      map[string]any{"type": "version", "versionId": versionID},
		"right":     map[string]any{"type": "version", "versionId": versionID},
		"persist":   true,
		"ruleSetId": ruleSetID,
	}, nil)
	assertStatus(t, diffResult, http.StatusOK)
	snapshotID := responseString(t, diffResult, "snapshotId")

	// listDiffSnapshots / getDiffSnapshot。
	snapshots := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/diff-snapshots", nil, nil)
	assertStatus(t, snapshots, http.StatusOK)
	if total := responseInt(t, snapshots, "total"); total != 1 {
		t.Fatalf("diff snapshots total = %d, want 1", total)
	}
	snapshot := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/diff-snapshots/"+snapshotID, nil, nil)
	assertStatus(t, snapshot, http.StatusOK)

	// exportDiffSnapshot：签名内容令牌。
	exported := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/diff-snapshots/"+snapshotID+"/export?format=json", nil, nil)
	assertStatus(t, exported, http.StatusOK)
	exportURL := responseString(t, exported, "url")

	// downloadSignedContent：匿名下载导出的 blob。
	if strings.Contains(exportURL, "/api/v1/content/") {
		contentToken := exportURL[strings.LastIndex(exportURL, "/")+1:]
		download := f.request(t, nil, http.MethodGet, "/api/v1/content/"+contentToken, nil, nil)
		assertStatus(t, download, http.StatusOK)
	}

	// createShareLink：冻结视图选择器铸造签名匿名分享令牌。
	share := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/share-links", map[string]any{
		"resourceType": "view", "viewId": "source",
		"inputs":           []map[string]any{{"type": "version", "versionId": versionID}},
		"expiresInSeconds": 3600,
	}, nil)
	assertStatus(t, share, http.StatusCreated)
	shareLinkID := responseString(t, share, "id")
	shareToken := responseString(t, share, "token")

	// getSharedView 解析视图分享令牌。
	shared := f.request(t, nil, http.MethodGet, "/api/v1/shared/"+shareToken, nil, nil)
	assertStatus(t, shared, http.StatusOK)

	// listShareLinks / revokeShareLink。
	links := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/share-links", nil, nil)
	assertStatus(t, links, http.StatusOK)
	if total := responseInt(t, links, "total"); total != 1 {
		t.Fatalf("share links total = %d, want 1", total)
	}
	revoked := f.request(t, &f.acme, http.MethodDelete, "/api/v1/t/acme/share-links/"+shareLinkID, nil, nil)
	assertStatus(t, revoked, http.StatusNoContent)

	// createDiffUpload：multipart 上传（contentType 枚举为 yaml/json）。
	upload := createDiffUpload(t, f, "openapi", "yaml", "openapi: 3.0.3\ninfo: {title: u, version: 1.0.0}\npaths: {}\n")
	assertStatus(t, upload, http.StatusCreated)
	if responseString(t, upload, "digest") == "" {
		t.Fatal("diff upload missing digest")
	}

	// deleteDiffSnapshot / deleteDiffRuleSet。
	deletedSnapshot := f.request(t, &f.acme, http.MethodDelete, "/api/v1/t/acme/diff-snapshots/"+snapshotID, nil, nil)
	assertStatus(t, deletedSnapshot, http.StatusNoContent)
	deletedRuleSet := f.request(t, &f.acme, http.MethodDelete, "/api/v1/t/acme/diff-rule-sets/"+ruleSetID, nil, map[string]string{"If-Match": responseString(t, updatedRuleSet, "etag")})
	assertStatus(t, deletedRuleSet, http.StatusNoContent)
}

func smokeM6AiGenerate(t *testing.T) {
	f := newM6CoverageFixture(t)
	_, _, assetID, _ := f.setupOrderService(t)

	// 以 ai 类别创建生产者配置（runProducer 按名称后缀 -success 选择伪造成功行为）。
	aiProfile := f.request(t, &f.platform, http.MethodPost, "/api/v1/admin/producer-profiles", map[string]any{
		"name": "fake-ai-success", "kind": "ai", "executable": "/bin/sh",
		"args": []string{}, "envAllowlist": []string{}, "supportedKinds": []string{"openapi"},
		"replaySafe": true, "network": "none", "timeoutSec": 30, "memoryMiB": 1024, "cpuSeconds": 600, "pids": 128,
		"enabled": true,
	}, nil)
	assertStatus(t, aiProfile, http.StatusCreated)
	aiProfileID := responseString(t, aiProfile, "id")

	generated := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/assets/"+assetID+":ai-generate", map[string]any{
		"producerProfileId": aiProfileID, "refType": "branch", "ref": "main",
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, generated, http.StatusAccepted)
	if responseString(t, generated, "jobId") == "" {
		t.Fatal("ai generate missing jobId")
	}
	if responseString(t, generated, "assetId") != assetID {
		t.Fatal("ai generate assetId mismatch")
	}
}

func smokeM6Tags(t *testing.T) {
	f := newM6CoverageFixture(t)

	list := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/tags", nil, nil)
	assertStatus(t, list, http.StatusOK)

	created := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/tags", map[string]any{"name": "critical", "color": "#ff0000"}, nil)
	assertStatus(t, created, http.StatusCreated)
	tagID := responseString(t, created, "id")
	tagEtag := responseString(t, created, "etag")

	// 重复名称返回 409。
	dup := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/tags", map[string]any{"name": "critical"}, nil)
	assertStatus(t, dup, http.StatusConflict)

	updated := f.request(t, &f.acme, http.MethodPatch, "/api/v1/t/acme/tags/"+tagID, map[string]any{"name": "critical-v2"}, map[string]string{"If-Match": tagEtag})
	assertStatus(t, updated, http.StatusOK)
	if responseString(t, updated, "name") != "critical-v2" {
		t.Fatal("tag rename failed")
	}

	deleted := f.request(t, &f.acme, http.MethodDelete, "/api/v1/t/acme/tags/"+tagID, nil, map[string]string{"If-Match": responseString(t, updated, "etag")})
	assertStatus(t, deleted, http.StatusNoContent)
}

func smokeM6SystemGroups(t *testing.T) {
	f := newM6CoverageFixture(t)
	_, _, _, _ = f.setupOrderService(t)

	// 空成员创建系统分组。
	created := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/system-groups", map[string]any{
		"slug": "core-group", "displayName": "Core Group",
	}, nil)
	assertStatus(t, created, http.StatusCreated)
	groupID := responseString(t, created, "id")
	groupEtag := responseString(t, created, "etag")

	list := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/system-groups", nil, nil)
	assertStatus(t, list, http.StatusOK)

	got := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/system-groups/"+groupID, nil, nil)
	assertStatus(t, got, http.StatusOK)
	if responseString(t, got, "slug") != "core-group" {
		t.Fatal("system group slug mismatch")
	}

	updated := f.request(t, &f.acme, http.MethodPatch, "/api/v1/t/acme/system-groups/"+groupID, map[string]any{"displayName": "Core Group Renamed"}, map[string]string{"If-Match": groupEtag})
	assertStatus(t, updated, http.StatusOK)
	if responseString(t, updated, "displayName") != "Core Group Renamed" {
		t.Fatal("system group rename failed")
	}

	deleted := f.request(t, &f.acme, http.MethodDelete, "/api/v1/t/acme/system-groups/"+groupID, nil, map[string]string{"If-Match": responseString(t, updated, "etag")})
	assertStatus(t, deleted, http.StatusNoContent)
}

func smokeM6ChannelsCommentsExport(t *testing.T) {
	f := newM6CoverageFixture(t)
	_, serviceID, _, _ := f.setupOrderService(t)
	_ = serviceID

	// createNotificationChannel（in_app 不需要 secret/endpoint）。
	channel := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/notification-channels", map[string]any{
		"name": "inbox", "kind": "in_app", "enabled": true,
	}, nil)
	assertStatus(t, channel, http.StatusCreated)
	channelID := responseString(t, channel, "id")
	channelEtag := channel.Header().Get("ETag")

	// listNotificationChannels。
	channels := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/notification-channels", nil, nil)
	assertStatus(t, channels, http.StatusOK)

	// updateNotificationChannel。
	updatedChannel := f.request(t, &f.acme, http.MethodPatch, "/api/v1/t/acme/notification-channels/"+channelID, map[string]any{"name": "inbox-renamed"}, map[string]string{"If-Match": channelEtag})
	assertStatus(t, updatedChannel, http.StatusOK)

	// listSubscriptions：初始为空。
	subs := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/subscriptions", nil, nil)
	assertStatus(t, subs, http.StatusOK)

	// listNotifications：初始为空。
	notifs := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/notifications", nil, nil)
	assertStatus(t, notifs, http.StatusOK)

	// markAllNotificationsRead：无未读时仍返回 204。
	readAll := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/notifications:read-all", nil, nil)
	assertStatus(t, readAll, http.StatusNoContent)

	// createServiceComment。
	comment := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/services/order-service/comments", map[string]any{"body": "looks good"}, nil)
	assertStatus(t, comment, http.StatusCreated)

	// listServiceComments。
	comments := f.request(t, &f.acme, http.MethodGet, "/api/v1/t/acme/services/order-service/comments", nil, nil)
	assertStatus(t, comments, http.StatusOK)
	if total := responseInt(t, comments, "total"); total != 1 {
		t.Fatalf("service comments total = %d, want 1", total)
	}

	// createTenantExport。
	exported := f.request(t, &f.acme, http.MethodPost, "/api/v1/t/acme/exports", nil, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, exported, http.StatusAccepted)
	if responseString(t, exported, "jobId") == "" {
		t.Fatal("tenant export missing jobId")
	}

	// deleteNotificationChannel。
	deletedChannel := f.request(t, &f.acme, http.MethodDelete, "/api/v1/t/acme/notification-channels/"+channelID, nil, map[string]string{"If-Match": responseString(t, updatedChannel, "etag")})
	assertStatus(t, deletedChannel, http.StatusNoContent)
}

func smokeM6TenantDeleteAndMetrics(t *testing.T) {
	f := newM6CoverageFixture(t)

	// metrics：Prometheus 文本格式。
	metrics := f.request(t, nil, http.MethodGet, "/metrics", nil, nil)
	assertStatus(t, metrics, http.StatusOK)

	// listTenants 获取 rival 的 id 与 etag。
	tenants := f.request(t, &f.platform, http.MethodGet, "/api/v1/admin/tenants", nil, nil)
	assertStatus(t, tenants, http.StatusOK)
	var tenantPage struct {
		Items []struct {
			Slug string `json:"slug"`
			ID   string `json:"id"`
			Etag string `json:"etag"`
		} `json:"items"`
	}
	if err := json.Unmarshal(tenants.Body.Bytes(), &tenantPage); err != nil {
		t.Fatalf("decode tenants: %v", err)
	}
	var rivalEtag string
	for _, item := range tenantPage.Items {
		if item.Slug == "rival" {
			rivalEtag = item.Etag
			break
		}
	}
	if rivalEtag == "" {
		t.Fatalf("rival tenant not found: %s", tenants.Body.String())
	}

	// deleteTenant：平台管理员回显密码 + 确认 slug。
	deleted := f.request(t, &f.platform, http.MethodDelete, "/api/v1/admin/tenants/rival", map[string]any{
		"confirmationSlug": "rival", "currentPassword": "smoke secure password",
	}, map[string]string{"If-Match": rivalEtag})
	assertStatus(t, deleted, http.StatusAccepted)
	if responseString(t, deleted, "jobId") == "" {
		t.Fatal("deleteTenant missing jobId")
	}
}

// createDiffUpload 通过 multipart 表单上传一个差异文档。
func createDiffUpload(t *testing.T, f *m6CoverageFixture, kind, contentType, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	// 契约 encoding 约定 contentType 字段按 application/json 编码，kind 按 text/plain，
	// 因此用 CreateFormField 声明 MIME 类型以满足请求校验器。
	if err := writer.WriteField("kind", kind); err != nil {
		t.Fatalf("write kind field: %v", err)
	}
	contentTypePart, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": []string{`form-data; name="contentType"`},
		"Content-Type":        []string{"application/json"},
	})
	if err != nil {
		t.Fatalf("create contentType field: %v", err)
	}
	// contentType 字段按契约 encoding 以 application/json 编码，值为 JSON 字符串。
	if _, err := contentTypePart.Write([]byte(`"` + contentType + `"`)); err != nil {
		t.Fatalf("write contentType field: %v", err)
	}
	part, err := writer.CreateFormFile("file", "diff.yaml")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/t/acme/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+f.acme.token)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}
