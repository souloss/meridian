package service

import (
	"context"
	"time"
	"uuid"
)

// AuditRecord is the redacted append-only audit projection exposed by the API.
type AuditRecord struct {
	// ID uniquely identifies the audit fact.
	ID uuid.UUID
	// TenantSlug identifies the owning tenant, or is nil for a platform fact.
	TenantSlug *string
	// ActorID identifies the user actor when one exists.
	ActorID *uuid.UUID
	// Action is the stable audited operation identifier.
	Action string
	// ResourceType identifies the affected resource category.
	ResourceType string
	// ResourceID identifies the affected resource when one exists.
	ResourceID *string
	// RequestID correlates the fact with an HTTP request when one exists.
	RequestID *string
	// Metadata contains redacted identifiers, counters, and hashes only.
	Metadata map[string]any
	// CreatedAt is the UTC transaction time at which the fact committed.
	CreatedAt time.Time
}

// AuditFilter selects audit metadata using bounded, typed predicates.
type AuditFilter struct {
	// ActorID restricts results to one user actor when non-nil.
	ActorID *uuid.UUID
	// Actions restricts results to exact stable action identifiers.
	Actions []string
	// ResourceType restricts results to one exact resource category.
	ResourceType string
	// ResourceID restricts results to one exact resource identifier.
	ResourceID string
	// From includes facts created at or after this instant when non-nil.
	From *time.Time
	// To includes facts created at or before this instant when non-nil.
	To *time.Time
	// TenantSlug restricts platform results to one tenant when non-empty.
	TenantSlug string
}

// AuditStore is the persistence boundary for redacted audit queries.
type AuditStore interface {
	ListTenantAudits(context.Context, uuid.UUID, AuditFilter, int32, int32) ([]AuditRecord, int64, error)
	ListPlatformAudits(context.Context, AuditFilter, int32, int32) ([]AuditRecord, int64, error)
}
