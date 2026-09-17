//go:build integration

package handler

import (
	"bytes"
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
	"strconv"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/database"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/task"
)

func TestM1RepositorySmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M1 repository smoke requires MERIDIAN_TEST_DATABASE_URL; use make smoke-m1-repository")
	}
	t.Run("SMK-032", smokeM1ProducerProfileSelection)
	t.Run("SMK-040", smokeM1RepositoryDiscovery)
}

type m1RepositoryFixture struct {
	db          *database.Database
	handler     http.Handler
	identity    *service.Identity
	admin       service.Principal
	platform    m1SmokeSession
	tenantID    uuid.UUID
	workspace   string
	gitRemote   string
	resolvedSHA string
}

type m1SmokeSession struct {
	token string
}

func newM1RepositoryFixture(t *testing.T) *m1RepositoryFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open M1 smoke database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close M1 smoke database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate M1 smoke database: %v", err)
	}
	for _, table := range []string{"source_bindings", "source_specs", "discovery_candidates", "services", "producer_profiles", "repositories", "users", "tenants"} {
		if _, err := db.Pool.Exec(t.Context(), "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("reset M1 smoke table %s: %v", table, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset M1 smoke queue: %v", err)
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
	keyring, err := service.NewCredentialKeyring(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32)), 1,
		base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32)))
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

	discoveryStore := repository.NewDiscoveryStore(db.Pool)
	workspace := t.TempDir()
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions: repository.NewRepositoryStore(db.Pool),
		DiscoverRunner: service.NewDiscoveryRunner(discoveryStore, workspace),
	}, logger)
	if err != nil {
		t.Fatalf("create M1 smoke queue runtime: %v", err)
	}
	discoveryStore.BindRiver(runtime.Client())

	f := &m1RepositoryFixture{db: db, identity: identity, admin: actor, workspace: workspace}
	runtime.Start(t.Context())
	t.Cleanup(func() {
		stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runtime.Stop(stopContext)
	})
	f.handler = NewWithRuntimeServices(Dependencies{
		Identity:     identity,
		Credentials:  service.NewCredentials(repository.NewCredentialStoreWithRiver(db.Pool, runtime.Client()), store, keyring),
		Repositories: service.NewRepositories(repository.NewRepositoryStore(db.Pool), store),
		Jobs:         service.NewJobs(repository.NewJobControlStore(db.Pool, runtime.Client()), store),
		Producers:    service.NewProducers(discoveryStore, store),
		Discovery:    service.NewDiscovery(discoveryStore, store),
	}, false).Handler()

	tenant, err := identity.CreateTenant(t.Context(), actor, service.CreateTenantInput{
		Slug: "acme", DisplayName: "Acme", Quota: new(service.Quota{MaxRepositories: 20, MaxServices: 20, MaxStorageBytes: 1073741824, MaxCollectConcurrency: 2}),
	})
	if err != nil {
		t.Fatalf("create smoke tenant: %v", err)
	}
	f.tenantID = tenant.ID
	user, err := identity.CreateUser(t.Context(), actor, service.CreateUserInput{
		Username: "alice", DisplayName: "Alice", Password: "smoke secure password",
	})
	if err != nil {
		t.Fatalf("create smoke user: %v", err)
	}
	if _, _, err := identity.PutTenantMembership(t.Context(), actor, "acme", user.ID, "tenant_admin"); err != nil {
		t.Fatalf("grant smoke tenant membership: %v", err)
	}

	for _, session := range []struct{ username, password string }{{"padmin", "smoke secure password"}, {"alice", "smoke secure password"}} {
		response := requestJSON(t, f.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"username": session.username, "password": session.password,
		}, nil)
		assertStatus(t, response, http.StatusOK)
		token := responseString(t, response, "accessToken")
		if session.username == "padmin" {
			f.platform = m1SmokeSession{token: token}
		}
	}
	return f
}

func (f *m1RepositoryFixture) request(t *testing.T, session *m1SmokeSession, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
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

func smokeM1ProducerProfileSelection(t *testing.T) {
	f := newM1RepositoryFixture(t)

	// A command profile backed by a real executable that supports the openapi kind.
	executable := filepath.Join(t.TempDir(), "producer")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write producer executable: %v", err)
	}
	alice := m1SmokeSession{token: ""}
	aliceLogin := requestJSON(t, f.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "alice", "password": "smoke secure password",
	}, nil)
	assertStatus(t, aliceLogin, http.StatusOK)
	alice.token = responseString(t, aliceLogin, "accessToken")

	commandProfile := f.request(t, &f.platform, http.MethodPost, "/api/v1/admin/producer-profiles", map[string]any{
		"name": "go-producer", "kind": "command", "executable": executable,
		"args": []string{}, "envAllowlist": []string{}, "supportedKinds": []string{"openapi"},
		"replaySafe": true, "network": "none",
	}, nil)
	assertStatus(t, commandProfile, http.StatusCreated)
	commandProfileID := responseString(t, commandProfile, "id")

	// A disabled profile must not be returned to tenants.
	disabledExec := filepath.Join(t.TempDir(), "disabled-producer")
	if err := os.WriteFile(disabledExec, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write disabled producer executable: %v", err)
	}
	disabledProfile := f.request(t, &f.platform, http.MethodPost, "/api/v1/admin/producer-profiles", map[string]any{
		"name": "disabled-producer", "kind": "command", "executable": disabledExec,
		"args": []string{}, "envAllowlist": []string{}, "supportedKinds": []string{"openapi"},
		"replaySafe": true, "network": "none", "enabled": false,
	}, nil)
	assertStatus(t, disabledProfile, http.StatusCreated)

	// An unavailable profile (missing executable) must also be excluded.
	unavailableProfile := f.request(t, &f.platform, http.MethodPost, "/api/v1/admin/producer-profiles", map[string]any{
		"name": "missing-producer", "kind": "command", "executable": "/nonexistent/meridian-producer",
		"args": []string{}, "envAllowlist": []string{}, "supportedKinds": []string{"openapi"},
		"replaySafe": true, "network": "none",
	}, nil)
	assertStatus(t, unavailableProfile, http.StatusCreated)

	available := f.request(t, &alice, http.MethodGet, "/api/v1/t/acme/producer-profiles", nil, nil)
	assertStatus(t, available, http.StatusOK)
	var availableBody struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(available.Body.Bytes(), &availableBody); err != nil {
		t.Fatalf("decode available producer profiles: %v", err)
	}
	names := make([]string, 0, len(availableBody.Items))
	for _, item := range availableBody.Items {
		names = append(names, item.Name)
		if item.ID == commandProfileID {
			// sanity: the selectable profile is present
		}
	}
	if !containsString(names, "go-producer") {
		t.Fatalf("available producer profiles %v missing go-producer", names)
	}
	if containsString(names, "disabled-producer") || containsString(names, "missing-producer") {
		t.Fatalf("disabled or unavailable profile leaked into tenant list: %v", names)
	}

	// Kind filtering only returns profiles supporting that kind.
	filtered := f.request(t, &alice, http.MethodGet, "/api/v1/t/acme/producer-profiles?kind=openapi", nil, nil)
	assertStatus(t, filtered, http.StatusOK)
	if !strings.Contains(filtered.Body.String(), "go-producer") {
		t.Fatalf("kind filter dropped the supported profile: %s", filtered.Body.String())
	}

	// Selecting an unavailable profile for a source spec must 422.
	// The path service (svc) does not exist, but the producer-profile
	// validation must fire first and reject the unavailable profile.
	createSourceUnavailable := f.request(t, &alice, http.MethodPost, "/api/v1/t/acme/services/svc/sources", map[string]any{
		"kind": "openapi", "role": "base", "origin": "repo", "mode": "command",
		"producerProfileId": responseString(t, unavailableProfile, "id"),
	}, nil)
	assertError(t, createSourceUnavailable, http.StatusUnprocessableEntity, "producer_profile_unavailable")
}

// setupOrderService accepts the order-service candidate so SMK-040 can bind a source spec to it.
func (f *m1RepositoryFixture) acceptOrderService(t *testing.T, alice *m1SmokeSession, repositoryID string) {
	t.Helper()
	candidates := f.request(t, alice, http.MethodGet, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates", nil, nil)
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
	accepted := f.request(t, alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates:accept", map[string]any{
		"candidateIds": []string{orderID},
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, accepted, http.StatusOK)
}

func smokeM1RepositoryDiscovery(t *testing.T) {
	f := newM1RepositoryFixture(t)
	alice := m1SmokeSession{token: ""}
	aliceLogin := requestJSON(t, f.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "alice", "password": "smoke secure password",
	}, nil)
	assertStatus(t, aliceLogin, http.StatusOK)
	alice.token = responseString(t, aliceLogin, "accessToken")

	bare, _ := seedDiscoveryRepository(t)
	server := discoveryGitHTTPServer(t, bare)
	// The git CLI (probe and discovery runner) trusts this test CA so it can
	// clone over the loopback HTTPS server without host-key/cert failures.
	certificatePath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatalf("write local Git CA: %v", err)
	}
	t.Setenv("GIT_SSL_CAINFO", certificatePath)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	remote := server.URL + "/repository.git"

	check := f.request(t, &alice, http.MethodPost, "/api/v1/t/acme/repositories:check-connection", map[string]any{
		"url": remote,
	}, nil)
	assertStatus(t, check, http.StatusOK)
	var checkBody struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(check.Body.Bytes(), &checkBody); err != nil {
		t.Fatalf("decode connection check: %v", err)
	}
	if !checkBody.OK {
		t.Fatalf("connection check failed: %s", check.Body.String())
	}

	created := f.request(t, &alice, http.MethodPost, "/api/v1/t/acme/repositories", map[string]any{
		"url": remote, "defaultBranch": "main",
	}, nil)
	assertStatus(t, created, http.StatusCreated)
	repositoryID := responseString(t, created, "id")

	discovered := f.request(t, &alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+":discover", map[string]any{
		"refType": "branch", "ref": "main",
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, discovered, http.StatusAccepted)
	jobID := responseString(t, discovered, "jobId")

	// Wait for the discovery job to reach a terminal state.
	var jobBody struct {
		Status string      `json:"status"`
		Error  interface{} `json:"error"`
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		job := f.request(t, &alice, http.MethodGet, "/api/v1/t/acme/jobs/"+jobID, nil, nil)
		assertStatus(t, job, http.StatusOK)
		if err := json.Unmarshal(job.Body.Bytes(), &jobBody); err != nil {
			t.Fatalf("decode job: %v", err)
		}
		if jobBody.Status == "succeeded" || jobBody.Status == "failed" || jobBody.Status == "cancelled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("discovery job did not finish: %s", job.Body.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
	if jobBody.Status != "succeeded" {
		t.Fatalf("discovery job status = %q, want succeeded; error = %v", jobBody.Status, jobBody.Error)
	}

	candidates := f.request(t, &alice, http.MethodGet, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates", nil, nil)
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
	rootDirs := make([]string, 0, len(candidateBody.Items))
	for _, item := range candidateBody.Items {
		rootDirs = append(rootDirs, item.Path)
	}
	if !containsString(rootDirs, "") {
		t.Fatalf("candidates missing repository root (empty rootDir): %v", rootDirs)
	}
	if !containsString(rootDirs, "order-service") || !containsString(rootDirs, "pay-service") {
		t.Fatalf("candidates missing nested services: %v", rootDirs)
	}

	candidateIDs := make([]string, 0, len(candidateBody.Items))
	for _, item := range candidateBody.Items {
		if item.Path == "order-service" || item.Path == "pay-service" {
			candidateIDs = append(candidateIDs, item.ID)
		}
	}
	accepted := f.request(t, &alice, http.MethodPost, "/api/v1/t/acme/repositories/"+repositoryID+"/candidates:accept", map[string]any{
		"candidateIds": candidateIDs,
	}, map[string]string{"Idempotency-Key": uuid.NewV7().String()})
	assertStatus(t, accepted, http.StatusOK)
	var acceptedBody struct {
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
	}
	if err := json.Unmarshal(accepted.Body.Bytes(), &acceptedBody); err != nil {
		t.Fatalf("decode accepted services: %v", err)
	}
	slugs := make([]string, 0, len(acceptedBody.Items))
	for _, item := range acceptedBody.Items {
		slugs = append(slugs, item.Slug)
	}
	if !containsString(slugs, "order-service") || !containsString(slugs, "pay-service") {
		t.Fatalf("accepted services missing slugs [order-service, pay-service]: %v", slugs)
	}

	createSource := f.request(t, &alice, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "base", "origin": "repo", "mode": "builtin", "path": "order-service/openapi.yaml",
	}, nil)
	assertStatus(t, createSource, http.StatusCreated)
	var sourceBody struct {
		BindingsCount  int  `json:"bindingsCount"`
		InitialLayerID any  `json:"initialLayerId"`
		ID             string `json:"id"`
	}
	if err := json.Unmarshal(createSource.Body.Bytes(), &sourceBody); err != nil {
		t.Fatalf("decode created source spec: %v", err)
	}
	if sourceBody.BindingsCount != 0 {
		t.Fatalf("bindingsCount = %d, want 0", sourceBody.BindingsCount)
	}
	if sourceBody.InitialLayerID != nil {
		t.Fatalf("initialLayerId = %v, want null", sourceBody.InitialLayerID)
	}

	bindings := f.request(t, &alice, http.MethodGet, "/api/v1/t/acme/sources/"+sourceBody.ID+"/bindings", nil, nil)
	assertStatus(t, bindings, http.StatusOK)
	var bindingsBody struct {
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(bindings.Body.Bytes(), &bindingsBody); err != nil {
		t.Fatalf("decode source bindings: %v", err)
	}
	if len(bindingsBody.Items) != 0 {
		t.Fatalf("source bindings length = %d, want 0", len(bindingsBody.Items))
	}
}

// seedDiscoveryRepository builds a local bare Git repository with two nested
// services (order-service with pom.xml, pay-service with go.mod) and a root
// package.json marker. It returns the bare repo path and its working-tree path.
func seedDiscoveryRepository(t *testing.T) (bare string, working string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "seed")
	bare = filepath.Join(root, "repository.git")
	for _, dir := range []string{repo, filepath.Join(repo, "order-service"), filepath.Join(repo, "pay-service"), filepath.Join(repo, "order-service", "apis", "billing"), filepath.Join(repo, "order-service", "apis", "inventory"), filepath.Join(repo, "order-service", "apis", "shipping")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir seed repo: %v", err)
		}
	}
	const openapiDocument = `openapi: 3.1.0
info:
  title: Order Service
  version: 1.0.0
paths:
  /orders:
    get:
      summary: List orders
      responses: {}
    post:
      summary: Create order
      responses: {}
  /orders/{id}:
    get:
      summary: Get order
      responses: {}
`
	files := map[string]string{
		"package.json":                              "{\"name\":\"root\"}\n",
		"order-service/pom.xml":                     "<project/>\n",
		"order-service/openapi.yaml":                openapiDocument,
		"order-service/apis/billing/openapi.yaml":   openapiDocument,
		"order-service/apis/inventory/openapi.yaml": openapiDocument,
		"order-service/apis/shipping/openapi.yaml":  openapiDocument,
		"pay-service/go.mod":                        "module example/pay\n\ngo 1.21\n",
	}
	for path, contents := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(contents), 0o644); err != nil {
			t.Fatalf("write seed file %s: %v", path, err)
		}
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
	runGit("-C", repo, "-c", "user.name=smoke", "-c", "user.email=smoke@example.com", "commit", "-q", "-m", "seed services")
	runGit("clone", "-q", "--bare", repo, bare)
	return bare, repo
}

// discoveryGitHTTPServer serves a bare repository over HTTP using git-http-backend.
func discoveryGitHTTPServer(t *testing.T, bare string) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		command := exec.Command("/usr/lib/git-core/git-http-backend")
		command.Dir = bare
		command.Env = append(os.Environ(),
			"GIT_PROJECT_ROOT="+filepath.Dir(bare),
			"GIT_HTTP_EXPORT_ALL=1",
			"PATH_INFO="+r.URL.Path,
			"QUERY_STRING="+r.URL.RawQuery,
			"REQUEST_METHOD="+r.Method,
			"CONTENT_TYPE="+r.Header.Get("Content-Type"),
			"CONTENT_LENGTH="+r.Header.Get("Content-Length"),
			"GIT_PROTOCOL="+r.Header.Get("Git-Protocol"),
		)
		command.Stdin = r.Body
		output, err := command.Output()
		if err != nil {
			http.Error(w, "git backend failed", http.StatusInternalServerError)
			return
		}
		// git-http-backend emits CGI headers and the body on one stream.
		// Split them and copy the headers onto the HTTP response.
		headerBlock, body, found := bytes.Cut(output, []byte("\r\n\r\n"))
		if !found {
			w.Write(output)
			return
		}
		status := http.StatusOK
		for _, line := range strings.Split(string(headerBlock), "\r\n") {
			name, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			value = strings.TrimSpace(value)
			if strings.EqualFold(name, "Status") {
				if code, err := strconv.Atoi(strings.Fields(value)[0]); err == nil {
					status = code
				}
				continue
			}
			w.Header().Set(name, value)
		}
		w.WriteHeader(status)
		w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

var _ = base64.StdEncoding

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
