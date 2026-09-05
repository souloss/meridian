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

// StartJob claims a pending or running domain job and records its first stage event atomically.
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
	if err := appendJobStageLog(ctx, queries, input.TenantID, input.JobID, input.Stage, "info", "pipeline stage started", input.StartedAt); err != nil {
		return task.ClaimResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return task.ClaimResult{}, normalizeError(err)
	}
	return task.ClaimResult{Claimed: row.ID == input.JobID, Attempt: int(row.Attempt)}, nil
}

// SetJobStage updates the active stage and appends one ordered, replayable log event atomically.
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
	if changed != 1 {
		return serviceErrNotFound()
	}
	if err := appendJobStageLog(ctx, queries, input.TenantID, input.JobID, input.Stage, input.Level, input.Message, input.OccurredAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	return nil
}

// FinishJob persists a terminal or retryable state and its final stage log atomically.
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
	if err := appendJobStageLog(ctx, queries, input.TenantID, input.JobID, input.Stage, input.Level, input.Message, input.FinishedAt); err != nil {
		return err
	}
	if input.Terminal && input.Status == "failed" {
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
	// Stage is the pipeline stage that produced the failure.
	Stage string `json:"stage"`
	// ErrorCode is the stable secret-free failure classification.
	ErrorCode string `json:"errorCode"`
}

type collectFailedPayload struct {
	// RepositoryID identifies the repository whose collection failed.
	RepositoryID uuid.UUID `json:"repositoryId"`
	// JobID identifies the durable job that reached a failed terminal state.
	JobID uuid.UUID `json:"jobId"`
	// Stage identifies the pipeline stage that failed.
	Stage string `json:"stage"`
	// ErrorCode is the stable secret-free failure classification.
	ErrorCode string `json:"errorCode"`
}

type collectFailedEnvelope struct {
	// EventID is the stable receiver deduplication identifier.
	EventID uuid.UUID `json:"eventId"`
	// EventType identifies the AsyncAPI message schema.
	EventType string `json:"eventType"`
	// OccurredAt is the UTC instant when the job failure transaction committed.
	OccurredAt time.Time `json:"occurredAt"`
	// TenantSlug identifies the tenant without exposing an internal lookup key.
	TenantSlug string `json:"tenantSlug"`
	// AggregateType identifies the event-producing domain aggregate category.
	AggregateType string `json:"aggregateType"`
	// AggregateID identifies the job aggregate that emitted the event.
	AggregateID uuid.UUID `json:"aggregateId"`
	// AggregateVersion orders failure events for the job aggregate.
	AggregateVersion int `json:"aggregateVersion"`
	// Payload contains collect.failed fields frozen by the AsyncAPI contract.
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
		EventID: eventID, EventType: "collect.failed", OccurredAt: input.FinishedAt.UTC(),
		TenantSlug: tenantSlug, AggregateType: "job", AggregateID: input.JobID,
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
			TenantID: input.TenantID, ID: uuid.NewV7(), EventID: eventID, EventType: "collect.failed",
			AggregateID: input.JobID, AggregateVersion: int64(input.ExpectedAttempt), Payload: payload,
			ChannelID: channelID,
		}); err != nil {
			return normalizeError(err)
		}
	}
	return nil
}

func appendJobStageLog(ctx context.Context, queries *generated.Queries, tenantID, jobID uuid.UUID, stage task.Stage, level, message string, occurredAt time.Time) error {
	lockKey := "job-stage:" + tenantID.String() + ":" + jobID.String()
	if err := queries.LockJobStageSequence(ctx, lockKey); err != nil {
		return normalizeError(err)
	}
	sequence, err := queries.NextJobStageSequence(ctx, generated.NextJobStageSequenceParams{TenantID: tenantID, JobID: jobID})
	if err != nil {
		return normalizeError(err)
	}
	if _, err := queries.AppendJobStageLog(ctx, generated.AppendJobStageLogParams{
		TenantID: tenantID, JobID: jobID, Sequence: int64(sequence), Stage: string(stage),
		Level: level, Message: message, OccurredAt: timestamp(occurredAt),
	}); err != nil {
		return normalizeError(err)
	}
	return nil
}

func serviceErrNotFound() error {
	return fmt.Errorf("job execution row not found: %w", service.ErrNotFound)
}
