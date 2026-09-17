package task

import (
	"context"
	"encoding/json/v2"
	"errors"
	"time"
	"uuid"

	"github.com/riverqueue/river"
)

// DiscoverArgs is the durable, non-secret argument carried by a repository
// discovery job. Credentials are deliberately absent: the runner resolves the
// current repository configuration at execution time and never receives secrets.
type DiscoverArgs struct {
	// TenantID identifies the tenant boundary for every execution query.
	TenantID uuid.UUID `json:"tenantId"`
	// JobID identifies the application-owned durable job row.
	JobID uuid.UUID `json:"jobId"`
	// RepositoryID identifies the repository that requested discovery.
	RepositoryID uuid.UUID `json:"repositoryId"`
	// RefType identifies the Git ref category (branch or tag).
	RefType string `json:"refType"`
	// RefName identifies the normalized Git ref to discover.
	RefName string `json:"refName"`
}

// Kind returns the stable River kind name persisted in the River schema.
func (DiscoverArgs) Kind() string { return "meridian_repo_discover" }

// DiscoverResult is the secret-free outcome of one repository discovery run.
type DiscoverResult struct {
	// ResolvedCommit is the commit SHA resolved at discovery time.
	ResolvedCommit string
	// CandidateCount is the number of candidate root directories persisted.
	CandidateCount int
}

// DiscoverRunner performs repository discovery after the durable job is claimed.
// The concrete implementation walks the checkout tree and upserts candidates.
type DiscoverRunner interface {
	RunDiscovery(context.Context, DiscoverArgs) (DiscoverResult, error)
}

// DiscoverWorker advances durable Meridian state around one repository
// discovery River attempt and persists the resolved commit in the job result.
type DiscoverWorker struct {
	river.WorkerDefaults[DiscoverArgs]
	store  ExecutionStore
	runner DiscoverRunner
	now    func() time.Time
}

// NewDiscoverWorker constructs the discovery worker with explicit persistence
// and runner dependencies.
func NewDiscoverWorker(store ExecutionStore, runner DiscoverRunner) *DiscoverWorker {
	return &DiscoverWorker{store: store, runner: runner, now: time.Now}
}

// Work claims the durable job, records the resolve and discover stage
// transitions, runs discovery, and finishes the job with a redacted result.
func (worker *DiscoverWorker) Work(ctx context.Context, job *river.Job[DiscoverArgs]) error {
	if worker.store == nil {
		return errors.New("discovery worker has no execution store")
	}
	if worker.runner == nil {
		return errors.New("discovery worker has no runner")
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
	if err := worker.store.SetJobStage(ctx, StageInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageDiscover,
		ExpectedAttempt: job.Attempt, Level: "info", Message: "repository discovery started", OccurredAt: worker.now().UTC(),
	}); err != nil {
		return err
	}
	result, err := worker.runner.RunDiscovery(ctx, job.Args)
	if err != nil {
		return worker.finishFailure(ctx, job.Args, job.Attempt, err)
	}
	payload, err := json.Marshal(struct {
		RepositoryID   string `json:"repositoryId"`
		RefType        string `json:"refType"`
		RefName        string `json:"refName"`
		ResolvedCommit string `json:"resolvedCommit"`
		CandidateCount int    `json:"candidateCount"`
	}{
		RepositoryID: job.Args.RepositoryID.String(), RefType: job.Args.RefType, RefName: job.Args.RefName,
		ResolvedCommit: result.ResolvedCommit, CandidateCount: result.CandidateCount,
	})
	if err != nil {
		return err
	}
	return worker.store.FinishJob(ctx, FinishInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Status: "succeeded", Result: payload,
		ExpectedAttempt: job.Attempt, Stage: StageDiscover, Level: "info",
		Message: "repository discovery completed", Terminal: true, FinishedAt: worker.now().UTC(),
	})
}

func (worker *DiscoverWorker) finishFailure(ctx context.Context, args DiscoverArgs, expectedAttempt int, cause error) error {
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
		Stage:           StageDiscover, Level: "error", Message: "repository discovery failed", Terminal: true, FinishedAt: worker.now().UTC(),
	})
	if finishErr != nil {
		return errors.Join(cause, finishErr)
	}
	return nil
}
