package service

import (
	"context"
	"errors"
	"time"
	"uuid"
)

var (
	// ErrJobNotCancellable indicates that a job is already terminal and cannot accept cancellation.
	ErrJobNotCancellable = errors.New("job is not cancellable")
	// ErrJobNotRetryable indicates that a job state or active generation forbids manual retry.
	ErrJobNotRetryable = errors.New("job is not retryable")
)

// JobFilter selects tenant jobs using the finite contract-defined fields.
type JobFilter struct {
	// Types restricts results to registered job behavior identifiers.
	Types []string
	// Statuses restricts results to durable lifecycle states.
	Statuses []string
	// ScopeType restricts results to one resource category.
	ScopeType string
	// ScopeID restricts results to one resource UUID string.
	ScopeID string
}

// JobError is a secret-free asynchronous failure projection.
type JobError struct {
	// Code is a stable API error category.
	Code string
	// Message is safe for an authenticated tenant member.
	Message string
	// RequestID correlates the failure with the durable job when no HTTP request created it.
	RequestID string
	// Details contains optional non-secret structured diagnostics.
	Details map[string]any
}

// JobStageAttempt summarizes one stage during one River execution attempt.
type JobStageAttempt struct {
	// Stage identifies the pipeline stage.
	Stage string
	// Attempt is the one-based River execution attempt.
	Attempt int
	// Status is the stage outcome derived from durable logs and current job state.
	Status string
	// StartedAt is the first persisted event time for this stage.
	StartedAt *time.Time
	// FinishedAt is the terminal or next-stage boundary when known.
	FinishedAt *time.Time
	// Error contains a secret-free failure only when this stage failed.
	Error *JobError
}

// JobRecord is the tenant-visible durable job projection.
type JobRecord struct {
	// ID identifies the Meridian job.
	ID uuid.UUID
	// TenantSlug identifies the owning tenant in URLs and responses.
	TenantSlug string
	// RetryOfJobID identifies the same-tenant source job for a manual retry.
	RetryOfJobID *uuid.UUID
	// Type identifies the registered job behavior.
	Type string
	// Trigger identifies the request origin.
	Trigger string
	// Status is the current durable lifecycle state.
	Status string
	// Stage is the active or final pipeline stage when present.
	Stage *string
	// ScopeType identifies the resource category used for authorization.
	ScopeType string
	// ScopeID identifies the scoped resource when present.
	ScopeID *string
	// RefType identifies a branch or tag when the job is ref-scoped.
	RefType *string
	// Ref contains the normalized Git ref name when present.
	Ref *string
	// Result contains operation-specific non-secret identifiers and counters.
	Result map[string]any
	// Progress is the deterministic percentage derived from status and stage.
	Progress int
	// Dirty indicates that newer equivalent work arrived during execution.
	Dirty bool
	// Attempt is the number of River attempts already started.
	Attempt int
	// MaxAttempts is the maximum automatic River attempts.
	MaxAttempts int
	// NextAttemptAt is the next automatic retry time when present.
	NextAttemptAt *time.Time
	// Attempts contains ordered persisted stage-attempt summaries.
	Attempts []JobStageAttempt
	// Error contains the current secret-free asynchronous failure when present.
	Error *JobError
	// CreatedAt is when the job was accepted.
	CreatedAt time.Time
	// StartedAt is when execution first began.
	StartedAt *time.Time
	// FinishedAt is when the job reached a terminal state.
	FinishedAt *time.Time
	// UpdatedAt tracks material state changes for SSE snapshots.
	UpdatedAt time.Time
	// Capabilities lists currently authorized operations for this job.
	Capabilities []string
}

// JobLogRecord is one persisted, replayable and secret-free stage event.
type JobLogRecord struct {
	// Sequence is the strictly increasing per-job replay cursor.
	Sequence int64
	// Stage is the pipeline stage active for the event when present.
	Stage *string
	// Message is the persisted redacted diagnostic.
	Message string
	// OccurredAt is when the event was persisted.
	OccurredAt time.Time
}

// JobStateEvent is one material state snapshot emitted to a job stream.
type JobStateEvent struct {
	// Cursor is the latest persisted log sequence covered by the snapshot.
	Cursor int64
	// Status is the current durable job lifecycle state.
	Status string
	// Progress is the deterministic completion percentage.
	Progress int
	// At is the latest durable state update time.
	At time.Time
}

// JobEventSink receives ordered stream state, log, and heartbeat events.
type JobEventSink interface {
	State(JobStateEvent) error
	Log(JobLogRecord) error
	Heartbeat() error
}

// JobAccepted identifies one newly accepted or exactly replayed asynchronous job.
type JobAccepted struct {
	// JobID identifies the durable Meridian job.
	JobID uuid.UUID `json:"jobId"`
	// Deduplicated reports whether semantic coalescing selected existing work.
	Deduplicated bool `json:"deduplicated"`
}

// RetryJobRequest carries the authenticated idempotency identity into one atomic retry transaction.
type RetryJobRequest struct {
	// TenantID identifies the owning tenant.
	TenantID uuid.UUID
	// SourceJobID identifies the failed or cancelled source job.
	SourceJobID uuid.UUID
	// PrincipalType identifies session or PAT authentication.
	PrincipalType string
	// PrincipalID identifies the exact authenticated session or PAT.
	PrincipalID uuid.UUID
	// IdempotencyKey identifies this semantic retry request for 24 hours.
	IdempotencyKey uuid.UUID
	// RequestHash is the canonical 32-byte semantic request digest.
	RequestHash []byte
	// RequestedAt is the UTC time at which the retry was accepted.
	RequestedAt time.Time
}

// PlatformJobRecord is the redacted cross-tenant job projection exposed to platform administrators.
// It intentionally excludes payloads, errors, attempts, refs, and River internals.
type PlatformJobRecord struct {
	// ID identifies the durable Meridian job.
	ID uuid.UUID
	// TenantSlug identifies the tenant that owns the job.
	TenantSlug string
	// Type identifies the registered job behavior.
	Type string
	// Trigger identifies the request origin.
	Trigger string
	// Status is the current durable job state.
	Status string
	// Stage is the active or final pipeline stage, when present.
	Stage *string
	// ScopeType identifies the resource category used for authorization.
	ScopeType string
	// ScopeID is exposed only for tenant and repository scopes.
	ScopeID *string
	// CreatedAt is the UTC instant when the job was accepted.
	CreatedAt time.Time
	// StartedAt is the UTC instant when execution began, when present.
	StartedAt *time.Time
	// FinishedAt is the UTC instant when execution reached a terminal state, when present.
	FinishedAt *time.Time
}

// PlatformJobFilter selects redacted jobs without permitting arbitrary SQL expressions.
type PlatformJobFilter struct {
	// Types restricts results to known job behavior identifiers.
	Types []string
	// Statuses restricts results to known durable job states.
	Statuses []string
	// ScopeType restricts results to one known scope category.
	ScopeType string
	// ScopeID restricts results to one scope identifier string.
	ScopeID string
	// TenantSlug restricts results to one active tenant slug.
	TenantSlug string
}

// JobStore is the persistence boundary for tenant control and redacted platform queries.
type JobStore interface {
	ListPlatformJobs(context.Context, PlatformJobFilter, int32, int32) ([]PlatformJobRecord, int64, error)
	GetPlatformJob(context.Context, uuid.UUID) (PlatformJobRecord, error)
	ListTenantJobs(context.Context, uuid.UUID, JobFilter, int32, int32) ([]JobRecord, int64, error)
	GetTenantJob(context.Context, uuid.UUID, uuid.UUID) (JobRecord, error)
	GetTenantJobStreamState(context.Context, uuid.UUID, uuid.UUID) (JobRecord, int64, error)
	ListTenantJobLogsAfter(context.Context, uuid.UUID, uuid.UUID, int64, int32) ([]JobLogRecord, error)
	CancelTenantJob(context.Context, uuid.UUID, uuid.UUID, time.Time) (JobAccepted, error)
	RetryTenantJob(context.Context, RetryJobRequest) (JobAccepted, error)
}
