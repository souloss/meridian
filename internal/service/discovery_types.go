package service

import (
	"context"
	"time"
	"uuid"
)

// ProducerProfileKind identifies the producer category.
const (
	producerKindCommand = "command"
	producerKindAI      = "ai"
)

// ProducerProfileOption is the tenant-visible producer selection projection.
type ProducerProfileOption struct {
	ID                uuid.UUID
	Name              string
	Kind              string
	SupportedKinds    []string
	Network           string
	DependencyStatus  string
	UnavailableReason *string
}

// ProducerProfile is the platform-managed producer configuration record.
type ProducerProfile struct {
	ID                uuid.UUID
	Name              string
	Kind              string
	Executable        string
	Args              []string
	EnvAllowlist      []string
	SupportedKinds    []string
	ReplaySafe        bool
	Network           string
	TimeoutSec        int
	MemoryMiB         int
	CPUSeconds        int
	Pids              int
	Enabled           bool
	DependencyStatus  string
	UnavailableReason *string
	Revision          int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NewProducerProfile carries validated values ready for an atomic insert.
type NewProducerProfile struct {
	ID                uuid.UUID
	Name              string
	Kind              string
	Executable        string
	Args              []string
	EnvAllowlist      []string
	SupportedKinds    []string
	ReplaySafe        bool
	Network           string
	TimeoutSec        int
	MemoryMiB         int
	CPUSeconds        int
	Pids              int
	Enabled           bool
	DependencyStatus  string
	UnavailableReason *string
}

// ServiceRecord is the tenant-visible service projection used by discovery acceptance.
type ServiceRecord struct {
	TenantID     uuid.UUID
	ID           uuid.UUID
	RepositoryID uuid.UUID
	Slug         string
	DisplayName  string
	Description  *string
	RootDir      string
	Language     *string
	Framework    *string
	Visibility   string
	Lifecycle    string
	Revision     int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SourceSpecRecord is the tenant-visible source configuration projection.
type SourceSpecRecord struct {
	ID                uuid.UUID
	ServiceID         uuid.UUID
	Kind              string
	AssetNameTemplate string
	Role              string
	Origin            string
	Mode              string
	Path              *string
	ProducerProfileID *uuid.UUID
	Ord               int
	TimeoutSec        int
	BranchPatterns    []string
	Enabled           bool
	ConfigOrigin      string
	BindingsCount     int
	// InitialLayerID is non-null only for a manual source spec: createSourceSpec
	// atomically creates its one global binding and layer and returns that layer id.
	InitialLayerID *uuid.UUID
	Revision       int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewSourceSpec carries validated values ready for an atomic insert.
type NewSourceSpec struct {
	TenantID          uuid.UUID
	ID                uuid.UUID
	ServiceID         uuid.UUID
	Kind              string
	AssetNameTemplate string
	Role              string
	Origin            string
	Mode              string
	Path              *string
	ProducerProfileID *uuid.UUID
	Ord               int
	TimeoutSec        int
	BranchPatterns    []string
	Enabled           bool
	ConfigOrigin      string
	// TargetAssetID is required only for manual mode; the asset must belong to
	// the path service and match kind.
	TargetAssetID *uuid.UUID
	// ReplaceAiBase explicitly archives an existing AI-generated base before
	// creating a repository base (SMK-033).
	ReplaceAiBase bool
}

// SourceBindingRecord is one materialized source binding projection.
type SourceBindingRecord struct {
	ID             uuid.UUID
	SourceSpecID   uuid.UUID
	ScopeType      string
	ScopeKey       string
	ExpansionKey   string
	ResolvedPath   *string
	SourceSystem   *string
	AssetID        uuid.UUID
	LayerID        uuid.UUID
	State          string
	LastSeenCommit *string
}

// DiscoveryCandidateRecord is one discovery candidate projection.
type DiscoveryCandidateRecord struct {
	ID           uuid.UUID
	RepositoryID uuid.UUID
	CommitSHA    string
	RootDir      string
	Detected     map[string]any
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DiscoverJobInput describes one idempotent repository discovery request.
type DiscoverJobInput struct {
	TenantID       uuid.UUID
	RepositoryID   uuid.UUID
	RefType        string
	RefName        string
	IdempotencyKey uuid.UUID
}

// RepositorySyncInput describes one repository synchronization request.
type RepositorySyncInput struct {
	RefType string
	RefName string
	Force   *bool
}

// SourceSpecPatchInput carries explicit PATCH fields for one source spec update.
type SourceSpecPatchInput struct {
	AssetNameTemplate *string
	Role              *string
	Origin            *string
	Mode              *string
	Path              *string
	ProducerProfileID *uuid.UUID
	Ord               *int
	TimeoutSec        *int
	BranchPatterns    []string
	Enabled           *bool
}

// NewService carries validated values ready for an atomic service insert.
type NewService struct {
	TenantID     uuid.UUID
	ID           uuid.UUID
	RepositoryID uuid.UUID
	Slug         string
	DisplayName  string
	Description  *string
	RootDir      string
	Visibility   string
}

// ProducerStore is the platform producer configuration persistence boundary.
type ProducerStore interface {
	CreateProducerProfile(context.Context, NewProducerProfile) (ProducerProfile, error)
	ListAvailableProducerProfiles(context.Context, string) ([]ProducerProfile, error)
	GetProducerProfile(context.Context, uuid.UUID) (ProducerProfile, error)
}

// DiscoveryStore is the persistence boundary for repository discovery, service
// acceptance, and source configuration. It also enqueues discovery and sync
// jobs transactionally. Every method retains the tenant predicate.
type DiscoveryStore interface {
	ProducerStore
	GetRepository(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	EnqueueDiscoveryJob(context.Context, DiscoverJobInput) (JobAccepted, error)
	EnqueueSyncJob(context.Context, SyncJobInput) (JobAccepted, error)
	UpsertDiscoveryCandidate(context.Context, NewDiscoveryCandidate) (DiscoveryCandidateRecord, error)
	ListDiscoveryCandidates(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]DiscoveryCandidateRecord, int64, error)
	GetDiscoveryCandidate(context.Context, uuid.UUID, uuid.UUID) (DiscoveryCandidateRecord, error)
	AcceptDiscoveryCandidate(context.Context, uuid.UUID, uuid.UUID) error
	CreateService(context.Context, NewService) (ServiceRecord, error)
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	CountServices(context.Context, uuid.UUID) (int64, int64, error)
	CreateSourceSpec(context.Context, NewSourceSpec) (SourceSpecRecord, error)
	GetSourceSpec(context.Context, uuid.UUID, uuid.UUID) (SourceSpecRecord, error)
	// GetAiBaseForService returns an AI-generated base for a service/kind when one exists.
	GetAiBaseForService(context.Context, uuid.UUID, uuid.UUID, string) (SourceSpecRecord, error)
	// ReplaceAiBaseForService archives the AI base and creates the repository base in one transaction.
	ReplaceAiBaseForService(context.Context, uuid.UUID, uuid.UUID, string) error
	UpdateSourceSpec(context.Context, SourceSpecPatch) (SourceSpecRecord, error)
	ListSourceSpecsForService(context.Context, uuid.UUID, uuid.UUID) ([]SourceSpecRecord, error)
	ListSourceBindings(context.Context, uuid.UUID, uuid.UUID) ([]SourceBindingRecord, error)
	CountSourceBindings(context.Context, uuid.UUID, uuid.UUID) (int64, error)
	CountActiveBindings(context.Context, uuid.UUID, uuid.UUID) (int64, error)
	UpsertRecentService(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
	ListRecentServices(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]ServiceRecord, int64, error)
}

// NewDiscoveryCandidate carries one discovered service root for an upsert.
type NewDiscoveryCandidate struct {
	TenantID     uuid.UUID
	ID           uuid.UUID
	RepositoryID uuid.UUID
	CommitSHA    string
	RootDir      string
	Detected     map[string]any
}
