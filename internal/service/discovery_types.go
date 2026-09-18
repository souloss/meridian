package service

import (
	"context"
	"time"
	"uuid"
)

// ProducerProfileKind 标识生产者类别（producerKind 枚举）。
type ProducerProfileKind string

// 生产者类别常量（producerKind 枚举，与 discovery_types.go 的 producerKindCommand/AI 同值）。
const (
	// producerKindCommand 是命令式生产者。
	producerKindCommand = "command"
	// producerKindAI 是 AI 生产者。
	producerKindAI = "ai"
)

// ProducerProfileOption 是租户可见的生产者选择投影。
type ProducerProfileOption struct {
	// ID 是生产者配置的标识。
	ID uuid.UUID
	// Name 是配置名。
	Name string
	// Kind 是生产者类别（command/ai）。
	Kind string
	// SupportedKinds 是该生产者支持的资产类别列表。
	SupportedKinds []string
	// Network 是网络策略（none/inherit）。
	Network string
	// DependencyStatus 是依赖状态（available/unavailable）。
	DependencyStatus string
	// UnavailableReason 是不可用原因（可为空）。
	UnavailableReason *string
}

// ProducerProfile 是平台管理的生产者配置记录。
type ProducerProfile struct {
	// ID 是生产者配置的标识。
	ID uuid.UUID
	// Name 是配置名。
	Name string
	// Kind 是生产者类别（command/ai）。
	Kind string
	// Executable 是可执行文件绝对路径。
	Executable string
	// Args 是可执行文件参数。
	Args []string
	// EnvAllowlist 是允许注入的环境变量白名单。
	EnvAllowlist []string
	// SupportedKinds 是支持的资产类别列表。
	SupportedKinds []string
	// ReplaySafe 表示生产者输出是否可确定性重放。
	ReplaySafe bool
	// Network 是网络策略（none/inherit）。
	Network string
	// TimeoutSec 是超时秒数。
	TimeoutSec int
	// MemoryMiB 是内存上限（MiB）。
	MemoryMiB int
	// CPUSeconds 是 CPU 时间配额（秒）。
	CPUSeconds int
	// Pids 是进程数上限。
	Pids int
	// Enabled 表示是否启用。
	Enabled bool
	// DependencyStatus 是依赖状态（available/unavailable）。
	DependencyStatus string
	// UnavailableReason 是不可用原因（可为空）。
	UnavailableReason *string
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// NewProducerProfile 承载校验后的值，用于一次原子插入。
type NewProducerProfile struct {
	// ID 是生产者配置的标识。
	ID uuid.UUID
	// Name 是配置名。
	Name string
	// Kind 是生产者类别（command/ai）。
	Kind string
	// Executable 是可执行文件绝对路径。
	Executable string
	// Args 是可执行文件参数。
	Args []string
	// EnvAllowlist 是环境变量白名单。
	EnvAllowlist []string
	// SupportedKinds 是支持的资产类别列表。
	SupportedKinds []string
	// ReplaySafe 表示是否可确定性重放。
	ReplaySafe bool
	// Network 是网络策略（none/inherit）。
	Network string
	// TimeoutSec 是超时秒数。
	TimeoutSec int
	// MemoryMiB 是内存上限（MiB）。
	MemoryMiB int
	// CPUSeconds 是 CPU 时间配额（秒）。
	CPUSeconds int
	// Pids 是进程数上限。
	Pids int
	// Enabled 表示是否启用。
	Enabled bool
	// DependencyStatus 是依赖状态（available/unavailable）。
	DependencyStatus string
	// UnavailableReason 是不可用原因（可为空）。
	UnavailableReason *string
}

// ServiceRecord 是发现验收使用的租户可见服务投影。
type ServiceRecord struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是服务的标识。
	ID uuid.UUID
	// RepositoryID 是所属仓库的标识。
	RepositoryID uuid.UUID
	// Slug 是服务的 URL 标识。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
	// Description 是描述（可为空）。
	Description *string
	// RootDir 是服务在仓库中的根目录。
	RootDir string
	// Language 是主要语言（可为空）。
	Language *string
	// Framework 是主要框架（可为空）。
	Framework *string
	// Visibility 是可见性（private/internal/public）。
	Visibility string
	// Lifecycle 是生命周期状态（draft/published/deprecated/retired）。
	Lifecycle string
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// SourceSpecRecord 是租户可见的源配置投影。
type SourceSpecRecord struct {
	// ID 是源配置的标识。
	ID uuid.UUID
	// ServiceID 是所属服务的标识。
	ServiceID uuid.UUID
	// Kind 是资产类别。
	Kind string
	// AssetNameTemplate 是资产命名模板。
	AssetNameTemplate string
	// Role 是层角色（base/overlay）。
	Role string
	// Origin 是层来源（repo/third_party/manual/ai_generated）。
	Origin string
	// Mode 是源模式（builtin/command/push/manual/ai）。
	Mode string
	// Path 是源路径（可为空）。
	Path *string
	// ProducerProfileID 是生产者配置标识（可为空）。
	ProducerProfileID *uuid.UUID
	// Ord 是排序号。
	Ord int
	// TimeoutSec 是超时秒数。
	TimeoutSec int
	// BranchPatterns 是分支模式列表。
	BranchPatterns []string
	// Enabled 表示是否启用。
	Enabled bool
	// ConfigOrigin 是配置来源（api/repository）。
	ConfigOrigin string
	// BindingsCount 是活跃绑定数量。
	BindingsCount int
	// InitialLayerID 仅对 manual 模式源配置非空：createSourceSpec
	// 原子地创建其全局绑定与层并返回该层标识。
	InitialLayerID *uuid.UUID
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// NewSourceSpec 承载校验后的值，用于一次原子插入。
type NewSourceSpec struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是源配置的标识。
	ID uuid.UUID
	// ServiceID 是所属服务的标识。
	ServiceID uuid.UUID
	// Kind 是资产类别。
	Kind string
	// AssetNameTemplate 是资产命名模板。
	AssetNameTemplate string
	// Role 是层角色（base/overlay）。
	Role string
	// Origin 是层来源（repo/third_party/manual/ai_generated）。
	Origin string
	// Mode 是源模式（builtin/command/push/manual/ai）。
	Mode string
	// Path 是源路径（可为空）。
	Path *string
	// ProducerProfileID 是生产者配置标识（可为空）。
	ProducerProfileID *uuid.UUID
	// Ord 是排序号。
	Ord int
	// TimeoutSec 是超时秒数。
	TimeoutSec int
	// BranchPatterns 是分支模式列表。
	BranchPatterns []string
	// Enabled 表示是否启用。
	Enabled bool
	// ConfigOrigin 是配置来源（api/repository）。
	ConfigOrigin string
	// TargetAssetID 仅 manual 模式必填；目标资产必须属于路径服务且类别匹配。
	TargetAssetID *uuid.UUID
	// ReplaceAiBase 显式归档现有 AI base 后再创建仓库 base（SMK-033）。
	ReplaceAiBase bool
}

// SourceBindingRecord 是一个物化的源绑定投影。
type SourceBindingRecord struct {
	// ID 是绑定的标识。
	ID uuid.UUID
	// SourceSpecID 是关联源配置的标识。
	SourceSpecID uuid.UUID
	// ScopeType 是作用域类型。
	ScopeType string
	// ScopeKey 是作用域键。
	ScopeKey string
	// ExpansionKey 是展开键。
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

// DiscoveryCandidateRecord 是一个发现候选投影。
type DiscoveryCandidateRecord struct {
	// ID 是候选的标识。
	ID uuid.UUID
	// RepositoryID 是所属仓库的标识。
	RepositoryID uuid.UUID
	// CommitSHA 是候选对应的提交 SHA。
	CommitSHA string
	// RootDir 是候选服务根目录。
	RootDir string
	// Detected 是检测到的标记信息。
	Detected map[string]any
	// Status 是候选状态。
	Status string
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// DiscoverJobInput 描述一次幂等的仓库发现请求。
type DiscoverJobInput struct {
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
}

// RepositorySyncInput 描述一次仓库同步请求。
type RepositorySyncInput struct {
	// RefType 是引用类型（branch/tag）。
	RefType string
	// RefName 是引用名。
	RefName string
	// Force 表示是否强制同步（可为空）。
	Force *bool
}

// SourceSpecPatchInput 承载一次源配置更新的显式 PATCH 字段。
type SourceSpecPatchInput struct {
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

// NewService 承载校验后的值，用于一次原子服务插入。
type NewService struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是服务的标识。
	ID uuid.UUID
	// RepositoryID 是所属仓库的标识。
	RepositoryID uuid.UUID
	// Slug 是服务的 URL 标识。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
	// Description 是描述（可为空）。
	Description *string
	// RootDir 是服务根目录。
	RootDir string
	// Visibility 是可见性（private/internal/public）。
	Visibility string
}

// ProducerStore 是平台生产者配置的持久化边界。
type ProducerStore interface {
	CreateProducerProfile(context.Context, NewProducerProfile) (ProducerProfile, error)
	ListAvailableProducerProfiles(context.Context, string) ([]ProducerProfile, error)
	GetProducerProfile(context.Context, uuid.UUID) (ProducerProfile, error)
}

// DiscoveryStore 是仓库发现、服务验收与源配置的持久化边界。它同时事务性
// 地入队发现与同步任务。每个方法都保留租户谓词。
type DiscoveryStore interface {
	ProducerStore
	GetRepository(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	EnqueueDiscoveryJob(context.Context, DiscoverJobInput) (JobAccepted, error)
	EnqueueSyncJob(context.Context, SyncJobInput) (JobAccepted, error)
	UpsertDiscoveryCandidate(context.Context, NewDiscoveryCandidate) (DiscoveryCandidateRecord, error)
	ListDiscoveryCandidates(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]DiscoveryCandidateRecord, int64, error)
	GetDiscoveryCandidate(context.Context, uuid.UUID, uuid.UUID) (DiscoveryCandidateRecord, error)
	AcceptDiscoveryCandidate(context.Context, uuid.UUID, uuid.UUID) error
	CreateService(context.Context, NewService) (ServiceRecord, error)
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	CountServices(context.Context, uuid.UUID) (int64, int64, error)
	CreateSourceSpec(context.Context, NewSourceSpec) (SourceSpecRecord, error)
	GetSourceSpec(context.Context, uuid.UUID, uuid.UUID) (SourceSpecRecord, error)
	// GetAiBaseForService 返回某服务/类别现有的 AI 生成 base（存在时）。
	GetAiBaseForService(context.Context, uuid.UUID, uuid.UUID, string) (SourceSpecRecord, error)
	// ReplaceAiBaseForService 归档 AI base 并在同一事务中创建仓库 base。
	ReplaceAiBaseForService(context.Context, uuid.UUID, uuid.UUID, string) error
	UpdateSourceSpec(context.Context, SourceSpecPatch) (SourceSpecRecord, error)
	ListSourceSpecsForService(context.Context, uuid.UUID, uuid.UUID) ([]SourceSpecRecord, error)
	ListSourceBindings(context.Context, uuid.UUID, uuid.UUID) ([]SourceBindingRecord, error)
	CountSourceBindings(context.Context, uuid.UUID, uuid.UUID) (int64, error)
	CountActiveBindings(context.Context, uuid.UUID, uuid.UUID) (int64, error)
	UpsertRecentService(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
	ListRecentServices(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]ServiceRecord, int64, error)
}

// NewDiscoveryCandidate 承载一次 upsert 所需的已发现服务根信息。
type NewDiscoveryCandidate struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是候选的标识。
	ID uuid.UUID
	// RepositoryID 是所属仓库的标识。
	RepositoryID uuid.UUID
	// CommitSHA 是候选对应的提交 SHA。
	CommitSHA string
	// RootDir 是候选服务根目录。
	RootDir string
	// Detected 是检测到的标记信息。
	Detected map[string]any
}
