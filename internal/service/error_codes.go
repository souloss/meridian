package service

// 本文件集中放置 API 错误码常量，供跨层（task / repository / handler）复用。
// 值来源：contracts/openapi.yaml 的 ErrorCode 冻结枚举，不得改动；
// 每个错误码面向用户的消息文本在 internal/i18n/locales/*.yaml 中按语言维护。

// API 错误码常量（值 = ErrorCode 枚举原样）。
const (
	// ErrorCodeUnauthenticated 表示未建立有效主体上下文。
	ErrorCodeUnauthenticated = "unauthenticated"
	// ErrorCodeNotFound 表示资源不存在或无权访问（两者对外合并）。
	ErrorCodeNotFound = "not_found"
	// ErrorCodeValidation 表示请求违反冻结的 API/领域校验规则。
	ErrorCodeValidation = "validation_error"
	// ErrorCodeOverlayInvalid 表示 overlay 文档非法或严格模式目标缺失。
	ErrorCodeOverlayInvalid = "overlay_invalid"
	// ErrorCodeInputSpecMismatch 表示视图输入不满足视图契约。
	ErrorCodeInputSpecMismatch = "input_spec_mismatch"
	// ErrorCodeBranchNotIndexed 表示所选 ref 尚未建立索引。
	ErrorCodeBranchNotIndexed = "branch_not_indexed"
	// ErrorCodeProducerUnavailable 表示所选生产者配置不可用。
	ErrorCodeProducerUnavailable = "producer_profile_unavailable"
	// ErrorCodeDuplicate 表示契约定义的唯一标识已存在。
	ErrorCodeDuplicate = "duplicate"
	// ErrorCodeVersionNotPublishable 表示版本被待审核候选阻塞不可发布。
	ErrorCodeVersionNotPublishable = "version_not_publishable"
	// ErrorCodeBaseLayerExists 表示仓库 base 会替换已存在的 AI base。
	ErrorCodeBaseLayerExists = "base_layer_exists"
	// ErrorCodeQuotaExceeded 表示租户资源配额将超出限制。
	ErrorCodeQuotaExceeded = "quota_exceeded"
	// ErrorCodeCredentialInUse 表示凭据仍被仓库引用。
	ErrorCodeCredentialInUse = "credential_in_use"
	// ErrorCodeInvalidState 表示资源生命周期禁止该操作。
	ErrorCodeInvalidState = "invalid_state"
	// ErrorCodeIdempotencyConflict 表示幂等键被不同请求摘要复用。
	ErrorCodeIdempotencyConflict = "idempotency_conflict"
	// ErrorCodeJobNotCancellable 表示任务已进入终态不可取消。
	ErrorCodeJobNotCancellable = "job_not_cancellable"
	// ErrorCodePreconditionFailed 表示 If-Match 令牌缺失或与行版本不匹配。
	ErrorCodePreconditionFailed = "precondition_failed"
	// ErrorCodeInternal 是所有未分类内部失败的兜底错误码。
	ErrorCodeInternal = "internal_error"
)
