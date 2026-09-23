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

// DiffStore 实现 M3 的差异、快照分享、破坏性待办、上传与推送持久化边界。
// 它内嵌 AssetStore 复用版本/track 读取路径，并在每次读取时保留租户谓词。
type DiffStore struct {
	*AssetStore
	queries     *generated.Queries
	riverClient *river.Client[pgx.Tx]
}

// NewDiffStore 将差异持久化绑定到原生 pgx 连接池。
func NewDiffStore(pool *pgxpool.Pool) *DiffStore {
	return &DiffStore{AssetStore: NewAssetStore(pool), queries: generated.New(pool)}
}

// BindRiver 挂接进程 River 客户端，使推送合并任务可与领域行在同一事务内入队。
func (store *DiffStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
}

// CreateSourceSpec 插入一条推送源配置。
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

// CreateLayer 插入一条推送层。
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

// EnqueueMergeJob 为一次推送修订记录一条 asset.merge 任务。
func (store *DiffStore) EnqueueMergeJob(ctx context.Context, input service.MergeJobInput) (service.JobAccepted, error) {
	if store.riverClient == nil {
		return service.JobAccepted{}, errors.New("diff store has no River client")
	}
	dedupeKey := dedupeKeyPrefixMerge + input.TrackID.String()
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
	} else if changed != rowsAffectedOne {
		return service.JobAccepted{}, fmt.Errorf("attach River job %d to domain job %s: %w", inserted.Job.ID, row.ID, service.ErrPrecondition)
	}
	if err := tx.Commit(ctx); err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	return service.JobAccepted{JobID: row.ID, Deduplicated: false}, nil
}

// GetServiceBySlug 按 slug 返回一条活跃服务。
func (store *DiffStore) GetServiceBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceBySlug(ctx, generated.GetServiceBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// ListServicesByRepositoryOwners 返回拥有某资产的服务 id。
func (store *DiffStore) ListServicesByRepositoryOwners(ctx context.Context, tenantID, assetID uuid.UUID) ([]uuid.UUID, error) {
	asset, err := store.GetAsset(ctx, tenantID, assetID)
	if err != nil {
		return nil, err
	}
	return []uuid.UUID{asset.ServiceID}, nil
}

// GetSourceSpec 返回一条活跃源配置。
func (store *DiffStore) GetSourceSpec(ctx context.Context, tenantID, id uuid.UUID) (service.SourceSpecRecord, error) {
	row, err := store.queries.GetSourceSpec(ctx, generated.GetSourceSpecParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	return sourceSpecFromRow(row, 0), nil
}

// GetLayerRevision 返回一条不可变的层修订。
func (store *DiffStore) GetLayerRevision(ctx context.Context, tenantID, id uuid.UUID) (service.LayerRevisionRecord, error) {
	row, err := store.queries.GetLayerRevision(ctx, generated.GetLayerRevisionParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return layerRevisionFromRow(row), nil
}

// GetAssetRefTrackByID 按其 id 返回一条资产 ref track。
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

// CreateUpload 持久化一次差异上传。
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

// GetUpload 返回一次未过期的上传。
func (store *DiffStore) GetUpload(ctx context.Context, tenantID, id uuid.UUID) (service.UploadRecord, error) {
	row, err := store.queries.GetUpload(ctx, generated.GetUploadParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.UploadRecord{}, normalizeError(err)
	}
	return uploadFromRow(row), nil
}

// CreateDiffSnapshot 持久化一份冻结的差异结果。
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

// GetDiffSnapshot 返回一份快照。
func (store *DiffStore) GetDiffSnapshot(ctx context.Context, tenantID, id uuid.UUID) (service.DiffSnapshotRecord, error) {
	row, err := store.queries.GetDiffSnapshot(ctx, generated.GetDiffSnapshotParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.DiffSnapshotRecord{}, normalizeError(err)
	}
	return diffSnapshotFromRow(row), nil
}

// CreateShareLink 持久化一条分享链接。
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

// GetShareLinkByTokenHash 返回一条活跃分享链接。
func (store *DiffStore) GetShareLinkByTokenHash(ctx context.Context, tokenHash []byte) (service.ShareLinkRecord, error) {
	row, err := store.queries.GetShareLinkByTokenHash(ctx, tokenHash)
	if err != nil {
		return service.ShareLinkRecord{}, normalizeError(err)
	}
	return shareLinkFromRow(row), nil
}

// CreateBreakingTodoIfAbsent 按 version+service 键创建一条待办。
func (store *DiffStore) CreateBreakingTodoIfAbsent(ctx context.Context, tenantID, assetVersionID, serviceID uuid.UUID) (service.TodoRecord, error) {
	row, err := store.queries.CreateBreakingTodoIfAbsent(ctx, generated.CreateBreakingTodoIfAbsentParams{
		TenantID: tenantID, ID: uuid.NewV7(), AssetVersionID: assetVersionID, ServiceID: serviceID,
	})
	if err != nil {
		return service.TodoRecord{}, normalizeError(err)
	}
	return todoFromRow(row), nil
}

// ListBreakingTodos 按状态分页返回待办。
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

// AckBreakingTodo 确认一条未关闭的待办。
func (store *DiffStore) AckBreakingTodo(ctx context.Context, tenantID, todoID, ackedBy uuid.UUID, ackedAt time.Time, comment *string) (service.TodoRecord, error) {
	row, err := store.queries.AckBreakingTodo(ctx, generated.AckBreakingTodoParams{
		TenantID: tenantID, ID: todoID, AckedBy: new(ackedBy), AckedAt: timestamp(ackedAt), Comment: comment,
	})
	if err != nil {
		return service.TodoRecord{}, normalizeError(err)
	}
	return todoFromRow(row), nil
}

// GetSourceLayerByPushKey 按其稳定推送身份（kind + name）返回资产的推送模式 overlay 层，
// 使重复推送复用同一 overlay 层，而非给仓库 base 起别名。
func (store *DiffStore) GetSourceLayerByPushKey(ctx context.Context, tenantID, assetID uuid.UUID) (service.LayerRecord, error) {
	row, err := store.queries.GetSourceLayerByPushKey(ctx, generated.GetSourceLayerByPushKeyParams{
		TenantID: tenantID, AssetID: assetID,
	})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

// ListDiffRuleSets 返回租户内全部差异规则集。
func (store *DiffStore) ListDiffRuleSets(ctx context.Context, tenantID uuid.UUID) ([]service.DiffRuleSetRecord, error) {
	rows, err := store.queries.ListDiffRuleSets(ctx, tenantID)
	if err != nil {
		return nil, normalizeError(err)
	}
	records := make([]service.DiffRuleSetRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, diffRuleSetFromRow(row))
	}
	return records, nil
}

// GetDiffRuleSet 返回一条差异规则集。
func (store *DiffStore) GetDiffRuleSet(ctx context.Context, tenantID, id uuid.UUID) (service.DiffRuleSetRecord, error) {
	row, err := store.queries.GetDiffRuleSet(ctx, generated.GetDiffRuleSetParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.DiffRuleSetRecord{}, normalizeError(err)
	}
	return diffRuleSetFromRow(row), nil
}

// CreateDiffRuleSet 创建一条差异规则集。
func (store *DiffStore) CreateDiffRuleSet(ctx context.Context, input service.NewDiffRuleSet) (service.DiffRuleSetRecord, error) {
	row, err := store.queries.CreateDiffRuleSet(ctx, generated.CreateDiffRuleSetParams{
		TenantID: input.TenantID, ID: input.ID, Kind: input.Kind, Name: input.Name,
		Version: input.Version, Rules: input.Rules, Enabled: input.Enabled,
	})
	if err != nil {
		return service.DiffRuleSetRecord{}, normalizeError(err)
	}
	return diffRuleSetFromRow(row), nil
}

// UpdateDiffRuleSet 在 If-Match 下更新一条差异规则集。
func (store *DiffStore) UpdateDiffRuleSet(ctx context.Context, tenantID, id uuid.UUID, expectedRevision int64, patch service.DiffRuleSetPatch) (service.DiffRuleSetRecord, error) {
	row, err := store.queries.UpdateDiffRuleSet(ctx, generated.UpdateDiffRuleSetParams{
		Name: patch.Name, Rules: patch.Rules, Enabled: patch.Enabled,
		TenantID: tenantID, ID: id, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.DiffRuleSetRecord{}, service.ErrPrecondition
		}
		return service.DiffRuleSetRecord{}, normalizeError(err)
	}
	return diffRuleSetFromRow(row), nil
}

// DeleteDiffRuleSet 在 If-Match 下删除一条差异规则集。
func (store *DiffStore) DeleteDiffRuleSet(ctx context.Context, tenantID, id uuid.UUID, expectedRevision int64) error {
	changed, err := store.queries.DeleteDiffRuleSet(ctx, generated.DeleteDiffRuleSetParams{
		TenantID: tenantID, ID: id, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrPrecondition
	}
	return nil
}

// ListDiffSnapshots 返回租户内一页差异快照。
func (store *DiffStore) ListDiffSnapshots(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]service.DiffSnapshotRecord, int64, error) {
	rows, err := store.queries.ListDiffSnapshots(ctx, generated.ListDiffSnapshotsParams{
		TenantID: tenantID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	total, err := store.queries.CountDiffSnapshots(ctx, tenantID)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	records := make([]service.DiffSnapshotRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, diffSnapshotFromRow(row))
	}
	return records, total, nil
}

// DeleteDiffSnapshot 删除一条差异快照。
func (store *DiffStore) DeleteDiffSnapshot(ctx context.Context, tenantID, id uuid.UUID) error {
	changed, err := store.queries.DeleteDiffSnapshot(ctx, generated.DeleteDiffSnapshotParams{TenantID: tenantID, ID: id})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrNotFound
	}
	return nil
}

// ListShareLinks 返回租户内一页分享链接。
func (store *DiffStore) ListShareLinks(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]service.ShareLinkRecord, int64, error) {
	rows, err := store.queries.ListShareLinks(ctx, generated.ListShareLinksParams{
		TenantID: tenantID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	total, err := store.queries.CountShareLinks(ctx, tenantID)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	records := make([]service.ShareLinkRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, shareLinkFromRow(row))
	}
	return records, total, nil
}

// RevokeShareLink 幂等撤销一条分享链接。
func (store *DiffStore) RevokeShareLink(ctx context.Context, tenantID, id uuid.UUID) error {
	if _, err := store.queries.RevokeShareLink(ctx, generated.RevokeShareLinkParams{TenantID: tenantID, ID: id}); err != nil {
		return normalizeError(err)
	}
	return nil
}

func diffRuleSetFromRow(row generated.DiffRuleSet) service.DiffRuleSetRecord {
	return service.DiffRuleSetRecord{
		ID: row.ID, Kind: row.Kind, Name: row.Name, Version: row.Version, Rules: row.Rules,
		Enabled: row.Enabled, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
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

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
