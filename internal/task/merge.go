// Package task contains River job arguments and execution orchestration.
package task

import (
	"context"
	"encoding/json/v2"
	"errors"
	"time"
	"uuid"

	"github.com/riverqueue/river"
)

// MergeArgs is the durable, non-secret argument carried by an asset.merge job.
type MergeArgs struct {
	// TenantID identifies the tenant boundary for every execution query.
	TenantID uuid.UUID `json:"tenantId"`
	// JobID identifies the application-owned durable job row.
	JobID uuid.UUID `json:"jobId"`
	// TrackID identifies the asset ref track whose layers are re-merged.
	TrackID uuid.UUID `json:"trackId"`
}

// Kind returns the stable River kind name persisted in the River schema.
func (MergeArgs) Kind() string { return "meridian_asset_merge" }

// MergeResult carries the materialized version for a completed merge.
type MergeResult struct {
	// VersionID identifies the new or reused version, when one was produced.
	VersionID uuid.UUID `json:"versionId,omitempty"`
	// Noop reports whether the merge reused an identical existing version.
	Noop bool `json:"noop"`
}

// MergeRunner re-merges one track's effective layers into a version.
type MergeRunner interface {
	Run(context.Context, MergeArgs) (MergeResult, error)
}

// MergeWorker advances durable Meridian state around one asset.merge attempt.
type MergeWorker struct {
	river.WorkerDefaults[MergeArgs]
	store  ExecutionStore
	runner MergeRunner
	now    func() time.Time
}

// NewMergeWorker constructs the M2 asset.merge worker.
func NewMergeWorker(store ExecutionStore, runner MergeRunner) *MergeWorker {
	return &MergeWorker{store: store, runner: runner, now: time.Now}
}

// Work claims the durable job, runs the merge, and finishes it with a redacted
// result or error.
func (worker *MergeWorker) Work(ctx context.Context, job *river.Job[MergeArgs]) error {
	if worker.store == nil {
		return errors.New("merge worker has no execution store")
	}
	if worker.runner == nil {
		return errors.New("merge worker has no runner")
	}
	startedAt := worker.now().UTC()
	claim, err := worker.store.StartJob(ctx, StartInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageMerge,
		ExpectedAttempt: job.Attempt, StartedAt: startedAt,
	})
	if err != nil {
		return err
	}
	if !claim.Claimed {
		return nil
	}
	result, err := worker.runner.Run(ctx, job.Args)
	if err != nil {
		return worker.finishFailure(ctx, job.Args, job.Attempt, err)
	}
	if err := worker.store.SetJobStage(ctx, StageInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageIndex,
		ExpectedAttempt: job.Attempt,
		Level:           "info", Message: "merge completed", OccurredAt: worker.now().UTC(),
	}); err != nil {
		return err
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return worker.store.FinishJob(ctx, FinishInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Status: "succeeded", Result: resultBytes,
		ExpectedAttempt: job.Attempt,
		Stage:           StageIndex, Level: "info", Message: "asset merge completed",
		Terminal: true, FinishedAt: worker.now().UTC(),
	})
}

func (worker *MergeWorker) finishFailure(ctx context.Context, args MergeArgs, expectedAttempt int, cause error) error {
	errorPayload, err := json.Marshal(struct {
		Code string `json:"code"`
	}{Code: "internal_error"})
	if err != nil {
		return err
	}
	finishErr := worker.store.FinishJob(ctx, FinishInput{
		TenantID: args.TenantID, JobID: args.JobID, Status: "failed", Error: errorPayload, ErrorCode: "internal_error",
		ExpectedAttempt: expectedAttempt,
		Stage:           StageMerge, Level: "error", Message: "asset merge failed", Terminal: true, FinishedAt: worker.now().UTC(),
	})
	if finishErr != nil {
		return errors.Join(cause, finishErr)
	}
	return nil
}
