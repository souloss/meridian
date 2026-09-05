package repository

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// credentialRotationReplay is the non-secret response projection retained for a rotation replay.
// Encrypted ciphertext, nonce, key version, and all write-only secret fields are deliberately absent.
type credentialRotationReplay struct {
	TenantID    uuid.UUID                   `json:"tenantId"`
	ID          uuid.UUID                   `json:"id"`
	IsGlobal    bool                        `json:"isGlobal"`
	Name        string                      `json:"name"`
	Kind        string                      `json:"kind"`
	Fingerprint string                      `json:"fingerprint"`
	SharedScope string                      `json:"sharedScope"`
	TeamIDs     []uuid.UUID                 `json:"teamIds"`
	CreatedBy   uuid.UUID                   `json:"createdBy"`
	LastUsedAt  *time.Time                  `json:"lastUsedAt,omitempty"`
	Revision    int64                       `json:"revision"`
	CreatedAt   time.Time                   `json:"createdAt"`
	UpdatedAt   time.Time                   `json:"updatedAt"`
	SyncJobs    []service.CredentialSyncJob `json:"syncJobs"`
}

func rotationIdempotencyEnabled(key uuid.UUID) bool {
	return key != uuid.Nil()
}

func validateRotationIdempotency(key uuid.UUID, requestHash []byte, principalType string, principalID uuid.UUID) error {
	if !rotationIdempotencyEnabled(key) {
		return nil
	}
	if len(requestHash) != 32 || principalType == "" || principalID == uuid.Nil() {
		return service.ErrValidation
	}
	return nil
}

func rotationLockKey(scope, principalType string, principalID, idempotencyKey uuid.UUID) string {
	return strings.Join([]string{scope, principalType, principalID.String(), idempotencyKey.String()}, ":")
}

func loadTenantRotationReplay(ctx context.Context, queries *generated.Queries, tenantID uuid.UUID, principalType string, principalID, idempotencyKey uuid.UUID, requestHash []byte) (service.CredentialRecord, []service.CredentialSyncJob, bool, error) {
	if !rotationIdempotencyEnabled(idempotencyKey) {
		return service.CredentialRecord{}, nil, false, nil
	}
	if err := queries.LockCredentialRotationIdempotency(ctx, rotationLockKey("tenant", principalType, principalID, idempotencyKey)); err != nil {
		return service.CredentialRecord{}, nil, false, normalizeError(err)
	}
	row, err := queries.GetCredentialRotationIdempotency(ctx, generated.GetCredentialRotationIdempotencyParams{
		TenantID: tenantID, PrincipalType: principalType, PrincipalID: principalID, IdempotencyKey: idempotencyKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return service.CredentialRecord{}, nil, false, nil
	}
	if err != nil {
		return service.CredentialRecord{}, nil, false, normalizeError(err)
	}
	if !row.ExpiresAt.Valid || !row.ExpiresAt.Time.After(time.Now()) {
		if err := queries.DeleteCredentialRotationIdempotency(ctx, generated.DeleteCredentialRotationIdempotencyParams{
			TenantID: tenantID, PrincipalType: principalType, PrincipalID: principalID, IdempotencyKey: idempotencyKey,
		}); err != nil {
			return service.CredentialRecord{}, nil, false, normalizeError(err)
		}
		return service.CredentialRecord{}, nil, false, nil
	}
	if !bytes.Equal(row.RequestHash, requestHash) {
		return service.CredentialRecord{}, nil, false, service.ErrIdempotencyConflict
	}
	var replay credentialRotationReplay
	if err := json.Unmarshal(row.ResponseBody, &replay); err != nil {
		return service.CredentialRecord{}, nil, false, fmt.Errorf("decode credential rotation replay: %w", err)
	}
	return replay.tenantRecord(), replay.SyncJobs, true, nil
}

func saveTenantRotationReplay(ctx context.Context, queries *generated.Queries, tenantID uuid.UUID, principalType string, principalID, idempotencyKey uuid.UUID, requestHash []byte, record service.CredentialRecord, jobs []service.CredentialSyncJob) error {
	if !rotationIdempotencyEnabled(idempotencyKey) {
		return nil
	}
	replay, err := json.Marshal(newCredentialRotationReplay(record, jobs))
	if err != nil {
		return fmt.Errorf("encode credential rotation replay: %w", err)
	}
	return normalizeError(queries.CreateCredentialRotationIdempotency(ctx, generated.CreateCredentialRotationIdempotencyParams{
		TenantID: tenantID, PrincipalType: principalType, PrincipalID: principalID, IdempotencyKey: idempotencyKey,
		RequestHash: bytes.Clone(requestHash), ResponseBody: replay,
	}))
}

func loadGlobalRotationReplay(ctx context.Context, queries *generated.Queries, principalType string, principalID, idempotencyKey uuid.UUID, requestHash []byte) (service.GlobalCredentialRecord, []service.CredentialSyncJob, bool, error) {
	if !rotationIdempotencyEnabled(idempotencyKey) {
		return service.GlobalCredentialRecord{}, nil, false, nil
	}
	if err := queries.LockCredentialRotationIdempotency(ctx, rotationLockKey("platform", principalType, principalID, idempotencyKey)); err != nil {
		return service.GlobalCredentialRecord{}, nil, false, normalizeError(err)
	}
	row, err := queries.GetGlobalCredentialRotationIdempotency(ctx, generated.GetGlobalCredentialRotationIdempotencyParams{
		PrincipalType: principalType, PrincipalID: principalID, IdempotencyKey: idempotencyKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return service.GlobalCredentialRecord{}, nil, false, nil
	}
	if err != nil {
		return service.GlobalCredentialRecord{}, nil, false, normalizeError(err)
	}
	if !row.ExpiresAt.Valid || !row.ExpiresAt.Time.After(time.Now()) {
		if err := queries.DeleteGlobalCredentialRotationIdempotency(ctx, generated.DeleteGlobalCredentialRotationIdempotencyParams{
			PrincipalType: principalType, PrincipalID: principalID, IdempotencyKey: idempotencyKey,
		}); err != nil {
			return service.GlobalCredentialRecord{}, nil, false, normalizeError(err)
		}
		return service.GlobalCredentialRecord{}, nil, false, nil
	}
	if !bytes.Equal(row.RequestHash, requestHash) {
		return service.GlobalCredentialRecord{}, nil, false, service.ErrIdempotencyConflict
	}
	var replay credentialRotationReplay
	if err := json.Unmarshal(row.ResponseBody, &replay); err != nil {
		return service.GlobalCredentialRecord{}, nil, false, fmt.Errorf("decode global credential rotation replay: %w", err)
	}
	return replay.globalRecord(), replay.SyncJobs, true, nil
}

func saveGlobalRotationReplay(ctx context.Context, queries *generated.Queries, principalType string, principalID, idempotencyKey uuid.UUID, requestHash []byte, record service.GlobalCredentialRecord, jobs []service.CredentialSyncJob) error {
	if !rotationIdempotencyEnabled(idempotencyKey) {
		return nil
	}
	replay, err := json.Marshal(newGlobalCredentialRotationReplay(record, jobs))
	if err != nil {
		return fmt.Errorf("encode global credential rotation replay: %w", err)
	}
	return normalizeError(queries.CreateGlobalCredentialRotationIdempotency(ctx, generated.CreateGlobalCredentialRotationIdempotencyParams{
		PrincipalType: principalType, PrincipalID: principalID, IdempotencyKey: idempotencyKey,
		RequestHash: bytes.Clone(requestHash), ResponseBody: replay,
	}))
}

func newCredentialRotationReplay(record service.CredentialRecord, jobs []service.CredentialSyncJob) credentialRotationReplay {
	return credentialRotationReplay{
		TenantID: record.TenantID, ID: record.ID, IsGlobal: record.IsGlobal, Name: record.Name, Kind: record.Kind,
		Fingerprint: record.Encrypted.Fingerprint, SharedScope: record.SharedScope, TeamIDs: record.TeamIDs,
		CreatedBy: record.CreatedBy, LastUsedAt: record.LastUsedAt, Revision: record.Revision,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, SyncJobs: jobs,
	}
}

func newGlobalCredentialRotationReplay(record service.GlobalCredentialRecord, jobs []service.CredentialSyncJob) credentialRotationReplay {
	return credentialRotationReplay{
		ID: record.ID, IsGlobal: true, Name: record.Name, Kind: record.Kind, Fingerprint: record.Encrypted.Fingerprint,
		CreatedBy: record.CreatedBy, LastUsedAt: record.LastUsedAt, Revision: record.Revision,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, SyncJobs: jobs,
	}
}

func (replay credentialRotationReplay) tenantRecord() service.CredentialRecord {
	return service.CredentialRecord{
		TenantID: replay.TenantID, ID: replay.ID, IsGlobal: replay.IsGlobal, Name: replay.Name, Kind: replay.Kind,
		Encrypted: service.EncryptedCredential{Fingerprint: replay.Fingerprint}, SharedScope: replay.SharedScope,
		TeamIDs: replay.TeamIDs, CreatedBy: replay.CreatedBy, LastUsedAt: replay.LastUsedAt,
		Revision: replay.Revision, CreatedAt: replay.CreatedAt, UpdatedAt: replay.UpdatedAt,
	}
}

func (replay credentialRotationReplay) globalRecord() service.GlobalCredentialRecord {
	return service.GlobalCredentialRecord{
		ID: replay.ID, Name: replay.Name, Kind: replay.Kind, Encrypted: service.EncryptedCredential{Fingerprint: replay.Fingerprint},
		CreatedBy: replay.CreatedBy, LastUsedAt: replay.LastUsedAt, Revision: replay.Revision,
		CreatedAt: replay.CreatedAt, UpdatedAt: replay.UpdatedAt,
	}
}
