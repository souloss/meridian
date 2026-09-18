package service

import (
	"context"
	"time"
	"uuid"
)

// TenantAISettings 是租户设置 JSON blob 中携带的 AI/审核子快照。
// 它镜像 contracts/domain.yaml 的 defaults.tenant。
type TenantAISettings struct {
	// ExternalRevisionTrustMode 选择 review_required / trust_ai / trust_ai_and_third_party。
	ExternalRevisionTrustMode string `json:"externalRevisionTrustMode"`
	// AutoPublish 选择发布新 AI 版本是否自动（v1 恒为 false）。
	AutoPublish bool `json:"autoPublish"`
	// DefaultAiProducerProfileId 是可选的租户默认 AI 生产者配置。
	DefaultAiProducerProfileId *uuid.UUID `json:"defaultAiProducerProfileId,omitempty"`
}

// AiGenerateInput 承载一次 generateMissingAssetWithAi 请求在校验后的
// 值，用于在任务入队前传递。
type AiGenerateInput struct {
	// ServiceID 是所属服务；缺失资产在其下创建。
	ServiceID uuid.UUID
	// Kind 标识要生成的资产类别（如 openapi）。
	Kind string
	// Name 是缺失资产的校验后名称。
	Name string
	// RefType 默认是 branch；Ref 是请求的引用（可为空）。
	RefType string
	Ref     string
	// Hint 是转发给生产者的可选自由文本提示。
	Hint string
	// ProducerProfileID 是解析出的生产者配置（请求指定或租户默认）。
	ProducerProfileID uuid.UUID
	// IdempotencyKey 在 24 小时内去重该请求。
	IdempotencyKey uuid.UUID
	// PrincipalType 与 PrincipalID 绑定重放身份。
	PrincipalType string
	PrincipalID   uuid.UUID
	// RequestHash 是 32 字节 RFC 8785 摘要，用于重放比较。
	RequestHash []byte
}

// AiGenerateAccepted 是 generateMissingAssetWithAi 的 202 响应投影。
type AiGenerateAccepted struct {
	// AssetID 是资产标识。
	AssetID uuid.UUID
	// SourceID 是源配置标识。
	SourceID uuid.UUID
	// JobID 是任务标识。
	JobID uuid.UUID
	// Deduplicated 表示是否被语义合并去重。
	Deduplicated bool
}

// AiGenerateEnqueue 承载 store 在一个事务中原子创建 AI 源配置、
// 其层与生成任务所需的全部信息。
type AiGenerateEnqueue struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ServiceID 是所属服务的标识。
	ServiceID uuid.UUID
	// Kind 是资产类别。
	Kind string
	// Name 是资产名。
	Name string
	// Hint 是提示。
	Hint string
	// ProducerProfileID 是生产者配置标识。
	ProducerProfileID uuid.UUID
	// AssetID 是资产标识。
	AssetID uuid.UUID
	// SourceID 是源配置标识。
	SourceID uuid.UUID
	// LayerID 是层标识。
	LayerID uuid.UUID
	// Role 是层角色。
	Role string
	// Ord 是排序号。
	Ord int
	// ScopeType 是作用域类型。
	ScopeType string
	// ScopeKey 是作用域键。
	ScopeKey string
	// RefType 是引用类型。
	RefType string
	// RefName 是引用名。
	RefName string
	// ServiceRoot 是服务根目录。
	ServiceRoot string
	// IdempotencyKey 是幂等键。
	IdempotencyKey uuid.UUID
	// PrincipalType 是重放主体类型。
	PrincipalType string
	// PrincipalID 是重放主体标识。
	PrincipalID uuid.UUID
	// RequestHash 是请求摘要。
	RequestHash []byte
}

// AiGenerationJobContext 是 worker 从持久化任务输入重新读取的不可变上下文，
// 用于执行生产者并持久化修订。
type AiGenerationJobContext struct {
	// AssetID 是资产标识。
	AssetID uuid.UUID
	// SourceID 是源配置标识。
	SourceID uuid.UUID
	// LayerID 是层标识。
	LayerID uuid.UUID
	// Kind 是资产类别。
	Kind string
	// Name 是资产名。
	Name string
	// ProducerProfileID 是生产者配置标识。
	ProducerProfileID uuid.UUID
	// RefType 是引用类型。
	RefType string
	// RefName 是引用名。
	RefName string
	// ScopeType 是作用域类型。
	ScopeType string
	// ScopeKey 是作用域键。
	ScopeKey string
	// ServiceRoot 是服务根目录。
	ServiceRoot string
	// Hint 是提示。
	Hint string
}

// AiGenerationOutcome 捕获一次 AI 生成任务的终态结果，用于持久化
// ai_generation_results 行。
type AiGenerationOutcome struct {
	// JobID 是任务标识。
	JobID uuid.UUID
	// Stage 是终态阶段。
	Stage string
	// Status 是任务状态。
	Status string
	// ErrorCode 是错误码（成功时为空）。
	ErrorCode string
	// ContentRef 是内容 blob 引用（可为空）。
	ContentRef *string
	// ContentHash 是内容哈希（可为空）。
	ContentHash *string
	// ContentType 是内容媒体类型（可为空）。
	ContentType *string
	// Manifest 是完成清单投影。
	Manifest map[string]any
	// RevisionID 是修订标识（可为空）。
	RevisionID *uuid.UUID
}

// ReviewContextResult 承载一次 getReviewContext 响应：候选修订、
// 当前生效修订（冷启动 base 时为 nil）与候选作者。
type ReviewContextResult struct {
	// Revision 是候选修订。
	Revision LayerRevisionRecord
	// CurrentEffectiveRevision 是当前生效修订（可为空）。
	CurrentEffectiveRevision *LayerRevisionRecord
	// Author 是候选作者（可为空）。
	Author *User
}

// ReviewDecision 承载一次批准/拒绝决策的结果。
type ReviewDecision struct {
	// Revision 是更新后的修订。
	Revision LayerRevisionRecord
	// EffectiveRevisionID 是生效修订标识（可为空）。
	EffectiveRevisionID *uuid.UUID
	// MergeJobID 是合并任务标识（可为空）。
	MergeJobID *uuid.UUID
	// SupersededRevisionIDs 是被新候选替换的旧修订 ID 列表。
	SupersededRevisionIDs []uuid.UUID
	// Deduplicated 表示合并任务是否被去重。
	Deduplicated bool
}

// PublishInput 承载一次 publishAssetVersion 请求的校验后值。
type PublishInput struct {
	// VersionID 是版本标识。
	VersionID uuid.UUID
	// ExpectedRevision 是乐观并发所需版本号。
	ExpectedRevision int64
	// Version 是可选的替换版本标签。
	Version *string
	// Labels 是发布标签。
	Labels map[string]string
	// IdempotencyKey 是幂等键。
	IdempotencyKey uuid.UUID
	// PrincipalType 是重放主体类型。
	PrincipalType string
	// PrincipalID 是重放主体标识。
	PrincipalID uuid.UUID
	// RequestHash 是请求摘要。
	RequestHash []byte
}

// ReviewRevisionRecord 是携带层/资产上下文的层修订。
type ReviewRevisionRecord struct {
	// LayerRevisionRecord 是嵌入的层修订。
	LayerRevisionRecord
	// AssetID 是资产标识。
	AssetID uuid.UUID
	// Role 是层角色。
	Role string
	// Origin 是层来源。
	Origin string
}

// AiGenerationStore 是 M3 AI 生成、审核与发布的持久化边界。
// 它用 AI 任务、审核与发布操作扩展 LayerEditStore。每个方法都保留租户谓词。
type AiGenerationStore interface {
	LayerEditStore
	// GetAssetByName 按服务、类别与名称返回一个资产。
	GetAssetByName(context.Context, uuid.UUID, uuid.UUID, string, string) (AssetRecord, error)
	// GetBaseLayerForAsset 在存在时返回资产的 base 层。
	GetBaseLayerForAsset(context.Context, uuid.UUID, uuid.UUID) (LayerRecord, error)
	// GetLatestLayerRevision 返回一个层作用域内的最新修订。
	GetLatestLayerRevision(context.Context, uuid.UUID, uuid.UUID, string, string) (LayerRevisionRecord, error)
	// GetServiceBySlug 返回拥有缺失资产的服务。
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	// GetProducerProfile 返回一个生产者配置用于可用性校验。
	GetProducerProfile(context.Context, uuid.UUID) (ProducerProfile, error)
	// GetTenantSettings 返回租户设置 JSON，用于 trust/autopublish 解析。
	GetTenantSettings(context.Context, uuid.UUID) ([]byte, error)
	// EnqueueAiGeneration 原子创建 AI 源配置、层与任务。
	EnqueueAiGeneration(context.Context, AiGenerateEnqueue) (AiGenerateAccepted, error)
	// GetAiGenerationJobContext 返回 worker 的持久化任务输入。
	GetAiGenerationJobContext(context.Context, uuid.UUID, uuid.UUID) (AiGenerationJobContext, error)
	// GetAiGenerationResult 返回一个持久化的 AI 生成结果。
	GetAiGenerationResult(context.Context, uuid.UUID, uuid.UUID) (AiGenerationOutcome, error)
	// UpsertAiGenerationResult 持久化一个 AI 生成结果。
	UpsertAiGenerationResult(context.Context, uuid.UUID, AiGenerationOutcome) error
	// GetLayerRevisionForReview 返回带层/资产上下文的修订。
	GetLayerRevisionForReview(context.Context, uuid.UUID, uuid.UUID) (ReviewRevisionRecord, error)
	// ListEnabledLayerHeadsForAssetScope 列出带候选指针的层头，用于发布门控。
	ListEnabledLayerHeadsForAssetScope(context.Context, uuid.UUID, uuid.UUID, string, string) ([]LayerHeadRecord, error)
	// UpdateLayerRevisionReview 将一个待审核修订标记为批准或拒绝。
	UpdateLayerRevisionReview(context.Context, uuid.UUID, uuid.UUID, string, *string) (LayerRevisionRecord, error)
	// SupersedeLayerRevision 将一个待审核修订标记为被替换。
	SupersedeLayerRevision(context.Context, uuid.UUID, uuid.UUID) error
	// UnpublishAssetVersion 将已发布版本降级为 draft，用于历史输入复用。
	UnpublishAssetVersion(context.Context, uuid.UUID, uuid.UUID) error
	// LockAssetRefTrack 为发布事务锁定一个轨道行。
	LockAssetRefTrack(context.Context, uuid.UUID, uuid.UUID) (AssetRefTrackRecord, error)
	// BumpAssetRefTrackGeneration 递增轨道期望代次。
	BumpAssetRefTrackGeneration(context.Context, uuid.UUID, uuid.UUID) error
	// UpdateAssetVersionPublish 在乐观并发下发布一个版本。
	UpdateAssetVersionPublish(context.Context, uuid.UUID, uuid.UUID, string, int64) (AssetVersionRecord, error)
}

var _ = time.Now
