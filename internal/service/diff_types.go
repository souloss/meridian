package service

import (
	"context"
	"time"
	"uuid"
)

// DiffChangeKind 承载一条结构化的差异变更分类。
type DiffChangeKind struct {
	// ID 是稳定、确定性的单次运行内标识。
	ID string
	// Level 是破坏级别（breaking / risky / non_breaking / informational）。
	Level string
	// Code 是破坏规则或变更码（如 operation-removed）。
	Code string
	// Path 定位被变更的元素。
	Path string
	// Summary 是人类可读的变更描述。
	Summary string
	// Before 与 After 分别携带两侧原始 JSON 值（缺失时为 nil）。
	Before any
	// After 承载 DiffChangeKind 的生成 After 值。
	After any
}

// DiffCountsSummary 聚合一次差异结果的变更计数。
type DiffCountsSummary struct {
	// Added 是新增数量。
	Added int
	// Removed 是移除数量。
	Removed int
	// Modified 是修改数量。
	Modified int
	// Breaking 是破坏性变更数量。
	Breaking int
	// Risky 是有风险变更数量。
	Risky int
	// NonBreaking 是非破坏性变更数量。
	NonBreaking int
	// Informational 是信息性变更数量。
	Informational int
}

// ResolvedDocRef 是差异结果所报告的不变文档引用。
type ResolvedDocRef struct {
	// SourceType 是来源类型（version/ref/upload）。
	SourceType string
	// Kind 是资产类别。
	Kind string
	// ContentHash 是内容哈希。
	ContentHash string
	// AssetID 是资产标识（可为空）。
	AssetID *uuid.UUID
	// VersionID 是版本标识（可为空）。
	VersionID *uuid.UUID
	// UploadID 是上传标识（可为空）。
	UploadID *uuid.UUID
	// RequestedRefType 是请求的引用类型（可为空）。
	RequestedRefType *string
	// RequestedRef 是请求的引用（可为空）。
	RequestedRef *string
}

// DiffOutcome 是运行一次差异的结果。
type DiffOutcome struct {
	// Kind 是资产类别。
	Kind string
	// Left 是左侧文档引用。
	Left ResolvedDocRef
	// Right 是右侧文档引用。
	Right ResolvedDocRef
	// SnapshotID 是快照标识（可为空）。
	SnapshotID *uuid.UUID
	// Summary 是变更计数摘要。
	Summary DiffCountsSummary
	// Changes 是变更明细列表。
	Changes []DiffChangeKind
	// GeneratedAt 是生成时间。
	GeneratedAt time.Time
}

// DiffSelector 是一个已解析的差异输入（版本、引用或上传）。
type DiffSelector struct {
	// Type 是选择器类型（version | ref | upload）。
	Type string
	// VersionID 是版本标识（可为空）。
	VersionID *uuid.UUID
	// AssetID 是资产标识（可为空）。
	AssetID *uuid.UUID
	// RefType 是引用类型（可为空）。
	RefType *string
	// RefName 是引用名（可为空）。
	RefName *string
	// UploadID 是上传标识（可为空）。
	UploadID *uuid.UUID
	// Kind 是资产类别。
	Kind string
}

// DiffRunInput 承载一次 runDiff 请求。
type DiffRunInput struct {
	// Left 是左侧选择器。
	Left DiffSelector
	// Right 是右侧选择器。
	Right DiffSelector
	// RuleSetID 是规则集标识（可为空）。
	RuleSetID *uuid.UUID
	// Persist 表示是否持久化快照。
	Persist bool
}

// ShareLinkCreatedResult 是创建分享链接的响应投影。
type ShareLinkCreatedResult struct {
	// ID 是分享链接标识。
	ID uuid.UUID
	// Token 是签名分享令牌。
	Token string
	// ResourceType 是资源类型。
	ResourceType string
	// ResourceID 是资源标识（可为空）。
	ResourceID *uuid.UUID
	// DescriptorDigest 是描述符摘要。
	DescriptorDigest string
	// URL 是公开分享 URL。
	URL string
	// ExpiresAt 是过期时间。
	ExpiresAt time.Time
	// RevokedAt 是撤销时间（可为空）。
	RevokedAt *time.Time
	// CreatedAt 是创建时间。
	CreatedAt time.Time
}

// SharedViewResult 是匿名分享视图的解析结果。
type SharedViewResult struct {
	// ResourceType 是资源类型（view | diff_snapshot）。
	ResourceType string
	// ExpiresAt 是过期时间。
	ExpiresAt time.Time
	// SnapshotID 是快照标识（diff_snapshot 链接时有效）。
	SnapshotID uuid.UUID
	// CreatedBy 是创建者标识（diff_snapshot 链接时有效）。
	CreatedBy uuid.UUID
	// CreatedAt 是创建时间（diff_snapshot 链接时有效）。
	CreatedAt time.Time
	// Snapshot 是冻结的差异结果（diff_snapshot 链接时有效）。
	Snapshot DiffOutcome
	// Resolution 是冻结的视图解析结果（view 链接时有效）。
	Resolution *ViewResolution
}

// TodoRecord 是一条破坏性变更待办投影。
type TodoRecord struct {
	// ID 是待办的标识。
	ID uuid.UUID
	// AssetVersionID 是关联版本标识。
	AssetVersionID uuid.UUID
	// ServiceID 是关联服务标识。
	ServiceID uuid.UUID
	// Status 是待办状态。
	Status string
	// AckedBy 是确认人标识（可为空）。
	AckedBy *uuid.UUID
	// AckedAt 是确认时间（可为空）。
	AckedAt *time.Time
	// Comment 是备注（可为空）。
	Comment *string
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// PushRevisionInput 承载一次 pushAssetRevision 请求。
type PushRevisionInput struct {
	// ServiceSlug 是目标服务 slug。
	ServiceSlug string
	// Kind 是资产类别。
	Kind string
	// Name 是资产名。
	Name string
	// RefType 是引用类型。
	RefType string
	// Ref 是引用名。
	Ref string
	// SourceSystem 是来源系统标识。
	SourceSystem string
	// CreateIfMissing 表示缺失时是否创建。
	CreateIfMissing bool
	// Content 是推送的文档内容。
	Content string
	// ContentType 是内容媒体类型。
	ContentType string
	// Role 是层角色。
	Role string
	// Dialect 是方言（可为空）。
	Dialect *string
	// SourceCommit 是来源提交（可为空）。
	SourceCommit *string
	// IdempotencyKey 是幂等键。
	IdempotencyKey uuid.UUID
	// PrincipalType 是重放主体类型。
	PrincipalType string
	// PrincipalID 是重放主体标识。
	PrincipalID uuid.UUID
	// RequestHash 是请求摘要。
	RequestHash []byte
}

// PushRevisionResult 是 pushAssetRevision 的响应投影。
type PushRevisionResult struct {
	// AssetID 是资产标识。
	AssetID uuid.UUID
	// LayerID 是层标识。
	LayerID uuid.UUID
	// RevisionID 是修订标识。
	RevisionID uuid.UUID
	// JobID 是任务标识。
	JobID uuid.UUID
	// Deduplicated 表示是否被语义合并去重。
	Deduplicated bool
}

// DiffStore 是差异、分享、待办、上传与推送的持久化边界。
type DiffStore interface {
	AssetStore
	// GetServiceBySlug 按 slug 返回一个活跃服务。
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	// GetLayerRevision 返回一个不可变的层修订。
	GetLayerRevision(context.Context, uuid.UUID, uuid.UUID) (LayerRevisionRecord, error)
	// CreateSourceSpec 插入一个推送源配置。
	CreateSourceSpec(context.Context, NewSourceSpec) (SourceSpecRecord, error)
	// CreateUpload 持久化一个差异上传。
	CreateUpload(context.Context, NewUpload) (UploadRecord, error)
	// GetUpload 返回一个未过期的上传。
	GetUpload(context.Context, uuid.UUID, uuid.UUID) (UploadRecord, error)
	// CreateDiffSnapshot 持久化一个冻结的差异结果。
	CreateDiffSnapshot(context.Context, NewDiffSnapshot) (DiffSnapshotRecord, error)
	// GetDiffSnapshot 返回一个快照。
	GetDiffSnapshot(context.Context, uuid.UUID, uuid.UUID) (DiffSnapshotRecord, error)
	// CreateShareLink 持久化一个分享链接。
	CreateShareLink(context.Context, NewShareLink) (ShareLinkRecord, error)
	// GetShareLinkByTokenHash 返回一个活跃分享链接。
	GetShareLinkByTokenHash(context.Context, []byte) (ShareLinkRecord, error)
	// CreateBreakingTodoIfAbsent 按版本 + 服务创建一条待办（不存在时）。
	CreateBreakingTodoIfAbsent(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (TodoRecord, error)
	// ListBreakingTodos 按状态分页列出待办。
	ListBreakingTodos(context.Context, uuid.UUID, string, int32, int32) ([]TodoRecord, int64, error)
	// AckBreakingTodo 确认一条未完成待办。
	AckBreakingTodo(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time, *string) (TodoRecord, error)
	// GetSourceLayerByPushKey 通过稳定推送标识（kind + name）返回资产的推送层，
	// 使重复推送复用同一 overlay 层。
	GetSourceLayerByPushKey(context.Context, uuid.UUID, uuid.UUID) (LayerRecord, error)
	// ListServicesByRepositoryOwners 返回拥有某资产的服务 ID。
	ListServicesByRepositoryOwners(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error)
	// ListDiffRuleSets 返回租户内全部差异规则集。
	ListDiffRuleSets(context.Context, uuid.UUID) ([]DiffRuleSetRecord, error)
	// GetDiffRuleSet 返回一条差异规则集。
	GetDiffRuleSet(context.Context, uuid.UUID, uuid.UUID) (DiffRuleSetRecord, error)
	// CreateDiffRuleSet 创建一条差异规则集。
	CreateDiffRuleSet(context.Context, NewDiffRuleSet) (DiffRuleSetRecord, error)
	// UpdateDiffRuleSet 在 If-Match 下更新一条差异规则集。
	UpdateDiffRuleSet(context.Context, uuid.UUID, uuid.UUID, int64, DiffRuleSetPatch) (DiffRuleSetRecord, error)
	// DeleteDiffRuleSet 在 If-Match 下删除一条差异规则集。
	DeleteDiffRuleSet(context.Context, uuid.UUID, uuid.UUID, int64) error
	// ListDiffSnapshots 返回租户内一页差异快照。
	ListDiffSnapshots(context.Context, uuid.UUID, int32, int32) ([]DiffSnapshotRecord, int64, error)
	// DeleteDiffSnapshot 删除一条差异快照。
	DeleteDiffSnapshot(context.Context, uuid.UUID, uuid.UUID) error
	// ListShareLinks 返回租户内一页分享链接。
	ListShareLinks(context.Context, uuid.UUID, int32, int32) ([]ShareLinkRecord, int64, error)
	// RevokeShareLink 幂等撤销一条分享链接。
	RevokeShareLink(context.Context, uuid.UUID, uuid.UUID) error
}

// DiffRuleSetRecord 是一个差异规则集投影。
type DiffRuleSetRecord struct {
	// ID 是规则集的标识。
	ID uuid.UUID
	// Kind 是规则集适用的资产类别。
	Kind string
	// Name 是规则集名称。
	Name string
	// Version 是规则集版本号。
	Version int32
	// Rules 是规则列表 JSON。
	Rules []byte
	// Enabled 表示规则集是否启用。
	Enabled bool
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// NewDiffRuleSet 承载一次差异规则集插入。
type NewDiffRuleSet struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是规则集的标识。
	ID uuid.UUID
	// Kind 是规则集适用的资产类别。
	Kind string
	// Name 是规则集名称。
	Name string
	// Version 是规则集版本号。
	Version int32
	// Rules 是规则列表 JSON。
	Rules []byte
	// Enabled 表示规则集是否启用。
	Enabled bool
}

// DiffRuleSetPatch 承载一次差异规则集 PATCH 的显式字段。
type DiffRuleSetPatch struct {
	// Name 是替换名称（可为空）。
	Name *string
	// Rules 是替换规则列表 JSON（可为空）。
	Rules []byte
	// Enabled 是替换启用状态（可为空）。
	Enabled *bool
}

// UploadRecord 是一个差异上传投影。
type UploadRecord struct {
	// ID 是上传的标识。
	ID uuid.UUID
	// BlobDigest 是内容 blob 摘要。
	BlobDigest string
	// Kind 是资产类别。
	Kind string
	// ContentType 是内容媒体类型。
	ContentType string
	// SizeBytes 是内容大小（字节）。
	SizeBytes int64
	// ExpiresAt 是过期时间。
	ExpiresAt time.Time
	// CreatedBy 是创建者标识（可为空）。
	CreatedBy *uuid.UUID
	// CreatedAt 是创建时间。
	CreatedAt time.Time
}

// NewUpload 承载一次上传插入。
type NewUpload struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是上传的标识。
	ID uuid.UUID
	// BlobDigest 是内容 blob 摘要。
	BlobDigest string
	// Kind 是资产类别。
	Kind string
	// ContentType 是内容媒体类型。
	ContentType string
	// SizeBytes 是内容大小（字节）。
	SizeBytes int64
	// ExpiresAt 是过期时间。
	ExpiresAt time.Time
	// CreatedBy 是创建者标识（可为空）。
	CreatedBy *uuid.UUID
}

// DiffSnapshotRecord 是一个冻结的差异快照投影。
type DiffSnapshotRecord struct {
	// ID 是快照的标识。
	ID uuid.UUID
	// LeftSelector 是左侧选择器序列化字节。
	LeftSelector []byte
	// RightSelector 是右侧选择器序列化字节。
	RightSelector []byte
	// LeftArtifactRef 是左侧产物 blob 引用。
	LeftArtifactRef string
	// RightArtifactRef 是右侧产物 blob 引用。
	RightArtifactRef string
	// RuleSetID 是规则集标识（可为空）。
	RuleSetID *uuid.UUID
	// ResultRef 是结果 blob 引用。
	ResultRef string
	// Summary 是摘要序列化字节。
	Summary []byte
	// CreatedBy 是创建者标识。
	CreatedBy uuid.UUID
	// CreatedAt 是创建时间。
	CreatedAt time.Time
}

// NewDiffSnapshot 承载一次快照插入。
type NewDiffSnapshot struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是快照的标识。
	ID uuid.UUID
	// LeftSelector 是左侧选择器序列化字节。
	LeftSelector []byte
	// RightSelector 是右侧选择器序列化字节。
	RightSelector []byte
	// LeftArtifactRef 是左侧产物 blob 引用。
	LeftArtifactRef string
	// RightArtifactRef 是右侧产物 blob 引用。
	RightArtifactRef string
	// RuleSetID 是规则集标识（可为空）。
	RuleSetID *uuid.UUID
	// ResultRef 是结果 blob 引用。
	ResultRef string
	// Summary 是摘要序列化字节。
	Summary []byte
	// CreatedBy 是创建者标识。
	CreatedBy uuid.UUID
}

// ShareLinkRecord 是一个分享链接投影。
type ShareLinkRecord struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是分享链接的标识。
	ID uuid.UUID
	// TokenHash 是令牌哈希。
	TokenHash []byte
	// CreatorID 是创建者标识。
	CreatorID uuid.UUID
	// ResourceType 是资源类型。
	ResourceType string
	// ResourceID 是资源标识（可为空）。
	ResourceID *uuid.UUID
	// Descriptor 是描述符序列化字节。
	Descriptor []byte
	// ViewID 是视图标识（可为空）。
	ViewID *string
	// Options 是选项序列化字节。
	Options []byte
	// ArtifactAllowlist 是产物白名单序列化字节。
	ArtifactAllowlist []byte
	// ExpiresAt 是过期时间。
	ExpiresAt time.Time
	// RevokedAt 是撤销时间（可为空）。
	RevokedAt *time.Time
	// CreatedAt 是创建时间。
	CreatedAt time.Time
}

// NewShareLink 承载一次分享链接插入。
type NewShareLink struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是分享链接的标识。
	ID uuid.UUID
	// TokenHash 是令牌哈希。
	TokenHash []byte
	// CreatorID 是创建者标识。
	CreatorID uuid.UUID
	// ResourceType 是资源类型。
	ResourceType string
	// ResourceID 是资源标识（可为空）。
	ResourceID *uuid.UUID
	// Descriptor 是描述符序列化字节。
	Descriptor []byte
	// ViewID 是视图标识（可为空）。
	ViewID *string
	// Options 是选项序列化字节。
	Options []byte
	// ArtifactAllowlist 是产物白名单序列化字节。
	ArtifactAllowlist []byte
	// ExpiresAt 是过期时间。
	ExpiresAt time.Time
}
