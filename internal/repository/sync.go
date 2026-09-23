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

// EnqueueSyncJob 原子地记录一条 repo.sync 任务及其 River 工作。
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
	// 重放完全一致的已存响应；摘要不同则返回 409。
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
		generation := jobGenerationInitial
		if err == nil {
			if latest.Status == service.JobStatusPending || latest.Status == service.JobStatusRunning {
				// 任务运行期间的重复请求置位 dirty 标志，
				// 使一个后继在完成后处理最新输入。
				if latest.Status == service.JobStatusRunning {
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
				} else if changed != rowsAffectedOne {
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
	return dedupeKeyPrefixSync + repositoryID.String() + ":" + refType + ":" + refName
}

// MarkSyncJobDirty 将一条运行中的 repo.sync 任务标记为脏，使其完成后派生一个后继。
func (store *DiscoveryStore) MarkSyncJobDirty(ctx context.Context, tenantID uuid.UUID, dedupeKey string, activeGeneration int64) error {
	if _, err := store.queries.MarkSyncJobDirty(ctx, generated.MarkSyncJobDirtyParams{TenantID: tenantID, DedupeKey: dedupeKey, ActiveGeneration: activeGeneration}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// GetSyncJobForSuccessor 返回用于判断脏同步任务是否必须派生后继的完成状态。
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

// ClearSyncJobDirty 在后继已入队后清除脏标记。
func (store *DiscoveryStore) ClearSyncJobDirty(ctx context.Context, tenantID, jobID uuid.UUID) error {
	if _, err := store.queries.ClearSyncJobDirty(ctx, generated.ClearSyncJobDirtyParams{TenantID: tenantID, ID: jobID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

func syncIdempotencyLockKey(input service.SyncJobInput) string {
	return syncIdempotencyLockPrefix + input.TenantID.String() + ":" + input.PrincipalType + ":" + input.PrincipalID.String() + ":" + input.IdempotencyKey.String()
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
	if len(input.RequestHash) != sha256DigestBytes || string(row.RequestHash) != string(input.RequestHash) {
		return service.JobAccepted{}, false, service.ErrIdempotencyConflict
	}
	var replay syncReplay
	if err := json.Unmarshal(row.ResponseBody, &replay); err != nil || replay.JobID == uuid.Nil() {
		return service.JobAccepted{}, false, errors.New("invalid sync idempotency record")
	}
	return service.JobAccepted{JobID: replay.JobID, Deduplicated: replay.Deduplicated}, true, nil
}

type syncReplay struct {
	// JobID 是回放响应中的任务标识。
	JobID uuid.UUID `json:"jobId"`
	// Deduplicated 标识该响应来自去重回放。
	Deduplicated bool `json:"deduplicated"`
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

// EnqueueWebhookSyncJob 为入站 webhook 记录一条 repo.sync 任务（trigger=webhook）。
func (store *DiscoveryStore) EnqueueWebhookSyncJob(ctx context.Context, tenantID, repositoryID uuid.UUID, refType, refName string) (service.JobAccepted, error) {
	dedupeKey := syncDedupeKey(repositoryID, refType, refName)
	jobInput, err := json.Marshal(struct {
		Source string `json:"source,omitempty"`
	}{Source: "webhook"})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("encode webhook sync job input: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("begin webhook sync job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)

	for {
		latest, err := queries.LockLatestDiscoveryJob(ctx, generated.LockLatestDiscoveryJobParams{TenantID: tenantID, DedupeKey: dedupeKey})
		generation := jobGenerationInitial
		if err == nil {
			if latest.Status == service.JobStatusPending || latest.Status == service.JobStatusRunning {
				return service.JobAccepted{JobID: latest.ID, Deduplicated: true}, nil
			}
			generation = latest.ActiveGeneration + 1
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return service.JobAccepted{}, normalizeError(err)
		}

		jobID := uuid.NewV7()
		row, err := queries.CreateWebhookSyncJob(ctx, generated.CreateWebhookSyncJobParams{
			TenantID: tenantID, ID: jobID, RepositoryID: new(repositoryID),
			RefType: nullableString(refType), RefName: nullableString(refName), JobInput: jobInput,
			DedupeKey: dedupeKey, ActiveGeneration: generation,
		})
		if err == nil {
			if err := tx.Commit(ctx); err != nil {
				return service.JobAccepted{}, normalizeError(err)
			}
			return service.JobAccepted{JobID: row.ID, Deduplicated: false}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return service.JobAccepted{}, normalizeError(err)
		}
	}
}
