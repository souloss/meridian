package service

import (
	"context"
	"time"
	"uuid"
)

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

// JobStore is the persistence boundary for redacted platform job queries.
type JobStore interface {
	ListPlatformJobs(context.Context, PlatformJobFilter, int32, int32) ([]PlatformJobRecord, int64, error)
	GetPlatformJob(context.Context, uuid.UUID) (PlatformJobRecord, error)
}
