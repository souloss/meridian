package service

// 本文件集中放置 M3 AI 生成、评审与发布域的领域常量。
// 值来源：contracts/domain.yaml 的 revisionReview / sourceCompatibility / versions 段。
// DB 列值常量见对应 model 文件与 migration 注释；技术维度常量不在此列。

const (
	// aiOrigin 是 AI 生成层的来源标识（layerOrigin: ai_generated）。
	aiOrigin = "ai_generated"
	// aiMode 是 AI 生成源配置的模式（sourceMode: ai）。
	aiMode = "ai"
	// aiJobType 是 AI 生成任务的持久化行为标识（jobs.type: asset.ai_generate）。
	aiJobType = "asset.ai_generate"
	// aiJobScope 是 AI 生成任务的资源类别（jobs.scope_type: asset）。
	aiJobScope = "asset"
	// aiSourceScope 是 AI 源配置的任务资源类别（jobs.scope_type: source）。
	aiSourceScope = "source"
)

const (
	// reviewStatusPending 是待审核候选修订的审核状态。
	reviewStatusPending = "pending_review"
	// reviewStatusApproved 是已批准修订的审核状态。
	reviewStatusApproved = "approved"
	// reviewStatusRejected 是已拒绝修订的审核状态。
	reviewStatusRejected = "rejected"
	// reviewStatusSuperseded 是被新候选替换的旧候选修订状态。
	reviewStatusSuperseded = "superseded"
	// reviewStatusNotRequired 是无需审核修订的审核状态。
	reviewStatusNotRequired = "not_required"
)

const (
	// trustModeReviewRequired 要求所有外部（AI/第三方）修订经人工批准后生效。
	trustModeReviewRequired = "review_required"
	// trustModeTrustAI 直接信任 AI 生成内容为已批准，第三方仍需审核。
	trustModeTrustAI = "trust_ai"
	// trustModeTrustAIAndThirdParty 同时信任 AI 与第三方生成内容。
	trustModeTrustAIAndThirdParty = "trust_ai_and_third_party"
)

const (
	// lifecycleDraft 是资产版本的初始生命周期状态。
	lifecycleDraft = "draft"
	// lifecyclePublished 是资产版本的已发布生命周期状态。
	lifecyclePublished = "published"
)

const (
	// producerManifestFile 是生产者成功时的完成清单文件名（$OUTPUT_DIR/manifest.yaml）。
	producerManifestFile = "manifest.yaml"
	// aiDefaultTimeoutSec 是 AI 源配置缺省超时（与 discovery.go defaultSourceTimeoutAI 同口径）。
	aiDefaultTimeoutSec = 600
)

// AI 生成结果错误码（ai_generation_results.error_code 与 task.AiGenerateResult.ErrorCode）。
const (
	// errProducerTimedOutCode 表示生产者执行超时。
	errProducerTimedOutCode = "timeout"
	// errProducerFailedCode 表示生产者进程执行失败（非零退出）。
	errProducerFailedCode = "producer_failed"
)

const (
	// aiGenerationRefTypeDefault 是 AI 生成任务缺省的引用类别。
	aiGenerationRefTypeDefault = "branch"
	// aiRevisionContentTypeDefault 是 AI 生成内容缺省的媒体类型。
	aiRevisionContentTypeDefault = "application/yaml"
)

// trustApprovedForOrigin 判定给定 trust 模式是否让某来源的初始审核状态直接为 approved。
//
//	review_required           → ai_generated-pending, third_party-pending
//	trust_ai                  → ai_generated-approved, third_party-pending
//	trust_ai_and_third_party  → ai_generated-approved, third_party-approved
func trustApprovedForOrigin(trustMode, origin string) bool {
	switch trustMode {
	case trustModeTrustAI:
		return origin == aiOrigin
	case trustModeTrustAIAndThirdParty:
		return origin == aiOrigin || origin == layerOriginThirdParty
	default:
		return false
	}
}
