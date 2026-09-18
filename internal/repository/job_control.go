package repository

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/task"
	"github.com/riverqueue/river"
)

// JobControlStore 组合任务查询与事务感知的 River 控制。
type JobControlStore struct {
	*RepositoryStore
	riverClient *river.Client[pgx.Tx]
}

// NewJobControlStore 将租户任务控制绑定到 PostgreSQL 与进程 River 客户端。
func NewJobControlStore(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *JobControlStore {
	return &JobControlStore{RepositoryStore: NewRepositoryStore(pool), riverClient: riverClient}
}

// ListTenantJobs 返回一页租户任务，并以一次批量查询挂接全部尝试日志。
func (store *JobControlStore) ListTenantJobs(ctx context.Context, tenantID uuid.UUID, filter service.JobFilter, limit, offset int32) ([]service.JobRecord, int64, error) {
	query := generated.CountTenantJobsParams{
		TenantID: tenantID, TypeFilter: filter.Types, StatusFilter: filter.Statuses,
		ScopeType: filter.ScopeType, ScopeID: filter.ScopeID,
	}
	total, err := store.queries.CountTenantJobs(ctx, query)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListTenantJobs(ctx, generated.ListTenantJobsParams{
		TenantID: tenantID, TypeFilter: filter.Types, StatusFilter: filter.Statuses,
		ScopeType: filter.ScopeType, ScopeID: filter.ScopeID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.JobRecord, len(rows))
	jobIDs := make([]uuid.UUID, len(rows))
	for index, row := range rows {
		items[index], err = tenantJobFromRow(row)
		if err != nil {
			return nil, 0, err
		}
		jobIDs[index] = row.ID
	}
	if len(jobIDs) == 0 {
		return items, total, nil
	}
	logs, err := store.queries.ListTenantJobAttemptLogs(ctx, generated.ListTenantJobAttemptLogsParams{TenantID: tenantID, JobIds: jobIDs})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	attachJobAttempts(items, logs)
	return items, total, nil
}

// GetTenantJob 返回一个租户任务及其完整持久化的尝试历史。
func (store *JobControlStore) GetTenantJob(ctx context.Context, tenantID, id uuid.UUID) (service.JobRecord, error) {
	row, err := store.queries.GetTenantJob(ctx, generated.GetTenantJobParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.JobRecord{}, normalizeError(err)
	}
	item, err := tenantJobFromRow(row)
	if err != nil {
		return service.JobRecord{}, err
	}
	logs, err := store.queries.ListTenantJobAttemptLogs(ctx, generated.ListTenantJobAttemptLogsParams{TenantID: tenantID, JobIds: []uuid.UUID{id}})
	if err != nil {
		return service.JobRecord{}, normalizeError(err)
	}
	items := []service.JobRecord{item}
	attachJobAttempts(items, logs)
	return items[0], nil
}

// GetTenantJobStreamState 返回一份轻量状态及其事务内观测到的日志游标。
func (store *JobControlStore) GetTenantJobStreamState(ctx context.Context, tenantID, id uuid.UUID) (service.JobRecord, int64, error) {
	row, err := store.queries.GetTenantJobStreamState(ctx, generated.GetTenantJobStreamStateParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.JobRecord{}, 0, normalizeError(err)
	}
	item, err := tenantJobFromRow(generated.Job{
		TenantID: row.TenantID, ID: row.ID, RetryOfJobID: row.RetryOfJobID, RiverJobID: row.RiverJobID,
		Type: row.Type, ScopeType: row.ScopeType, ScopeID: row.ScopeID, RefType: row.RefType, RefName: row.RefName,
		Trigger: row.Trigger, Input: row.Input, Result: row.Result, Status: row.Status, Stage: row.Stage,
		Attempt: row.Attempt, MaxAttempts: row.MaxAttempts, NextAttemptAt: row.NextAttemptAt,
		DedupeKey: row.DedupeKey, ActiveGeneration: row.ActiveGeneration, Dirty: row.Dirty, ReplaySafe: row.ReplaySafe,
		Error: row.Error, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	})
	if err != nil {
		return service.JobRecord{}, 0, err
	}
	return item, row.LogCursor, nil
}

// ListTenantJobLogsAfter 返回一批按序列号有序的、有界的回放日志。
func (store *JobControlStore) ListTenantJobLogsAfter(ctx context.Context, tenantID, jobID uuid.UUID, afterSequence int64, limit int32) ([]service.JobLogRecord, error) {
	rows, err := store.queries.ListTenantJobLogsAfter(ctx, generated.ListTenantJobLogsAfterParams{
		TenantID: tenantID, JobID: jobID, AfterSequence: afterSequence, EventLimit: limit,
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.JobLogRecord, len(rows))
	for index, row := range rows {
		items[index] = service.JobLogRecord{
			Sequence: row.Sequence, Stage: row.Stage, Message: row.Message, OccurredAt: row.OccurredAt.Time,
		}
	}
	return items, nil
}

// CancelTenantJob 原子地记录领域终态并取消其 River 行。
func (store *JobControlStore) CancelTenantJob(ctx context.Context, tenantID, id uuid.UUID, cancelledAt time.Time) (service.JobAccepted, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("begin cancel job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	job, err := queries.LockTenantJobForControl(ctx, generated.LockTenantJobForControlParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	if job.Status != service.JobStatusPending && job.Status != service.JobStatusRunning {
		return service.JobAccepted{}, service.ErrJobNotCancellable
	}
	if job.RiverJobID != nil {
		if store.riverClient == nil {
			return service.JobAccepted{}, errors.New("job control has no River client")
		}
		if _, err := store.riverClient.JobCancelTx(ctx, tx, *job.RiverJobID); err != nil {
			return service.JobAccepted{}, fmt.Errorf("cancel River job: %w", err)
		}
	}
	if _, err := queries.CancelTenantJob(ctx, generated.CancelTenantJobParams{
		TenantID: tenantID, ID: id, FinishedAt: timestamp(cancelledAt),
	}); err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	return service.JobAccepted{JobID: id, Deduplicated: false}, nil
}

// RetryTenantJob 原子地创建并入队一个独立的幂等重试代次。
func (store *JobControlStore) RetryTenantJob(ctx context.Context, request service.RetryJobRequest) (service.JobAccepted, error) {
	if len(request.RequestHash) != sha256DigestBytes || request.PrincipalType == "" || request.PrincipalID == uuid.Nil() || request.IdempotencyKey == uuid.Nil() {
		return service.JobAccepted{}, service.ErrValidation
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("begin retry job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	if err := queries.LockRetryJobIdempotency(ctx, retryJobLockKey(request)); err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	if replay, found, err := loadRetryJobReplay(ctx, queries, request); err != nil {
		return service.JobAccepted{}, err
	} else if found {
		return replay, nil
	}
	source, err := queries.LockTenantJobForControl(ctx, generated.LockTenantJobForControlParams{TenantID: request.TenantID, ID: request.SourceJobID})
	if err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	if (source.Status != service.JobStatusFailed && source.Status != service.JobStatusCancelled) || source.Type != jobTypeRepoSync || source.ScopeID == nil || source.RefName == nil {
		return service.JobAccepted{}, service.ErrJobNotRetryable
	}
	latest, err := queries.LockLatestTenantJobGeneration(ctx, generated.LockLatestTenantJobGenerationParams{TenantID: request.TenantID, DedupeKey: source.DedupeKey})
	if err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	if latest.ID != source.ID && (latest.Status == service.JobStatusPending || latest.Status == service.JobStatusRunning) {
		return service.JobAccepted{}, service.ErrJobNotRetryable
	}
	jobID := uuid.NewV7()
	created, err := queries.CreateRetriedTenantJob(ctx, generated.CreateRetriedTenantJobParams{
		NewJobID: jobID, ActiveGeneration: latest.ActiveGeneration + 1,
		TenantID: request.TenantID, SourceJobID: source.ID,
	})
	if err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	if store.riverClient == nil {
		return service.JobAccepted{}, errors.New("job control has no River client")
	}
	inserted, err := store.riverClient.InsertTx(ctx, tx, task.CredentialSyncArgs{
		TenantID: request.TenantID, JobID: created.ID, RepositoryID: *created.ScopeID, RefName: *created.RefName,
	}, &river.InsertOpts{MaxAttempts: int(created.MaxAttempts)})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("insert retried River job: %w", err)
	}
	changed, err := queries.AttachRiverJobID(ctx, generated.AttachRiverJobIDParams{
		TenantID: request.TenantID, ID: created.ID, RiverJobID: new(inserted.Job.ID), UpdatedAt: timestamp(request.RequestedAt),
	})
	if err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.JobAccepted{}, fmt.Errorf("attach retried River job: %w", service.ErrPrecondition)
	}
	accepted := service.JobAccepted{JobID: created.ID, Deduplicated: false}
	if err := saveRetryJobReplay(ctx, queries, request, accepted); err != nil {
		return service.JobAccepted{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	return accepted, nil
}

type retryJobReplay struct {
	// JobID 标识原请求返回的确切任务。
	JobID uuid.UUID `json:"jobId"`
	// Deduplicated 保留原响应字段的精确值。
	Deduplicated bool `json:"deduplicated"`
}

func loadRetryJobReplay(ctx context.Context, queries *generated.Queries, request service.RetryJobRequest) (service.JobAccepted, bool, error) {
	params := generated.GetRetryJobIdempotencyParams{
		TenantID: request.TenantID, PrincipalType: request.PrincipalType, PrincipalID: request.PrincipalID, IdempotencyKey: request.IdempotencyKey,
	}
	row, err := queries.GetRetryJobIdempotency(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return service.JobAccepted{}, false, nil
	}
	if err != nil {
		return service.JobAccepted{}, false, normalizeError(err)
	}
	if !row.ExpiresAt.Valid || !row.ExpiresAt.Time.After(request.RequestedAt) {
		if err := queries.DeleteRetryJobIdempotency(ctx, generated.DeleteRetryJobIdempotencyParams{
			TenantID: request.TenantID, PrincipalType: request.PrincipalType, PrincipalID: request.PrincipalID, IdempotencyKey: request.IdempotencyKey,
		}); err != nil {
			return service.JobAccepted{}, false, normalizeError(err)
		}
		return service.JobAccepted{}, false, nil
	}
	if !bytes.Equal(row.RequestHash, request.RequestHash) {
		return service.JobAccepted{}, false, service.ErrIdempotencyConflict
	}
	var replay retryJobReplay
	if err := json.Unmarshal(row.ResponseBody, &replay); err != nil || replay.JobID == uuid.Nil() {
		return service.JobAccepted{}, false, errors.New("invalid retry job idempotency record")
	}
	return service.JobAccepted{JobID: replay.JobID, Deduplicated: replay.Deduplicated}, true, nil
}

func saveRetryJobReplay(ctx context.Context, queries *generated.Queries, request service.RetryJobRequest, accepted service.JobAccepted) error {
	body, err := json.Marshal(retryJobReplay{JobID: accepted.JobID, Deduplicated: accepted.Deduplicated})
	if err != nil {
		return fmt.Errorf("encode retry job replay: %w", err)
	}
	return normalizeError(queries.CreateRetryJobIdempotency(ctx, generated.CreateRetryJobIdempotencyParams{
		TenantID: request.TenantID, PrincipalType: request.PrincipalType, PrincipalID: request.PrincipalID,
		IdempotencyKey: request.IdempotencyKey, RequestHash: bytes.Clone(request.RequestHash), ResponseBody: body,
	}))
}

func retryJobLockKey(request service.RetryJobRequest) string {
	return retryJobLockPrefix + request.TenantID.String() + ":" + request.PrincipalType + ":" + request.PrincipalID.String() + ":" + request.IdempotencyKey.String()
}

func tenantJobFromRow(row generated.Job) (service.JobRecord, error) {
	result, err := decodeJobResult(row.Result)
	if err != nil {
		return service.JobRecord{}, err
	}
	jobError, err := decodeJobError(row.ID, row.Error)
	if err != nil {
		return service.JobRecord{}, err
	}
	return service.JobRecord{
		ID: row.ID, RetryOfJobID: row.RetryOfJobID, Type: row.Type, Trigger: row.Trigger,
		Status: row.Status, Stage: row.Stage, ScopeType: row.ScopeType, ScopeID: uuidString(row.ScopeID),
		RefType: row.RefType, Ref: row.RefName, Result: result, Dirty: row.Dirty,
		Attempt: int(row.Attempt), MaxAttempts: int(row.MaxAttempts), NextAttemptAt: timePointer(row.NextAttemptAt),
		Attempts: []service.JobStageAttempt{}, Error: jobError, CreatedAt: row.CreatedAt.Time,
		StartedAt: timePointer(row.StartedAt), FinishedAt: timePointer(row.FinishedAt), UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func decodeJobResult(payload []byte) (map[string]any, error) {
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return nil, nil
	}
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, fmt.Errorf("decode job result: %w", err)
	}
	return result, nil
}

func decodeJobError(jobID uuid.UUID, payload []byte) (*service.JobError, error) {
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return nil, nil
	}
	var stored struct {
		// Code 是持久化的稳定失败分类。
		Code string `json:"code"`
		// Message 是可选的预脱敏说明。
		Message string `json:"message"`
		// RequestID 是可选的来源请求关联标识。
		RequestID string `json:"requestId"`
		// Details 包含可选的预脱敏结构化诊断信息。
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, fmt.Errorf("decode job error: %w", err)
	}
	if stored.Code == "" || stored.Code == jobErrorCodeWorkerFailed {
		stored.Code = service.ErrorCodeInternal
	}
	if stored.Message == "" {
		stored.Message = jobErrorDefaultMessage
	}
	if stored.RequestID == "" {
		stored.RequestID = jobID.String()
	}
	return &service.JobError{Code: stored.Code, Message: stored.Message, RequestID: stored.RequestID, Details: stored.Details}, nil
}

func uuidString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	return new(value.String())
}

type jobAttemptBuilder struct {
	attempt service.JobStageAttempt
	lastAt  time.Time
	failed  bool
}

func attachJobAttempts(items []service.JobRecord, logs []generated.JobStageLog) {
	indices := make(map[uuid.UUID]int, len(items))
	builders := make(map[uuid.UUID][]jobAttemptBuilder, len(items))
	for index, item := range items {
		indices[item.ID] = index
	}
	for _, event := range logs {
		if event.Stage == nil {
			continue
		}
		attempts := builders[event.JobID]
		if len(attempts) == 0 || attempts[len(attempts)-1].attempt.Attempt != int(event.Attempt) || attempts[len(attempts)-1].attempt.Stage != *event.Stage {
			startedAt := event.OccurredAt.Time
			attempts = append(attempts, jobAttemptBuilder{attempt: service.JobStageAttempt{
				Stage: *event.Stage, Attempt: int(event.Attempt), Status: service.JobStatusRunning, StartedAt: new(startedAt),
			}, lastAt: startedAt})
		}
		current := &attempts[len(attempts)-1]
		current.lastAt = event.OccurredAt.Time
		if event.Level == jobStageLevelError {
			current.failed = true
			current.attempt.Status = service.JobStatusFailed
			current.attempt.FinishedAt = new(event.OccurredAt.Time)
		}
		builders[event.JobID] = attempts
	}
	for jobID, attempts := range builders {
		index, ok := indices[jobID]
		if !ok {
			continue
		}
		job := &items[index]
		for attemptIndex := range attempts {
			current := &attempts[attemptIndex]
			isCurrent := current.attempt.Attempt == job.Attempt && job.Stage != nil && current.attempt.Stage == *job.Stage
			if current.failed {
				current.attempt.Error = job.Error
				if current.attempt.Error == nil {
					current.attempt.Error = &service.JobError{Code: service.ErrorCodeInternal, Message: jobStageFailedMessage, RequestID: job.ID.String()}
				}
			} else if isCurrent && job.Status == service.JobStatusRunning {
				current.attempt.Status = service.JobStatusRunning
			} else if isCurrent && terminalJobRowStatus(job.Status) {
				current.attempt.Status = job.Status
				current.attempt.FinishedAt = job.FinishedAt
			} else {
				current.attempt.Status = service.JobStatusSucceeded
				if attemptIndex+1 < len(attempts) && attempts[attemptIndex+1].attempt.Attempt == current.attempt.Attempt {
					current.attempt.FinishedAt = attempts[attemptIndex+1].attempt.StartedAt
				} else {
					current.attempt.FinishedAt = new(current.lastAt)
				}
			}
			job.Attempts = append(job.Attempts, current.attempt)
		}
	}
}

func terminalJobRowStatus(status string) bool {
	return status == service.JobStatusSucceeded || status == service.JobStatusSucceededWithWarnings || status == service.JobStatusFailed || status == service.JobStatusOutcomeUnknown || status == service.JobStatusCancelled
}

var _ service.JobStore = (*JobControlStore)(nil)
