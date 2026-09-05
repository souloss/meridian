//go:build integration

package database

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/storage"
	"github.com/meridian-labs/meridian/internal/task"
	"github.com/riverqueue/river"
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

func TestBlobReferenceRegistryEnforcesUniqueByteQuota(t *testing.T) {
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
		t.Fatalf("reset blob fixtures: %v", err)
	}
	local, err := storage.NewLocalStore(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatalf("construct local blob store: %v", err)
	}
	content := []byte("tenant unique bytes")
	blob, err := local.Put(t.Context(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("write local blob: %v", err)
	}
	tenantID, secondTenantID := uuid.NewV7(), uuid.NewV7()
	if _, err := db.Pool.Exec(t.Context(), `
		INSERT INTO tenants (id, slug, display_name, quota, settings)
		VALUES
		  ($1, 'blob-acme', 'Blob Acme', jsonb_build_object('maxStorageBytes', $3::bigint), '{}'::jsonb),
		  ($2, 'blob-rival', 'Blob Rival', jsonb_build_object('maxStorageBytes', $3::bigint), '{}'::jsonb)
	`, tenantID, secondTenantID, blob.Size); err != nil {
		t.Fatalf("create blob tenants: %v", err)
	}
	registry := repository.NewRepositoryStore(db.Pool)
	now := time.Now().UTC()
	if err := registry.AddBlobReference(t.Context(), tenantID, blob, "application/json", now); err != nil {
		t.Fatalf("register first tenant blob reference: %v", err)
	}
	if err := registry.AddBlobReference(t.Context(), tenantID, blob, "application/json", now.Add(time.Second)); err != nil {
		t.Fatalf("register duplicate tenant blob reference: %v", err)
	}
	if err := registry.AddBlobReference(t.Context(), secondTenantID, blob, "application/json", now); err != nil {
		t.Fatalf("reuse global blob for second tenant: %v", err)
	}
	var blobCount, tenantReferenceCount int
	var firstRefCount int64
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM blobs`).Scan(&blobCount); err != nil {
		t.Fatalf("count global blobs: %v", err)
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM tenant_blob_refs WHERE blob_digest = $1`, blob.Digest).Scan(&tenantReferenceCount); err != nil {
		t.Fatalf("count tenant blob references: %v", err)
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT ref_count FROM tenant_blob_refs WHERE tenant_id = $1 AND blob_digest = $2`, tenantID, blob.Digest).Scan(&firstRefCount); err != nil {
		t.Fatalf("read first tenant reference count: %v", err)
	}
	if blobCount != 1 || tenantReferenceCount != 2 || firstRefCount != 2 {
		t.Fatalf("blob/reference counts = %d/%d/%d, want 1/2/2", blobCount, tenantReferenceCount, firstRefCount)
	}
	if err := registry.AddBlobReference(t.Context(), secondTenantID, blob, "text/plain", now); !errors.Is(err, storage.ErrBlobMetadataConflict) {
		t.Fatalf("conflicting blob metadata error = %v, want ErrBlobMetadataConflict", err)
	}
	additional, err := local.Put(t.Context(), bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatalf("write additional local blob: %v", err)
	}
	if err := registry.AddBlobReference(t.Context(), tenantID, additional, "text/plain", now); !errors.Is(err, service.ErrQuotaExceeded) {
		t.Fatalf("over-quota blob reference error = %v, want ErrQuotaExceeded", err)
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM blobs`).Scan(&blobCount); err != nil {
		t.Fatalf("count blobs after rejected reference: %v", err)
	}
	if blobCount != 1 {
		t.Fatalf("blob metadata count after quota rejection = %d, want 1", blobCount)
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
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset River fixtures: %v", err)
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

	riverStore := repository.NewRepositoryStore(db.Pool)
	riverRuntime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions: riverStore, SyncRunner: task.UnsupportedSyncRunner{}, Outbox: riverStore,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("configure River runtime: %v", err)
	}
	store := repository.NewCredentialStoreWithRiver(db.Pool, riverRuntime.Client())
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
	var riverJobID *int64
	if err := db.Pool.QueryRow(t.Context(), `SELECT river_job_id FROM jobs WHERE tenant_id = $1 AND id = $2`, tenantID, jobs[0].JobID).Scan(&riverJobID); err != nil {
		t.Fatalf("read domain River job id: %v", err)
	}
	if riverJobID == nil {
		t.Fatal("domain sync job has no River job id")
	}
	var riverKind string
	if err := db.Pool.QueryRow(t.Context(), `SELECT kind FROM river.river_job WHERE id = $1`, *riverJobID).Scan(&riverKind); err != nil {
		t.Fatalf("read River job row: %v", err)
	}
	if riverKind != "meridian_repo_sync" {
		t.Fatalf("River job kind = %q, want meridian_repo_sync", riverKind)
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

func TestRiverWorkerPersistsTerminalFailureAndStageLogs(t *testing.T) {
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
		t.Fatalf("reset worker fixtures: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `TRUNCATE river.river_job CASCADE`); err != nil {
		t.Fatalf("reset River worker fixtures: %v", err)
	}
	tenantID, jobID, repositoryID := uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
	if _, err := db.Pool.Exec(t.Context(), `
		INSERT INTO tenants (id, slug, display_name, quota, settings)
		VALUES ($1, 'worker-acme', 'Worker Acme', '{}'::jsonb, '{}'::jsonb)
	`, tenantID); err != nil {
		t.Fatalf("create worker tenant: %v", err)
	}
	channelID := uuid.NewV7()
	if _, err := db.Pool.Exec(t.Context(), `
		INSERT INTO notification_channels (tenant_id, id, type, name, encrypted_config)
		VALUES ($1, $2, 'webhook', 'Worker failures', $3)
	`, tenantID, channelID, []byte("opaque-encrypted-config")); err != nil {
		t.Fatalf("create worker notification channel: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `
		INSERT INTO jobs (tenant_id, id, type, scope_type, scope_id, trigger, input, dedupe_key, replay_safe)
		VALUES ($1, $2, 'repo.sync', 'repository', $3, 'system', '{}'::jsonb, $4, true)
	`, tenantID, jobID, repositoryID, "worker:"+jobID.String()); err != nil {
		t.Fatalf("create worker domain job: %v", err)
	}

	riverStore := repository.NewRepositoryStore(db.Pool)
	runtime, err := task.NewRuntime(db.Pool, task.RuntimeDependencies{
		Executions: riverStore, SyncRunner: task.UnsupportedSyncRunner{}, Outbox: riverStore,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("configure River runtime: %v", err)
	}
	completed, cancel := runtime.Client().Subscribe(river.EventKindJobCompleted)
	t.Cleanup(cancel)
	if err := runtime.Start(t.Context()); err != nil {
		t.Fatalf("start River runtime: %v", err)
	}
	t.Cleanup(func() {
		stopContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := runtime.Stop(stopContext); err != nil {
			t.Errorf("stop River runtime: %v", err)
		}
	})

	tx, err := db.Pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin River insert transaction: %v", err)
	}
	if _, err := runtime.Client().InsertTx(t.Context(), tx, task.CredentialSyncArgs{
		TenantID: tenantID, JobID: jobID, RepositoryID: repositoryID, RefName: "main",
	}, nil); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("insert River job: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit River insert: %v", err)
	}
	waitForRiverCompletionKind(t, completed, task.CredentialSyncArgs{}.Kind())

	var status string
	var stage *string
	if err := db.Pool.QueryRow(t.Context(), `SELECT status, stage FROM jobs WHERE tenant_id = $1 AND id = $2`, tenantID, jobID).Scan(&status, &stage); err != nil {
		t.Fatalf("read worker domain state: %v", err)
	}
	if status != "failed" || stage == nil || *stage != "resolve" {
		t.Fatalf("worker domain state = status %q stage %v, want failed/resolve", status, stage)
	}
	var logCount int
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM job_stage_logs WHERE tenant_id = $1 AND job_id = $2`, tenantID, jobID).Scan(&logCount); err != nil {
		t.Fatalf("read worker stage logs: %v", err)
	}
	if logCount != 2 {
		t.Fatalf("worker stage log count = %d, want start and terminal failure", logCount)
	}
	var auditCount int
	if err := db.Pool.QueryRow(t.Context(), `
		SELECT count(*) FROM audit_logs
		WHERE tenant_id = $1 AND action = 'job.failed' AND target_type = 'job' AND target_id = $2
	`, tenantID, jobID).Scan(&auditCount); err != nil {
		t.Fatalf("read job failure audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("job failure audit count = %d, want 1", auditCount)
	}
	var eventID uuid.UUID
	var outboxID uuid.UUID
	var outboxStatus string
	var outboxPayload []byte
	var aggregateVersion int64
	if err := db.Pool.QueryRow(t.Context(), `
		SELECT id, event_id, status, payload, aggregate_version
		FROM notify_outbox
		WHERE tenant_id = $1 AND channel_id = $2 AND aggregate_id = $3
	`, tenantID, channelID, jobID).Scan(&outboxID, &eventID, &outboxStatus, &outboxPayload, &aggregateVersion); err != nil {
		t.Fatalf("read collect.failed outbox row: %v", err)
	}
	var envelope struct {
		EventID          string `json:"eventId"`
		EventType        string `json:"eventType"`
		TenantSlug       string `json:"tenantSlug"`
		AggregateType    string `json:"aggregateType"`
		AggregateID      string `json:"aggregateId"`
		AggregateVersion int    `json:"aggregateVersion"`
		Payload          struct {
			RepositoryID string `json:"repositoryId"`
			JobID        string `json:"jobId"`
			Stage        string `json:"stage"`
			ErrorCode    string `json:"errorCode"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(outboxPayload, &envelope); err != nil {
		t.Fatalf("decode collect.failed envelope: %v", err)
	}
	if (outboxStatus != "pending" && outboxStatus != "delivering" && outboxStatus != "failed") || eventID.String() != envelope.EventID || envelope.EventType != "collect.failed" || envelope.TenantSlug != "worker-acme" || envelope.AggregateType != "job" || envelope.AggregateID != jobID.String() || envelope.AggregateVersion != 1 || aggregateVersion != 1 || envelope.Payload.RepositoryID != repositoryID.String() || envelope.Payload.JobID != jobID.String() || envelope.Payload.Stage != "resolve" || envelope.Payload.ErrorCode != "worker_failed" {
		t.Fatalf("collect.failed outbox/envelope = %s/%#v", outboxStatus, envelope)
	}
	if _, err := runtime.Client().Insert(t.Context(), task.OutboxDispatchArgs{}, nil); err != nil {
		t.Fatalf("insert outbox dispatch job: %v", err)
	}
	waitForRiverCompletionKind(t, completed, task.OutboxDispatchArgs{}.Kind())
	retryCount, lastError := waitForOutboxFailure(t, db, tenantID, eventID, channelID, &outboxStatus)
	if outboxStatus != "failed" || retryCount != 1 || lastError != "delivery_unavailable" {
		t.Fatalf("dispatched outbox state = %s/%d/%q, want failed/1/delivery_unavailable", outboxStatus, retryCount, lastError)
	}
	staleClaimedAt := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := db.Pool.Exec(t.Context(), `
		UPDATE notify_outbox
		SET status = 'delivering', updated_at = $4, next_attempt_at = now() + interval '1 hour'
		WHERE tenant_id = $1 AND id = $2 AND event_id = $3
	`, tenantID, outboxID, eventID, staleClaimedAt); err != nil {
		t.Fatalf("make outbox lease stale: %v", err)
	}
	reclaimedAt := time.Now().UTC()
	reclaimed, claimed, err := riverStore.ClaimOutboxDelivery(t.Context(), task.ClaimDeliveryInput{
		ClaimedAt: reclaimedAt, LeaseExpiredAt: reclaimedAt.Add(-5 * time.Minute), MaxAttempts: 6,
	})
	if err != nil || !claimed || reclaimed.ID != outboxID {
		t.Fatalf("reclaim stale outbox = %#v/%v/%v, want outbox %s", reclaimed, claimed, err, outboxID)
	}
	if err := riverStore.MarkOutboxDelivered(t.Context(), task.OutboxDelivery{
		TenantID: tenantID, ID: outboxID, ClaimedAt: staleClaimedAt,
	}, time.Now().UTC()); err != nil {
		t.Fatalf("fence stale delivery completion: %v", err)
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM notify_outbox WHERE tenant_id = $1 AND id = $2`, tenantID, outboxID).Scan(&outboxStatus); err != nil {
		t.Fatalf("read fenced outbox status: %v", err)
	}
	if outboxStatus != "delivering" {
		t.Fatalf("stale completion changed outbox status to %q", outboxStatus)
	}
	if err := riverStore.MarkOutboxDelivered(t.Context(), reclaimed, time.Now().UTC()); err != nil {
		t.Fatalf("complete reclaimed outbox: %v", err)
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM notify_outbox WHERE tenant_id = $1 AND id = $2`, tenantID, outboxID).Scan(&outboxStatus); err != nil {
		t.Fatalf("read completed outbox status: %v", err)
	}
	if outboxStatus != "delivered" {
		t.Fatalf("reclaimed outbox status = %q, want delivered", outboxStatus)
	}
}

func waitForRiverCompletionKind(t *testing.T, completed <-chan *river.Event, kind string) {
	t.Helper()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case event := <-completed:
			if event != nil && event.Job != nil && event.Job.Kind == kind {
				return
			}
		case <-timeout.C:
			t.Fatalf("timed out waiting for River completion kind %q", kind)
		}
	}
}

func waitForOutboxFailure(t *testing.T, database *Database, tenantID, eventID, channelID uuid.UUID, status *string) (int, string) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var retryCount int
		var lastError string
		if err := database.Pool.QueryRow(t.Context(), `
			SELECT status, retry_count, last_error FROM notify_outbox
			WHERE tenant_id = $1 AND event_id = $2 AND channel_id = $3
		`, tenantID, eventID, channelID).Scan(status, &retryCount, &lastError); err != nil {
			t.Fatalf("read dispatched outbox row: %v", err)
		}
		if *status == "failed" {
			return retryCount, lastError
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for outbox failure; last status %q", *status)
		}
	}
}
