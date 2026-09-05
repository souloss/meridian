//go:build integration

package database

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"uuid"

	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

func TestGeneratedRepositoryUsesStandardUUIDAndTransaction(t *testing.T) {
	databaseURL := os.Getenv("MERIDIAN_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MERIDIAN_TEST_DATABASE_URL is not set")
	}

	db, err := Open(t.Context(), databaseURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

	tx, err := db.Pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback(t.Context())
	})

	queries := generated.New(tx)
	userID := uuid.NewV7()
	created, err := queries.CreateUser(t.Context(), generated.CreateUserParams{
		ID:              userID,
		Username:        "repository-transaction-test",
		PasswordHash:    "$argon2id$v=19$m=65536,t=3,p=1$fixture$fixture",
		DisplayName:     "Repository transaction test",
		IsPlatformAdmin: false,
	})
	if err != nil {
		t.Fatalf("create user with standard UUID: %v", err)
	}
	if created.ID != userID {
		t.Fatalf("created user ID = %s, want %s", created.ID, userID)
	}
	if _, err := queries.CreateDefaultUserPreferences(t.Context(), userID); err != nil {
		t.Fatalf("create user preferences: %v", err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatalf("roll back transaction: %v", err)
	}

	var count int
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE id = $1`, userID).Scan(&count); err != nil {
		t.Fatalf("count rolled-back user: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back user count = %d, want 0", count)
	}
}

func TestCredentialRotationEnqueuesAtomicSyncJob(t *testing.T) {
	databaseURL := os.Getenv("MERIDIAN_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MERIDIAN_TEST_DATABASE_URL is not set")
	}

	db, err := Open(t.Context(), databaseURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
		t.Fatalf("reset credential fixtures: %v", err)
	}

	userID, tenantID, credentialID, repositoryID := uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
	if _, err := db.Pool.Exec(t.Context(), `INSERT INTO users (id, username, password_hash, display_name) VALUES ($1, $2, $3, $4)`, userID, "rotation-test", "fixture-password-hash", "Rotation Test"); err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `INSERT INTO tenants (id, slug, display_name, quota, settings) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb)`, tenantID, "acme", "Acme"); err != nil {
		t.Fatalf("create fixture tenant: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `INSERT INTO tenant_members (tenant_id, user_id, role) VALUES ($1, $2, 'tenant_admin')`, tenantID, userID); err != nil {
		t.Fatalf("create fixture membership: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `
		INSERT INTO credentials (tenant_id, id, name, kind, ciphertext, nonce, key_version, fingerprint, created_by)
		VALUES ($1, $2, 'deploy key', 'http_token', $3, $4, 1, 'fingerprint-old', $5)
	`, tenantID, credentialID, []byte("old-ciphertext"), bytes.Repeat([]byte{0x01}, 12), userID); err != nil {
		t.Fatalf("create fixture credential: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `
		INSERT INTO repositories (tenant_id, id, url, canonical_url, credential_id, branch_policy, fetch_config)
		VALUES ($1, $2, 'https://git.example/acme/api.git', 'https://git.example/acme/api.git', $3, '{}'::jsonb, '{}'::jsonb)
	`, tenantID, repositoryID, credentialID); err != nil {
		t.Fatalf("create credential fixtures: %v", err)
	}

	store := repository.NewCredentialStore(db.Pool)
	idempotencyKey := uuid.NewV7()
	requestHash := bytes.Repeat([]byte{0x55}, 32)
	rotated, jobs, err := store.RotateCredential(t.Context(), service.RotateCredential{
		TenantID: tenantID, ID: credentialID, ExpectedRevision: 1,
		Encrypted:          service.EncryptedCredential{Ciphertext: []byte("new-ciphertext"), Nonce: bytes.Repeat([]byte{0x02}, 12), KeyVersion: 1, Fingerprint: "fingerprint-new"},
		ResyncRepositories: true,
		IdempotencyKey:     idempotencyKey, RequestHash: requestHash, PrincipalType: "session", PrincipalID: userID,
	})
	if err != nil {
		t.Fatalf("rotate credential: %v", err)
	}
	if rotated.Revision != 2 || len(jobs) != 1 {
		t.Fatalf("rotation result = revision %d, jobs %d; want revision 2 and one job", rotated.Revision, len(jobs))
	}
	if jobs[0].TenantSlug != "acme" || jobs[0].RepositoryID != repositoryID || jobs[0].Deduplicated {
		t.Fatalf("rotation job = %#v, want first non-deduplicated acme repository job", jobs[0])
	}
	var jobType, trigger, status string
	var jobCount int
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*), type, trigger, status FROM jobs WHERE tenant_id = $1 AND id = $2 GROUP BY type, trigger, status`, tenantID, jobs[0].JobID).Scan(&jobCount, &jobType, &trigger, &status); err != nil {
		t.Fatalf("read enqueued job: %v", err)
	}
	if jobCount != 1 || jobType != "repo.sync" || trigger != "credential-rotated" || status != "pending" {
		t.Fatalf("stored job = count %d type %q trigger %q status %q", jobCount, jobType, trigger, status)
	}

	replayed, replayJobs, err := store.RotateCredential(t.Context(), service.RotateCredential{
		TenantID: tenantID, ID: credentialID, ExpectedRevision: 999,
		Encrypted:      service.EncryptedCredential{Ciphertext: []byte("must-not-write"), Nonce: bytes.Repeat([]byte{0x09}, 12), KeyVersion: 1, Fingerprint: "must-not-write"},
		IdempotencyKey: idempotencyKey, RequestHash: requestHash, PrincipalType: "session", PrincipalID: userID,
	})
	if err != nil {
		t.Fatalf("replay credential rotation: %v", err)
	}
	if replayed.Revision != rotated.Revision || replayed.Encrypted.Fingerprint != rotated.Encrypted.Fingerprint || len(replayJobs) != 1 || replayJobs[0] != jobs[0] {
		t.Fatalf("rotation replay = record %#v jobs %#v, want original result", replayed, replayJobs)
	}
	if _, _, err := store.RotateCredential(t.Context(), service.RotateCredential{
		TenantID: tenantID, ID: credentialID, ExpectedRevision: 999,
		Encrypted:      service.EncryptedCredential{Ciphertext: []byte("different"), Nonce: bytes.Repeat([]byte{0x08}, 12), KeyVersion: 1, Fingerprint: "different"},
		IdempotencyKey: idempotencyKey, RequestHash: bytes.Repeat([]byte{0x77}, 32), PrincipalType: "session", PrincipalID: userID,
	}); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatalf("different rotation request error = %v, want ErrIdempotencyConflict", err)
	}

	rotatedAgain, jobsAgain, err := store.RotateCredential(t.Context(), service.RotateCredential{
		TenantID: tenantID, ID: credentialID, ExpectedRevision: rotated.Revision,
		Encrypted:          service.EncryptedCredential{Ciphertext: []byte("third-ciphertext"), Nonce: bytes.Repeat([]byte{0x03}, 12), KeyVersion: 1, Fingerprint: "fingerprint-third"},
		ResyncRepositories: true,
		IdempotencyKey:     uuid.NewV7(), RequestHash: bytes.Repeat([]byte{0x66}, 32), PrincipalType: "session", PrincipalID: userID,
	})
	if err != nil {
		t.Fatalf("rotate credential with active dedupe job: %v", err)
	}
	if rotatedAgain.Revision != 3 || len(jobsAgain) != 1 || jobsAgain[0].JobID != jobs[0].JobID || !jobsAgain[0].Deduplicated {
		t.Fatalf("deduplicated rotation = revision %d jobs %#v", rotatedAgain.Revision, jobsAgain)
	}
}
