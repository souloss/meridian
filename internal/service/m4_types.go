package service

import (
	"context"
	"time"
	"uuid"
)

// SystemGroupRecord 是租户可见的系统分组投影。
type SystemGroupRecord struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是分组的标识。
	ID uuid.UUID
	// Slug 是分组的 URL 标识。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
	// Description 是描述（可为空）。
	Description *string
	// ServiceIDs 是成员服务 ID 列表。
	ServiceIDs []uuid.UUID
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// NewSystemGroup 承载一次分组插入所需的校验后值。
type NewSystemGroup struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是分组的标识。
	ID uuid.UUID
	// Slug 是分组的 URL 标识。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
	// Description 是描述（可为空）。
	Description *string
}

// SystemGroupStore 是 M4 系统分组与搜索的持久化边界。
type SystemGroupStore interface {
	CreateSystemGroup(context.Context, NewSystemGroup) (SystemGroupRecord, error)
	GetSystemGroup(context.Context, uuid.UUID, uuid.UUID) (SystemGroupRecord, error)
	ListSystemGroups(context.Context, uuid.UUID) ([]SystemGroupRecord, error)
	ListSystemGroupMembers(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error)
	// ListSystemGroupMembersForGroups 一次批量返回多个分组的成员，供列表/搜索去 N+1。
	ListSystemGroupMembersForGroups(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	ReplaceSystemGroupMembers(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) error
	BumpSystemGroupRevision(context.Context, uuid.UUID, uuid.UUID, int64) error
	// ListServicesByIDs 按 ID 返回租户内服务，按 ID 排序。
	ListServicesByIDs(context.Context, uuid.UUID, []uuid.UUID) ([]ServiceRecord, error)
}

// SearchInput 承载一次租户搜索请求。
type SearchInput struct {
	// Query 是搜索查询串。
	Query string
	// Filter 是 facet 过滤器。
	Filter SearchFilter
	// Page 是页码（从 1 开始）。
	Page int
	// PageSize 是每页条数。
	PageSize int
}

// SearchFilter 承载租户搜索 facet 过滤条件。
type SearchFilter struct {
	// RepositoryIDs 是按仓库过滤的 ID 列表。
	RepositoryIDs []uuid.UUID
	// ServiceIDs 是按服务过滤的 ID 列表。
	ServiceIDs []uuid.UUID
	// GroupIDs 是按系统分组过滤的 ID 列表。
	GroupIDs []uuid.UUID
	// Kinds 是按资产类别过滤。
	Kinds []string
	// ItemTypes 是按条目类型过滤。
	ItemTypes []string
	// Languages 是按语言过滤。
	Languages []string
	// Lifecycles 是按生命周期过滤。
	Lifecycles []string
	// HasAiLayer 按是否有 AI 层过滤（可为空）。
	HasAiLayer *bool
	// HasBreakingChanges 按是否有破坏性变更过滤（可为空）。
	HasBreakingChanges *bool
}

// SearchHitRecord 是一个租户搜索结果投影。
type SearchHitRecord struct {
	// Type 是命中类型（repository | service | asset | item）。
	Type string
	// ID 是命中标识。
	ID string
	// Title 是标题。
	Title string
	// Subtitle 是副标题（可为空）。
	Subtitle *string
	// Score 是相关度得分。
	Score float32
	// Repository 是所属仓库引用。
	Repository RepositoryRef
	// Service 是所属服务引用（可为空）。
	Service *ServiceRef
	// Kind 是资产类别。
	Kind string
	// DeepLink 是条目命中的定位信息（可为空）。
	DeepLink *SearchDeepLink
	// Highlights 是高亮片段。
	Highlights map[string][]string
}

// RepositoryRef 是搜索命中所属仓库的引用。
type RepositoryRef struct {
	// ID 是仓库标识。
	ID uuid.UUID
	// DefaultBranch 是默认分支。
	DefaultBranch string
}

// ServiceRef 是搜索命中所属服务的引用。
type ServiceRef struct {
	// ID 是服务标识。
	ID uuid.UUID
	// Slug 是服务 slug。
	Slug string
	// DisplayName 是服务展示名。
	DisplayName string
}

// SearchDeepLink 定位一个条目搜索命中。
type SearchDeepLink struct {
	// AssetID 是资产标识。
	AssetID uuid.UUID
	// VersionID 是版本标识。
	VersionID uuid.UUID
	// ItemKey 是条目键（可为空）。
	ItemKey *string
}

// SearchFacetBucket 是一个 facet 的值计数。
type SearchFacetBucket struct {
	// Value 是 facet 值。
	Value string
	// Count 是命中计数。
	Count int
}

// SearchResultRecord 是一页租户搜索投影。
type SearchResultRecord struct {
	// Total 是命中总数。
	Total int
	// Page 是当前页码。
	Page int
	// PageSize 是每页条数。
	PageSize int
	// Items 是命中列表。
	Items []SearchHitRecord
	// Facets 是 facet 聚合。
	Facets SearchFacetSet
}

// SearchFacetSet 聚合一次搜索结果的 facet 桶。
// Teams 与 Tags 对应冻结契约字段，但当前数据模型无团队-服务关系与标签实体，
// 后端恒返回空桶，保留以维持契约形状（见 contracts/openapi.yaml SearchFacetSet）。
type SearchFacetSet struct {
	// Repositories 是仓库 facet 桶。
	Repositories []SearchFacetBucket
	// Teams 是团队 facet 桶（无数据模型支撑，恒空）。
	Teams []SearchFacetBucket
	// Groups 是分组 facet 桶。
	Groups []SearchFacetBucket
	// Kinds 是类别 facet 桶。
	Kinds []SearchFacetBucket
	// Lifecycles 是生命周期 facet 桶。
	Lifecycles []SearchFacetBucket
	// Tags 是标签 facet 桶（无数据模型支撑，恒空）。
	Tags []SearchFacetBucket
	// Languages 是语言 facet 桶。
	Languages []SearchFacetBucket
	// ItemTypes 是条目类型 facet 桶。
	ItemTypes []SearchFacetBucket
	// HasAiLayer 是是否有 AI 层 facet 桶。
	HasAiLayer []SearchFacetBucket
	// HasBreakingChanges 是是否有破坏性变更 facet 桶。
	HasBreakingChanges []SearchFacetBucket
}

// SearchFacetCounts 是一次搜索各维度 facet 桶的持久化聚合结果。
// 仅包含有命中的桶；类别维度由服务层叠加全量启用类别作为零计数基线。
type SearchFacetCounts struct {
	// Kinds 是类别计数。
	Kinds []SearchFacetBucket
	// Lifecycles 是生命周期计数。
	Lifecycles []SearchFacetBucket
	// Languages 是语言计数。
	Languages []SearchFacetBucket
	// ItemTypes 是条目类型计数。
	ItemTypes []SearchFacetBucket
	// Repositories 是仓库计数。
	Repositories []SearchFacetBucket
	// Groups 是系统分组计数。
	Groups []SearchFacetBucket
	// HasAiLayer 是有 AI 层的条目计数。
	HasAiLayer []SearchFacetBucket
	// HasBreakingChanges 是有破坏性变更的条目计数。
	HasBreakingChanges []SearchFacetBucket
}
