package service

import (
	"context"
	"time"
	"uuid"
)

// RepositoryError is the redacted health error recorded after a repository operation.
type RepositoryError struct {
	// Class is the stable non-secret error category used by health projections.
	Class string `json:"class"`
	// Message is the redacted operator-facing diagnostic.
	Message string `json:"message"`
}

// RepositoryHealth is the durable, non-secret synchronization summary.
type RepositoryHealth struct {
	// LastSyncAt is the UTC instant of the latest completed repository operation.
	LastSyncAt *time.Time
	// LastCommit is the latest fetched commit SHA, when available.
	LastCommit *string
	// LastError is the latest redacted failure, when an operation failed.
	LastError *RepositoryError
	// FailStreak is the number of consecutive failed operations.
	FailStreak int
	// DurationMS is the elapsed duration of the latest completed operation.
	DurationMS *int
}

// RepositoryBranchPolicy controls which branch and tag refs are eligible for synchronization.
type RepositoryBranchPolicy struct {
	BranchPatterns []string `json:"branchPatterns"`
	TagPatterns    []string `json:"tagPatterns"`
}

// RepositoryFetchConfig controls checkout depth, paths, submodules, proxy, and SSH trust policy.
type RepositoryFetchConfig struct {
	Shallow         bool     `json:"shallow"`
	Depth           *int     `json:"depth,omitempty"`
	Submodules      bool     `json:"submodules"`
	Proxy           *string  `json:"proxy,omitempty"`
	PathAllow       []string `json:"pathAllow"`
	PathIgnore      []string `json:"pathIgnore"`
	KnownHostPolicy string   `json:"knownHostPolicy"`
}

// RepositoryRecord is the tenant-scoped repository metadata projection.
// It never contains decrypted credential material.
type RepositoryRecord struct {
	// TenantID is the tenant boundary for this repository.
	TenantID uuid.UUID
	// ID is the application-generated repository identifier.
	ID uuid.UUID
	// URL is the credential-free URL submitted by the operator.
	URL string
	// CanonicalURL is the normalized URL used for uniqueness checks.
	CanonicalURL string
	// CredentialID is the tenant or global credential selected for fetching.
	CredentialID *uuid.UUID
	// GlobalCredentialID is populated when CredentialID resolves to a platform credential.
	GlobalCredentialID *uuid.UUID
	// DefaultBranch is the default Git ref used when a request omits one.
	DefaultBranch string
	// BranchPolicy controls eligible branch and tag patterns.
	BranchPolicy RepositoryBranchPolicy
	// FetchConfig controls checkout and SSH trust behavior.
	FetchConfig RepositoryFetchConfig
	// SyncCron is the optional five-field UTC schedule.
	SyncCron *string
	// Note is the optional operator note.
	Note *string
	// Health is the durable non-secret synchronization summary.
	Health RepositoryHealth
	// Capabilities lists actions available to the authenticated principal.
	Capabilities []string
	// Revision is the optimistic-concurrency version.
	Revision int64
	// CreatedAt is the UTC creation instant.
	CreatedAt time.Time
	// UpdatedAt is the UTC metadata update instant.
	UpdatedAt time.Time
}

// NewRepository contains validated values ready for an atomic repository insert.
type NewRepository struct {
	// TenantID identifies the owning tenant.
	TenantID uuid.UUID
	// ID identifies the new repository.
	ID uuid.UUID
	// URL is the display URL without credential material.
	URL string
	// CanonicalURL is the normalized uniqueness key.
	CanonicalURL string
	// CredentialID is an optional same-tenant credential reference.
	CredentialID *uuid.UUID
	// GlobalCredID is an optional platform credential reference.
	GlobalCredID *uuid.UUID
	// DefaultBranch is the selected default Git ref.
	DefaultBranch string
	// BranchPolicy is the validated ref inclusion policy.
	BranchPolicy RepositoryBranchPolicy
	// FetchConfig is the validated checkout configuration.
	FetchConfig RepositoryFetchConfig
	// SyncCron is the optional schedule.
	SyncCron *string
	// Note is the optional operator note.
	Note *string
}

// RepositoryPatch contains fields explicitly supplied by a PATCH request.
// Pointer-to-pointer fields distinguish omitted from explicit JSON null.
type RepositoryPatch struct {
	// CredentialID distinguishes omitted from explicit null or a UUID.
	CredentialID **uuid.UUID
	// DefaultBranch is an optional replacement default ref.
	DefaultBranch *string
	// BranchPolicy is an optional complete policy replacement.
	BranchPolicy *RepositoryBranchPolicy
	// FetchConfig is an optional complete fetch configuration replacement.
	FetchConfig *RepositoryFetchConfig
	// SyncCron distinguishes omitted from explicit null or a cron string.
	SyncCron **string
	// Note distinguishes omitted from explicit null or a note string.
	Note **string
}

// UpdateRepository contains one optimistic-concurrency repository mutation.
type UpdateRepository struct {
	// TenantID identifies the owning tenant.
	TenantID uuid.UUID
	// ID identifies the repository being changed.
	ID uuid.UUID
	// ExpectedRevision is the ETag revision required by the mutation.
	ExpectedRevision int64
	// CredentialID distinguishes omitted from explicit null or a UUID.
	CredentialID **uuid.UUID
	// GlobalCredID distinguishes omitted from explicit null or a UUID.
	GlobalCredID **uuid.UUID
	// DefaultBranch is an optional replacement default ref.
	DefaultBranch *string
	// BranchPolicy is an optional complete policy replacement.
	BranchPolicy *RepositoryBranchPolicy
	// FetchConfig is an optional complete fetch configuration replacement.
	FetchConfig *RepositoryFetchConfig
	// SyncCron distinguishes omitted from explicit null or a cron string.
	SyncCron **string
	// Note distinguishes omitted from explicit null or a note string.
	Note **string
	// UpdatedAt is the UTC mutation timestamp.
	UpdatedAt time.Time
}

// CredentialReference identifies which encrypted credential table owns a UUID.
type CredentialReference struct {
	// ID is the resolved credential identifier.
	ID uuid.UUID
	// IsGlobal indicates the platform-owned credential table.
	IsGlobal bool
}

// RepositoryStore is the persistence boundary for tenant repository configuration.
// Every implementation must retain tenant predicates and never return soft-deleted rows.
type RepositoryStore interface {
	ResolveCredentialReference(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (CredentialReference, bool, error)
	CountRepositories(context.Context, uuid.UUID) (int64, int64, error)
	ListRepositories(context.Context, uuid.UUID, string, int32, int32) ([]RepositoryRecord, int64, error)
	GetRepository(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	CreateRepository(context.Context, NewRepository) (RepositoryRecord, error)
	UpdateRepository(context.Context, UpdateRepository) (RepositoryRecord, error)
	DeleteRepository(context.Context, uuid.UUID, uuid.UUID, int64, time.Time) error
}
