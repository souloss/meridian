package repository

import (
	"context"
	"encoding/json/v2"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// AssetStore 在生成的 repository 查询之上实现 M1 资产流水线持久化边界。
// 它内嵌 RepositoryStore 复用仓库读取路径，并在每次读取时保留租户谓词。
type AssetStore struct {
	*RepositoryStore
	queries *generated.Queries
}

// NewAssetStore 将资产流水线持久化绑定到原生 pgx 连接池。
func NewAssetStore(pool *pgxpool.Pool) *AssetStore {
	return &AssetStore{RepositoryStore: NewRepositoryStore(pool), queries: generated.New(pool)}
}

// GetAssetKind 返回一条平台资产 kind 注册。
func (store *AssetStore) GetAssetKind(ctx context.Context, id string) (service.AssetKindRecord, error) {
	row, err := store.queries.GetAssetKind(ctx, id)
	if err != nil {
		return service.AssetKindRecord{}, normalizeError(err)
	}
	return service.AssetKindRecord{ID: row.ID, ContractVersion: row.ContractVersion, Enabled: row.Enabled, PluginVersion: row.PluginVersion}, nil
}

// ListAssetKinds 返回所有已注册的资产 kind，包括已禁用的。
func (store *AssetStore) ListAssetKinds(ctx context.Context) ([]service.AssetKindRecord, error) {
	rows, err := store.queries.ListAssetKinds(ctx)
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.AssetKindRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.AssetKindRecord{ID: row.ID, ContractVersion: row.ContractVersion, Enabled: row.Enabled, PluginVersion: row.PluginVersion})
	}
	return items, nil
}

// GetAssetRepositoryDefaultBranch 解析资产所属仓库的默认分支。
func (store *AssetStore) GetAssetRepositoryDefaultBranch(ctx context.Context, tenantID, assetID uuid.UUID) (string, error) {
	return store.queries.GetAssetRepositoryDefaultBranch(ctx, generated.GetAssetRepositoryDefaultBranchParams{TenantID: tenantID, AssetID: assetID})
}

// ListServicesByRepository 返回某一仓库的活跃服务。
func (store *AssetStore) ListServicesByRepository(ctx context.Context, tenantID, repositoryID uuid.UUID) ([]service.ServiceRecord, error) {
	rows, err := store.queries.ListServicesByRepository(ctx, generated.ListServicesByRepositoryParams{TenantID: tenantID, RepositoryID: repositoryID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.ServiceRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.ServiceRecord{
			TenantID: row.TenantID, ID: row.ID, RepositoryID: row.RepositoryID, Slug: row.Slug, DisplayName: row.DisplayName,
			Description: row.Description, RootDir: row.RootDir, Language: row.Language, Framework: row.Framework,
			Visibility: row.Visibility, Lifecycle: row.Lifecycle, Revision: row.Revision,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return items, nil
}

// ListAssetsForService 返回某一服务的活跃资产。
func (store *AssetStore) ListAssetsForService(ctx context.Context, tenantID, serviceID uuid.UUID) ([]service.AssetRecord, error) {
	rows, err := store.queries.ListAssetsForService(ctx, generated.ListAssetsForServiceParams{TenantID: tenantID, ServiceID: serviceID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.AssetRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.AssetRecord{ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, Name: row.Name, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time})
	}
	return items, nil
}

// ListSourceSpecsForService 返回某一服务的活跃源配置。
func (store *AssetStore) ListSourceSpecsForService(ctx context.Context, tenantID, serviceID uuid.UUID) ([]service.SourceSpecRecord, error) {
	rows, err := store.queries.ListSourceSpecsForService(ctx, generated.ListSourceSpecsForServiceParams{TenantID: tenantID, ServiceID: serviceID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.SourceSpecRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.SourceSpecRecord{
			ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, AssetNameTemplate: row.AssetNameTemplate,
			Role: row.Role, Origin: row.Origin, Mode: row.Mode, Path: row.Path, ProducerProfileID: row.ProducerProfileID,
			Ord: int(row.Ord), TimeoutSec: int(row.TimeoutSec), BranchPatterns: append([]string(nil), row.BranchPatterns...),
			Enabled: row.Enabled, ConfigOrigin: row.ConfigOrigin, BindingsCount: 0, Revision: row.Revision,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return items, nil
}

// UpsertAsset 按唯一 (service, kind, name) 键创建或复用一条资产。
func (store *AssetStore) UpsertAsset(ctx context.Context, input service.NewAsset) (service.AssetRecord, error) {
	row, err := store.queries.UpsertAsset(ctx, generated.UpsertAssetParams{
		TenantID: input.TenantID, ID: input.ID, ServiceID: input.ServiceID, Kind: input.Kind, Name: input.Name,
	})
	if err != nil {
		return service.AssetRecord{}, normalizeError(err)
	}
	return service.AssetRecord{ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, Name: row.Name, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

// GetAssetByName 按其唯一键返回一条活跃资产。
func (store *AssetStore) GetAssetByName(ctx context.Context, tenantID, serviceID uuid.UUID, kind, name string) (service.AssetRecord, error) {
	row, err := store.queries.GetAssetByName(ctx, generated.GetAssetByNameParams{TenantID: tenantID, ServiceID: serviceID, Kind: kind, Name: name})
	if err != nil {
		return service.AssetRecord{}, normalizeError(err)
	}
	return service.AssetRecord{ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, Name: row.Name, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

// GetAsset 按 id 返回一条活跃资产。
func (store *AssetStore) GetAsset(ctx context.Context, tenantID, id uuid.UUID) (service.AssetRecord, error) {
	row, err := store.queries.GetAsset(ctx, generated.GetAssetParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.AssetRecord{}, normalizeError(err)
	}
	return service.AssetRecord{ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, Name: row.Name, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

// CreateAssetRefTrack 创建或重新激活一条资产 ref track。
func (store *AssetStore) CreateAssetRefTrack(ctx context.Context, input service.NewAssetRefTrack) (service.AssetRefTrackRecord, error) {
	row, err := store.queries.CreateAssetRefTrack(ctx, generated.CreateAssetRefTrackParams{
		TenantID: input.TenantID, ID: input.ID, AssetID: input.AssetID, RefType: input.RefType, RefName: input.RefName, Health: input.Health,
	})
	if err != nil {
		return service.AssetRefTrackRecord{}, normalizeError(err)
	}
	return service.AssetRefTrackRecord{ID: row.ID, AssetID: row.AssetID, RefType: row.RefType, RefName: row.RefName, LatestVersionID: row.LatestVersionID, CurrentVersionID: row.CurrentVersionID, Health: row.Health}, nil
}

// GetAssetRefTrack 返回一条资产 ref track。
func (store *AssetStore) GetAssetRefTrack(ctx context.Context, tenantID, assetID uuid.UUID, refType, refName string) (service.AssetRefTrackRecord, error) {
	row, err := store.queries.GetAssetRefTrack(ctx, generated.GetAssetRefTrackParams{TenantID: tenantID, AssetID: assetID, RefType: refType, RefName: refName})
	if err != nil {
		return service.AssetRefTrackRecord{}, normalizeError(err)
	}
	return service.AssetRefTrackRecord{ID: row.ID, AssetID: row.AssetID, RefType: row.RefType, RefName: row.RefName, LatestVersionID: row.LatestVersionID, CurrentVersionID: row.CurrentVersionID, Health: row.Health}, nil
}

// CreateLayer 插入一条资产层。
func (store *AssetStore) CreateLayer(ctx context.Context, input service.NewLayer) (service.LayerRecord, error) {
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

// GetBaseLayerForAsset 返回资产的 base 层。
func (store *AssetStore) GetBaseLayerForAsset(ctx context.Context, tenantID, assetID uuid.UUID) (service.LayerRecord, error) {
	row, err := store.queries.GetBaseLayerForAsset(ctx, generated.GetBaseLayerForAssetParams{TenantID: tenantID, AssetID: assetID})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

// CreateLayerRevision 插入一条不可变的层修订。
func (store *AssetStore) CreateLayerRevision(ctx context.Context, input service.NewLayerRevision) (service.LayerRevisionRecord, error) {
	row, err := store.queries.CreateLayerRevision(ctx, generated.CreateLayerRevisionParams{
		TenantID: input.TenantID, ID: input.ID, LayerID: input.LayerID, ScopeType: input.ScopeType, ScopeKey: input.ScopeKey,
		ContentHash: input.ContentHash, ContentRef: input.ContentRef, ContentType: input.ContentType, Dialect: input.Dialect,
		SourceBranch: input.SourceBranch, ReviewStatus: input.ReviewStatus, GitCommit: input.GitCommit, CreatedBy: input.CreatedBy, ProducerRunID: input.ProducerRunID,
	})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return service.LayerRevisionRecord{
		ID: row.ID, LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		ContentHash: row.ContentHash, ContentRef: row.ContentRef, ContentType: row.ContentType, Dialect: row.Dialect, ReviewStatus: row.ReviewStatus,
		GitCommit: row.GitCommit, SourceBranch: row.SourceBranch, CreatedAt: row.CreatedAt.Time,
	}, nil
}

// GetLatestLayerRevision 返回某一层作用域内最新的修订。
func (store *AssetStore) GetLatestLayerRevision(ctx context.Context, tenantID, layerID uuid.UUID, scopeType, scopeKey string) (service.LayerRevisionRecord, error) {
	row, err := store.queries.GetLatestLayerRevision(ctx, generated.GetLatestLayerRevisionParams{TenantID: tenantID, LayerID: layerID, ScopeType: scopeType, ScopeKey: scopeKey})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return service.LayerRevisionRecord{
		ID: row.ID, LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		ContentHash: row.ContentHash, ContentRef: row.ContentRef, ContentType: row.ContentType, Dialect: row.Dialect, ReviewStatus: row.ReviewStatus,
		GitCommit: row.GitCommit, SourceBranch: row.SourceBranch, CreatedAt: row.CreatedAt.Time,
	}, nil
}

// UpsertLayerHead 覆盖写入一条层头指针。
func (store *AssetStore) UpsertLayerHead(ctx context.Context, input service.NewLayerHead) (service.LayerHeadRecord, error) {
	row, err := store.queries.UpsertLayerHead(ctx, generated.UpsertLayerHeadParams{
		TenantID: input.TenantID, LayerID: input.LayerID, ScopeType: input.ScopeType, ScopeKey: input.ScopeKey,
		LatestRevisionID: input.LatestRevisionID, EffectiveRevisionID: input.EffectiveRevisionID, CandidateRevisionID: input.CandidateRevisionID, Generation: input.Generation,
	})
	if err != nil {
		return service.LayerHeadRecord{}, normalizeError(err)
	}
	return service.LayerHeadRecord{
		LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		LatestRevisionID: row.LatestRevisionID, EffectiveRevisionID: row.EffectiveRevisionID, CandidateRevisionID: row.CandidateRevisionID, Generation: row.Generation,
	}, nil
}

// CreateAssetVersion 插入一条资产版本。
func (store *AssetStore) CreateAssetVersion(ctx context.Context, input service.NewAssetVersion) (service.AssetVersionRecord, error) {
	row, err := store.queries.CreateAssetVersion(ctx, generated.CreateAssetVersionParams{
		TenantID: input.TenantID, ID: input.ID, AssetID: input.AssetID, TrackID: input.TrackID, SequenceNo: input.SequenceNo, Version: input.Version, Lifecycle: input.Lifecycle, Revision: input.Revision,
		QualityScore: input.QualityScore, MergeRequestID: input.MergeRequestID, InputFingerprint: input.InputFingerprint, MergeEngineVersion: input.MergeEngineVersion,
		OverlayCompilerVersion: input.OverlayCompilerVersion, OverlayMode: input.OverlayMode, NormalizerVersion: input.NormalizerVersion, KindPluginVersion: input.KindPluginVersion,
		LayerManifest: input.LayerManifest, MergedHash: input.MergedHash, MergedRef: input.MergedRef, NormalizedRef: input.NormalizedRef, BundledRef: input.BundledRef, ProvenanceRef: input.ProvenanceRef,
		SourceCommit: input.SourceCommit, BaselineVersionID: input.BaselineVersionID, DiffSummary: input.DiffSummary, Labels: input.Labels, IndexComplete: input.IndexComplete,
	})
	if err != nil {
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// GetAssetVersion 返回一条资产版本。
func (store *AssetStore) GetAssetVersion(ctx context.Context, tenantID, id uuid.UUID) (service.AssetVersionRecord, error) {
	row, err := store.queries.GetAssetVersion(ctx, generated.GetAssetVersionParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// MarkAssetVersionIndexed 将一条资产版本标记为条目索引已完成。
func (store *AssetStore) MarkAssetVersionIndexed(ctx context.Context, tenantID, id uuid.UUID) error {
	changed, err := store.queries.MarkAssetVersionIndexed(ctx, generated.MarkAssetVersionIndexedParams{TenantID: tenantID, ID: id})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrNotFound
	}
	return nil
}

// GetLatestVersionInTrack 返回 track 中最新的版本。
func (store *AssetStore) GetLatestVersionInTrack(ctx context.Context, tenantID, trackID uuid.UUID) (service.AssetVersionRecord, error) {
	row, err := store.queries.GetLatestVersionInTrack(ctx, generated.GetLatestVersionInTrackParams{TenantID: tenantID, TrackID: trackID})
	if err != nil {
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// GetCurrentVersionInTrack 返回 track 中当前已发布的版本。
func (store *AssetStore) GetCurrentVersionInTrack(ctx context.Context, tenantID, trackID uuid.UUID) (service.AssetVersionRecord, error) {
	row, err := store.queries.GetCurrentVersionInTrack(ctx, generated.GetCurrentVersionInTrackParams{TenantID: tenantID, TrackID: trackID})
	if err != nil {
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// UpdateAssetRefTrackHead 更新 track 上的版本指针。
func (store *AssetStore) UpdateAssetRefTrackHead(ctx context.Context, tenantID, trackID uuid.UUID, latestVersionID, currentVersionID *uuid.UUID, processedGeneration int64) error {
	changed, err := store.queries.UpdateAssetRefTrackHead(ctx, generated.UpdateAssetRefTrackHeadParams{
		TenantID: tenantID, ID: trackID, LatestVersionID: latestVersionID, CurrentVersionID: currentVersionID, ProcessedGeneration: processedGeneration,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrPrecondition
	}
	return nil
}

// UpdateSourceSpec 在其修订号下应用一次经过校验的源配置补丁。
func (store *AssetStore) UpdateSourceSpec(ctx context.Context, input service.SourceSpecPatch) (service.SourceSpecRecord, error) {
	row, err := store.queries.UpdateSourceSpec(ctx, generated.UpdateSourceSpecParams{
		TenantID: input.TenantID, ID: input.ID, ExpectedRevision: input.ExpectedRevision,
		AssetNameTemplate: input.AssetNameTemplate, Role: input.Role, Origin: input.Origin, Mode: input.Mode,
		Path: input.Path, ProducerProfileID: input.ProducerProfileID,
		Ord: int32Pointer(input.Ord), TimeoutSec: int32Pointer(input.TimeoutSec),
		BranchPatterns: input.BranchPatterns, Enabled: input.Enabled,
	})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	return service.SourceSpecRecord{
		ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, AssetNameTemplate: row.AssetNameTemplate,
		Role: row.Role, Origin: row.Origin, Mode: row.Mode, Path: row.Path, ProducerProfileID: row.ProducerProfileID,
		Ord: int(row.Ord), TimeoutSec: int(row.TimeoutSec), BranchPatterns: append([]string(nil), row.BranchPatterns...),
		Enabled: row.Enabled, ConfigOrigin: row.ConfigOrigin, Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func int32Pointer(value *int) *int32 {
	if value == nil {
		return nil
	}
	return new(int32(*value))
}

// CreateAssetItem 插入一条已索引的资产条目。
func (store *AssetStore) CreateAssetItem(ctx context.Context, input service.NewAssetItem) (service.AssetItemRecord, error) {
	row, err := store.queries.CreateAssetItem(ctx, generated.CreateAssetItemParams{
		TenantID: input.TenantID, ID: input.ID, AssetVersionID: input.AssetVersionID, AssetID: input.AssetID, ServiceID: input.ServiceID,
		Kind: input.Kind, ItemType: input.ItemType, Key: input.Key, Display: input.Display, SearchText: input.SearchText, SearchRaw: input.SearchRaw, Provenance: input.Provenance,
	})
	if err != nil {
		return service.AssetItemRecord{}, normalizeError(err)
	}
	return assetItemFromRow(row), nil
}

// UpdateAssetItemSearchVector 刷新某一条目的 tsvector。
func (store *AssetStore) UpdateAssetItemSearchVector(ctx context.Context, tenantID, id uuid.UUID, searchText string) error {
	changed, err := store.queries.UpdateAssetItemSearchVector(ctx, generated.UpdateAssetItemSearchVectorParams{TenantID: tenantID, ID: id, SearchText: []byte(searchText)})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrNotFound
	}
	return nil
}

// ListAssetVersionItems 返回一页资产版本条目。
func (store *AssetStore) ListAssetVersionItems(ctx context.Context, tenantID, versionID uuid.UUID, query string, limit, offset int32) ([]service.AssetItemRecord, int64, error) {
	total, err := store.queries.CountAssetVersionItems(ctx, generated.CountAssetVersionItemsParams{TenantID: tenantID, AssetVersionID: versionID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListAssetVersionItems(ctx, generated.ListAssetVersionItemsParams{
		TenantID: tenantID, AssetVersionID: versionID, SearchQuery: query, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.AssetItemRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, assetItemFromRow(row))
	}
	return items, total, nil
}

// SearchItems 按搜索查询分页检索跨 kind 的已索引资产条目，并可附带单个 kind 过滤。
// 多 kind 与布尔面过滤在 SQL 分页取出后由 Search 服务层应用。
func (store *AssetStore) SearchItems(ctx context.Context, tenantID uuid.UUID, query string, filter service.SearchFilter, limit, offset int32) ([]service.AssetItemRecord, int64, error) {
	kindFilter := ""
	if len(filter.Kinds) == searchKindFilterCount {
		kindFilter = filter.Kinds[0]
	}
	total, err := store.queries.CountSearchableAssetItems(ctx, generated.CountSearchableAssetItemsParams{
		TenantID: tenantID, SearchQuery: query, KindFilter: kindFilter,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListSearchableAssetItems(ctx, generated.ListSearchableAssetItemsParams{
		TenantID: tenantID, SearchQuery: query, KindFilter: kindFilter, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.AssetItemRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, assetItemFromRow(row))
	}
	return items, total, nil
}

// GetServiceByID 在租户内按 id 返回一条服务。
func (store *AssetStore) GetServiceByID(ctx context.Context, tenantID, id uuid.UUID) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceByID(ctx, generated.GetServiceByIDParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// GetRepositoryByService 返回拥有某一服务的仓库。
func (store *AssetStore) GetRepositoryByService(ctx context.Context, tenantID, serviceID uuid.UUID) (service.RepositoryRecord, error) {
	row, err := store.queries.GetRepositoryByService(ctx, generated.GetRepositoryByServiceParams{TenantID: tenantID, ServiceID: serviceID})
	if err != nil {
		return service.RepositoryRecord{}, normalizeError(err)
	}
	return repositoryFromRow(row)
}

// ListSystemGroupMembers 返回某一系统分组的成员服务 id。
func (store *AssetStore) ListSystemGroupMembers(ctx context.Context, tenantID, groupID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := store.queries.ListSystemGroupMembers(ctx, generated.ListSystemGroupMembersParams{TenantID: tenantID, GroupID: groupID})
	if err != nil {
		return nil, normalizeError(err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ServiceID)
	}
	return ids, nil
}

// ListServicesForTenant 分页返回租户内所有活跃服务。
func (store *AssetStore) ListServicesForTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]service.ServiceRecord, int64, error) {
	total, err := store.queries.CountListedServices(ctx, tenantID)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListServices(ctx, generated.ListServicesParams{TenantID: tenantID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.ServiceRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, serviceRecordFromRow(row))
	}
	return items, total, nil
}

// UpsertSourceBinding 按 scope+expansion 键覆盖写入一条源绑定。
func (store *AssetStore) UpsertSourceBinding(ctx context.Context, input service.NewSourceBinding) (service.SourceBindingRecord, error) {
	row, err := store.queries.UpsertSourceBinding(ctx, generated.UpsertSourceBindingParams{
		TenantID: input.TenantID, ID: input.ID, SourceSpecID: input.SourceSpecID, ScopeType: input.ScopeType, ScopeKey: input.ScopeKey,
		ExpansionKey: input.ExpansionKey, ResolvedPath: input.ResolvedPath, SourceSystem: input.SourceSystem, AssetID: input.AssetID, LayerID: input.LayerID, State: input.State, LastSeenCommit: input.LastSeenCommit,
	})
	if err != nil {
		return service.SourceBindingRecord{}, normalizeError(err)
	}
	return service.SourceBindingRecord{
		ID: row.ID, SourceSpecID: row.SourceSpecID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		ExpansionKey: row.ExpansionKey, ResolvedPath: row.ResolvedPath, SourceSystem: row.SourceSystem,
		AssetID: row.AssetID, LayerID: row.LayerID, State: row.State, LastSeenCommit: row.LastSeenCommit,
	}, nil
}

// ListActiveBindingsForScope 返回某一作用域的活跃绑定。
func (store *AssetStore) ListActiveBindingsForScope(ctx context.Context, tenantID, sourceSpecID uuid.UUID, scopeType, scopeKey string) ([]service.SourceBindingRecord, error) {
	rows, err := store.queries.ListActiveBindingsForScope(ctx, generated.ListActiveBindingsForScopeParams{
		TenantID: tenantID, SourceSpecID: sourceSpecID, ScopeType: scopeType, ScopeKey: scopeKey,
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.SourceBindingRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.SourceBindingRecord{
			ID: row.ID, SourceSpecID: row.SourceSpecID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
			ExpansionKey: row.ExpansionKey, ResolvedPath: row.ResolvedPath, SourceSystem: row.SourceSystem,
			AssetID: row.AssetID, LayerID: row.LayerID, State: row.State, LastSeenCommit: row.LastSeenCommit,
		})
	}
	return items, nil
}

// MarkBindingsStaleInScope 将某作用域内未再出现的绑定标记为失效。
func (store *AssetStore) MarkBindingsStaleInScope(ctx context.Context, tenantID, sourceSpecID uuid.UUID, scopeType, scopeKey string, seenIDs []uuid.UUID) error {
	if _, err := store.queries.MarkBindingsStaleInScope(ctx, generated.MarkBindingsStaleInScopeParams{
		TenantID: tenantID, SourceSpecID: sourceSpecID, ScopeType: scopeType, ScopeKey: scopeKey, SeenIds: seenIDs,
	}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// CountActiveBindings 返回某一源配置的活跃绑定数。
func (store *AssetStore) CountActiveBindings(ctx context.Context, tenantID, sourceSpecID uuid.UUID) (int64, error) {
	return store.queries.CountActiveBindings(ctx, generated.CountActiveBindingsParams{TenantID: tenantID, SourceSpecID: sourceSpecID})
}

// SetSourceLastError 记录一次源物化失败。
func (store *AssetStore) SetSourceLastError(ctx context.Context, tenantID, sourceSpecID uuid.UUID, message string) error {
	if _, err := store.queries.SetSourceLastError(ctx, generated.SetSourceLastErrorParams{TenantID: tenantID, ID: sourceSpecID, LastError: new(message)}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// ClearSourceLastError 在成功物化后清除源失败。
func (store *AssetStore) ClearSourceLastError(ctx context.Context, tenantID, sourceSpecID uuid.UUID) error {
	if _, err := store.queries.ClearSourceLastError(ctx, generated.ClearSourceLastErrorParams{TenantID: tenantID, ID: sourceSpecID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// MarkTracksStaleForSourceSpec 将某源资产的所有 track 标记为失效。
func (store *AssetStore) MarkTracksStaleForSourceSpec(ctx context.Context, tenantID, sourceSpecID uuid.UUID) error {
	if _, err := store.queries.MarkTracksStaleForSourceSpec(ctx, generated.MarkTracksStaleForSourceSpecParams{TenantID: tenantID, SourceSpecID: sourceSpecID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// MarkTracksHealthyForSourceSpec 将某源资产的所有 track 恢复为正常。
func (store *AssetStore) MarkTracksHealthyForSourceSpec(ctx context.Context, tenantID, sourceSpecID uuid.UUID) error {
	if _, err := store.queries.MarkTracksHealthyForSourceSpec(ctx, generated.MarkTracksHealthyForSourceSpecParams{TenantID: tenantID, SourceSpecID: sourceSpecID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// UpsertRecentService 记录一次成功的服务详情读取。
func (store *AssetStore) UpsertRecentService(ctx context.Context, tenantID, userID, serviceID uuid.UUID, viewedAt time.Time) error {
	if _, err := store.queries.UpsertRecentService(ctx, generated.UpsertRecentServiceParams{TenantID: tenantID, UserID: userID, ServiceID: serviceID, ViewedAt: timestamp(viewedAt)}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// ListRecentServices 返回一页最近查看过的服务。
func (store *AssetStore) ListRecentServices(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]service.ServiceRecord, int64, error) {
	total, err := store.queries.CountRecentServices(ctx, generated.CountRecentServicesParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListRecentServices(ctx, generated.ListRecentServicesParams{TenantID: tenantID, UserID: userID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.ServiceRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.ServiceRecord{
			TenantID: row.TenantID, ID: row.ID, RepositoryID: row.RepositoryID, Slug: row.Slug, DisplayName: row.DisplayName,
			Description: row.Description, RootDir: row.RootDir, Language: row.Language, Framework: row.Framework,
			Visibility: row.Visibility, Lifecycle: row.Lifecycle, Revision: row.Revision,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return items, total, nil
}

func layerFromRow(row generated.Layer) service.LayerRecord {
	return service.LayerRecord{
		ID: row.ID, AssetID: row.AssetID, SourceSpecID: row.SourceSpecID, Role: row.Role, Origin: row.Origin,
		Ord: int(row.Ord), Dialect: row.Dialect, Enabled: row.Enabled, BranchPatterns: append([]string(nil), row.BranchPatterns...),
		DisplayName: row.DisplayName, Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func assetVersionFromRow(row generated.AssetVersion) service.AssetVersionRecord {
	var manifest []service.LayerManifestEntry
	if len(row.LayerManifest) > 0 {
		_ = json.Unmarshal(row.LayerManifest, &manifest)
	}
	return service.AssetVersionRecord{
		ID: row.ID, AssetID: row.AssetID, TrackID: row.TrackID, SequenceNo: row.SequenceNo, Version: row.Version,
		Lifecycle: row.Lifecycle, Revision: row.Revision, InputFingerprint: row.InputFingerprint,
		MergeEngineVersion: row.MergeEngineVersion, LayerManifest: manifest, SourceCommit: row.SourceCommit,
		MergedRef: row.MergedRef, IndexComplete: row.IndexComplete, CreatedAt: row.CreatedAt.Time,
	}
}

func assetItemFromRow(row generated.AssetItem) service.AssetItemRecord {
	var display map[string]any
	if len(row.Display) > 0 {
		_ = json.Unmarshal(row.Display, &display)
	}
	if display == nil {
		display = map[string]any{}
	}
	return service.AssetItemRecord{
		ItemType: row.ItemType, Key: row.Key, Display: display,
		Kind: row.Kind, AssetID: row.AssetID, AssetVersionID: row.AssetVersionID, ServiceID: row.ServiceID, SearchText: row.SearchText,
	}
}

var _ service.AssetStore = (*AssetStore)(nil)
