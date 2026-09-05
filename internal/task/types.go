// Package task contains River job arguments and execution orchestration.
package task

import (
	"context"
	"time"
	"uuid"
)

// Stage identifies one persisted repository synchronization pipeline stage.
type Stage string

const (
	// StageResolve resolves the repository, commit, ref, and source binding.
	StageResolve Stage = "resolve"
	// StageDiscover discovers candidate assets without changing effective configuration.
	StageDiscover Stage = "discover"
	// StageExtract extracts producer output into an isolated workspace.
	StageExtract Stage = "extract"
	// StageMerge merges effective layer heads into a deterministic input.
	StageMerge Stage = "merge"
	// StageNormalize canonicalizes the merged input and derives provenance.
	StageNormalize Stage = "normalize"
	// StageIndex writes the version, item index, and dependent outbox events.
	StageIndex Stage = "index"
)

// CredentialSyncArgs is the durable, non-secret argument carried by a River job.
// The credential identifier is deliberately absent: the worker resolves the
// current repository configuration at execution time and never receives secret data.
type CredentialSyncArgs struct {
	// TenantID identifies the tenant boundary for every execution query.
	TenantID uuid.UUID `json:"tenantId"`
	// JobID identifies the application-owned durable job row.
	JobID uuid.UUID `json:"jobId"`
	// RepositoryID identifies the repository that requested synchronization.
	RepositoryID uuid.UUID `json:"repositoryId"`
	// RefName identifies the normalized Git branch or tag to synchronize.
	RefName string `json:"refName"`
}

// Kind returns the stable River kind name persisted in the River schema.
func (CredentialSyncArgs) Kind() string { return "meridian_repo_sync" }

// StartInput describes the initial durable state transition for one job attempt.
type StartInput struct {
	// TenantID identifies the tenant that owns the job.
	TenantID uuid.UUID
	// JobID identifies the application-owned job row.
	JobID uuid.UUID
	// Stage is the first pipeline stage to expose.
	Stage Stage
	// ExpectedAttempt is the one-based River attempt claiming the domain row.
	ExpectedAttempt int
	// StartedAt is the UTC time used for the first attempt timestamp.
	StartedAt time.Time
}

// StageInput describes an active stage transition and its redacted message.
type StageInput struct {
	// TenantID identifies the tenant that owns the job.
	TenantID uuid.UUID
	// JobID identifies the application-owned job row.
	JobID uuid.UUID
	// Stage is the pipeline stage being entered.
	Stage Stage
	// ExpectedAttempt is the one-based River attempt that owns the transition.
	ExpectedAttempt int
	// Level is the structured log severity.
	Level string
	// Message is a secret-free diagnostic message.
	Message string
	// OccurredAt is the UTC time at which the event was recorded.
	OccurredAt time.Time
}

// FinishInput describes the terminal or retryable durable state transition.
type FinishInput struct {
	// TenantID identifies the tenant that owns the job.
	TenantID uuid.UUID
	// JobID identifies the application-owned job row.
	JobID uuid.UUID
	// Status is one of the durable Meridian job states.
	Status string
	// ExpectedAttempt is the one-based River attempt that owns the transition.
	ExpectedAttempt int
	// Result is non-secret JSON result metadata, or nil.
	Result []byte
	// Error is non-secret structured error metadata, or nil.
	Error []byte
	// Terminal indicates whether finishedAt should be written.
	Terminal bool
	// Stage is the stage associated with the final log event.
	Stage Stage
	// Level is the structured log severity for the final event.
	Level string
	// Message is a secret-free terminal diagnostic.
	Message string
	// FinishedAt is the UTC time at which the transition was recorded.
	FinishedAt time.Time
}

// ExecutionStore persists the application-owned side of a River job.
type ExecutionStore interface {
	StartJob(context.Context, StartInput) (ClaimResult, error)
	SetJobStage(context.Context, StageInput) error
	FinishJob(context.Context, FinishInput) error
}

// ClaimResult describes whether the current River attempt owns the domain job.
type ClaimResult struct {
	// Claimed is true when this attempt may execute and finalize the job.
	Claimed bool
	// Attempt is the durable attempt number after the claim.
	Attempt int
}

// SyncRunner performs repository synchronization after the durable job is claimed.
// M0 supplies an explicit unsupported runner; M1 replaces it with the Git producer.
type SyncRunner interface {
	Run(context.Context, CredentialSyncArgs) error
}
