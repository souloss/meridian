package repository

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/task"
	"github.com/riverqueue/river"
)

// DiffStore implements the M3 diff, snapshot share, breaking todo, upload, and
// push persistence boundary. It embeds AssetStore for the version/track read
// path and keeps the tenant predicate on every read.
type DiffStore struct {
	*AssetStore
	queries     *generated.Queries
	riverClient *river.Client[pgx.Tx]
}

// NewDiffStore binds diff persistence to a native pgx pool.
func NewDiffStore(pool *pgxpool.Pool) *DiffStore {
	return &DiffStore{AssetStore: NewAssetStore(pool), queries: generated.New(pool)}
}

// BindRiver attaches the process River client so push merge jobs can be
// enqueued transactionally with their domain rows.
func (store *DiffStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
}

// CreateSourceSpec inserts one push source spec.
func (store *DiffStore) CreateSourceSpec(ctx context.Context, input service.NewSourceSpec) (service.SourceSpecRecord, error) {
	row, err := store.queries.CreateSourceSpec(ctx, generated.CreateSourceSpecParams{
		TenantID: input.TenantID, ID: input.ID, ServiceID: input.ServiceID, Kind: input.Kind,
		AssetNameTemplate: input.AssetNameTemplate, Role: input.Role, Origin: input.Origin, Mode: input.Mode,
		Path: input.Path, ProducerProfileID: input.ProducerProfileID, Ord: int32(input.Ord), TimeoutSec: int32(input.TimeoutSec),
		BranchPatterns: input.BranchPatterns, Enabled: input.Enabled, ConfigOrigin: input.ConfigOrigin,
	})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	return sourceSpecFromRow(row, 0), nil
}

// CreateLayer inserts one push layer.
func (store *DiffStore) CreateLayer(ctx context.Context, input service.NewLayer) (service.LayerRecord, error) {
	row, err := store.queries.CreateLayer(ctx, generated.CreateLayerParams{
		TenantID: input.TenantID, ID: input.ID, AssetID: input.AssetID, SourceSpecID: input.SourceSpecID,
		Role: input.Role, Origin: input.Origin, Ord: int32(input.Ord), Dialect: input.Dialect, Enabled: input.Enabled,
		BranchPatterns: input.BranchPatterns, DisplayName: input.DisplayName,
	})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

// EnqueueMergeJob records one asset.merge job for a pushed revision.
func (store *DiffStore) EnqueueMergeJob(ctx context.Context, input service.MergeJobInput) (service.JobAccepted, error) {
	if store.riverClient == nil {
		return service.JobAccepted{}, errors.New("diff store has no River client")
	}
	dedupeKey := "merge:" + input.TrackID.String()
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
	if err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
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
	if err := tx.Commit(ctx); err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	return service.JobAccepted{JobID: row.ID, Deduplicated: false}, nil
}

// GetServiceBySlug returns one active service by slug.
func (store *DiffStore) GetServiceBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceBySlug(ctx, generated.GetServiceBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// ListServicesByRepositoryOwners returns the service ids owning an asset.
func (store *DiffStore) ListServicesByRepositoryOwners(ctx context.Context, tenantID, assetID uuid.UUID) ([]uuid.UUID, error) {
	asset, err := store.GetAsset(ctx, tenantID, assetID)
	if err != nil {
		return nil, err
	}
	return []uuid.UUID{asset.ServiceID}, nil
}

// GetSourceSpec returns one active source configuration.
func (store *DiffStore) GetSourceSpec(ctx context.Context, tenantID, id uuid.UUID) (service.SourceSpecRecord, error) {
	row, err := store.queries.GetSourceSpec(ctx, generated.GetSourceSpecParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	return sourceSpecFromRow(row, 0), nil
}

// GetLayerRevision returns one immutable layer revision.
func (store *DiffStore) GetLayerRevision(ctx context.Context, tenantID, id uuid.UUID) (service.LayerRevisionRecord, error) {
	row, err := store.queries.GetLayerRevision(ctx, generated.GetLayerRevisionParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return layerRevisionFromRow(row), nil
}

// GetAssetRefTrackByID returns one asset ref track by its id.
func (store *DiffStore) GetAssetRefTrackByID(ctx context.Context, tenantID, id uuid.UUID) (service.AssetRefTrackRecord, error) {
	row, err := store.queries.GetAssetRefTrackByID(ctx, generated.GetAssetRefTrackByIDParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.AssetRefTrackRecord{}, normalizeError(err)
	}
	return service.AssetRefTrackRecord{
		ID: row.ID, AssetID: row.AssetID, RefType: row.RefType, RefName: row.RefName,
		LatestVersionID: row.LatestVersionID, CurrentVersionID: row.CurrentVersionID, Health: row.Health,
	}, nil
}

// CreateUpload persists one diff upload.
func (store *DiffStore) CreateUpload(ctx context.Context, input service.NewUpload) (service.UploadRecord, error) {
	row, err := store.queries.CreateUpload(ctx, generated.CreateUploadParams{
		TenantID: input.TenantID, ID: input.ID, BlobDigest: input.BlobDigest, Kind: input.Kind,
		ContentType: input.ContentType, SizeBytes: input.SizeBytes, ExpiresAt: timestamp(input.ExpiresAt), CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return service.UploadRecord{}, normalizeError(err)
	}
	return uploadFromRow(row), nil
}

// GetUpload returns one unexpired upload.
func (store *DiffStore) GetUpload(ctx context.Context, tenantID, id uuid.UUID) (service.UploadRecord, error) {
	row, err := store.queries.GetUpload(ctx, generated.GetUploadParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.UploadRecord{}, normalizeError(err)
	}
	return uploadFromRow(row), nil
}

// CreateDiffSnapshot persists one frozen diff result.
func (store *DiffStore) CreateDiffSnapshot(ctx context.Context, input service.NewDiffSnapshot) (service.DiffSnapshotRecord, error) {
	row, err := store.queries.CreateDiffSnapshot(ctx, generated.CreateDiffSnapshotParams{
		TenantID: input.TenantID, ID: input.ID, LeftSelector: input.LeftSelector, RightSelector: input.RightSelector,
		LeftArtifactRef: input.LeftArtifactRef, RightArtifactRef: input.RightArtifactRef, RuleSetID: input.RuleSetID,
		ResultRef: input.ResultRef, Summary: input.Summary, CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return service.DiffSnapshotRecord{}, normalizeError(err)
	}
	return diffSnapshotFromRow(row), nil
}

// GetDiffSnapshot returns one snapshot.
func (store *DiffStore) GetDiffSnapshot(ctx context.Context, tenantID, id uuid.UUID) (service.DiffSnapshotRecord, error) {
	row, err := store.queries.GetDiffSnapshot(ctx, generated.GetDiffSnapshotParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.DiffSnapshotRecord{}, normalizeError(err)
	}
	return diffSnapshotFromRow(row), nil
}

// CreateShareLink persists one share link.
func (store *DiffStore) CreateShareLink(ctx context.Context, input service.NewShareLink) (service.ShareLinkRecord, error) {
	row, err := store.queries.CreateShareLink(ctx, generated.CreateShareLinkParams{
		TenantID: input.TenantID, ID: input.ID, TokenHash: input.TokenHash, CreatorID: input.CreatorID,
		ResourceType: input.ResourceType, ResourceID: input.ResourceID, Descriptor: input.Descriptor,
		ViewID: input.ViewID, Options: input.Options, ArtifactAllowlist: input.ArtifactAllowlist, ExpiresAt: timestamp(input.ExpiresAt),
	})
	if err != nil {
		return service.ShareLinkRecord{}, normalizeError(err)
	}
	return shareLinkFromRow(row), nil
}

// GetShareLinkByTokenHash returns one active share link.
func (store *DiffStore) GetShareLinkByTokenHash(ctx context.Context, tokenHash []byte) (service.ShareLinkRecord, error) {
	row, err := store.queries.GetShareLinkByTokenHash(ctx, tokenHash)
	if err != nil {
		return service.ShareLinkRecord{}, normalizeError(err)
	}
	return shareLinkFromRow(row), nil
}

// CreateBreakingTodoIfAbsent creates one todo keyed by version+service.
func (store *DiffStore) CreateBreakingTodoIfAbsent(ctx context.Context, tenantID, assetVersionID, serviceID uuid.UUID) (service.TodoRecord, error) {
	row, err := store.queries.CreateBreakingTodoIfAbsent(ctx, generated.CreateBreakingTodoIfAbsentParams{
		TenantID: tenantID, ID: uuid.NewV7(), AssetVersionID: assetVersionID, ServiceID: serviceID,
	})
	if err != nil {
		return service.TodoRecord{}, normalizeError(err)
	}
	return todoFromRow(row), nil
}

// ListBreakingTodos pages todos by status.
func (store *DiffStore) ListBreakingTodos(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int32) ([]service.TodoRecord, int64, error) {
	rows, err := store.queries.ListBreakingTodos(ctx, generated.ListBreakingTodosParams{
		TenantID: tenantID, StatusFilter: status, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	total, err := store.queries.CountBreakingTodos(ctx, generated.CountBreakingTodosParams{TenantID: tenantID, StatusFilter: status})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.TodoRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, todoFromRow(row))
	}
	return items, total, nil
}

// AckBreakingTodo acknowledges one open todo.
func (store *DiffStore) AckBreakingTodo(ctx context.Context, tenantID, todoID, ackedBy uuid.UUID, ackedAt time.Time, comment *string) (service.TodoRecord, error) {
	row, err := store.queries.AckBreakingTodo(ctx, generated.AckBreakingTodoParams{
		TenantID: tenantID, ID: todoID, AckedBy: new(ackedBy), AckedAt: timestamp(ackedAt), Comment: comment,
	})
	if err != nil {
		return service.TodoRecord{}, normalizeError(err)
	}
	return todoFromRow(row), nil
}

// GetSourceLayerByPushKey returns the push-mode overlay layer of an asset by its
// stable push identity (kind + name), so repeated pushes reuse one overlay layer
// instead of aliasing the repo base.
func (store *DiffStore) GetSourceLayerByPushKey(ctx context.Context, tenantID, assetID uuid.UUID) (service.LayerRecord, error) {
	row, err := store.queries.GetSourceLayerByPushKey(ctx, generated.GetSourceLayerByPushKeyParams{
		TenantID: tenantID, AssetID: assetID,
	})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

func uploadFromRow(row generated.Upload) service.UploadRecord {
	return service.UploadRecord{
		ID: row.ID, BlobDigest: row.BlobDigest, Kind: row.Kind, ContentType: row.ContentType,
		SizeBytes: row.SizeBytes, ExpiresAt: row.ExpiresAt.Time, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
	}
}

func diffSnapshotFromRow(row generated.DiffSnapshot) service.DiffSnapshotRecord {
	return service.DiffSnapshotRecord{
		ID: row.ID, LeftSelector: row.LeftSelector, RightSelector: row.RightSelector,
		LeftArtifactRef: row.LeftArtifactRef, RightArtifactRef: row.RightArtifactRef, RuleSetID: row.RuleSetID,
		ResultRef: row.ResultRef, Summary: row.Summary, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
	}
}

func shareLinkFromRow(row generated.ShareLink) service.ShareLinkRecord {
	return service.ShareLinkRecord{
		TenantID: row.TenantID, ID: row.ID, TokenHash: row.TokenHash, CreatorID: row.CreatorID, ResourceType: row.ResourceType,
		ResourceID: row.ResourceID, Descriptor: row.Descriptor, ViewID: row.ViewID, Options: row.Options,
		ArtifactAllowlist: row.ArtifactAllowlist, ExpiresAt: row.ExpiresAt.Time, RevokedAt: timePtr(row.RevokedAt), CreatedAt: row.CreatedAt.Time,
	}
}

func todoFromRow(row generated.BreakingTodo) service.TodoRecord {
	var ackedAt *time.Time
	if row.AckedAt.Valid {
		t := row.AckedAt.Time
		ackedAt = &t
	}
	return service.TodoRecord{
		ID: row.ID, AssetVersionID: row.AssetVersionID, ServiceID: row.ServiceID, Status: row.Status,
		AckedBy: row.AckedBy, AckedAt: ackedAt, Comment: row.Comment, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

var _ service.DiffStore = (*DiffStore)(nil)
var _ = fmt.Sprintf
var _ = json.Marshal

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
