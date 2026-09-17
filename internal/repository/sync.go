package repository

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/task"
	"github.com/riverqueue/river"
)

// EnqueueSyncJob records one repo.sync job and its River work atomically.
func (store *DiscoveryStore) EnqueueSyncJob(ctx context.Context, input service.SyncJobInput) (service.JobAccepted, error) {
	dedupeKey := syncDedupeKey(input.RepositoryID, input.RefType, input.RefName)
	jobInput, err := json.Marshal(struct {
		Force *bool `json:"force,omitempty"`
	}{Force: input.Force})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("encode sync job input: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("begin sync job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)

	// Idempotency replay: one winner enqueues; concurrent same-key requests
	// replay the exact stored response, a different digest yields 409.
	if input.IdempotencyKey != uuid.Nil() {
		if err := queries.LockSyncIdempotency(ctx, syncIdempotencyLockKey(input)); err != nil {
			return service.JobAccepted{}, normalizeError(err)
		}
		if replay, found, err := loadSyncReplay(ctx, queries, input); err != nil {
			return service.JobAccepted{}, err
		} else if found {
			return replay, nil
		}
	}

	for {
		latest, err := queries.LockLatestDiscoveryJob(ctx, generated.LockLatestDiscoveryJobParams{TenantID: input.TenantID, DedupeKey: dedupeKey})
		generation := int64(1)
		if err == nil {
			if latest.Status == "pending" || latest.Status == "running" {
				// A duplicate request while a job is running sets the dirty flag so
				// one successor processes the latest input after completion.
				if latest.Status == "running" {
					if _, err := queries.MarkSyncJobDirty(ctx, generated.MarkSyncJobDirtyParams{TenantID: input.TenantID, DedupeKey: dedupeKey, ActiveGeneration: latest.ActiveGeneration}); err != nil {
						return service.JobAccepted{}, normalizeError(err)
					}
				}
				return service.JobAccepted{JobID: latest.ID, Deduplicated: true}, nil
			}
			generation = latest.ActiveGeneration + 1
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return service.JobAccepted{}, normalizeError(err)
		}

		jobID := uuid.NewV7()
		refType := input.RefType
		refName := input.RefName
		row, err := queries.CreateSyncJob(ctx, generated.CreateSyncJobParams{
			TenantID: input.TenantID, ID: jobID, RepositoryID: new(input.RepositoryID),
			RefType: new(refType), RefName: new(refName), JobInput: jobInput,
			DedupeKey: dedupeKey, ActiveGeneration: generation,
		})
		if err == nil {
			if store.riverClient != nil {
				inserted, err := store.riverClient.InsertTx(ctx, tx, task.CredentialSyncArgs{
					TenantID: input.TenantID, JobID: row.ID, RepositoryID: input.RepositoryID, RefName: refName,
				}, &river.InsertOpts{MaxAttempts: int(row.MaxAttempts)})
				if err != nil {
					return service.JobAccepted{}, fmt.Errorf("insert River sync job: %w", err)
				}
				if changed, err := queries.AttachRiverJobID(ctx, generated.AttachRiverJobIDParams{
					TenantID: input.TenantID, ID: row.ID, RiverJobID: new(inserted.Job.ID), UpdatedAt: timestamp(time.Now().UTC()),
				}); err != nil {
					return service.JobAccepted{}, normalizeError(err)
				} else if changed != 1 {
					return service.JobAccepted{}, fmt.Errorf("attach River job %d to domain job %s: %w", inserted.Job.ID, row.ID, service.ErrPrecondition)
				}
			}
			accepted := service.JobAccepted{JobID: row.ID, Deduplicated: false}
			if input.IdempotencyKey != uuid.Nil() {
				if err := saveSyncReplay(ctx, queries, input, accepted); err != nil {
					return service.JobAccepted{}, err
				}
			}
			if err := tx.Commit(ctx); err != nil {
				return service.JobAccepted{}, normalizeError(err)
			}
			return accepted, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return service.JobAccepted{}, normalizeError(err)
		}
	}
}

func syncDedupeKey(repositoryID uuid.UUID, refType, refName string) string {
	return "repo:" + repositoryID.String() + ":" + refType + ":" + refName
}

// MarkSyncJobDirty flags a running repo.sync job for one successor after completion.
func (store *DiscoveryStore) MarkSyncJobDirty(ctx context.Context, tenantID uuid.UUID, dedupeKey string, activeGeneration int64) error {
	if _, err := store.queries.MarkSyncJobDirty(ctx, generated.MarkSyncJobDirtyParams{TenantID: tenantID, DedupeKey: dedupeKey, ActiveGeneration: activeGeneration}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// GetSyncJobForSuccessor returns the completion state used to decide whether a
// dirty sync job must spawn a successor.
func (store *DiscoveryStore) GetSyncJobForSuccessor(ctx context.Context, tenantID, jobID uuid.UUID) (service.SyncJobSuccessorState, error) {
	row, err := store.queries.GetSyncJobForSuccessor(ctx, generated.GetSyncJobForSuccessorParams{TenantID: tenantID, ID: jobID})
	if err != nil {
		return service.SyncJobSuccessorState{}, normalizeError(err)
	}
	return service.SyncJobSuccessorState{
		JobID: row.ID, Status: row.Status, Dirty: row.Dirty, ScopeID: row.ScopeID, RefType: row.RefType,
		RefName: row.RefName, ActiveGeneration: row.ActiveGeneration,
	}, nil
}

// ClearSyncJobDirty clears the dirty flag once the successor has been enqueued.
func (store *DiscoveryStore) ClearSyncJobDirty(ctx context.Context, tenantID, jobID uuid.UUID) error {
	if _, err := store.queries.ClearSyncJobDirty(ctx, generated.ClearSyncJobDirtyParams{TenantID: tenantID, ID: jobID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

func syncIdempotencyLockKey(input service.SyncJobInput) string {
	return "syncRepository:" + input.TenantID.String() + ":" + input.PrincipalType + ":" + input.PrincipalID.String() + ":" + input.IdempotencyKey.String()
}

func loadSyncReplay(ctx context.Context, queries *generated.Queries, input service.SyncJobInput) (service.JobAccepted, bool, error) {
	params := generated.GetSyncIdempotencyParams{
		TenantID: input.TenantID, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID, IdempotencyKey: input.IdempotencyKey,
	}
	row, err := queries.GetSyncIdempotency(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return service.JobAccepted{}, false, nil
	}
	if err != nil {
		return service.JobAccepted{}, false, normalizeError(err)
	}
	if !row.ExpiresAt.Valid || !row.ExpiresAt.Time.After(time.Now().UTC()) {
		if err := queries.DeleteSyncIdempotency(ctx, generated.DeleteSyncIdempotencyParams{
			TenantID: input.TenantID, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID, IdempotencyKey: input.IdempotencyKey,
		}); err != nil {
			return service.JobAccepted{}, false, normalizeError(err)
		}
		return service.JobAccepted{}, false, nil
	}
	if len(input.RequestHash) != 32 || string(row.RequestHash) != string(input.RequestHash) {
		return service.JobAccepted{}, false, service.ErrIdempotencyConflict
	}
	var replay syncReplay
	if err := json.Unmarshal(row.ResponseBody, &replay); err != nil || replay.JobID == uuid.Nil() {
		return service.JobAccepted{}, false, errors.New("invalid sync idempotency record")
	}
	return service.JobAccepted{JobID: replay.JobID, Deduplicated: replay.Deduplicated}, true, nil
}

type syncReplay struct {
	JobID       uuid.UUID `json:"jobId"`
	Deduplicated bool     `json:"deduplicated"`
}

func saveSyncReplay(ctx context.Context, queries *generated.Queries, input service.SyncJobInput, accepted service.JobAccepted) error {
	body, err := json.Marshal(syncReplay{JobID: accepted.JobID, Deduplicated: accepted.Deduplicated})
	if err != nil {
		return fmt.Errorf("encode sync replay: %w", err)
	}
	return normalizeError(queries.CreateSyncIdempotency(ctx, generated.CreateSyncIdempotencyParams{
		TenantID: input.TenantID, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID,
		IdempotencyKey: input.IdempotencyKey, RequestHash: append([]byte(nil), input.RequestHash...), ResponseBody: body,
	}))
}
