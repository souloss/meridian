package service

import (
	"context"
	"time"
	"uuid"
)

// RepositoryError 是仓库操作后记录的脱敏健康错误。
type RepositoryError struct {
	// Class 是健康投影使用的稳定非秘密错误类别。
	Class string `json:"class"`
	// Message 是面向运营人员的脱敏诊断信息。
	Message string `json:"message"`
}

// RepositoryHealth 是持久化、非秘密的同步摘要。
type RepositoryHealth struct {
	// LastSyncAt 是最近一次完成仓库操作的 UTC 时刻。
	LastSyncAt *time.Time
	// LastCommit 是最近拉取到的提交 SHA（可用时）。
	LastCommit *string
	// LastError 是最近一次脱敏失败（操作失败时）。
	LastError *RepositoryError
	// FailStreak 是连续失败操作次数。
	FailStreak int
	// DurationMS 是最近一次完成操作的耗时。
	DurationMS *int
}

// RepositoryBranchPolicy 控制哪些 branch 与 tag 引用有资格同步。
type RepositoryBranchPolicy struct {
	// BranchPatterns 是分支匹配模式列表。
	BranchPatterns []string `json:"branchPatterns"`
	// TagPatterns 是标签匹配模式列表。
	TagPatterns []string `json:"tagPatterns"`
}

// RepositoryFetchConfig 控制检出深度、路径、子模块、代理与 SSH 信任策略。
type RepositoryFetchConfig struct {
	// Shallow 表示是否浅克隆。
	Shallow bool `json:"shallow"`
	// Depth 是浅克隆深度（可为空）。
	Depth *int `json:"depth,omitempty"`
	// Submodules 表示是否拉取子模块。
	Submodules bool `json:"submodules"`
	// Proxy 是代理地址（可为空）。
	Proxy *string `json:"proxy,omitempty"`
	// PathAllow 是允许的路径模式列表。
	PathAllow []string `json:"pathAllow"`
	// PathIgnore 是忽略的路径模式列表。
	PathIgnore []string `json:"pathIgnore"`
	// KnownHostPolicy 是 SSH 主机信任策略（strict/accept_new）。
	KnownHostPolicy string `json:"knownHostPolicy"`
}

// RepositoryRecord 是租户作用域的仓库元数据投影。
// 它绝不包含解密的凭据材料。
type RepositoryRecord struct {
	// TenantID 是该仓库的租户边界。
	TenantID uuid.UUID
	// ID 是应用生成的仓库标识。
	ID uuid.UUID
	// URL 是运营人员提交的不含凭据的 URL。
	URL string
	// CanonicalURL 是用于唯一性检查的规范化 URL。
	CanonicalURL string
	// CredentialID 是为拉取选定的租户或全局凭据。
	CredentialID *uuid.UUID
	// GlobalCredentialID 在 CredentialID 解析为平台凭据时填充。
	GlobalCredentialID *uuid.UUID
	// DefaultBranch 是请求省略引用时使用的默认 Git 引用。
	DefaultBranch string
	// BranchPolicy 控制有资格的分支与标签模式。
	BranchPolicy RepositoryBranchPolicy
	// FetchConfig 控制检出与 SSH 信任行为。
	FetchConfig RepositoryFetchConfig
	// SyncCron 是可选的五字段 UTC 调度。
	SyncCron *string
	// Note 是可选的运营人员备注。
	Note *string
	// Health 是持久化非秘密同步摘要。
	Health RepositoryHealth
	// Capabilities 列出认证主体可用的操作。
	Capabilities []string
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是 UTC 创建时刻。
	CreatedAt time.Time
	// UpdatedAt 是 UTC 元数据更新时刻。
	UpdatedAt time.Time
}

// NewRepository 包含校验后的值，用于一次原子仓库插入。
type NewRepository struct {
	// TenantID 标识所属租户。
	TenantID uuid.UUID
	// ID 标识新仓库。
	ID uuid.UUID
	// URL 是不含凭据材料的展示 URL。
	URL string
	// CanonicalURL 是规范化唯一键。
	CanonicalURL string
	// CredentialID 是可选的同租户凭据引用。
	CredentialID *uuid.UUID
	// GlobalCredID 是可选的平台凭据引用。
	GlobalCredID *uuid.UUID
	// DefaultBranch 是选定的默认 Git 引用。
	DefaultBranch string
	// BranchPolicy 是校验后的引用纳入策略。
	BranchPolicy RepositoryBranchPolicy
	// FetchConfig 是校验后的检出配置。
	FetchConfig RepositoryFetchConfig
	// SyncCron 是可选的调度。
	SyncCron *string
	// Note 是可选的运营人员备注。
	Note *string
}

// RepositoryPatch 包含 PATCH 请求显式提供的字段。
// 指针到指针的字段区分「省略」与显式 JSON null。
type RepositoryPatch struct {
	// CredentialID 区分省略、显式 null 或 UUID。
	CredentialID **uuid.UUID
	// DefaultBranch 是可选的替换默认引用。
	DefaultBranch *string
	// BranchPolicy 是可选的完整策略替换。
	BranchPolicy *RepositoryBranchPolicy
	// FetchConfig 是可选的完整检出配置替换。
	FetchConfig *RepositoryFetchConfig
	// SyncCron 区分省略、显式 null 或 cron 字符串。
	SyncCron **string
	// Note 区分省略、显式 null 或备注字符串。
	Note **string
}

// UpdateRepository 包含一次乐观并发仓库变更。
type UpdateRepository struct {
	// TenantID 标识所属租户。
	TenantID uuid.UUID
	// ID 标识被变更的仓库。
	ID uuid.UUID
	// ExpectedRevision 是变更所需的 ETag 版本号。
	ExpectedRevision int64
	// CredentialID 区分省略、显式 null 或 UUID。
	CredentialID **uuid.UUID
	// GlobalCredID 区分省略、显式 null 或 UUID。
	GlobalCredID **uuid.UUID
	// DefaultBranch 是可选的替换默认引用。
	DefaultBranch *string
	// BranchPolicy 是可选的完整策略替换。
	BranchPolicy *RepositoryBranchPolicy
	// FetchConfig 是可选的完整检出配置替换。
	FetchConfig *RepositoryFetchConfig
	// SyncCron 区分省略、显式 null 或 cron 字符串。
	SyncCron **string
	// Note 区分省略、显式 null 或备注字符串。
	Note **string
	// UpdatedAt 是 UTC 变更时间戳。
	UpdatedAt time.Time
}

// CredentialReference 标识哪个加密凭据表拥有一个 UUID。
type CredentialReference struct {
	// ID 是解析出的凭据标识。
	ID uuid.UUID
	// IsGlobal 表示平台持有的凭据表。
	IsGlobal bool
}

// RepositoryStore 是租户仓库配置的持久化边界。
// 每个实现都必须保留租户谓词，且绝不返回软删除行。
type RepositoryStore interface {
	// ResolveCredentialReference 承载 RepositoryStore 的生成 ResolveCredentialReference 值。
	ResolveCredentialReference(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (CredentialReference, bool, error)
	// CountRepositories 承载 RepositoryStore 的生成 CountRepositories 值。
	CountRepositories(context.Context, uuid.UUID) (int64, int64, error)
	// ListRepositories 承载 RepositoryStore 的生成 ListRepositories 值。
	ListRepositories(context.Context, uuid.UUID, string, int32, int32) ([]RepositoryRecord, int64, error)
	// GetRepository 承载 RepositoryStore 的生成 GetRepository 值。
	GetRepository(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	// CreateRepository 承载 RepositoryStore 的生成 CreateRepository 值。
	CreateRepository(context.Context, NewRepository) (RepositoryRecord, error)
	// UpdateRepository 承载 RepositoryStore 的生成 UpdateRepository 值。
	UpdateRepository(context.Context, UpdateRepository) (RepositoryRecord, error)
	// DeleteRepository 承载 RepositoryStore 的生成 DeleteRepository 值。
	DeleteRepository(context.Context, uuid.UUID, uuid.UUID, int64, time.Time) error
	// GetRepositoryWebhookSecretHash 返回仓库的 webhook 校验秘密摘要。
	GetRepositoryWebhookSecretHash(context.Context, uuid.UUID, uuid.UUID) ([]byte, error)
	// GetRepositoryTenantByID 跨租户按 id 定位仓库所属租户（入站 webhook）。
	GetRepositoryTenantByID(context.Context, uuid.UUID) (RepositoryTenantRef, error)
}

// RepositoryTenantRef 是仓库的租户与默认分支引用，供入站 webhook 定位。
type RepositoryTenantRef struct {
	// TenantID 是仓库所属租户的标识。
	TenantID uuid.UUID
	// DefaultBranch 是仓库默认分支。
	DefaultBranch string
}
