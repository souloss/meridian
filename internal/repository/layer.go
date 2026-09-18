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

// LayerStore 在生成的 repository 查询之上实现 M2 层编辑持久化边界。
// 它内嵌 AssetStore 复用 asset/track/version 读取路径，并在每次读取时保留租户谓词。
type LayerStore struct {
	*AssetStore
	queries     *generated.Queries
	riverClient *river.Client[pgx.Tx]
}

// NewLayerStore 将层编辑持久化绑定到原生 pgx 连接池。
func NewLayerStore(pool *pgxpool.Pool) *LayerStore {
	return &LayerStore{AssetStore: NewAssetStore(pool), queries: generated.New(pool)}
}

// BindRiver 挂接进程 River 客户端，使合并任务可与领域行在同一事务内入队。
func (store *LayerStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
}

// GetLayer 返回租户边界内的一条活跃层。
func (store *LayerStore) GetLayer(ctx context.Context, tenantID, id uuid.UUID) (service.LayerRecord, error) {
	row, err := store.queries.GetLayer(ctx, generated.GetLayerParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

// ListLayersForAsset 返回按 ord 排序的某一资产的全部活跃层。
func (store *LayerStore) ListLayersForAsset(ctx context.Context, tenantID, assetID uuid.UUID) ([]service.LayerRecord, error) {
	rows, err := store.queries.ListLayersForAsset(ctx, generated.ListLayersForAssetParams{TenantID: tenantID, AssetID: assetID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.LayerRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, layerFromRow(row))
	}
	return items, nil
}

// UpdateLayerOrd 在其修订号下将一层移动到新的 ord，成功后递增修订号。
func (store *LayerStore) UpdateLayerOrd(ctx context.Context, tenantID, id uuid.UUID, expectedRevision int64, ord int) (service.LayerRecord, error) {
	row, err := store.queries.UpdateLayerOrd(ctx, generated.UpdateLayerOrdParams{
		TenantID: tenantID, ID: id, ExpectedRevision: expectedRevision, Ord: int32(ord),
	})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

// GetLayerRevision 返回租户内的一条不可变层修订。
func (store *LayerStore) GetLayerRevision(ctx context.Context, tenantID, id uuid.UUID) (service.LayerRevisionRecord, error) {
	row, err := store.queries.GetLayerRevision(ctx, generated.GetLayerRevisionParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return layerRevisionFromRow(row), nil
}

// GetLayerHead 返回租户边界内的一条层头指针。
func (store *LayerStore) GetLayerHead(ctx context.Context, tenantID, layerID uuid.UUID, scopeType, scopeKey string) (service.LayerHeadRecord, error) {
	row, err := store.queries.GetLayerHead(ctx, generated.GetLayerHeadParams{
		TenantID: tenantID, LayerID: layerID, ScopeType: scopeType, ScopeKey: scopeKey,
	})
	if err != nil {
		return service.LayerHeadRecord{}, normalizeError(err)
	}
	return layerHeadFromRow(row), nil
}

// GetAssetRefTrackByID 按其 id 返回一条资产 ref track。
func (store *LayerStore) GetAssetRefTrackByID(ctx context.Context, tenantID, id uuid.UUID) (service.AssetRefTrackRecord, error) {
	row, err := store.queries.GetAssetRefTrackByID(ctx, generated.GetAssetRefTrackByIDParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.AssetRefTrackRecord{}, normalizeError(err)
	}
	return service.AssetRefTrackRecord{ID: row.ID, AssetID: row.AssetID, RefType: row.RefType, RefName: row.RefName, LatestVersionID: row.LatestVersionID, CurrentVersionID: row.CurrentVersionID, Health: row.Health}, nil
}

// UpdateLayerHeadPointers 更新层头指针并递增代次。
func (store *LayerStore) UpdateLayerHeadPointers(ctx context.Context, input service.NewLayerHead) (service.LayerHeadRecord, error) {
	row, err := store.queries.UpdateLayerHeadPointers(ctx, generated.UpdateLayerHeadPointersParams{
		TenantID: input.TenantID, LayerID: input.LayerID, ScopeType: input.ScopeType, ScopeKey: input.ScopeKey,
		LatestRevisionID: input.LatestRevisionID, EffectiveRevisionID: input.EffectiveRevisionID, CandidateRevisionID: input.CandidateRevisionID,
	})
	if err != nil {
		return service.LayerHeadRecord{}, normalizeError(err)
	}
	return layerHeadFromRow(row), nil
}

// EnqueueMergeJob 原子地记录一条 asset.merge 任务及其 River 工作。
func (store *LayerStore) EnqueueMergeJob(ctx context.Context, input service.MergeJobInput) (service.JobAccepted, error) {
	dedupeKey := mergeDedupeKey(input.TrackID)
	jobInput, err := json.Marshal(struct {
		TrackID string `json:"trackId"`
	}{TrackID: input.TrackID.String()})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("encode merge job input: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("begin merge job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)

	for {
		latest, err := queries.LockLatestDiscoveryJob(ctx, generated.LockLatestDiscoveryJobParams{TenantID: input.TenantID, DedupeKey: dedupeKey})
		generation := jobGenerationInitial
		if err == nil {
			if latest.Status == service.JobStatusPending || latest.Status == service.JobStatusRunning {
				return service.JobAccepted{JobID: latest.ID, Deduplicated: true}, nil
			}
			generation = latest.ActiveGeneration + 1
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return service.JobAccepted{}, normalizeError(err)
		}

		jobID := uuid.NewV7()
		row, err := queries.CreateMergeJob(ctx, generated.CreateMergeJobParams{
			TenantID: input.TenantID, ID: jobID, TrackID: new(input.TrackID),
			JobInput: jobInput, DedupeKey: dedupeKey, ActiveGeneration: generation,
		})
		if err == nil {
			if store.riverClient != nil {
				inserted, err := store.riverClient.InsertTx(ctx, tx, task.MergeArgs{
					TenantID: input.TenantID, JobID: row.ID, TrackID: input.TrackID,
				}, &river.InsertOpts{MaxAttempts: int(row.MaxAttempts)})
				if err != nil {
					return service.JobAccepted{}, fmt.Errorf("insert River merge job: %w", err)
				}
				if changed, err := queries.AttachRiverJobID(ctx, generated.AttachRiverJobIDParams{
					TenantID: input.TenantID, ID: row.ID, RiverJobID: new(inserted.Job.ID), UpdatedAt: timestamp(time.Now().UTC()),
				}); err != nil {
					return service.JobAccepted{}, normalizeError(err)
				} else if changed != rowsAffectedOne {
					return service.JobAccepted{}, fmt.Errorf("attach River job %d to domain job %s: %w", inserted.Job.ID, row.ID, service.ErrPrecondition)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				return service.JobAccepted{}, normalizeError(err)
			}
			return service.JobAccepted{JobID: row.ID, Deduplicated: false}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return service.JobAccepted{}, normalizeError(err)
		}
	}
}

func mergeDedupeKey(trackID uuid.UUID) string {
	return dedupeKeyPrefixMerge + trackID.String()
}

func layerRevisionFromRow(row generated.LayerRevision) service.LayerRevisionRecord {
	return service.LayerRevisionRecord{
		ID: row.ID, LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		ContentHash: row.ContentHash, ContentRef: row.ContentRef, ContentType: row.ContentType, Dialect: row.Dialect,
		ReviewStatus: row.ReviewStatus, GitCommit: row.GitCommit, SourceBranch: row.SourceBranch,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
	}
}

func layerHeadFromRow(row generated.LayerHead) service.LayerHeadRecord {
	return service.LayerHeadRecord{
		LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		LatestRevisionID: row.LatestRevisionID, EffectiveRevisionID: row.EffectiveRevisionID,
		CandidateRevisionID: row.CandidateRevisionID, Generation: row.Generation,
	}
}

var _ service.LayerEditStore = (*LayerStore)(nil)
