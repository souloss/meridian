package service

import (
	"context"
	"time"
	"uuid"
)
// TenantAISettings is the AI/review sub-snapshot carried inside the tenant
// settings JSON blob. It mirrors contracts/domain.yaml defaults.tenant.
type TenantAISettings struct {
	// ExternalRevisionTrustMode selects review_required / trust_ai / trust_ai_and_third_party.
	ExternalRevisionTrustMode string `json:"externalRevisionTrustMode"`
	// AutoPublish selects whether publishing new AI versions is automatic (always false in v1).
	AutoPublish bool `json:"autoPublish"`
	// DefaultAiProducerProfileId is the optional tenant-default AI producer profile.
	DefaultAiProducerProfileId *uuid.UUID `json:"defaultAiProducerProfileId,omitempty"`
}

// AiGenerateInput carries one generateMissingAssetWithAi request's validated
// values before the job is enqueued.
type AiGenerateInput struct {
	// ServiceID is the owning service; the missing asset is created under it.
	ServiceID uuid.UUID
	// Kind identifies the asset kind to generate (e.g. openapi).
	Kind string
	// Name is the validated asset name for the missing asset.
	Name string
	// RefType defaults to branch; Ref is the requested ref (may be empty).
	RefType string
	Ref     string
	// Hint is the optional free-form hint forwarded to the producer.
	Hint string
	// ProducerProfileID is the resolved producer profile (requested or tenant default).
	ProducerProfileID uuid.UUID
	// IdempotencyKey deduplicates the request for 24 hours.
	IdempotencyKey uuid.UUID
	// PrincipalType and PrincipalID bound the replay identity.
	PrincipalType string
	PrincipalID   uuid.UUID
	// RequestHash is the 32-byte RFC 8785 digest for replay comparison.
	RequestHash []byte
}

// AiGenerateAccepted is the 202 response projection for generateMissingAssetWithAi.
type AiGenerateAccepted struct {
	AssetID      uuid.UUID
	SourceID     uuid.UUID
	JobID        uuid.UUID
	Deduplicated bool
}

// AiGenerateEnqueue carries everything the store needs to atomically create the
// AI source spec, its layer, and the generation job in one transaction.
type AiGenerateEnqueue struct {
	TenantID          uuid.UUID
	ServiceID         uuid.UUID
	Kind              string
	Name              string
	Hint              string
	ProducerProfileID uuid.UUID
	AssetID           uuid.UUID
	SourceID          uuid.UUID
	LayerID           uuid.UUID
	Role              string
	Ord               int
	ScopeType         string
	ScopeKey          string
	RefType           string
	RefName           string
	ServiceRoot       string
	IdempotencyKey    uuid.UUID
	PrincipalType     string
	PrincipalID       uuid.UUID
	RequestHash       []byte
}

// AiGenerationJobContext is the immutable context the worker re-reads from the
// durable job input to execute the producer and persist the revision.
type AiGenerationJobContext struct {
	AssetID           uuid.UUID
	SourceID          uuid.UUID
	LayerID           uuid.UUID
	Kind              string
	Name              string
	ProducerProfileID uuid.UUID
	RefType           string
	RefName           string
	ScopeType         string
	ScopeKey          string
	ServiceRoot       string
	Hint              string
}

// AiGenerationOutcome captures the terminal result of one AI generation job for
// the durable ai_generation_results row.
type AiGenerationOutcome struct {
	JobID       uuid.UUID
	Stage       string
	Status      string
	ErrorCode   string
	ContentRef  *string
	ContentHash *string
	ContentType *string
	Manifest    map[string]any
	RevisionID  *uuid.UUID
}

// ReviewContextResult carries one getReviewContext response: the candidate
// revision, the current effective revision (nil for a cold-start base), and the
// author of the candidate.
type ReviewContextResult struct {
	Revision                 LayerRevisionRecord
	CurrentEffectiveRevision *LayerRevisionRecord
	Author                   *User
}

// ReviewDecision carries one approve/reject decision result.
type ReviewDecision struct {
	Revision              LayerRevisionRecord
	EffectiveRevisionID   *uuid.UUID
	MergeJobID            *uuid.UUID
	SupersededRevisionIDs []uuid.UUID
	Deduplicated          bool
}

// PublishInput carries one publishAssetVersion request's validated values.
type PublishInput struct {
	VersionID        uuid.UUID
	ExpectedRevision int64
	Version          *string
	Labels           map[string]string
	IdempotencyKey   uuid.UUID
	PrincipalType    string
	PrincipalID      uuid.UUID
	RequestHash      []byte
}

// ReviewRevisionRecord is one layer revision enriched with its layer/asset context.
type ReviewRevisionRecord struct {
	LayerRevisionRecord
	AssetID uuid.UUID
	Role    string
	Origin  string
}

// AiGenerationStore is the persistence boundary for M3 AI generation, review,
// and publish. It extends LayerEditStore with the AI job, review, and publish
// operations. Every method retains the tenant predicate.
type AiGenerationStore interface {
	LayerEditStore
	// GetAssetByName returns an asset by service, kind, and name.
	GetAssetByName(context.Context, uuid.UUID, uuid.UUID, string, string) (AssetRecord, error)
	// GetBaseLayerForAsset returns the base layer of an asset when one exists.
	GetBaseLayerForAsset(context.Context, uuid.UUID, uuid.UUID) (LayerRecord, error)
	// GetLatestLayerRevision returns the newest revision in one layer scope.
	GetLatestLayerRevision(context.Context, uuid.UUID, uuid.UUID, string, string) (LayerRevisionRecord, error)
	// GetServiceBySlug returns the service owning the missing asset.
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	// GetProducerProfile returns one producer profile for availability validation.
	GetProducerProfile(context.Context, uuid.UUID) (ProducerProfile, error)
	// GetTenantSettings returns the tenant settings JSON for trust/autopublish resolution.
	GetTenantSettings(context.Context, uuid.UUID) ([]byte, error)
	// EnqueueAiGeneration atomically creates the AI source spec, layer, and job.
	EnqueueAiGeneration(context.Context, AiGenerateEnqueue) (AiGenerateAccepted, error)
	// GetAiGenerationJobContext returns the durable job input for the worker.
	GetAiGenerationJobContext(context.Context, uuid.UUID, uuid.UUID) (AiGenerationJobContext, error)
	// GetAiGenerationResult returns one durable AI generation outcome.
	GetAiGenerationResult(context.Context, uuid.UUID, uuid.UUID) (AiGenerationOutcome, error)
	// UpsertAiGenerationResult persists one AI generation outcome.
	UpsertAiGenerationResult(context.Context, uuid.UUID, AiGenerationOutcome) error
	// GetLayerRevisionForReview returns a revision with its layer/asset context.
	GetLayerRevisionForReview(context.Context, uuid.UUID, uuid.UUID) (ReviewRevisionRecord, error)
	// ListEnabledLayerHeadsForAssetScope lists heads with candidate pointers for publish gating.
	ListEnabledLayerHeadsForAssetScope(context.Context, uuid.UUID, uuid.UUID, string, string) ([]LayerHeadRecord, error)
	// UpdateLayerRevisionReview marks a pending revision approved or rejected.
	UpdateLayerRevisionReview(context.Context, uuid.UUID, uuid.UUID, string, *string) (LayerRevisionRecord, error)
	// SupersedeLayerRevision marks a pending revision superseded.
	SupersedeLayerRevision(context.Context, uuid.UUID, uuid.UUID) error
	// UnpublishAssetVersion demotes a published version to draft for historical input reuse.
	UnpublishAssetVersion(context.Context, uuid.UUID, uuid.UUID) error
	// LockAssetRefTrack locks a track row for the publish transaction.
	LockAssetRefTrack(context.Context, uuid.UUID, uuid.UUID) (AssetRefTrackRecord, error)
	// BumpAssetRefTrackGeneration increments the track desired generation.
	BumpAssetRefTrackGeneration(context.Context, uuid.UUID, uuid.UUID) error
	// UpdateAssetVersionPublish publishes a version under optimistic concurrency.
	UpdateAssetVersionPublish(context.Context, uuid.UUID, uuid.UUID, string, int64) (AssetVersionRecord, error)
}

var _ = time.Now
