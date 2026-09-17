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

// LayerStore implements the M2 layer editing persistence boundary on top of the
// generated repository queries. It embeds AssetStore to reuse the asset/track/
// version read path and keeps the tenant predicate on every read.
type LayerStore struct {
	*AssetStore
	queries     *generated.Queries
	riverClient *river.Client[pgx.Tx]
}

// NewLayerStore binds layer editing persistence to a native pgx pool.
func NewLayerStore(pool *pgxpool.Pool) *LayerStore {
	return &LayerStore{AssetStore: NewAssetStore(pool), queries: generated.New(pool)}
}

// BindRiver attaches the process River client so merge jobs can be enqueued
// transactionally with their domain rows.
func (store *LayerStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
}

// GetLayer returns one active layer within the tenant boundary.
func (store *LayerStore) GetLayer(ctx context.Context, tenantID, id uuid.UUID) (service.LayerRecord, error) {
	row, err := store.queries.GetLayer(ctx, generated.GetLayerParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

// ListLayersForAsset returns every active layer of one asset ordered by ord.
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

// UpdateLayerOrd moves one layer to a new ord under its revision, incrementing
// the revision on success.
func (store *LayerStore) UpdateLayerOrd(ctx context.Context, tenantID, id uuid.UUID, expectedRevision int64, ord int) (service.LayerRecord, error) {
	row, err := store.queries.UpdateLayerOrd(ctx, generated.UpdateLayerOrdParams{
		TenantID: tenantID, ID: id, ExpectedRevision: expectedRevision, Ord: int32(ord),
	})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

// GetLayerRevision returns one immutable layer revision within the tenant.
func (store *LayerStore) GetLayerRevision(ctx context.Context, tenantID, id uuid.UUID) (service.LayerRevisionRecord, error) {
	row, err := store.queries.GetLayerRevision(ctx, generated.GetLayerRevisionParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return layerRevisionFromRow(row), nil
}

// GetLayerHead returns one layer head pointer within the tenant boundary.
func (store *LayerStore) GetLayerHead(ctx context.Context, tenantID, layerID uuid.UUID, scopeType, scopeKey string) (service.LayerHeadRecord, error) {
	row, err := store.queries.GetLayerHead(ctx, generated.GetLayerHeadParams{
		TenantID: tenantID, LayerID: layerID, ScopeType: scopeType, ScopeKey: scopeKey,
	})
	if err != nil {
		return service.LayerHeadRecord{}, normalizeError(err)
	}
	return layerHeadFromRow(row), nil
}

// GetAssetRefTrackByID returns one asset ref track by its id.
func (store *LayerStore) GetAssetRefTrackByID(ctx context.Context, tenantID, id uuid.UUID) (service.AssetRefTrackRecord, error) {
	row, err := store.queries.GetAssetRefTrackByID(ctx, generated.GetAssetRefTrackByIDParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.AssetRefTrackRecord{}, normalizeError(err)
	}
	return service.AssetRefTrackRecord{ID: row.ID, AssetID: row.AssetID, RefType: row.RefType, RefName: row.RefName, LatestVersionID: row.LatestVersionID, CurrentVersionID: row.CurrentVersionID, Health: row.Health}, nil
}

// UpdateLayerHeadPointers updates the head pointers and increments generation.
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

// EnqueueMergeJob records one asset.merge job and its River work atomically.
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
		generation := int64(1)
		if err == nil {
			if latest.Status == "pending" || latest.Status == "running" {
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
				} else if changed != 1 {
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
	return "merge:" + trackID.String()
}

func layerRevisionFromRow(row generated.LayerRevision) service.LayerRevisionRecord {
	return service.LayerRevisionRecord{
		ID: row.ID, LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		ContentHash: row.ContentHash, ContentRef: row.ContentRef, ContentType: row.ContentType, Dialect: row.Dialect,
		ReviewStatus: row.ReviewStatus, GitCommit: row.GitCommit, SourceBranch: row.SourceBranch,
		CreatedAt: row.CreatedAt.Time,
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
