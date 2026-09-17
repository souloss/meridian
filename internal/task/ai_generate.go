package task

import (
	"context"
	"encoding/json/v2"
	"errors"
	"time"
	"uuid"

	"github.com/riverqueue/river"
)

// AiGenerateArgs is the durable, non-secret argument carried by an asset.ai_generate
// River job. The producer profile and tenant settings are resolved at execution
// time; no secret data ever enters the queue.
type AiGenerateArgs struct {
	// TenantID identifies the tenant boundary for every execution query.
	TenantID uuid.UUID `json:"tenantId"`
	// JobID identifies the application-owned durable job row.
	JobID uuid.UUID `json:"jobId"`
	// AssetID identifies the missing asset being generated.
	AssetID uuid.UUID `json:"assetId"`
}

// Kind returns the stable River kind name persisted in the River schema.
func (AiGenerateArgs) Kind() string { return "meridian_asset_ai_generate" }

// AiGenerateResult is the secret-free outcome of one AI generation run.
type AiGenerateResult struct {
	// Stage identifies the terminal pipeline stage (normalize on success, extract/merge on failure).
	Stage string `json:"stage"`
	// ErrorCode carries a stable failure classification; empty on success.
	ErrorCode string `json:"errorCode,omitempty"`
	// RevisionID identifies the created revision; empty when no revision was created.
	RevisionID uuid.UUID `json:"revisionId,omitempty"`
}

// AiGenerateRunner executes one AI generation after the durable job is claimed.
type AiGenerateRunner interface {
	RunAiGeneration(context.Context, AiGenerateArgs) (AiGenerateResult, error)
}

// AiGenerateWorker advances durable Meridian state around one asset.ai_generate attempt.
type AiGenerateWorker struct {
	river.WorkerDefaults[AiGenerateArgs]
	store  ExecutionStore
	runner AiGenerateRunner
	now    func() time.Time
}

// NewAiGenerateWorker constructs the M3 AI generation worker.
func NewAiGenerateWorker(store ExecutionStore, runner AiGenerateRunner) *AiGenerateWorker {
	return &AiGenerateWorker{store: store, runner: runner, now: time.Now}
}

// Work claims the durable job, runs the generation, and finishes it with a
// redacted result or a structured error carrying the failure stage and code.
func (worker *AiGenerateWorker) Work(ctx context.Context, job *river.Job[AiGenerateArgs]) error {
	if worker.store == nil {
		return errors.New("AI generation worker has no execution store")
	}
	if worker.runner == nil {
		return errors.New("AI generation worker has no runner")
	}
	startedAt := worker.now().UTC()
	claim, err := worker.store.StartJob(ctx, StartInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageExtract,
		ExpectedAttempt: job.Attempt, StartedAt: startedAt,
	})
	if err != nil {
		return err
	}
	if !claim.Claimed {
		return nil
	}
	result, err := worker.runner.RunAiGeneration(ctx, job.Args)
	if err != nil {
		return worker.finishFailure(ctx, job.Args, job.Attempt, Stage(result.Stage), result.ErrorCode, err)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return worker.store.FinishJob(ctx, FinishInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Status: "succeeded", Result: payload,
		ExpectedAttempt: job.Attempt, Stage: StageIndex, Level: "info",
		Message: "asset AI generation completed", Terminal: true, FinishedAt: worker.now().UTC(),
	})
}

// finishFailure persists a failed terminal state with the failure stage and a
// stable error code. A non-terminal error is deliberately re-raised so River
// retries it; here all outcomes are terminal because the runner persists its own
// outcome snapshot, so an error from the runner is a hard infrastructure failure.
func (worker *AiGenerateWorker) finishFailure(ctx context.Context, args AiGenerateArgs, expectedAttempt int, stage Stage, errorCode string, cause error) error {
	if errorCode == "" {
		errorCode = "internal_error"
	}
	errorPayload, err := json.Marshal(struct {
		Code  string `json:"code"`
		Stage string `json:"stage"`
	}{Code: errorCode, Stage: string(stage)})
	if err != nil {
		return err
	}
	finishErr := worker.store.FinishJob(ctx, FinishInput{
		TenantID: args.TenantID, JobID: args.JobID, Status: "failed", Error: errorPayload, ErrorCode: errorCode,
		ExpectedAttempt: expectedAttempt,
		Stage:           stage, Level: "error", Message: "asset AI generation failed", Terminal: true, FinishedAt: worker.now().UTC(),
	})
	if finishErr != nil {
		return errors.Join(cause, finishErr)
	}
	return nil
}
