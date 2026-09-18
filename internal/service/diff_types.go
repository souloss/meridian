package service

import (
	"context"
	"time"
	"uuid"
)

// DiffChangeKind carries one structured diff change classification.
type DiffChangeKind struct {
	// ID is a stable, deterministic per-run identifier.
	ID string
	// Level is breaking / risky / non_breaking / informational.
	Level string
	// Code is the breaking-rule or change code (e.g. operation-removed).
	Code string
	// Path locates the changed element.
	Path string
	// Summary is a human-readable change description.
	Summary string
	// Before and After carry the raw JSON value on each side (nil when absent).
	Before any
	After  any
}

// DiffCountsSummary aggregates a diff result's change counters.
type DiffCountsSummary struct {
	Added         int
	Removed       int
	Modified      int
	Breaking      int
	Risky         int
	NonBreaking   int
	Informational int
}

// ResolvedDocRef is the immutable document reference reported by a diff.
type ResolvedDocRef struct {
	SourceType       string
	Kind             string
	ContentHash      string
	AssetID          *uuid.UUID
	VersionID        *uuid.UUID
	UploadID         *uuid.UUID
	RequestedRefType *string
	RequestedRef     *string
}

// DiffOutcome is the result of running one diff.
type DiffOutcome struct {
	Kind       string
	Left       ResolvedDocRef
	Right      ResolvedDocRef
	SnapshotID *uuid.UUID
	Summary    DiffCountsSummary
	Changes    []DiffChangeKind
	GeneratedAt time.Time
}

// DiffSelector is one resolved diff input (version, ref, or upload).
type DiffSelector struct {
	Type      string // version | ref | upload
	VersionID *uuid.UUID
	AssetID   *uuid.UUID
	RefType   *string
	RefName   *string
	UploadID  *uuid.UUID
	Kind      string
}

// DiffRunInput carries one runDiff request.
type DiffRunInput struct {
	Left      DiffSelector
	Right     DiffSelector
	RuleSetID *uuid.UUID
	Persist   bool
}

// ShareLinkCreatedResult is the create-share-link response projection.
type ShareLinkCreatedResult struct {
	ID              uuid.UUID
	Token           string
	ResourceType    string
	ResourceID      *uuid.UUID
	DescriptorDigest string
	URL             string
	ExpiresAt       time.Time
	RevokedAt       *time.Time
	CreatedAt       time.Time
}

// SharedViewResult is the anonymous shared view resolution.
type SharedViewResult struct {
	ResourceType string
	ExpiresAt    time.Time
	SnapshotID   uuid.UUID
	CreatedBy    uuid.UUID
	CreatedAt    time.Time
	Snapshot     DiffOutcome
}

// TodoRecord is one breaking-change todo projection.
type TodoRecord struct {
	ID            uuid.UUID
	AssetVersionID uuid.UUID
	ServiceID     uuid.UUID
	Status        string
	AckedBy       *uuid.UUID
	AckedAt       *time.Time
	Comment       *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PushRevisionInput carries one pushAssetRevision request.
type PushRevisionInput struct {
	ServiceSlug     string
	Kind            string
	Name            string
	RefType         string
	Ref             string
	SourceSystem    string
	CreateIfMissing bool
	Content         string
	ContentType     string
	Role            string
	Dialect         *string
	SourceCommit    *string
	IdempotencyKey  uuid.UUID
	PrincipalType   string
	PrincipalID     uuid.UUID
	RequestHash     []byte
}

// PushRevisionResult is the pushAssetRevision response projection.
type PushRevisionResult struct {
	AssetID      uuid.UUID
	LayerID      uuid.UUID
	RevisionID   uuid.UUID
	JobID        uuid.UUID
	Deduplicated bool
}

// DiffStore is the persistence boundary for diff, share, todo, upload, and push.
type DiffStore interface {
	AssetStore
	// GetServiceBySlug returns one active service by slug.
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	// GetLayerRevision returns one immutable layer revision.
	GetLayerRevision(context.Context, uuid.UUID, uuid.UUID) (LayerRevisionRecord, error)
	// CreateSourceSpec inserts one push source spec.
	CreateSourceSpec(context.Context, NewSourceSpec) (SourceSpecRecord, error)
	// CreateUpload persists one diff upload.
	CreateUpload(context.Context, NewUpload) (UploadRecord, error)
	// GetUpload returns one unexpired upload.
	GetUpload(context.Context, uuid.UUID, uuid.UUID) (UploadRecord, error)
	// CreateDiffSnapshot persists one frozen diff result.
	CreateDiffSnapshot(context.Context, NewDiffSnapshot) (DiffSnapshotRecord, error)
	// GetDiffSnapshot returns one snapshot.
	GetDiffSnapshot(context.Context, uuid.UUID, uuid.UUID) (DiffSnapshotRecord, error)
	// CreateShareLink persists one share link.
	CreateShareLink(context.Context, NewShareLink) (ShareLinkRecord, error)
	// GetShareLinkByTokenHash returns one active share link.
	GetShareLinkByTokenHash(context.Context, []byte) (ShareLinkRecord, error)
	// CreateBreakingTodoIfAbsent creates one todo keyed by version+service.
	CreateBreakingTodoIfAbsent(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (TodoRecord, error)
	// ListBreakingTodos pages todos by status.
	ListBreakingTodos(context.Context, uuid.UUID, string, int32, int32) ([]TodoRecord, int64, error)
	// AckBreakingTodo acknowledges one open todo.
	AckBreakingTodo(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time, *string) (TodoRecord, error)
	// GetSourceLayerByPushKey returns the push layer of an asset by the stable
	// push identity (kind + name), so repeated pushes reuse one overlay layer.
	GetSourceLayerByPushKey(context.Context, uuid.UUID, uuid.UUID) (LayerRecord, error)
	// ListServicesByRepositoryOwners returns the service ids owning an asset.
	ListServicesByRepositoryOwners(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error)
}

// UploadRecord is one diff upload projection.
type UploadRecord struct {
	ID          uuid.UUID
	BlobDigest  string
	Kind        string
	ContentType string
	SizeBytes   int64
	ExpiresAt   time.Time
	CreatedBy   *uuid.UUID
	CreatedAt   time.Time
}

// NewUpload carries one upload insert.
type NewUpload struct {
	TenantID    uuid.UUID
	ID          uuid.UUID
	BlobDigest  string
	Kind        string
	ContentType string
	SizeBytes   int64
	ExpiresAt   time.Time
	CreatedBy   *uuid.UUID
}

// DiffSnapshotRecord is one frozen diff snapshot projection.
type DiffSnapshotRecord struct {
	ID               uuid.UUID
	LeftSelector     []byte
	RightSelector    []byte
	LeftArtifactRef  string
	RightArtifactRef string
	RuleSetID        *uuid.UUID
	ResultRef        string
	Summary          []byte
	CreatedBy        uuid.UUID
	CreatedAt        time.Time
}

// NewDiffSnapshot carries one snapshot insert.
type NewDiffSnapshot struct {
	TenantID         uuid.UUID
	ID               uuid.UUID
	LeftSelector     []byte
	RightSelector    []byte
	LeftArtifactRef  string
	RightArtifactRef string
	RuleSetID        *uuid.UUID
	ResultRef        string
	Summary          []byte
	CreatedBy        uuid.UUID
}

// ShareLinkRecord is one share link projection.
type ShareLinkRecord struct {
	TenantID          uuid.UUID
	ID                uuid.UUID
	TokenHash         []byte
	CreatorID         uuid.UUID
	ResourceType      string
	ResourceID        *uuid.UUID
	Descriptor        []byte
	ViewID            *string
	Options           []byte
	ArtifactAllowlist []byte
	ExpiresAt         time.Time
	RevokedAt         *time.Time
	CreatedAt         time.Time
}

// NewShareLink carries one share link insert.
type NewShareLink struct {
	TenantID          uuid.UUID
	ID                uuid.UUID
	TokenHash         []byte
	CreatorID         uuid.UUID
	ResourceType      string
	ResourceID        *uuid.UUID
	Descriptor        []byte
	ViewID            *string
	Options           []byte
	ArtifactAllowlist []byte
	ExpiresAt         time.Time
}
