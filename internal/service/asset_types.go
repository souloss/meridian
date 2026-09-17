package service

import (
	"context"
	"time"
	"uuid"
)

// AssetRecord is the tenant-visible asset projection.
type AssetRecord struct {
	ID        uuid.UUID
	ServiceID uuid.UUID
	Kind      string
	Name      string
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AssetVersionRecord is the tenant-visible asset version projection.
type AssetVersionRecord struct {
	ID                 uuid.UUID
	AssetID            uuid.UUID
	TrackID            uuid.UUID
	SequenceNo         int64
	Version            string
	Lifecycle          string
	Revision           int64
	InputFingerprint   string
	MergeEngineVersion string
	LayerManifest      []LayerManifestEntry
	SourceCommit       *string
	IndexComplete      bool
	CreatedAt          time.Time
}

// LayerManifestEntry identifies one layer revision in a version manifest.
type LayerManifestEntry struct {
	LayerID      uuid.UUID
	RevisionID   uuid.UUID
	Role         string
	Origin       string
	Ord          int
	ScopeType    string
	ScopeKey     string
	ReviewStatus string
	ContentHash  string
}

// AssetItemRecord is one indexed asset item.
type AssetItemRecord struct {
	ItemType string
	Key      string
	Display  map[string]any
}

// LayerRecord is one asset layer.
type LayerRecord struct {
	ID             uuid.UUID
	AssetID        uuid.UUID
	SourceSpecID   *uuid.UUID
	Role           string
	Origin         string
	Ord            int
	Dialect        *string
	Enabled        bool
	BranchPatterns []string
	DisplayName    string
	Revision       int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// LayerRevisionRecord is one immutable layer content revision.
type LayerRevisionRecord struct {
	ID           uuid.UUID
	LayerID      uuid.UUID
	ScopeType    string
	ScopeKey     string
	ContentHash  string
	ContentRef   string
	ContentType  string
	Dialect      *string
	ReviewStatus string
	GitCommit    *string
	SourceBranch *string
	CreatedAt    time.Time
}

// NewLayerRevision carries values for one layer revision insert.
type NewLayerRevision struct {
	TenantID      uuid.UUID
	ID            uuid.UUID
	LayerID       uuid.UUID
	ScopeType     string
	ScopeKey      string
	ContentHash   string
	ContentRef    string
	ContentType   string
	Dialect       *string
	SourceBranch  *string
	ReviewStatus  string
	GitCommit     *string
	CreatedBy     *uuid.UUID
	ProducerRunID *uuid.UUID
}

// NewLayerHead carries values for one layer head upsert.
type NewLayerHead struct {
	TenantID            uuid.UUID
	LayerID             uuid.UUID
	ScopeType           string
	ScopeKey            string
	LatestRevisionID    *uuid.UUID
	EffectiveRevisionID *uuid.UUID
	CandidateRevisionID *uuid.UUID
	Generation          int64
}

// NewAssetVersion carries values for one version insert.
type NewAssetVersion struct {
	TenantID               uuid.UUID
	ID                     uuid.UUID
	AssetID                uuid.UUID
	TrackID                uuid.UUID
	SequenceNo             int64
	Version                string
	Lifecycle              string
	Revision               int64
	QualityScore           *int32
	MergeRequestID         *uuid.UUID
	InputFingerprint       string
	MergeEngineVersion     string
	OverlayCompilerVersion *string
	OverlayMode            *string
	NormalizerVersion      *string
	KindPluginVersion      *string
	LayerManifest          []byte
	MergedHash             *string
	MergedRef              *string
	NormalizedRef          *string
	BundledRef             *string
	ProvenanceRef          *string
	SourceCommit           *string
	BaselineVersionID      *uuid.UUID
	DiffSummary            []byte
	Labels                 []byte
	IndexComplete          bool
}

// NewAssetItem carries values for one item insert.
type NewAssetItem struct {
	TenantID       uuid.UUID
	ID             uuid.UUID
	AssetVersionID uuid.UUID
	AssetID        uuid.UUID
	ServiceID      uuid.UUID
	Kind           string
	ItemType       string
	Key            string
	Display        []byte
	SearchText     string
	SearchRaw      []byte
	Provenance     []byte
}

// NewSourceBinding carries values for one binding upsert.
type NewSourceBinding struct {
	TenantID       uuid.UUID
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

// SyncJobInput describes one idempotent repository synchronization request.
type SyncJobInput struct {
	TenantID       uuid.UUID
	RepositoryID   uuid.UUID
	RefType        string
	RefName        string
	IdempotencyKey uuid.UUID
	Force          *bool
	// PrincipalType and PrincipalID bound the replay identity to the caller.
	PrincipalType string
	PrincipalID   uuid.UUID
	// RequestHash is the 32-byte RFC 8785 request digest for replay comparison.
	RequestHash []byte
}

// SyncStore is the persistence boundary for repository synchronization enqueueing.
type SyncStore interface {
	GetRepository(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	EnqueueSyncJob(context.Context, SyncJobInput) (JobAccepted, error)
	MarkSyncJobDirty(context.Context, uuid.UUID, string, int64) error
	GetSyncJobForSuccessor(context.Context, uuid.UUID, uuid.UUID) (SyncJobSuccessorState, error)
	ClearSyncJobDirty(context.Context, uuid.UUID, uuid.UUID) error
}

// SyncJobSuccessorState captures whether a completed sync must spawn a successor.
type SyncJobSuccessorState struct {
	JobID            uuid.UUID
	Status           string
	Dirty            bool
	ScopeID          *uuid.UUID
	RefType          *string
	RefName          *string
	ActiveGeneration int64
}

// AssetStore is the persistence boundary for the M1 asset pipeline.
type AssetStore interface {
	GetRepository(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	GetAssetRepositoryDefaultBranch(context.Context, uuid.UUID, uuid.UUID) (string, error)
	GetAssetKind(context.Context, string) (AssetKindRecord, error)
	ListAssetKinds(context.Context) ([]AssetKindRecord, error)
	ListServicesByRepository(context.Context, uuid.UUID, uuid.UUID) ([]ServiceRecord, error)
	ListAssetsForService(context.Context, uuid.UUID, uuid.UUID) ([]AssetRecord, error)
	ListSourceSpecsForService(context.Context, uuid.UUID, uuid.UUID) ([]SourceSpecRecord, error)
	UpsertAsset(context.Context, NewAsset) (AssetRecord, error)
	GetAssetByName(context.Context, uuid.UUID, uuid.UUID, string, string) (AssetRecord, error)
	GetAsset(context.Context, uuid.UUID, uuid.UUID) (AssetRecord, error)
	CreateAssetRefTrack(context.Context, NewAssetRefTrack) (AssetRefTrackRecord, error)
	GetAssetRefTrack(context.Context, uuid.UUID, uuid.UUID, string, string) (AssetRefTrackRecord, error)
	CreateLayer(context.Context, NewLayer) (LayerRecord, error)
	GetBaseLayerForAsset(context.Context, uuid.UUID, uuid.UUID) (LayerRecord, error)
	CreateLayerRevision(context.Context, NewLayerRevision) (LayerRevisionRecord, error)
	GetLatestLayerRevision(context.Context, uuid.UUID, uuid.UUID, string, string) (LayerRevisionRecord, error)
	UpsertLayerHead(context.Context, NewLayerHead) (LayerHeadRecord, error)
	CreateAssetVersion(context.Context, NewAssetVersion) (AssetVersionRecord, error)
	GetAssetVersion(context.Context, uuid.UUID, uuid.UUID) (AssetVersionRecord, error)
	MarkAssetVersionIndexed(context.Context, uuid.UUID, uuid.UUID) error
	GetLatestVersionInTrack(context.Context, uuid.UUID, uuid.UUID) (AssetVersionRecord, error)
	GetCurrentVersionInTrack(context.Context, uuid.UUID, uuid.UUID) (AssetVersionRecord, error)
	UpdateAssetRefTrackHead(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, *uuid.UUID, int64) error
	CreateAssetItem(context.Context, NewAssetItem) (AssetItemRecord, error)
	UpdateAssetItemSearchVector(context.Context, uuid.UUID, uuid.UUID, string) error
	ListAssetVersionItems(context.Context, uuid.UUID, uuid.UUID, string, int32, int32) ([]AssetItemRecord, int64, error)
	UpdateSourceSpec(context.Context, SourceSpecPatch) (SourceSpecRecord, error)
	UpsertSourceBinding(context.Context, NewSourceBinding) (SourceBindingRecord, error)
	ListActiveBindingsForScope(context.Context, uuid.UUID, uuid.UUID, string, string) ([]SourceBindingRecord, error)
	MarkBindingsStaleInScope(context.Context, uuid.UUID, uuid.UUID, string, string, []uuid.UUID) error
	CountActiveBindings(context.Context, uuid.UUID, uuid.UUID) (int64, error)
	SetSourceLastError(context.Context, uuid.UUID, uuid.UUID, string) error
	ClearSourceLastError(context.Context, uuid.UUID, uuid.UUID) error
	MarkTracksStaleForSourceSpec(context.Context, uuid.UUID, uuid.UUID) error
	MarkTracksHealthyForSourceSpec(context.Context, uuid.UUID, uuid.UUID) error
	UpsertRecentService(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
	ListRecentServices(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]ServiceRecord, int64, error)
}

// AssetKindRecord is one platform asset kind registration.
type AssetKindRecord struct {
	ID              string
	ContractVersion string
	Enabled         bool
	PluginVersion   string
}

// AssetRefTrackRecord is one asset ref track.
type AssetRefTrackRecord struct {
	ID               uuid.UUID
	AssetID          uuid.UUID
	RefType          string
	RefName          string
	LatestVersionID  *uuid.UUID
	CurrentVersionID *uuid.UUID
	Health           string
}

// LayerHeadRecord is one layer head pointer.
type LayerHeadRecord struct {
	LayerID             uuid.UUID
	ScopeType           string
	ScopeKey            string
	LatestRevisionID    *uuid.UUID
	EffectiveRevisionID *uuid.UUID
	CandidateRevisionID *uuid.UUID
	Generation          int64
}

// NewAsset carries values for one asset upsert.
type NewAsset struct {
	TenantID  uuid.UUID
	ID        uuid.UUID
	ServiceID uuid.UUID
	Kind      string
	Name      string
}

// NewAssetRefTrack carries values for one track create.
type NewAssetRefTrack struct {
	TenantID uuid.UUID
	ID       uuid.UUID
	AssetID  uuid.UUID
	RefType  string
	RefName  string
	Health   string
}

// SourceSpecPatch carries explicit PATCH fields for one source spec update.
type SourceSpecPatch struct {
	TenantID          uuid.UUID
	ID                uuid.UUID
	ExpectedRevision  int64
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

// NewLayer carries values for one layer create.
type NewLayer struct {
	TenantID       uuid.UUID
	ID             uuid.UUID
	AssetID        uuid.UUID
	SourceSpecID   *uuid.UUID
	Role           string
	Origin         string
	Ord            int
	Dialect        *string
	Enabled        bool
	BranchPatterns []string
	DisplayName    string
}

// VersionRefRecord is a version reference embedded in an asset summary.
type VersionRefRecord struct {
	ID        uuid.UUID
	Version   string
	Lifecycle string
}

// AssetSummaryRecord is the service-embedded asset summary projection.
type AssetSummaryRecord struct {
	ID             uuid.UUID
	Kind           string
	Name           string
	Lifecycle      string
	Health         string
	CurrentVersion *VersionRefRecord
	LatestVersion  *VersionRefRecord
	RefType        string
	RefName        string
}

// MissingKindRecord reports a registered kind the service has no asset for.
type MissingKindRecord struct {
	Kind              string
	CanConfigure      bool
	CanGenerateWithAI bool
}
