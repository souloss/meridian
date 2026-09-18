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

// AIStore 实现 M3 的 AI 生成、评审与发布持久化边界。它内嵌 LayerStore 复用
// layer/head/version/track 的读写路径，并新增 AI 任务、生成结果、评审与发布查询。
type AIStore struct {
	*LayerStore
	queries     *generated.Queries
	riverClient *river.Client[pgx.Tx]
}

// NewAIStore 将 AI 生成持久化绑定到原生 pgx 连接池。
func NewAIStore(pool *pgxpool.Pool) *AIStore {
	return &AIStore{LayerStore: NewLayerStore(pool), queries: generated.New(pool)}
}

// BindRiver 挂接进程 River 客户端，使 AI 生成任务可与领域行在同一事务内入队。
func (store *AIStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
	store.LayerStore.BindRiver(riverClient)
}

// GetServiceBySlug 返回租户边界内的一个活跃服务。
func (store *AIStore) GetServiceBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceBySlug(ctx, generated.GetServiceBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// GetProducerProfile 按 id 返回一个平台生产者配置。
func (store *AIStore) GetProducerProfile(ctx context.Context, id uuid.UUID) (service.ProducerProfile, error) {
	row, err := store.queries.GetProducerProfile(ctx, id)
	if err != nil {
		return service.ProducerProfile{}, normalizeError(err)
	}
	return producerProfileFromRow(row), nil
}

// GetTenantSettings 返回租户设置 JSON 快照。
func (store *AIStore) GetTenantSettings(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	row, err := store.queries.GetTenantSettings(ctx, tenantID)
	if err != nil {
		return nil, normalizeError(err)
	}
	return row, nil
}

// EnqueueAiGeneration 原子地创建 AI 源配置、其层与生成任务，随后记录 River 工作；
// 当幂等键此前已见过时，回放既有幂等记录。
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

	// 先创建资产，再在同一事务中创建其 AI 源配置与层，
	// 使任务在其终态修订写入时携带稳定的源/层标识。
	if _, err := queries.UpsertAsset(ctx, generated.UpsertAssetParams{
		TenantID: input.TenantID, ID: input.AssetID, ServiceID: input.ServiceID, Kind: input.Kind, Name: input.Name,
	}); err != nil {
		return service.AiGenerateAccepted{}, normalizeError(err)
	}
	if _, err := queries.CreateSourceSpec(ctx, generated.CreateSourceSpecParams{
		TenantID: input.TenantID, ID: input.SourceID, ServiceID: input.ServiceID, Kind: input.Kind,
		AssetNameTemplate: input.Name, Role: input.Role, Origin: sourceOriginAI, Mode: sourceModeAI,
		Path: nil, ProducerProfileID: new(input.ProducerProfileID), Ord: int32(input.Ord),
		TimeoutSec: int32(aiSourceTimeoutSec), BranchPatterns: []string{branchGlobAll}, Enabled: true, ConfigOrigin: sourceConfigOriginAPI,
	}); err != nil {
		return service.AiGenerateAccepted{}, normalizeError(err)
	}
	if _, err := queries.CreateLayer(ctx, generated.CreateLayerParams{
		TenantID: input.TenantID, ID: input.LayerID, AssetID: input.AssetID, SourceSpecID: new(input.SourceID),
		Role: input.Role, Origin: sourceOriginAI, Ord: int32(input.Ord), Dialect: nil, Enabled: true,
		BranchPatterns: []string{branchGlobAll}, DisplayName: input.Name,
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
		generation := jobGenerationInitial
		if err == nil {
			if latest.Status == service.JobStatusPending || latest.Status == service.JobStatusRunning {
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
			} else if changed != rowsAffectedOne {
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

// GetAiGenerationJobContext 返回供 worker 使用的持久化任务输入。
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
	return dedupeKeyPrefixAI + assetID.String()
}

func aiGenerationIdempotencyLockKey(input service.AiGenerateEnqueue) string {
	return aiGenerationIdempotencyLockPrefix + input.TenantID.String() + ":" + input.PrincipalType + ":" + input.PrincipalID.String() + ":" + input.IdempotencyKey.String()
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
	if len(input.RequestHash) != sha256DigestBytes || string(row.RequestHash) != string(input.RequestHash) {
		return service.AiGenerateAccepted{}, false, service.ErrIdempotencyConflict
	}
	var replay aiGenerationReplay
	if err := json.Unmarshal(row.ResponseBody, &replay); err != nil || replay.JobID == uuid.Nil() {
		return service.AiGenerateAccepted{}, false, errors.New("invalid AI generation idempotency record")
	}
	return service.AiGenerateAccepted{AssetID: replay.AssetID, SourceID: replay.SourceID, JobID: replay.JobID, Deduplicated: replay.Deduplicated}, true, nil
}

type aiGenerationReplay struct {
	// AssetID 是回放响应中的资产标识。
	AssetID uuid.UUID `json:"assetId"`
	// SourceID 是回放响应中的源配置标识。
	SourceID uuid.UUID `json:"sourceId"`
	// JobID 是回放响应中的任务标识。
	JobID uuid.UUID `json:"jobId"`
	// Deduplicated 标识该响应来自去重回放。
	Deduplicated bool `json:"deduplicated"`
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

// GetAiGenerationResult 返回一份持久化的 AI 生成结果。
func (store *AIStore) GetAiGenerationResult(ctx context.Context, tenantID, jobID uuid.UUID) (service.AiGenerationOutcome, error) {
	row, err := store.queries.GetAiGenerationResult(ctx, generated.GetAiGenerationResultParams{TenantID: tenantID, JobID: jobID})
	if err != nil {
		return service.AiGenerationOutcome{}, normalizeError(err)
	}
	return aiGenerationOutcomeFromRow(row), nil
}

// UpsertAiGenerationResult 持久化一份 AI 生成结果。
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

// GetLayerRevisionForReview 返回一份带有层/资产上下文的修订。
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

// ListEnabledLayerHeadsForAssetScope 列出带有候选指针的层头，用于发布门禁。
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

// UpdateLayerRevisionReview 将待审核修订标记为已批准或已拒绝。
func (store *AIStore) UpdateLayerRevisionReview(ctx context.Context, tenantID, id uuid.UUID, status string, comment *string) (service.LayerRevisionRecord, error) {
	row, err := store.queries.UpdateLayerRevisionReview(ctx, generated.UpdateLayerRevisionReviewParams{
		TenantID: tenantID, ID: id, ReviewStatus: status, ReviewComment: comment,
	})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return layerRevisionFromRow(row), nil
}

// SupersedeLayerRevision 将待审核修订标记为被替换。
func (store *AIStore) SupersedeLayerRevision(ctx context.Context, tenantID, id uuid.UUID) error {
	if _, err := store.queries.SupersedeLayerRevision(ctx, generated.SupersedeLayerRevisionParams{TenantID: tenantID, ID: id}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// UnpublishAssetVersion 将已发布版本降级为草稿。
func (store *AIStore) UnpublishAssetVersion(ctx context.Context, tenantID, versionID uuid.UUID) error {
	if _, err := store.queries.UnpublishAssetVersion(ctx, generated.UnpublishAssetVersionParams{TenantID: tenantID, ID: versionID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// LockAssetRefTrack 为发布事务锁定一个 track 行。
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

// BumpAssetRefTrackGeneration 递增 track 的目标代次。
func (store *AIStore) BumpAssetRefTrackGeneration(ctx context.Context, tenantID, trackID uuid.UUID) error {
	if _, err := store.queries.BumpAssetRefTrackGeneration(ctx, generated.BumpAssetRefTrackGenerationParams{TenantID: tenantID, ID: trackID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// UpdateAssetVersionPublish 在乐观并发下发布一个版本。
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
