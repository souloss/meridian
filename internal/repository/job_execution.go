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
)

// StartJob 认领一个待执行或执行中的领域任务，并原子地记录其首个阶段事件。
func (store *RepositoryStore) StartJob(ctx context.Context, input task.StartInput) (task.ClaimResult, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return task.ClaimResult{}, fmt.Errorf("begin start job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	row, err := queries.StartJobExecution(ctx, generated.StartJobExecutionParams{
		TenantID: input.TenantID, ID: input.JobID, Stage: string(input.Stage),
		ExpectedAttempt: int32(input.ExpectedAttempt),
		StartedAt:       timestamp(input.StartedAt), UpdatedAt: timestamp(input.StartedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return task.ClaimResult{}, nil
		}
		return task.ClaimResult{}, normalizeError(err)
	}
	if err := appendJobStageLog(ctx, queries, input.TenantID, input.JobID, input.ExpectedAttempt, input.Stage, jobStageLevelInfo, jobStageLogMessageStarted, input.StartedAt); err != nil {
		return task.ClaimResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return task.ClaimResult{}, normalizeError(err)
	}
	return task.ClaimResult{Claimed: row.ID == input.JobID, Attempt: int(row.Attempt)}, nil
}

// SetJobStage 更新活跃阶段，并原子地追加一条有序、可回放的日志事件。
func (store *RepositoryStore) SetJobStage(ctx context.Context, input task.StageInput) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin set job stage transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	changed, err := queries.SetJobExecutionStage(ctx, generated.SetJobExecutionStageParams{
		TenantID: input.TenantID, ID: input.JobID, Stage: string(input.Stage), UpdatedAt: timestamp(input.OccurredAt),
		ExpectedAttempt: int32(input.ExpectedAttempt),
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return serviceErrNotFound()
	}
	if err := appendJobStageLog(ctx, queries, input.TenantID, input.JobID, input.ExpectedAttempt, input.Stage, input.Level, input.Message, input.OccurredAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	return nil
}

// FinishJob 原子地持久化终态或可重试状态及其最终阶段日志。
func (store *RepositoryStore) FinishJob(ctx context.Context, input task.FinishInput) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin finish job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	row, err := queries.FinishJobExecution(ctx, generated.FinishJobExecutionParams{
		TenantID: input.TenantID, ID: input.JobID, Status: input.Status,
		ExpectedAttempt: int32(input.ExpectedAttempt),
		Result:          input.Result, Error: input.Error, Terminal: input.Terminal,
		FinishedAt: timestamp(input.FinishedAt), UpdatedAt: timestamp(input.FinishedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return serviceErrNotFound()
		}
		return normalizeError(err)
	}
	if err := appendJobStageLog(ctx, queries, input.TenantID, input.JobID, input.ExpectedAttempt, input.Stage, input.Level, input.Message, input.FinishedAt); err != nil {
		return err
	}
	if input.Terminal && input.Status == service.JobStatusFailed {
		if err := appendJobFailureFacts(ctx, queries, input); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	_ = row
	return nil
}

type jobFailureAuditDetail struct {
	// Stage 是产生失败的流水线阶段。
	Stage string `json:"stage"`
	// ErrorCode 是稳定的、不含机密的失败分类。
	ErrorCode string `json:"errorCode"`
}

type collectFailedPayload struct {
	// RepositoryID 标识采集失败的仓库。
	RepositoryID uuid.UUID `json:"repositoryId"`
	// JobID 标识进入失败终态的持久化任务。
	JobID uuid.UUID `json:"jobId"`
	// Stage 标识失败的流水线阶段。
	Stage string `json:"stage"`
	// ErrorCode 是稳定的、不含机密的失败分类。
	ErrorCode string `json:"errorCode"`
}

type collectFailedEnvelope struct {
	// EventID 是接收方稳定的去重标识。
	EventID uuid.UUID `json:"eventId"`
	// EventType 标识 AsyncAPI 消息 schema。
	EventType string `json:"eventType"`
	// OccurredAt 是任务失败事务提交的 UTC 时刻。
	OccurredAt time.Time `json:"occurredAt"`
	// TenantSlug 标识租户，且不暴露内部查询键。
	TenantSlug string `json:"tenantSlug"`
	// AggregateType 标识产生事件的领域聚合类别。
	AggregateType string `json:"aggregateType"`
	// AggregateID 标识发出事件的任务聚合。
	AggregateID uuid.UUID `json:"aggregateId"`
	// AggregateVersion 对任务聚合的失败事件进行排序。
	AggregateVersion int `json:"aggregateVersion"`
	// Payload 承载由 AsyncAPI 契约冻结的 collect.failed 字段。
	Payload collectFailedPayload `json:"payload"`
}

func appendJobFailureFacts(ctx context.Context, queries *generated.Queries, input task.FinishInput) error {
	auditDetail, err := json.Marshal(jobFailureAuditDetail{Stage: string(input.Stage), ErrorCode: input.ErrorCode})
	if err != nil {
		return fmt.Errorf("encode job failure audit metadata: %w", err)
	}
	if _, err := queries.AppendJobFailureAudit(ctx, generated.AppendJobFailureAuditParams{
		ID: uuid.NewV7(), TenantID: new(input.TenantID), JobID: new(input.JobID),
		Detail: auditDetail,
	}); err != nil {
		return normalizeError(err)
	}
	channelIDs, err := queries.ListEnabledNotificationChannelIDs(ctx, input.TenantID)
	if err != nil {
		return normalizeError(err)
	}
	if len(channelIDs) == 0 {
		return nil
	}
	tenantSlug, err := queries.GetTenantSlugForEvent(ctx, input.TenantID)
	if err != nil {
		return normalizeError(err)
	}
	eventID := uuid.NewV7()
	payload, err := json.Marshal(collectFailedEnvelope{
		EventID: eventID, EventType: service.DomainEventCollectFailed, OccurredAt: input.FinishedAt.UTC(),
		TenantSlug: tenantSlug, AggregateType: collectFailedAggregateType, AggregateID: input.JobID,
		AggregateVersion: input.ExpectedAttempt,
		Payload: collectFailedPayload{
			RepositoryID: input.RepositoryID, JobID: input.JobID, Stage: string(input.Stage), ErrorCode: input.ErrorCode,
		},
	})
	if err != nil {
		return fmt.Errorf("encode collect.failed event envelope: %w", err)
	}
	for _, channelID := range channelIDs {
		if _, err := queries.CreateNotifyOutbox(ctx, generated.CreateNotifyOutboxParams{
			TenantID: input.TenantID, ID: uuid.NewV7(), EventID: eventID, EventType: service.DomainEventCollectFailed,
			AggregateID: input.JobID, AggregateVersion: int64(input.ExpectedAttempt), Payload: payload,
			ChannelID: channelID,
		}); err != nil {
			return normalizeError(err)
		}
	}
	return nil
}

func appendJobStageLog(ctx context.Context, queries *generated.Queries, tenantID, jobID uuid.UUID, attempt int, stage task.Stage, level, message string, occurredAt time.Time) error {
	lockKey := jobStageSequenceLockPrefix + tenantID.String() + ":" + jobID.String()
	if err := queries.LockJobStageSequence(ctx, lockKey); err != nil {
		return normalizeError(err)
	}
	sequence, err := queries.NextJobStageSequence(ctx, generated.NextJobStageSequenceParams{TenantID: tenantID, JobID: jobID})
	if err != nil {
		return normalizeError(err)
	}
	if _, err := queries.AppendJobStageLog(ctx, generated.AppendJobStageLogParams{
		TenantID: tenantID, JobID: jobID, Sequence: int64(sequence), Attempt: int32(attempt), Stage: string(stage),
		Level: level, Message: message, OccurredAt: timestamp(occurredAt),
	}); err != nil {
		return normalizeError(err)
	}
	return nil
}

func serviceErrNotFound() error {
	return fmt.Errorf("job execution row not found: %w", service.ErrNotFound)
}
