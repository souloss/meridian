//go:build integration

package handler

import (
	"bytes"
	"cmp"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json/v2"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/database"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/task"
	"golang.org/x/crypto/ssh"
)

// Each case owns its database fixtures and can run independently via -run.
func TestM0CredentialSmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M0 credential smoke requires MERIDIAN_TEST_DATABASE_URL; use make smoke-m0-credentials")
	}
	t.Run("SMK-005", smokeTenantCredential)
	t.Run("SMK-031", smokeGlobalCredential)
	t.Run("SMK-035", smokeKnownHost)
}

type credentialSmokeFixture struct {
	db      *database.Database
	handler http.Handler
	keyring service.CredentialKeyring
	tenants map[string]uuid.UUID
	users   map[string]credentialSmokeSession
}

type credentialSmokeSession struct {
	token string
}

type credentialSmokeRepository struct {
	tenant string
	id     string
	branch string
}

func newCredentialSmokeFixture(t *testing.T) *credentialSmokeFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), os.Getenv("MERIDIAN_TEST_DATABASE_URL"), logger)
	if err != nil {
		t.Fatalf("open smoke database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close smoke database: %v", err)
		}
	})
	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("migrate smoke database: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE users, tenants CASCADE`); err != nil {
		t.Fatalf("reset smoke fixtures: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset smoke queue: %v", err)
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
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{}, logger)
	if err != nil {
		t.Fatalf("create smoke queue runtime: %v", err)
	}
	f := &credentialSmokeFixture{db: db, keyring: keyring, tenants: make(map[string]uuid.UUID), users: make(map[string]credentialSmokeSession)}
	f.handler = NewWithRuntimeServices(Dependencies{
		Identity:     identity,
		Credentials:  service.NewCredentials(repository.NewCredentialStoreWithRiver(db.Pool, runtime.Client()), store, keyring),
		Repositories: service.NewRepositories(repository.NewRepositoryStore(db.Pool), store),
		Jobs:         service.NewJobs(repository.NewJobControlStore(db.Pool, runtime.Client()), store),
	}, false).Handler()
	for _, slug := range []string{"acme", "rival"} {
		tenant, err := identity.CreateTenant(t.Context(), actor, service.CreateTenantInput{
			Slug: slug, DisplayName: slug, Quota: new(service.Quota{MaxRepositories: 20, MaxServices: 20, MaxStorageBytes: 1073741824, MaxCollectConcurrency: 2}),
		})
		if err != nil {
			t.Fatalf("create smoke tenant: %v", err)
		}
		f.tenants[slug] = tenant.ID
		user, err := identity.CreateUser(t.Context(), actor, service.CreateUserInput{
			Username: slug, DisplayName: slug, Password: "smoke secure password",
		})
		if err != nil {
			t.Fatalf("create smoke user: %v", err)
		}
		if _, _, err := identity.PutTenantMembership(t.Context(), actor, slug, user.ID, "tenant_admin"); err != nil {
			t.Fatalf("grant smoke tenant membership: %v", err)
		}
	}
	for _, username := range []string{"padmin", "acme", "rival"} {
		response := requestJSON(t, f.handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"username": username, "password": "smoke secure password",
		}, nil)
		assertStatus(t, response, http.StatusOK)
		f.users[username] = credentialSmokeSession{token: responseString(t, response, "accessToken")}
	}
	return f
}

func (f *credentialSmokeFixture) request(t *testing.T, user, method, path string, body any, etag string) *httptest.ResponseRecorder {
	t.Helper()
	session := f.users[user]
	headers := map[string]string{"Authorization": "Bearer " + session.token}
	if etag != "" {
		headers["If-Match"] = etag
	}
	if strings.HasSuffix(path, ":rotate") {
		headers["Idempotency-Key"] = uuid.NewV7().String()
	}
	return requestJSONWithHeaders(t, f.handler, method, path, body, nil, headers)
}

func smokeTenantCredential(t *testing.T) {
	f := newCredentialSmokeFixture(t)
	const first, second = "smoke-first-token", "smoke-rotated-token"
	created := f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/credentials", map[string]any{
		"name": "tenant smoke", "kind": "http_token", "httpToken": map[string]any{"username": "bot", "token": first},
	}, "")
	assertStatus(t, created, http.StatusCreated)
	assertCredentialSmokeNoSecrets(t, created, first)
	id, etag := responseString(t, created, "id"), responseString(t, created, "etag")
	if responseString(t, created, "fingerprint") != credentialSmokeTokenFingerprint("bot", first) {
		t.Fatal("HTTP fingerprint does not match length-prefixed HMAC-SHA256")
	}
	f.assertEncrypted(t, "acme", id, service.CredentialSecret{Kind: "http_token", HTTPUsername: "bot", HTTPToken: first})
	remote, authenticated := credentialSmokeGitServer(t, "bot", first)
	tested := f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/credentials/"+id+":test", map[string]any{"repositoryUrl": remote}, "")
	assertStatus(t, tested, http.StatusOK)
	var result struct {
		OK         bool `json:"ok"`
		ErrorClass any  `json:"errorClass"`
	}
	if err := json.Unmarshal(tested.Body.Bytes(), &result); err != nil || !result.OK || result.ErrorClass != nil || authenticated.Load() == 0 {
		t.Fatalf("real authenticated Git probe did not succeed: %s (decode error: %v)", tested.Body.String(), err)
	}
	assertCredentialSmokeNoSecrets(t, tested, first)
	refs := []credentialSmokeRepository{f.createRepository(t, "acme", "second", id), f.createRepository(t, "acme", "first", id)}
	deleted := f.createRepository(t, "acme", "deleted", id)
	f.deleteRepository(t, deleted)
	f.createRepository(t, "acme", "unbound", "")
	rotated := f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/credentials/"+id+":rotate", map[string]any{
		"secret": map[string]any{"username": "bot", "token": second}, "resyncRepositories": true,
	}, etag)
	f.assertRotation(t, rotated, refs, credentialSmokeTokenFingerprint("bot", second), false)
	assertCredentialSmokeNoSecrets(t, rotated, first, second)
	f.assertEncrypted(t, "acme", id, service.CredentialSecret{Kind: "http_token", HTTPUsername: "bot", HTTPToken: second})
	same := f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/credentials/"+id+":rotate", map[string]any{
		"secret": map[string]any{"username": "bot", "token": second},
	}, responseString(t, rotated, "etag"))
	assertError(t, same, http.StatusUnprocessableEntity, "validation_error")
	listed := f.request(t, "acme", http.MethodGet, "/api/v1/t/acme/credentials", nil, "")
	assertStatus(t, listed, http.StatusOK)
	assertCredentialSmokeNoSecrets(t, listed, first, second)
	metadata := f.request(t, "acme", http.MethodPatch, "/api/v1/t/acme/credentials/"+id, map[string]any{"name": "updated metadata"}, responseString(t, rotated, "etag"))
	assertStatus(t, metadata, http.StatusOK)
	assertCredentialSmokeNoSecrets(t, metadata, first, second)
	if responseString(t, metadata, "fingerprint") != credentialSmokeTokenFingerprint("bot", second) {
		t.Fatal("metadata-only update changed the credential fingerprint")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate SSH credential: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(private, "smoke")
	if err != nil {
		t.Fatalf("marshal SSH credential: %v", err)
	}
	privatePEM := string(pem.EncodeToMemory(block))
	sshCreated := f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/credentials", map[string]any{
		"name": "SSH smoke", "kind": "ssh_key", "sshKey": map[string]any{"privateKeyPem": privatePEM},
	}, "")
	assertStatus(t, sshCreated, http.StatusCreated)
	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatalf("marshal SSH public key: %v", err)
	}
	if responseString(t, sshCreated, "fingerprint") != ssh.FingerprintSHA256(sshPublic) {
		t.Fatal("SSH credential fingerprint is not SHA256 of its derived RFC4253 public key")
	}
	assertCredentialSmokeNoSecrets(t, sshCreated, privatePEM)
	sshID := responseString(t, sshCreated, "id")
	f.assertEncrypted(t, "acme", sshID, service.CredentialSecret{Kind: "ssh_key", PrivateKey: privatePEM})
	assertError(t, f.request(t, "acme", http.MethodDelete, "/api/v1/t/acme/credentials/"+id, nil, responseString(t, metadata, "etag")), http.StatusConflict, "credential_in_use")
	assertStatus(t, f.request(t, "acme", http.MethodDelete, "/api/v1/t/acme/credentials/"+id+"?force=true", nil, responseString(t, metadata, "etag")), http.StatusNoContent)
	f.assertArchivedReferenceCleared(t, deleted)
	archivedSSH := f.createRepository(t, "acme", "archived-ssh", sshID)
	f.deleteRepository(t, archivedSSH)
	assertStatus(t, f.request(t, "acme", http.MethodDelete, "/api/v1/t/acme/credentials/"+sshID, nil, responseString(t, sshCreated, "etag")), http.StatusNoContent)
	f.assertArchivedReferenceCleared(t, archivedSSH)
}

func smokeGlobalCredential(t *testing.T) {
	f := newCredentialSmokeFixture(t)
	const first, second = "global-smoke-first", "global-smoke-rotated"
	body := map[string]any{"name": "global smoke", "kind": "http_token", "httpToken": map[string]any{"username": "global-bot", "token": first}}
	created := f.request(t, "padmin", http.MethodPost, "/api/v1/admin/global-credentials", body, "")
	assertStatus(t, created, http.StatusCreated)
	assertCredentialSmokeNoSecrets(t, created, first)
	id, etag := responseString(t, created, "id"), responseString(t, created, "etag")
	if responseString(t, created, "fingerprint") != credentialSmokeTokenFingerprint("global-bot", first) {
		t.Fatal("global credential fingerprint was not derived from its secret")
	}
	f.assertEncrypted(t, "global", id, service.CredentialSecret{Kind: "http_token", HTTPUsername: "global-bot", HTTPToken: first})
	for _, user := range []string{"acme", "rival"} {
		assertError(t, f.request(t, user, http.MethodPost, "/api/v1/admin/global-credentials", body, ""), http.StatusNotFound, "not_found")
		assertError(t, f.request(t, user, http.MethodPost, "/api/v1/admin/global-credentials/"+id+":rotate", map[string]any{
			"secret": map[string]any{"username": "global-bot", "token": second}, "resyncRepositories": true,
		}, etag), http.StatusNotFound, "not_found")
		assertError(t, f.request(t, user, http.MethodDelete, "/api/v1/admin/global-credentials/"+id+"?force=true", nil, etag), http.StatusNotFound, "not_found")
		listed := f.request(t, user, http.MethodGet, "/api/v1/t/"+user+"/credentials", nil, "")
		assertStatus(t, listed, http.StatusOK)
		assertCredentialSmokeNoSecrets(t, listed, first)
		var page struct {
			Items []struct {
				ID       string `json:"id"`
				IsGlobal bool   `json:"isGlobal"`
			} `json:"items"`
		}
		if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Items[0].ID != id || !page.Items[0].IsGlobal {
			t.Fatalf("global credential is not selectable in tenant %s: %s", user, listed.Body.String())
		}
	}
	refs := []credentialSmokeRepository{f.createRepository(t, "rival", "z", id), f.createRepository(t, "acme", "b", id), f.createRepository(t, "acme", "a", id)}
	deleted := f.createRepository(t, "rival", "deleted", id)
	f.deleteRepository(t, deleted)
	unbound := f.createRepository(t, "acme", "unbound", "")
	rotated := f.request(t, "padmin", http.MethodPost, "/api/v1/admin/global-credentials/"+id+":rotate", map[string]any{
		"secret": map[string]any{"username": "global-bot", "token": second}, "resyncRepositories": true,
	}, etag)
	f.assertRotation(t, rotated, refs, credentialSmokeTokenFingerprint("global-bot", second), true)
	assertCredentialSmokeNoSecrets(t, rotated, first, second)
	f.assertEncrypted(t, "global", id, service.CredentialSecret{Kind: "http_token", HTTPUsername: "global-bot", HTTPToken: second})
	assertError(t, f.request(t, "padmin", http.MethodDelete, "/api/v1/admin/global-credentials/"+id, nil, responseString(t, rotated, "etag")), http.StatusConflict, "credential_in_use")
	deletedCredential := f.request(t, "padmin", http.MethodDelete, "/api/v1/admin/global-credentials/"+id+"?force=true", nil, responseString(t, rotated, "etag"))
	assertStatus(t, deletedCredential, http.StatusNoContent)
	f.assertArchivedReferenceCleared(t, deleted)
	for _, ref := range refs {
		response := f.request(t, ref.tenant, http.MethodGet, "/api/v1/t/"+ref.tenant+"/repositories/"+ref.id, nil, "")
		assertStatus(t, response, http.StatusOK)
		assertCredentialSmokeNoSecrets(t, response, first, second)
		var record struct {
			CredentialID *string `json:"credentialId"`
			Health       struct {
				LastError *struct {
					Class string `json:"class"`
				} `json:"lastError"`
			} `json:"health"`
		}
		// RepositoryHealth expresses auth-required through lastError.class, not a separate state field.
		if err := json.Unmarshal(response.Body.Bytes(), &record); err != nil || record.CredentialID != nil || record.Health.LastError == nil || record.Health.LastError.Class != "auth" {
			t.Fatalf("forced deletion did not clear reference and mark auth health: %s", response.Body.String())
		}
	}
	untouched := f.request(t, "acme", http.MethodGet, "/api/v1/t/acme/repositories/"+unbound.id, nil, "")
	assertStatus(t, untouched, http.StatusOK)
	if strings.Contains(untouched.Body.String(), `"class":"auth"`) {
		t.Fatal("forced delete changed an unrelated repository")
	}
	for _, user := range []string{"acme", "rival"} {
		listed := f.request(t, user, http.MethodGet, "/api/v1/t/"+user+"/credentials", nil, "")
		assertStatus(t, listed, http.StatusOK)
		assertCredentialSmokeNoSecrets(t, listed, first, second)
		if strings.Contains(listed.Body.String(), id) {
			t.Fatal("deleted global credential remained tenant-selectable")
		}
	}
	assertError(t, f.request(t, "padmin", http.MethodPost, "/api/v1/admin/global-credentials/"+id+":test", map[string]any{"repositoryUrl": "https://example.invalid/repository.git"}, ""), http.StatusNotFound, "not_found")
	onlyArchived := f.request(t, "padmin", http.MethodPost, "/api/v1/admin/global-credentials", body, "")
	assertStatus(t, onlyArchived, http.StatusCreated)
	archived := f.createRepository(t, "acme", "archived-global", responseString(t, onlyArchived, "id"))
	f.deleteRepository(t, archived)
	assertStatus(t, f.request(t, "padmin", http.MethodDelete, "/api/v1/admin/global-credentials/"+responseString(t, onlyArchived, "id"), nil, responseString(t, onlyArchived, "etag")), http.StatusNoContent)
	f.assertArchivedReferenceCleared(t, archived)
}

func smokeKnownHost(t *testing.T) {
	f := newCredentialSmokeFixture(t)
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate known-host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatalf("construct known-host signer: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(signer.PublicKey().Marshal())
	wantFingerprint := ssh.FingerprintSHA256(signer.PublicKey())
	body := map[string]any{"host": "git.example.com", "port": 2222, "publicKey": encoded}
	created := f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/known-hosts", body, "")
	assertStatus(t, created, http.StatusCreated)
	if responseString(t, created, "keyType") != ssh.KeyAlgoED25519 || responseString(t, created, "fingerprint") != wantFingerprint || responseString(t, created, "source") != "manual" {
		t.Fatalf("known-host fields are not server-derived: %s", created.Body.String())
	}
	assertError(t, f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/known-hosts", body, ""), http.StatusConflict, "duplicate")
	forged := map[string]any{"host": "git.example.com", "port": 2222, "publicKey": encoded, "keyType": "ssh-rsa", "fingerprint": "SHA256:forged"}
	assertError(t, f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/known-hosts", forged, ""), http.StatusUnprocessableEntity, "validation_error")
	unsupported, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate unsupported key: %v", err)
	}
	unsupportedPublic, err := ssh.NewPublicKey(&unsupported.PublicKey)
	if err != nil {
		t.Fatalf("marshal unsupported key: %v", err)
	}
	for _, value := range []string{"not base64!", base64.StdEncoding.EncodeToString([]byte("malformed")), base64.StdEncoding.EncodeToString(unsupportedPublic.Marshal())} {
		assertError(t, f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/known-hosts", map[string]any{
			"host": "git.example.com", "port": 2222, "publicKey": value,
		}, ""), http.StatusUnprocessableEntity, "validation_error")
	}
	listed := f.request(t, "acme", http.MethodGet, "/api/v1/t/acme/known-hosts", nil, "")
	assertStatus(t, listed, http.StatusOK)
	if !strings.Contains(listed.Body.String(), wantFingerprint) || strings.Contains(listed.Body.String(), encoded) {
		t.Fatalf("known-host list omitted identity or returned key blob: %s", listed.Body.String())
	}
	remote, port := credentialSmokeSSHServer(t, signer)
	t.Setenv("GIT_SSH_COMMAND", "ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -o ConnectTimeout=2")
	candidateResponse := f.request(t, "acme", http.MethodPost, "/api/v1/t/acme/repositories:check-connection", map[string]any{"url": remote}, "")
	assertStatus(t, candidateResponse, http.StatusOK)
	var connection struct {
		OK         bool   `json:"ok"`
		ErrorClass string `json:"errorClass"`
		Candidate  *struct {
			Host        string `json:"host"`
			Port        int    `json:"port"`
			KeyType     string `json:"keyType"`
			PublicKey   string `json:"publicKey"`
			Fingerprint string `json:"fingerprint"`
		} `json:"hostKeyCandidate"`
	}
	if err := json.Unmarshal(candidateResponse.Body.Bytes(), &connection); err != nil || connection.OK || connection.ErrorClass != "host_key" || connection.Candidate == nil {
		t.Fatalf("unknown SSH host did not return a candidate: %s", candidateResponse.Body.String())
	}
	candidate := connection.Candidate
	if candidate.Host != "127.0.0.1" || candidate.Port != port || candidate.KeyType != ssh.KeyAlgoED25519 || candidate.PublicKey != encoded || candidate.Fingerprint != wantFingerprint {
		t.Fatalf("SSH handshake candidate differs from known-host derivation: %#v", candidate)
	}
}

func (f *credentialSmokeFixture) createRepository(t *testing.T, tenant, name, credentialID string) credentialSmokeRepository {
	t.Helper()
	body := map[string]any{"url": "https://git.example.com/" + name + ".git", "defaultBranch": "branch-" + name}
	if credentialID != "" {
		body["credentialId"] = credentialID
	}
	response := f.request(t, tenant, http.MethodPost, "/api/v1/t/"+tenant+"/repositories", body, "")
	assertStatus(t, response, http.StatusCreated)
	if credentialID != "" && responseString(t, response, "credentialId") != credentialID {
		t.Fatal("repository did not bind selected credential")
	}
	return credentialSmokeRepository{tenant: tenant, id: responseString(t, response, "id"), branch: "branch-" + name}
}

func (f *credentialSmokeFixture) deleteRepository(t *testing.T, ref credentialSmokeRepository) {
	t.Helper()
	path := "/api/v1/t/" + ref.tenant + "/repositories/" + ref.id
	current := f.request(t, ref.tenant, http.MethodGet, path, nil, "")
	assertStatus(t, current, http.StatusOK)
	assertStatus(t, f.request(t, ref.tenant, http.MethodDelete, path, nil, responseString(t, current, "etag")), http.StatusNoContent)
}

func (f *credentialSmokeFixture) assertArchivedReferenceCleared(t *testing.T, ref credentialSmokeRepository) {
	t.Helper()
	var deleted, unbound, healthUnchanged bool
	if err := f.db.Pool.QueryRow(t.Context(), `SELECT deleted_at IS NOT NULL, credential_id IS NULL AND global_credential_id IS NULL, health = '{}'::jsonb FROM repositories WHERE tenant_id = $1 AND id = $2`, f.tenants[ref.tenant], ref.id).Scan(&deleted, &unbound, &healthUnchanged); err != nil {
		t.Fatalf("read archived repository reference: %v", err)
	}
	if !deleted || !unbound || !healthUnchanged {
		t.Fatal("credential deletion did not preserve archived repository health with cleared references")
	}
}

func (f *credentialSmokeFixture) assertEncrypted(t *testing.T, scope, id string, want service.CredentialSecret) {
	t.Helper()
	query := `SELECT ciphertext, nonce, key_version, fingerprint FROM credentials WHERE id = $1 AND tenant_id = $2`
	args := []any{id, f.tenants[scope]}
	if scope == "global" {
		query = `SELECT ciphertext, nonce, key_version, fingerprint FROM global_credentials WHERE id = $1`
		args = []any{id}
	}
	var encrypted service.EncryptedCredential
	if err := f.db.Pool.QueryRow(t.Context(), query, args...).Scan(&encrypted.Ciphertext, &encrypted.Nonce, &encrypted.KeyVersion, &encrypted.Fingerprint); err != nil {
		t.Fatalf("read encrypted row: %v", err)
	}
	if len(encrypted.Nonce) != 12 || encrypted.KeyVersion != 1 || len(encrypted.Ciphertext) < 16 {
		t.Fatal("stored credential does not have AES-GCM envelope metadata")
	}
	for _, secret := range []string{want.HTTPToken, want.PrivateKey} {
		if secret != "" && bytes.Contains(encrypted.Ciphertext, []byte(secret)) {
			t.Fatal("database row contains plaintext credential material")
		}
	}
	uuidID, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("parse credential UUID: %v", err)
	}
	if scope != "global" {
		scope = f.tenants[scope].String()
	}
	decrypted, err := f.keyring.Decrypt(scope, uuidID, want.Kind, encrypted)
	if err != nil || decrypted != want {
		t.Fatalf("stored ciphertext failed exact secret round-trip: %v", err)
	}
}

func (f *credentialSmokeFixture) assertRotation(t *testing.T, response *httptest.ResponseRecorder, expected []credentialSmokeRepository, fingerprint string, platform bool) {
	t.Helper()
	assertStatus(t, response, http.StatusOK)
	var result struct {
		ETag       string `json:"etag"`
		Credential struct {
			ETag        string `json:"etag"`
			Fingerprint string `json:"fingerprint"`
		} `json:"credential"`
		SyncJobs []struct {
			Tenant       string `json:"tenantSlug"`
			RepositoryID string `json:"repositoryId"`
			JobID        string `json:"jobId"`
			Deduplicated *bool  `json:"deduplicated"`
		} `json:"syncJobs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode rotation result: %v", err)
	}
	if result.ETag == "" || result.ETag != result.Credential.ETag || result.ETag != response.Header().Get("ETag") || result.Credential.Fingerprint != fingerprint {
		t.Fatalf("rotation ETag or derived fingerprint mismatch: %s", response.Body.String())
	}
	slices.SortFunc(expected, func(a, b credentialSmokeRepository) int {
		return cmp.Or(cmp.Compare(a.tenant, b.tenant), cmp.Compare(a.id, b.id))
	})
	if len(result.SyncJobs) != len(expected) {
		t.Fatalf("rotation has %d jobs, want %d active references", len(result.SyncJobs), len(expected))
	}
	seen := make(map[string]bool)
	for index, item := range result.SyncJobs {
		want := expected[index]
		if item.Tenant != want.tenant || item.RepositoryID != want.id || item.JobID == "" || item.Deduplicated == nil || *item.Deduplicated || seen[item.JobID] {
			t.Fatalf("rotation mapping %d = %#v, want unique non-deduplicated %s/%s", index, item, want.tenant, want.id)
		}
		seen[item.JobID] = true
		var tenantID uuid.UUID
		var scopeID uuid.UUID
		var jobType, scopeType, refType, ref, trigger string
		if err := f.db.Pool.QueryRow(t.Context(), `SELECT tenant_id, scope_id, type, scope_type, ref_type, ref_name, trigger FROM jobs WHERE id = $1`, item.JobID).Scan(&tenantID, &scopeID, &jobType, &scopeType, &refType, &ref, &trigger); err != nil {
			t.Fatalf("read rotation job: %v", err)
		}
		if tenantID != f.tenants[want.tenant] || scopeID.String() != want.id || jobType != "repo.sync" || scopeType != "repository" || refType != "branch" || ref != want.branch || trigger != "credential-rotated" {
			t.Fatalf("rotation job points to wrong tenant/repository/default branch: %s %s %s %s %s %s %s", tenantID, scopeID, jobType, scopeType, refType, ref, trigger)
		}
		if platform {
			job := f.request(t, "padmin", http.MethodGet, "/api/v1/admin/jobs/"+item.JobID, nil, "")
			assertStatus(t, job, http.StatusOK)
			var fields map[string]any
			if err := json.Unmarshal(job.Body.Bytes(), &fields); err != nil {
				t.Fatalf("decode platform job: %v", err)
			}
			for _, forbidden := range []string{"input", "result", "error", "riverJobId", "credentialId", "logs", "attempts"} {
				if _, exists := fields[forbidden]; exists {
					t.Fatalf("platform job exposes %s", forbidden)
				}
			}
			if fields["id"] != item.JobID || fields["tenantSlug"] != want.tenant {
				t.Fatalf("platform job identity mismatch: %s", job.Body.String())
			}
			assertError(t, f.request(t, want.tenant, http.MethodGet, "/api/v1/admin/jobs/"+item.JobID, nil, ""), http.StatusNotFound, "not_found")
		}
	}
}

func credentialSmokeTokenFingerprint(username, token string) string {
	mac := hmac.New(sha256.New, bytes.Repeat([]byte{0x22}, 32))
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(username)))
	_, _ = mac.Write(length[:])
	_, _ = io.WriteString(mac, username)
	_, _ = io.WriteString(mac, token)
	return "HMAC-SHA256:" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func assertCredentialSmokeNoSecrets(t *testing.T, response *httptest.ResponseRecorder, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(response.Body.String(), secret) {
			t.Fatal("HTTP response returned credential secret material")
		}
	}
	var value any
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode redaction check: %v", err)
	}
	var inspect func(any)
	inspect = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, nested := range typed {
				if slices.Contains([]string{"token", "httpToken", "sshKey", "privateKey", "privateKeyPem", "passphrase", "ciphertext", "nonce", "keyVersion"}, key) {
					t.Fatalf("HTTP response exposes secret field %q", key)
				}
				inspect(nested)
			}
		case []any:
			for _, nested := range typed {
				inspect(nested)
			}
		}
	}
	inspect(value)
}

func credentialSmokeGitServer(t *testing.T, username, token string) (string, *atomic.Int32) {
	t.Helper()
	var authenticated atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != username || password != token {
			w.Header().Set("WWW-Authenticate", `Basic realm="meridian-smoke"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/repository.git/info/refs" || r.URL.Query().Get("service") != "git-upload-pack" {
			http.NotFound(w, r)
			return
		}
		authenticated.Add(1)
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		packet := func(data string) { _, _ = fmt.Fprintf(w, "%04x%s", len(data)+4, data) }
		packet("# service=git-upload-pack\n")
		_, _ = io.WriteString(w, "0000")
		const commit = "0123456789012345678901234567890123456789"
		packet(commit + " HEAD\x00symref=HEAD:refs/heads/main\n")
		packet(commit + " refs/heads/main\n")
		_, _ = io.WriteString(w, "0000")
	}))
	t.Cleanup(server.Close)
	certificatePath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatalf("write local Git CA: %v", err)
	}
	t.Setenv("GIT_SSL_CAINFO", certificatePath)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return server.URL + "/repository.git", &authenticated
}

func credentialSmokeSSHServer(t *testing.T, signer ssh.Signer) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for local SSH fixture: %v", err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	var workers sync.WaitGroup
	workers.Go(func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Go(func() {
				defer connection.Close()
				_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
				server, channels, requests, err := ssh.NewServerConn(connection, config)
				if err != nil {
					return
				}
				defer server.Close()
				workers.Go(func() { ssh.DiscardRequests(requests) })
				for channel := range channels {
					_ = channel.Reject(ssh.Prohibited, "host-key fixture only")
				}
			})
		}
	})
	t.Cleanup(func() { _ = listener.Close(); workers.Wait() })
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("parse SSH listener address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse SSH listener port: %v", err)
	}
	return "ssh://" + listener.Addr().String() + "/repository.git", port
}
