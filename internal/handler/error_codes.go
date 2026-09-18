package handler

// 本文件集中放置 API 错误码常量与消息渲染入口。
// 错误码值来自 contracts/openapi.yaml 的 ErrorCode 冻结枚举，不得改动；
// 每个错误码的消息文本在 internal/i18n/locales/*.yaml 中按语言维护。

// API 错误码常量（值 = ErrorCode 枚举原样）。
const (
	// errorCodeUnauthenticated 表示未建立有效主体上下文。
	errorCodeUnauthenticated = "unauthenticated"
	// errorCodeNotFound 表示资源不存在或无权访问（两者对外合并）。
	errorCodeNotFound = "not_found"
	// errorCodeValidation 表示请求违反冻结的 API/领域校验规则。
	errorCodeValidation = "validation_error"
	// errorCodeOverlayInvalid 表示 overlay 文档非法或严格模式目标缺失。
	errorCodeOverlayInvalid = "overlay_invalid"
	// errorCodeInputSpecMismatch 表示视图输入不满足视图契约。
	errorCodeInputSpecMismatch = "input_spec_mismatch"
	// errorCodeBranchNotIndexed 表示所选 ref 尚未建立索引。
	errorCodeBranchNotIndexed = "branch_not_indexed"
	// errorCodeProducerUnavailable 表示所选生产者配置不可用。
	errorCodeProducerUnavailable = "producer_profile_unavailable"
	// errorCodeDuplicate 表示契约定义的唯一标识已存在。
	errorCodeDuplicate = "duplicate"
	// errorCodeVersionNotPublishable 表示版本被待审核候选阻塞不可发布。
	errorCodeVersionNotPublishable = "version_not_publishable"
	// errorCodeBaseLayerExists 表示仓库 base 会替换已存在的 AI base。
	errorCodeBaseLayerExists = "base_layer_exists"
	// errorCodeQuotaExceeded 表示租户资源配额将超出限制。
	errorCodeQuotaExceeded = "quota_exceeded"
	// errorCodeCredentialInUse 表示凭据仍被仓库引用。
	errorCodeCredentialInUse = "credential_in_use"
	// errorCodeInvalidState 表示资源生命周期禁止该操作。
	errorCodeInvalidState = "invalid_state"
	// errorCodeIdempotencyConflict 表示幂等键被不同请求摘要复用。
	errorCodeIdempotencyConflict = "idempotency_conflict"
	// errorCodeJobNotCancellable 表示任务已进入终态不可取消。
	errorCodeJobNotCancellable = "job_not_cancellable"
	// errorCodePreconditionFailed 表示 If-Match 令牌缺失或与行版本不匹配。
	errorCodePreconditionFailed = "precondition_failed"
	// errorCodeInternal 是所有未分类内部失败的兜底错误码。
	errorCodeInternal = "internal_error"
)

// errorMsgID 返回错误码对应的 i18n 消息键（统一前缀 error.）。
func errorMsgID(code string) string {
	return "error." + code
}
