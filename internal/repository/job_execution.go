package repository

import (
	"context"
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
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	_ = row
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
