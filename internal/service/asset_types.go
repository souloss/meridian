package service

import (
	"context"
	"time"
	"uuid"
)

// AssetRecord 是租户可见的资产投影。
type AssetRecord struct {
	// ID 是资产的唯一标识。
	ID uuid.UUID
	// ServiceID 是所属服务的标识。
	ServiceID uuid.UUID
	// Kind 是资产类别（如 openapi、dbschema）。
	Kind string
	// Name 是资产名。
	Name string
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// AssetVersionRecord 是租户可见的资产版本投影。
type AssetVersionRecord struct {
	// ID 是版本的唯一标识。
	ID uuid.UUID
	// AssetID 是所属资产的标识。
	AssetID uuid.UUID
	// TrackID 是所属引用轨道的标识。
	TrackID uuid.UUID
	// SequenceNo 是轨道内的递增序号。
	SequenceNo int64
	// Version 是语义化版本标签。
	Version string
	// Lifecycle 是版本生命周期状态（draft/published）。
	Lifecycle string
	// Revision 是乐观并发版本号。
	Revision int64
	// InputFingerprint 是输入内容的指纹，用于去重。
	InputFingerprint string
	// MergeEngineVersion 是生成该版本的合并引擎版本。
	MergeEngineVersion string
	// LayerManifest 是构成该版本的层修订清单。
	LayerManifest []LayerManifestEntry
	// SourceCommit 是源提交 SHA（可为空）。
	SourceCommit *string
	// MergedRef 是合并文档的 blob 引用（可为空）。
	MergedRef *string
	// IndexComplete 表示条目索引是否已建立。
	IndexComplete bool
	// CreatedAt 是创建时间。
	CreatedAt time.Time
}

// MergedHashOrFingerprint 返回 diff 解析所用的内容标识：
// 当前实现固定返回输入指纹（合并哈希在 M3 阶段尚未消费）。
func (record AssetVersionRecord) MergedHashOrFingerprint() string {
	return record.InputFingerprint
}

// LayerManifestEntry 标识版本清单中的一个层修订。
type LayerManifestEntry struct {
	// LayerID 是层的标识。
	LayerID uuid.UUID
	// RevisionID 是层修订的标识。
	RevisionID uuid.UUID
	// Role 是层角色（base/overlay）。
	Role string
	// Origin 是层来源（repo/third_party/manual/ai_generated）。
	Origin string
	// Ord 是 overlay 层的排序号。
	Ord int
	// ScopeType 是层作用域类型（global/ref）。
	ScopeType string
	// ScopeKey 是层作用域键。
	ScopeKey string
	// ReviewStatus 是修订审核状态。
	ReviewStatus string
	// ContentHash 是修订内容的哈希。
	ContentHash string
}

// AssetItemRecord 是一条已建立索引的资产条目。
type AssetItemRecord struct {
	// ItemType 是条目类型（如 operation、table、column、edge）。
	ItemType string
	// Key 是条目键，由 kinds.yaml 的 itemKey 模板生成。
	Key string
	// Display 是条目展示字段（非秘密）。
	Display map[string]any
	// Kind 是所属资产的类别。
	Kind string
	// AssetID 是所属资产的标识。
	AssetID uuid.UUID
	// AssetVersionID 是所属版本的标识。
	AssetVersionID uuid.UUID
	// ServiceID 是所属服务的标识。
	ServiceID uuid.UUID
	// SearchText 是用于全文检索的文本。
	SearchText string
}

// LayerRecord 是一个资产层。
type LayerRecord struct {
	// ID 是层的唯一标识。
	ID uuid.UUID
	// AssetID 是所属资产的标识。
	AssetID uuid.UUID
	// SourceSpecID 是关联源配置的标识（可为空）。
	SourceSpecID *uuid.UUID
	// Role 是层角色（base/overlay）。
	Role string
	// Origin 是层来源（repo/third_party/manual/ai_generated）。
	Origin string
	// Ord 是 overlay 层的排序号。
	Ord int
	// Dialect 是层方言（可为空）。
	Dialect *string
	// Enabled 表示层是否参与合并。
	Enabled bool
	// BranchPatterns 是层生效的分支模式列表。
	BranchPatterns []string
	// DisplayName 是层的展示名。
	DisplayName string
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// LayerRevisionRecord 是一个不可变的层内容修订。
type LayerRevisionRecord struct {
	// ID 是修订的唯一标识。
	ID uuid.UUID
	// LayerID 是所属层的标识。
	LayerID uuid.UUID
	// ScopeType 是修订作用域类型（global/ref）。
	ScopeType string
	// ScopeKey 是修订作用域键。
	ScopeKey string
	// ContentHash 是内容哈希。
	ContentHash string
	// ContentRef 是内容的 blob 引用。
	ContentRef string
	// ContentType 是内容媒体类型。
	ContentType string
	// Dialect 是修订方言（可为空）。
	Dialect *string
	// ReviewStatus 是审核状态。
	ReviewStatus string
	// GitCommit 是关联的 Git 提交（可为空）。
	GitCommit *string
	// SourceBranch 是来源分支（可为空）。
	SourceBranch *string
	// CreatedBy 是创建者用户标识（可为空）。
	CreatedBy *uuid.UUID
	// CreatedAt 是创建时间。
	CreatedAt time.Time
}

// NewLayerRevision 承载一次层修订插入所需的值。
type NewLayerRevision struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是新修订的标识。
	ID uuid.UUID
	// LayerID 是所属层的标识。
	LayerID uuid.UUID
	// ScopeType 是作用域类型。
	ScopeType string
	// ScopeKey 是作用域键。
	ScopeKey string
	// ContentHash 是内容哈希。
	ContentHash string
	// ContentRef 是内容的 blob 引用。
	ContentRef string
	// ContentType 是内容媒体类型。
	ContentType string
	// Dialect 是方言（可为空）。
	Dialect *string
	// SourceBranch 是来源分支（可为空）。
	SourceBranch *string
	// ReviewStatus 是审核状态。
	ReviewStatus string
	// GitCommit 是关联 Git 提交（可为空）。
	GitCommit *string
	// CreatedBy 是创建者标识（可为空）。
	CreatedBy *uuid.UUID
	// ProducerRunID 是生产者运行标识（可为空）。
	ProducerRunID *uuid.UUID
}

// NewLayerHead 承载一次层头指针 upsert 所需的值。
type NewLayerHead struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// LayerID 是所属层的标识。
	LayerID uuid.UUID
	// ScopeType 是作用域类型。
	ScopeType string
	// ScopeKey 是作用域键。
	ScopeKey string
	// LatestRevisionID 是最新修订标识（可为空）。
	LatestRevisionID *uuid.UUID
	// EffectiveRevisionID 是当前生效修订标识（可为空）。
	EffectiveRevisionID *uuid.UUID
	// CandidateRevisionID 是待审核候选修订标识（可为空）。
	CandidateRevisionID *uuid.UUID
	// Generation 是层头代次。
	Generation int64
}

// NewAssetVersion 承载一次版本插入所需的值。
type NewAssetVersion struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是新版本的标识。
	ID uuid.UUID
	// AssetID 是所属资产的标识。
	AssetID uuid.UUID
	// TrackID 是所属轨道的标识。
	TrackID uuid.UUID
	// SequenceNo 是轨道内序号。
	SequenceNo int64
	// Version 是语义化版本标签。
	Version string
	// Lifecycle 是生命周期状态。
	Lifecycle string
	// Revision 是乐观并发版本号。
	Revision int64
	// QualityScore 是质量评分（可为空）。
	QualityScore *int32
	// MergeRequestID 是合并请求标识（可为空）。
	MergeRequestID *uuid.UUID
	// InputFingerprint 是输入指纹。
	InputFingerprint string
	// MergeEngineVersion 是合并引擎版本。
	MergeEngineVersion string
	// OverlayCompilerVersion 是 overlay 编译器版本（可为空）。
	OverlayCompilerVersion *string
	// OverlayMode 是 overlay 模式（可为空）。
	OverlayMode *string
	// NormalizerVersion 是规范化器版本（可为空）。
	NormalizerVersion *string
	// KindPluginVersion 是 kind 插件版本（可为空）。
	KindPluginVersion *string
	// LayerManifest 是层清单的序列化字节。
	LayerManifest []byte
	// MergedHash 是合并文档哈希（可为空）。
	MergedHash *string
	// MergedRef 是合并文档 blob 引用（可为空）。
	MergedRef *string
	// NormalizedRef 是规范化文档 blob 引用（可为空）。
	NormalizedRef *string
	// BundledRef 是打包文档 blob 引用（可为空）。
	BundledRef *string
	// ProvenanceRef 是溯源文档 blob 引用（可为空）。
	ProvenanceRef *string
	// SourceCommit 是源提交 SHA（可为空）。
	SourceCommit *string
	// BaselineVersionID 是基线版本标识（可为空）。
	BaselineVersionID *uuid.UUID
	// DiffSummary 是差异摘要序列化字节。
	DiffSummary []byte
	// Labels 是标签序列化字节。
	Labels []byte
	// IndexComplete 表示条目索引是否已建立。
	IndexComplete bool
}

// NewAssetItem 承载一次条目插入所需的值。
type NewAssetItem struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是新条目的标识。
	ID uuid.UUID
	// AssetVersionID 是所属版本的标识。
	AssetVersionID uuid.UUID
	// AssetID 是所属资产的标识。
	AssetID uuid.UUID
	// ServiceID 是所属服务的标识。
	ServiceID uuid.UUID
	// Kind 是资产类别。
	Kind string
	// ItemType 是条目类型。
	ItemType string
	// Key 是条目键。
	Key string
	// Display 是展示字段序列化字节。
	Display []byte
	// SearchText 是检索文本。
	SearchText string
	// SearchRaw 是检索原始字节。
	SearchRaw []byte
	// Provenance 是溯源序列化字节。
	Provenance []byte
}

// NewSourceBinding 承载一次绑定 upsert 所需的值。
type NewSourceBinding struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是新绑定的标识。
	ID uuid.UUID
	// SourceSpecID 是关联源配置的标识。
	SourceSpecID uuid.UUID
	// ScopeType 是作用域类型。
	ScopeType string
	// ScopeKey 是作用域键。
	ScopeKey string
	// ExpansionKey 是展开键（如相对路径）。
	ExpansionKey string
	// ResolvedPath 是解析后的路径（可为空）。
	ResolvedPath *string
	// SourceSystem 是来源系统标识（可为空）。
	SourceSystem *string
	// AssetID 是绑定资产的标识。
	AssetID uuid.UUID
	// LayerID 是绑定层的标识。
	LayerID uuid.UUID
	// State 是绑定状态。
	State string
	// LastSeenCommit 是最近观测到的提交（可为空）。
	LastSeenCommit *string
}

// SyncJobInput 描述一次幂等的仓库同步请求。
type SyncJobInput struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// RepositoryID 是目标仓库的标识。
	RepositoryID uuid.UUID
	// RefType 是引用类型（branch/tag）。
	RefType string
	// RefName 是引用名。
	RefName string
	// IdempotencyKey 是幂等键。
	IdempotencyKey uuid.UUID
	// Force 表示是否强制同步（可为空）。
	Force *bool
	// PrincipalType 与 PrincipalID 将重放身份绑定到调用方。
	PrincipalType string
	// PrincipalID 承载 SyncJobInput 的生成 PrincipalID 值。
	PrincipalID uuid.UUID
	// RequestHash 是 32 字节 RFC 8785 请求摘要，用于重放比较。
	RequestHash []byte
}

// SyncStore 是仓库同步入队的持久化边界。
type SyncStore interface {
	// GetRepository 承载 SyncStore 的生成 GetRepository 值。
	GetRepository(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	// EnqueueSyncJob 承载 SyncStore 的生成 EnqueueSyncJob 值。
	EnqueueSyncJob(context.Context, SyncJobInput) (JobAccepted, error)
	// MarkSyncJobDirty 承载 SyncStore 的生成 MarkSyncJobDirty 值。
	MarkSyncJobDirty(context.Context, uuid.UUID, string, int64) error
	// GetSyncJobForSuccessor 承载 SyncStore 的生成 GetSyncJobForSuccessor 值。
	GetSyncJobForSuccessor(context.Context, uuid.UUID, uuid.UUID) (SyncJobSuccessorState, error)
	// ClearSyncJobDirty 承载 SyncStore 的生成 ClearSyncJobDirty 值。
	ClearSyncJobDirty(context.Context, uuid.UUID, uuid.UUID) error
}

// SyncJobSuccessorState 捕获一个已完成的同步任务是否需要派生后继任务。
type SyncJobSuccessorState struct {
	// JobID 是源任务标识。
	JobID uuid.UUID
	// Status 是任务状态。
	Status string
	// Dirty 表示执行期间有更新的等价工作到达。
	Dirty bool
	// ScopeID 是作用域资源标识（可为空）。
	ScopeID *uuid.UUID
	// RefType 是引用类型（可为空）。
	RefType *string
	// RefName 是引用名（可为空）。
	RefName *string
	// ActiveGeneration 是当前活跃代次。
	ActiveGeneration int64
}

// AssetStore 是 M1 资产管线的持久化边界。
type AssetStore interface {
	// GetRepository 承载 AssetStore 的生成 GetRepository 值。
	GetRepository(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	// GetAssetRepositoryDefaultBranch 承载 AssetStore 的生成 GetAssetRepositoryDefaultBranch 值。
	GetAssetRepositoryDefaultBranch(context.Context, uuid.UUID, uuid.UUID) (string, error)
	// GetAssetKind 承载 AssetStore 的生成 GetAssetKind 值。
	GetAssetKind(context.Context, string) (AssetKindRecord, error)
	// ListAssetKinds 承载 AssetStore 的生成 ListAssetKinds 值。
	ListAssetKinds(context.Context) ([]AssetKindRecord, error)
	// ListServicesByRepository 承载 AssetStore 的生成 ListServicesByRepository 值。
	ListServicesByRepository(context.Context, uuid.UUID, uuid.UUID) ([]ServiceRecord, error)
	// ListAssetsForService 承载 AssetStore 的生成 ListAssetsForService 值。
	ListAssetsForService(context.Context, uuid.UUID, uuid.UUID) ([]AssetRecord, error)
	// ListSourceSpecsForService 承载 AssetStore 的生成 ListSourceSpecsForService 值。
	ListSourceSpecsForService(context.Context, uuid.UUID, uuid.UUID) ([]SourceSpecRecord, error)
	// UpsertAsset 承载 AssetStore 的生成 UpsertAsset 值。
	UpsertAsset(context.Context, NewAsset) (AssetRecord, error)
	// GetAssetByName 承载 AssetStore 的生成 GetAssetByName 值。
	GetAssetByName(context.Context, uuid.UUID, uuid.UUID, string, string) (AssetRecord, error)
	// GetAsset 承载 AssetStore 的生成 GetAsset 值。
	GetAsset(context.Context, uuid.UUID, uuid.UUID) (AssetRecord, error)
	// CreateAssetRefTrack 承载 AssetStore 的生成 CreateAssetRefTrack 值。
	CreateAssetRefTrack(context.Context, NewAssetRefTrack) (AssetRefTrackRecord, error)
	// GetAssetRefTrack 承载 AssetStore 的生成 GetAssetRefTrack 值。
	GetAssetRefTrack(context.Context, uuid.UUID, uuid.UUID, string, string) (AssetRefTrackRecord, error)
	// CreateLayer 承载 AssetStore 的生成 CreateLayer 值。
	CreateLayer(context.Context, NewLayer) (LayerRecord, error)
	// GetBaseLayerForAsset 承载 AssetStore 的生成 GetBaseLayerForAsset 值。
	GetBaseLayerForAsset(context.Context, uuid.UUID, uuid.UUID) (LayerRecord, error)
	// CreateLayerRevision 承载 AssetStore 的生成 CreateLayerRevision 值。
	CreateLayerRevision(context.Context, NewLayerRevision) (LayerRevisionRecord, error)
	// GetLatestLayerRevision 承载 AssetStore 的生成 GetLatestLayerRevision 值。
	GetLatestLayerRevision(context.Context, uuid.UUID, uuid.UUID, string, string) (LayerRevisionRecord, error)
	// UpsertLayerHead 承载 AssetStore 的生成 UpsertLayerHead 值。
	UpsertLayerHead(context.Context, NewLayerHead) (LayerHeadRecord, error)
	// CreateAssetVersion 承载 AssetStore 的生成 CreateAssetVersion 值。
	CreateAssetVersion(context.Context, NewAssetVersion) (AssetVersionRecord, error)
	// GetAssetVersion 承载 AssetStore 的生成 GetAssetVersion 值。
	GetAssetVersion(context.Context, uuid.UUID, uuid.UUID) (AssetVersionRecord, error)
	// MarkAssetVersionIndexed 承载 AssetStore 的生成 MarkAssetVersionIndexed 值。
	MarkAssetVersionIndexed(context.Context, uuid.UUID, uuid.UUID) error
	// GetLatestVersionInTrack 承载 AssetStore 的生成 GetLatestVersionInTrack 值。
	GetLatestVersionInTrack(context.Context, uuid.UUID, uuid.UUID) (AssetVersionRecord, error)
	// GetCurrentVersionInTrack 承载 AssetStore 的生成 GetCurrentVersionInTrack 值。
	GetCurrentVersionInTrack(context.Context, uuid.UUID, uuid.UUID) (AssetVersionRecord, error)
	// UpdateAssetRefTrackHead 承载 AssetStore 的生成 UpdateAssetRefTrackHead 值。
	UpdateAssetRefTrackHead(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, *uuid.UUID, int64) error
	// CreateAssetItem 承载 AssetStore 的生成 CreateAssetItem 值。
	CreateAssetItem(context.Context, NewAssetItem) (AssetItemRecord, error)
	// UpdateAssetItemSearchVector 承载 AssetStore 的生成 UpdateAssetItemSearchVector 值。
	UpdateAssetItemSearchVector(context.Context, uuid.UUID, uuid.UUID, string) error
	// ListAssetVersionItems 承载 AssetStore 的生成 ListAssetVersionItems 值。
	ListAssetVersionItems(context.Context, uuid.UUID, uuid.UUID, string, int32, int32) ([]AssetItemRecord, int64, error)
	// ListAssetVersions 承载 AssetStore 的生成 ListAssetVersions 值。
	ListAssetVersions(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]AssetVersionRecord, int64, error)
	// DeprecateAssetVersion 承载 AssetStore 的生成 DeprecateAssetVersion 值。
	DeprecateAssetVersion(context.Context, uuid.UUID, uuid.UUID, int64) (AssetVersionRecord, error)
	// RetireAssetVersion 承载 AssetStore 的生成 RetireAssetVersion 值。
	RetireAssetVersion(context.Context, uuid.UUID, uuid.UUID, int64) (AssetVersionRecord, error)
	// GetPublicAsset 承载 AssetStore 的生成 GetPublicAsset 值。
	GetPublicAsset(context.Context, string, string, string, string) (PublicAssetRecord, error)
	// SearchItems 按搜索查询与 facet 分页返回已索引的资产条目。
	SearchItems(context.Context, uuid.UUID, string, SearchFilter, int32, int32) ([]AssetItemRecord, int64, error)
	// SearchFacets 一次返回各维度 facet 桶计数（不含零计数基线）。
	SearchFacets(context.Context, uuid.UUID, string, SearchFilter) (SearchFacetCounts, error)
	// GetServiceByID 返回租户内指定 ID 的服务。
	GetServiceByID(context.Context, uuid.UUID, uuid.UUID) (ServiceRecord, error)
	// ListServicesByIDs 批量返回租户内指定 ID 的活跃服务，供搜索命中去 N+1。
	ListServicesByIDs(context.Context, uuid.UUID, []uuid.UUID) ([]ServiceRecord, error)
	// GetRepositoryByService 返回拥有某服务的仓库。
	GetRepositoryByService(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	// ListRepositoriesByServices 批量返回拥有指定服务的仓库，供搜索命中去 N+1。
	ListRepositoriesByServices(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]RepositoryRecord, error)
	// ListSystemGroupMembers 返回某系统分组的成员服务 ID。
	ListSystemGroupMembers(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error)
	// ListServicesForTenant 分页返回租户内每个活跃服务。
	ListServicesForTenant(context.Context, uuid.UUID, int32, int32) ([]ServiceRecord, int64, error)
	// UpdateSourceSpec 承载 AssetStore 的生成 UpdateSourceSpec 值。
	UpdateSourceSpec(context.Context, SourceSpecPatch) (SourceSpecRecord, error)
	// UpsertSourceBinding 承载 AssetStore 的生成 UpsertSourceBinding 值。
	UpsertSourceBinding(context.Context, NewSourceBinding) (SourceBindingRecord, error)
	// ListActiveBindingsForScope 承载 AssetStore 的生成 ListActiveBindingsForScope 值。
	ListActiveBindingsForScope(context.Context, uuid.UUID, uuid.UUID, string, string) ([]SourceBindingRecord, error)
	// MarkBindingsStaleInScope 承载 AssetStore 的生成 MarkBindingsStaleInScope 值。
	MarkBindingsStaleInScope(context.Context, uuid.UUID, uuid.UUID, string, string, []uuid.UUID) error
	// CountActiveBindings 承载 AssetStore 的生成 CountActiveBindings 值。
	CountActiveBindings(context.Context, uuid.UUID, uuid.UUID) (int64, error)
	// SetSourceLastError 承载 AssetStore 的生成 SetSourceLastError 值。
	SetSourceLastError(context.Context, uuid.UUID, uuid.UUID, string) error
	// ClearSourceLastError 承载 AssetStore 的生成 ClearSourceLastError 值。
	ClearSourceLastError(context.Context, uuid.UUID, uuid.UUID) error
	// MarkTracksStaleForSourceSpec 承载 AssetStore 的生成 MarkTracksStaleForSourceSpec 值。
	MarkTracksStaleForSourceSpec(context.Context, uuid.UUID, uuid.UUID) error
	// MarkTracksHealthyForSourceSpec 承载 AssetStore 的生成 MarkTracksHealthyForSourceSpec 值。
	MarkTracksHealthyForSourceSpec(context.Context, uuid.UUID, uuid.UUID) error
	// UpsertRecentService 承载 AssetStore 的生成 UpsertRecentService 值。
	UpsertRecentService(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
	// ListRecentServices 承载 AssetStore 的生成 ListRecentServices 值。
	ListRecentServices(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]ServiceRecord, int64, error)
}

// AssetKindRecord 是一个平台资产类别注册记录。
type AssetKindRecord struct {
	// ID 是类别标识。
	ID string
	// ContractVersion 是类别契约版本。
	ContractVersion string
	// Enabled 表示类别是否启用。
	Enabled bool
	// PluginVersion 是类别插件版本。
	PluginVersion string
}

// AssetRefTrackRecord 是一个资产引用轨道。
type AssetRefTrackRecord struct {
	// ID 是轨道的唯一标识。
	ID uuid.UUID
	// AssetID 是所属资产的标识。
	AssetID uuid.UUID
	// RefType 是引用类型（branch/tag）。
	RefType string
	// RefName 是引用名。
	RefName string
	// LatestVersionID 是最新版本标识（可为空）。
	LatestVersionID *uuid.UUID
	// CurrentVersionID 是当前生效版本标识（可为空）。
	CurrentVersionID *uuid.UUID
	// Health 是轨道健康状态。
	Health string
}

// LayerHeadRecord 是一个层头指针。
type LayerHeadRecord struct {
	// LayerID 是所属层的标识。
	LayerID uuid.UUID
	// ScopeType 是作用域类型。
	ScopeType string
	// ScopeKey 是作用域键。
	ScopeKey string
	// LatestRevisionID 是最新修订标识（可为空）。
	LatestRevisionID *uuid.UUID
	// EffectiveRevisionID 是生效修订标识（可为空）。
	EffectiveRevisionID *uuid.UUID
	// CandidateRevisionID 是候选修订标识（可为空）。
	CandidateRevisionID *uuid.UUID
	// Generation 是层头代次。
	Generation int64
}

// NewAsset 承载一次资产 upsert 所需的值。
type NewAsset struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是资产的标识。
	ID uuid.UUID
	// ServiceID 是所属服务的标识。
	ServiceID uuid.UUID
	// Kind 是资产类别。
	Kind string
	// Name 是资产名。
	Name string
}

// NewAssetRefTrack 承载一次轨道创建所需的值。
type NewAssetRefTrack struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是轨道的标识。
	ID uuid.UUID
	// AssetID 是所属资产的标识。
	AssetID uuid.UUID
	// RefType 是引用类型（branch/tag）。
	RefType string
	// RefName 是引用名。
	RefName string
	// Health 是初始健康状态。
	Health string
}

// SourceSpecPatch 承载一次源配置更新的显式 PATCH 字段。
type SourceSpecPatch struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是源配置的标识。
	ID uuid.UUID
	// ExpectedRevision 是乐观并发所需版本号。
	ExpectedRevision int64
	// AssetNameTemplate 是资产命名模板（可为空）。
	AssetNameTemplate *string
	// Role 是层角色（可为空）。
	Role *string
	// Origin 是层来源（可为空）。
	Origin *string
	// Mode 是源模式（可为空）。
	Mode *string
	// Path 是源路径（可为空）。
	Path *string
	// ProducerProfileID 是生产者配置标识（可为空）。
	ProducerProfileID *uuid.UUID
	// Ord 是排序号（可为空）。
	Ord *int
	// TimeoutSec 是超时秒数（可为空）。
	TimeoutSec *int
	// BranchPatterns 是分支模式列表。
	BranchPatterns []string
	// Enabled 表示是否启用（可为空）。
	Enabled *bool
}

// NewLayer 承载一次层创建所需的值。
type NewLayer struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是层的标识。
	ID uuid.UUID
	// AssetID 是所属资产的标识。
	AssetID uuid.UUID
	// SourceSpecID 是关联源配置标识（可为空）。
	SourceSpecID *uuid.UUID
	// Role 是层角色（base/overlay）。
	Role string
	// Origin 是层来源（repo/third_party/manual/ai_generated）。
	Origin string
	// Ord 是排序号。
	Ord int
	// Dialect 是方言（可为空）。
	Dialect *string
	// Enabled 表示层是否启用。
	Enabled bool
	// BranchPatterns 是分支模式列表。
	BranchPatterns []string
	// DisplayName 是展示名。
	DisplayName string
}

// VersionRefRecord 是嵌在资产摘要中的版本引用。
type VersionRefRecord struct {
	// ID 是版本标识。
	ID uuid.UUID
	// Version 是版本标签。
	Version string
	// Lifecycle 是生命周期状态。
	Lifecycle string
}

// AssetSummaryRecord 是嵌在服务中的资产摘要投影。
type AssetSummaryRecord struct {
	// ID 是资产标识。
	ID uuid.UUID
	// Kind 是资产类别。
	Kind string
	// Name 是资产名。
	Name string
	// Lifecycle 是生命周期状态。
	Lifecycle string
	// Health 是健康状态。
	Health string
	// CurrentVersion 是当前生效版本引用（可为空）。
	CurrentVersion *VersionRefRecord
	// LatestVersion 是最新版本引用（可为空）。
	LatestVersion *VersionRefRecord
	// RefType 是引用类型（branch/tag）。
	RefType string
	// RefName 是引用名。
	RefName string
}

// MissingKindRecord 报告某服务缺少资产的一个已注册类别。
type MissingKindRecord struct {
	// Kind 是缺少的类别标识。
	Kind string
	// CanConfigure 表示该类别是否可手工配置。
	CanConfigure bool
	// CanGenerateWithAI 表示该类别是否可用 AI 生成。
	CanGenerateWithAI bool
}
