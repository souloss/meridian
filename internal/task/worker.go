package task

import (
	"context"
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/riverqueue/river"
)

// ErrRepositorySyncUnavailable marks the M0 boundary before the Git producer is implemented.
var ErrRepositorySyncUnavailable = errors.New("repository synchronization producer is unavailable")

// UnsupportedSyncRunner makes the M0 boundary explicit instead of reporting a
// successful synchronization without a producer, checkout, or completion manifest.
type UnsupportedSyncRunner struct{}

// Run returns a stable, non-secret error until the M1 repository producer exists.
func (UnsupportedSyncRunner) Run(context.Context, CredentialSyncArgs) error {
	return ErrRepositorySyncUnavailable
}

// CredentialSyncWorker advances durable Meridian state around one River attempt.
type CredentialSyncWorker struct {
	river.WorkerDefaults[CredentialSyncArgs]
	store  ExecutionStore
	runner SyncRunner
	now    func() time.Time
}

// NewCredentialSyncWorker constructs the M0 worker with explicit persistence and producer dependencies.
func NewCredentialSyncWorker(store ExecutionStore, runner SyncRunner) *CredentialSyncWorker {
	if runner == nil {
		runner = UnsupportedSyncRunner{}
	}
	return &CredentialSyncWorker{store: store, runner: runner, now: time.Now}
}

// Work claims the durable job, records stage transitions, and finishes it with
// a redacted result or error. A terminal domain row is treated as an idempotent
// River success so a replay cannot mutate it again.
func (worker *CredentialSyncWorker) Work(ctx context.Context, job *river.Job[CredentialSyncArgs]) error {
	if worker.store == nil {
		return errors.New("credential sync worker has no execution store")
	}
	startedAt := worker.now().UTC()
	claim, err := worker.store.StartJob(ctx, StartInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageResolve,
		ExpectedAttempt: job.Attempt, StartedAt: startedAt,
	})
	if err != nil {
		return err
	}
	if !claim.Claimed {
		return nil
	}

	if err := worker.runner.Run(ctx, job.Args); err != nil {
		return worker.finishFailure(ctx, job.Args, job.Attempt, err)
	}
	for _, stage := range [...]Stage{StageDiscover, StageExtract, StageMerge, StageNormalize, StageIndex} {
		if err := worker.store.SetJobStage(ctx, StageInput{
			TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: stage,
			ExpectedAttempt: job.Attempt,
			Level:           "info", Message: "pipeline stage completed", OccurredAt: worker.now().UTC(),
		}); err != nil {
			return err
		}
	}
	result, err := json.Marshal(struct {
		RepositoryID string `json:"repositoryId"`
		RefName      string `json:"refName"`
	}{RepositoryID: job.Args.RepositoryID.String(), RefName: job.Args.RefName})
	if err != nil {
		return err
	}
	return worker.store.FinishJob(ctx, FinishInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Status: "succeeded", Result: result,
		ExpectedAttempt: job.Attempt,
		Stage:           StageIndex, Level: "info", Message: "repository synchronization completed",
		Terminal: true, FinishedAt: worker.now().UTC(),
	})
}

func (worker *CredentialSyncWorker) finishFailure(ctx context.Context, args CredentialSyncArgs, expectedAttempt int, cause error) error {
	message := "repository synchronization failed"
	if errors.Is(cause, ErrRepositorySyncUnavailable) {
		message = "repository synchronization producer is unavailable"
	}
	errorPayload, err := json.Marshal(struct {
		Code string `json:"code"`
	}{Code: "internal_error"})
	if err != nil {
		return err
	}
	finishErr := worker.store.FinishJob(ctx, FinishInput{
		TenantID: args.TenantID, JobID: args.JobID, RepositoryID: args.RepositoryID,
		Status: "failed", Error: errorPayload, ErrorCode: "internal_error",
		ExpectedAttempt: expectedAttempt,
		Stage:           StageResolve, Level: "error", Message: message, Terminal: true, FinishedAt: worker.now().UTC(),
	})
	if finishErr != nil {
		return errors.Join(cause, finishErr)
	}
	return nil
}
