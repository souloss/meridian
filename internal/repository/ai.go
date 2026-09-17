package repository

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/task"
	"github.com/riverqueue/river"
)

// AIStore implements the M3 AI generation, review, and publish persistence
// boundary. It embeds LayerStore to reuse the layer/head/version/track read and
// write path and adds the AI job, generation-result, review, and publish queries.
type AIStore struct {
	*LayerStore
	queries     *generated.Queries
	riverClient *river.Client[pgx.Tx]
}

const (
	// aiOriginDB 是 AI 生成层在 DB 中的来源列值（layerOrigin: ai_generated）。
	aiOriginDB = "ai_generated"
	// aiModeDB 是 AI 生成源配置在 DB 中的模式列值（sourceMode: ai）。
	aiModeDB = "ai"
	// aiTimeoutDB 是 AI 源配置缺省超时秒数（domain.yaml timeoutSecondsByKind.ai）。
	aiTimeoutDB = 600
)

// NewAIStore binds AI generation persistence to a native pgx pool.
func NewAIStore(pool *pgxpool.Pool) *AIStore {
	return &AIStore{LayerStore: NewLayerStore(pool), queries: generated.New(pool)}
}

// BindRiver attaches the process River client so AI generation jobs can be
// enqueued transactionally with their domain rows.
func (store *AIStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
	store.LayerStore.BindRiver(riverClient)
}

// GetServiceBySlug returns one active service within the tenant boundary.
func (store *AIStore) GetServiceBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceBySlug(ctx, generated.GetServiceBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// GetProducerProfile returns one platform producer profile by id.
func (store *AIStore) GetProducerProfile(ctx context.Context, id uuid.UUID) (service.ProducerProfile, error) {
	row, err := store.queries.GetProducerProfile(ctx, id)
	if err != nil {
		return service.ProducerProfile{}, normalizeError(err)
	}
	return producerProfileFromRow(row), nil
}

// GetTenantSettings returns the tenant settings JSON snapshot.
func (store *AIStore) GetTenantSettings(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	row, err := store.queries.GetTenantSettings(ctx, tenantID)
	if err != nil {
		return nil, normalizeError(err)
	}
	return row, nil
}

// EnqueueAiGeneration atomically creates the AI source spec, its layer, and the
// generation job, then records the River work, replaying an existing idempotency
// record when the key was seen before.
func (store *AIStore) EnqueueAiGeneration(ctx context.Context, input service.AiGenerateEnqueue) (service.AiGenerateAccepted, error) {
	if store.riverClient == nil {
		return service.AiGenerateAccepted{}, errors.New("AI generation has no River client")
	}
	dedupeKey := aiGenerationDedupeKey(input.AssetID)
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.AiGenerateAccepted{}, fmt.Errorf("begin AI generation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)

	if input.IdempotencyKey != uuid.Nil() {
		if err := queries.LockAiGenerationIdempotency(ctx, aiGenerationIdempotencyLockKey(input)); err != nil {
			return service.AiGenerateAccepted{}, normalizeError(err)
		}
		if replayed, found, err := loadAiGenerationReplay(ctx, queries, input); err != nil {
			return service.AiGenerateAccepted{}, err
		} else if found {
			return replayed, nil
		}
	}

	// Create the asset first, then its AI source spec and layer in this
	// transaction so the job carries stable source/layer identifiers to its
	// terminal revision write.
	if _, err := queries.UpsertAsset(ctx, generated.UpsertAssetParams{
		TenantID: input.TenantID, ID: input.AssetID, ServiceID: input.ServiceID, Kind: input.Kind, Name: input.Name,
	}); err != nil {
		return service.AiGenerateAccepted{}, normalizeError(err)
	}
	if _, err := queries.CreateSourceSpec(ctx, generated.CreateSourceSpecParams{
		TenantID: input.TenantID, ID: input.SourceID, ServiceID: input.ServiceID, Kind: input.Kind,
		AssetNameTemplate: input.Name, Role: input.Role, Origin: aiOriginDB, Mode: aiModeDB,
		Path: nil, ProducerProfileID: new(input.ProducerProfileID), Ord: int32(input.Ord),
		TimeoutSec: int32(aiTimeoutDB), BranchPatterns: []string{"**"}, Enabled: true, ConfigOrigin: "api",
	}); err != nil {
		return service.AiGenerateAccepted{}, normalizeError(err)
	}
	if _, err := queries.CreateLayer(ctx, generated.CreateLayerParams{
		TenantID: input.TenantID, ID: input.LayerID, AssetID: input.AssetID, SourceSpecID: new(input.SourceID),
		Role: input.Role, Origin: aiOriginDB, Ord: int32(input.Ord), Dialect: nil, Enabled: true,
		BranchPatterns: []string{"**"}, DisplayName: input.Name,
	}); err != nil {
		return service.AiGenerateAccepted{}, normalizeError(err)
	}

	jobInput, err := json.Marshal(service.AiGenerationJobContext{
		AssetID: input.AssetID, SourceID: input.SourceID, LayerID: input.LayerID, Kind: input.Kind, Name: input.Name,
		ProducerProfileID: input.ProducerProfileID, RefType: input.RefType, RefName: input.RefName,
		ScopeType: input.ScopeType, ScopeKey: input.ScopeKey, ServiceRoot: input.ServiceRoot, Hint: input.Hint,
	})
	if err != nil {
		return service.AiGenerateAccepted{}, fmt.Errorf("encode AI generation job context: %w", err)
	}

	accepted := service.AiGenerateAccepted{AssetID: input.AssetID, SourceID: input.SourceID}
	for {
		latest, err := queries.LockLatestDiscoveryJob(ctx, generated.LockLatestDiscoveryJobParams{TenantID: input.TenantID, DedupeKey: dedupeKey})
		generation := int64(1)
		if err == nil {
			if latest.Status == "pending" || latest.Status == "running" {
				accepted.JobID = latest.ID
				accepted.Deduplicated = true
				return accepted, nil
			}
			generation = latest.ActiveGeneration + 1
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return service.AiGenerateAccepted{}, normalizeError(err)
		}

		jobID := uuid.NewV7()
		refType := input.RefType
		refName := input.RefName
		row, err := queries.CreateAiGenerationJob(ctx, generated.CreateAiGenerationJobParams{
			TenantID: input.TenantID, ID: jobID, AssetID: new(input.AssetID),
			RefType: new(refType), RefName: new(refName), JobInput: jobInput,
			DedupeKey: dedupeKey, ActiveGeneration: generation,
		})
		if err == nil {
			inserted, err := store.riverClient.InsertTx(ctx, tx, task.AiGenerateArgs{
				TenantID: input.TenantID, JobID: row.ID, AssetID: input.AssetID,
			}, &river.InsertOpts{MaxAttempts: int(row.MaxAttempts)})
			if err != nil {
				return service.AiGenerateAccepted{}, fmt.Errorf("insert River AI generation job: %w", err)
			}
			if changed, err := queries.AttachRiverJobID(ctx, generated.AttachRiverJobIDParams{
				TenantID: input.TenantID, ID: row.ID, RiverJobID: new(inserted.Job.ID), UpdatedAt: timestamp(time.Now().UTC()),
			}); err != nil {
				return service.AiGenerateAccepted{}, normalizeError(err)
			} else if changed != 1 {
				return service.AiGenerateAccepted{}, fmt.Errorf("attach River job %d to domain job %s: %w", inserted.Job.ID, row.ID, service.ErrPrecondition)
			}
			accepted.JobID = row.ID
			accepted.Deduplicated = false
			if input.IdempotencyKey != uuid.Nil() {
				if err := saveAiGenerationReplay(ctx, queries, input, accepted); err != nil {
					return service.AiGenerateAccepted{}, err
				}
			}
			if err := tx.Commit(ctx); err != nil {
				return service.AiGenerateAccepted{}, normalizeError(err)
			}
			return accepted, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return service.AiGenerateAccepted{}, normalizeError(err)
		}
	}
}

// GetAiGenerationJobContext returns the durable job input for the worker.
func (store *AIStore) GetAiGenerationJobContext(ctx context.Context, tenantID, jobID uuid.UUID) (service.AiGenerationJobContext, error) {
	row, err := store.queries.GetJobInput(ctx, generated.GetJobInputParams{TenantID: tenantID, ID: jobID})
	if err != nil {
		return service.AiGenerationJobContext{}, normalizeError(err)
	}
	var jobContext service.AiGenerationJobContext
	if err := json.Unmarshal(row, &jobContext); err != nil {
		return service.AiGenerationJobContext{}, fmt.Errorf("decode AI generation job context: %w", err)
	}
	return jobContext, nil
}

func aiGenerationDedupeKey(assetID uuid.UUID) string {
	return "ai:" + assetID.String()
}

func aiGenerationIdempotencyLockKey(input service.AiGenerateEnqueue) string {
	return "generateMissingAssetWithAi:" + input.TenantID.String() + ":" + input.PrincipalType + ":" + input.PrincipalID.String() + ":" + input.IdempotencyKey.String()
}

func loadAiGenerationReplay(ctx context.Context, queries *generated.Queries, input service.AiGenerateEnqueue) (service.AiGenerateAccepted, bool, error) {
	params := generated.GetAiGenerationIdempotencyParams{
		TenantID: input.TenantID, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID, IdempotencyKey: input.IdempotencyKey,
	}
	row, err := queries.GetAiGenerationIdempotency(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return service.AiGenerateAccepted{}, false, nil
	}
	if err != nil {
		return service.AiGenerateAccepted{}, false, normalizeError(err)
	}
	if !row.ExpiresAt.Valid || !row.ExpiresAt.Time.After(time.Now().UTC()) {
		if err := queries.DeleteAiGenerationIdempotency(ctx, generated.DeleteAiGenerationIdempotencyParams{
			TenantID: input.TenantID, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID, IdempotencyKey: input.IdempotencyKey,
		}); err != nil {
			return service.AiGenerateAccepted{}, false, normalizeError(err)
		}
		return service.AiGenerateAccepted{}, false, nil
	}
	if len(input.RequestHash) != 32 || string(row.RequestHash) != string(input.RequestHash) {
		return service.AiGenerateAccepted{}, false, service.ErrIdempotencyConflict
	}
	var replay aiGenerationReplay
	if err := json.Unmarshal(row.ResponseBody, &replay); err != nil || replay.JobID == uuid.Nil() {
		return service.AiGenerateAccepted{}, false, errors.New("invalid AI generation idempotency record")
	}
	return service.AiGenerateAccepted{AssetID: replay.AssetID, SourceID: replay.SourceID, JobID: replay.JobID, Deduplicated: replay.Deduplicated}, true, nil
}

type aiGenerationReplay struct {
	AssetID      uuid.UUID `json:"assetId"`
	SourceID     uuid.UUID `json:"sourceId"`
	JobID        uuid.UUID `json:"jobId"`
	Deduplicated bool      `json:"deduplicated"`
}

func saveAiGenerationReplay(ctx context.Context, queries *generated.Queries, input service.AiGenerateEnqueue, accepted service.AiGenerateAccepted) error {
	body, err := json.Marshal(aiGenerationReplay{AssetID: accepted.AssetID, SourceID: accepted.SourceID, JobID: accepted.JobID, Deduplicated: accepted.Deduplicated})
	if err != nil {
		return fmt.Errorf("encode AI generation replay: %w", err)
	}
	return normalizeError(queries.CreateAiGenerationIdempotency(ctx, generated.CreateAiGenerationIdempotencyParams{
		TenantID: input.TenantID, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID,
		IdempotencyKey: input.IdempotencyKey, RequestHash: append([]byte(nil), input.RequestHash...), ResponseBody: body,
	}))
}

// GetAiGenerationResult returns one durable AI generation outcome.
func (store *AIStore) GetAiGenerationResult(ctx context.Context, tenantID, jobID uuid.UUID) (service.AiGenerationOutcome, error) {
	row, err := store.queries.GetAiGenerationResult(ctx, generated.GetAiGenerationResultParams{TenantID: tenantID, JobID: jobID})
	if err != nil {
		return service.AiGenerationOutcome{}, normalizeError(err)
	}
	return aiGenerationOutcomeFromRow(row), nil
}

// UpsertAiGenerationResult persists one AI generation outcome.
func (store *AIStore) UpsertAiGenerationResult(ctx context.Context, tenantID uuid.UUID, outcome service.AiGenerationOutcome) error {
	manifest, err := json.Marshal(outcome.Manifest)
	if err != nil {
		return fmt.Errorf("encode AI generation manifest: %w", err)
	}
	_, err = store.queries.UpsertAiGenerationResult(ctx, generated.UpsertAiGenerationResultParams{
		TenantID: tenantID, ID: uuid.NewV7(), JobID: outcome.JobID, Stage: outcome.Stage, Status: outcome.Status,
		ErrorCode: outcome.ErrorCode, ContentRef: outcome.ContentRef, ContentHash: outcome.ContentHash, ContentType: outcome.ContentType,
		Manifest: manifest, RevisionID: outcome.RevisionID,
	})
	return normalizeError(err)
}

// GetLayerRevisionForReview returns one revision enriched with layer/asset context.
func (store *AIStore) GetLayerRevisionForReview(ctx context.Context, tenantID, id uuid.UUID) (service.ReviewRevisionRecord, error) {
	row, err := store.queries.GetLayerRevisionForReview(ctx, generated.GetLayerRevisionForReviewParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.ReviewRevisionRecord{}, normalizeError(err)
	}
	return service.ReviewRevisionRecord{
		LayerRevisionRecord: service.LayerRevisionRecord{
			ID: row.ID, LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
			ContentHash: row.ContentHash, ContentRef: row.ContentRef, ContentType: row.ContentType, Dialect: row.Dialect,
			ReviewStatus: row.ReviewStatus, GitCommit: row.GitCommit, SourceBranch: row.SourceBranch,
			CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
		},
		AssetID: row.AssetID, Role: row.Role, Origin: row.Origin,
	}, nil
}

// ListEnabledLayerHeadsForAssetScope lists heads with candidate pointers for publish gating.
func (store *AIStore) ListEnabledLayerHeadsForAssetScope(ctx context.Context, tenantID, assetID uuid.UUID, scopeType, scopeKey string) ([]service.LayerHeadRecord, error) {
	rows, err := store.queries.ListEnabledLayerHeadsForAssetScope(ctx, generated.ListEnabledLayerHeadsForAssetScopeParams{
		TenantID: tenantID, AssetID: assetID, ScopeType: scopeType, ScopeKey: scopeKey,
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	heads := make([]service.LayerHeadRecord, 0, len(rows))
	for _, row := range rows {
		heads = append(heads, layerHeadFromRow(row))
	}
	return heads, nil
}

// UpdateLayerRevisionReview marks a pending revision approved or rejected.
func (store *AIStore) UpdateLayerRevisionReview(ctx context.Context, tenantID, id uuid.UUID, status string, comment *string) (service.LayerRevisionRecord, error) {
	row, err := store.queries.UpdateLayerRevisionReview(ctx, generated.UpdateLayerRevisionReviewParams{
		TenantID: tenantID, ID: id, ReviewStatus: status, ReviewComment: comment,
	})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return layerRevisionFromRow(row), nil
}

// SupersedeLayerRevision marks a pending revision superseded.
func (store *AIStore) SupersedeLayerRevision(ctx context.Context, tenantID, id uuid.UUID) error {
	if _, err := store.queries.SupersedeLayerRevision(ctx, generated.SupersedeLayerRevisionParams{TenantID: tenantID, ID: id}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// UnpublishAssetVersion demotes a published version to draft.
func (store *AIStore) UnpublishAssetVersion(ctx context.Context, tenantID, versionID uuid.UUID) error {
	if _, err := store.queries.UnpublishAssetVersion(ctx, generated.UnpublishAssetVersionParams{TenantID: tenantID, ID: versionID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// LockAssetRefTrack locks a track row for the publish transaction.
func (store *AIStore) LockAssetRefTrack(ctx context.Context, tenantID, trackID uuid.UUID) (service.AssetRefTrackRecord, error) {
	row, err := store.queries.LockAssetRefTrack(ctx, generated.LockAssetRefTrackParams{TenantID: tenantID, ID: trackID})
	if err != nil {
		return service.AssetRefTrackRecord{}, normalizeError(err)
	}
	return service.AssetRefTrackRecord{
		ID: row.ID, AssetID: row.AssetID, RefType: row.RefType, RefName: row.RefName,
		LatestVersionID: row.LatestVersionID, CurrentVersionID: row.CurrentVersionID, Health: row.Health,
	}, nil
}

// BumpAssetRefTrackGeneration increments the track desired generation.
func (store *AIStore) BumpAssetRefTrackGeneration(ctx context.Context, tenantID, trackID uuid.UUID) error {
	if _, err := store.queries.BumpAssetRefTrackGeneration(ctx, generated.BumpAssetRefTrackGenerationParams{TenantID: tenantID, ID: trackID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// UpdateAssetVersionPublish publishes a version under optimistic concurrency.
func (store *AIStore) UpdateAssetVersionPublish(ctx context.Context, tenantID, versionID uuid.UUID, version string, expectedRevision int64) (service.AssetVersionRecord, error) {
	row, err := store.queries.UpdateAssetVersionPublish(ctx, generated.UpdateAssetVersionPublishParams{
		TenantID: tenantID, ID: versionID, Version: version, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

func aiGenerationOutcomeFromRow(row generated.AiGenerationResult) service.AiGenerationOutcome {
	var manifest map[string]any
	if len(row.Manifest) > 0 {
		_ = json.Unmarshal(row.Manifest, &manifest)
	}
	if manifest == nil {
		manifest = map[string]any{}
	}
	return service.AiGenerationOutcome{
		JobID: row.JobID, Stage: row.Stage, Status: row.Status, ErrorCode: row.ErrorCode,
		ContentRef: row.ContentRef, ContentHash: row.ContentHash, ContentType: row.ContentType,
		Manifest: manifest, RevisionID: row.RevisionID,
	}
}

var _ service.AiGenerationStore = (*AIStore)(nil)
