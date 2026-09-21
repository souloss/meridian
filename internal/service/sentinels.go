package service

import "errors"

// 本文件集中放置包级哨兵错误。每个哨兵下方注释说明其对外错误码映射
// （见 internal/handler/server.go 的 responseErrorHandler 与 error_codes.go 的 ErrorCode 冻结枚举）。
// 复用这些哨兵，避免在业务代码里裸写 errors.New("...") 或携带魔法业务 token 的 fmt.Errorf。

var (
	// ErrParseEntityTag 表示 If-Match ETag 的格式不符合 `${kind}:${id}:${revision}`。
	// 对外映射：ErrorCodePreconditionFailed（HTTP 412）。
	ErrParseEntityTag = errors.New("invalid entity tag")
	// ErrParseEntityTagRevision 表示 ETag 中的版本号无法解析为非负整数。
	// 对外映射：ErrorCodePreconditionFailed（HTTP 412）。
	ErrParseEntityTagRevision = errors.New("invalid entity tag revision")
	// ErrInvalidHost 表示主机名整体非法（超长或包含禁止字符）。
	// 对外映射：ErrorCodeValidation（HTTP 422，经调用方包装）。
	ErrInvalidHost = errors.New("invalid host")
	// ErrInvalidDNSHost 表示按点分标签校验失败的主机名。
	// 对外映射：ErrorCodeValidation（HTTP 422，经调用方包装）。
	ErrInvalidDNSHost = errors.New("invalid DNS host")
	// ErrMalformedShareToken 表示分享令牌不是 `payload.signature` 两段结构。
	// 对外映射：ErrorCodeNotFound（HTTP 404，经 GetSharedView 归并为 not_found）。
	ErrMalformedShareToken = errors.New("malformed share token")
	// ErrShareSigningKeyMissing 表示进程未配置分享签名密钥。
	// 对外映射：ErrorCodeNotFound（HTTP 404，经 GetSharedView 归并为 not_found）。
	ErrShareSigningKeyMissing = errors.New("share signing key not configured")
	// ErrShareTokenSignatureInvalid 表示分享令牌 HMAC 签名校验失败。
	// 对外映射：ErrorCodeNotFound（HTTP 404，经 GetSharedView 归并为 not_found）。
	ErrShareTokenSignatureInvalid = errors.New("share token signature invalid")
	// ErrShareTokenPayloadInvalid 表示分享令牌载荷无法解析或缺少必要字段。
	// 对外映射：ErrorCodeNotFound（HTTP 404，经 GetSharedView 归并为 not_found）。
	ErrShareTokenPayloadInvalid = errors.New("share token payload invalid")
	// ErrShareTokenExpired 表示分享令牌已过有效期。
	// 对外映射：ErrorCodeNotFound（HTTP 404，经 GetSharedView 归并为 not_found）。
	ErrShareTokenExpired = errors.New("share token expired")
	// ErrInvalidGitRemote 表示 Git 远端整体非法（空、超长或含空白/控制字符）。
	// 对外映射：ErrorCodeValidation（HTTP 422，经连接探测结果承载）。
	ErrInvalidGitRemote = errors.New("invalid Git remote")
	// ErrInvalidGitRemoteURI 表示 URL 形式的 Git 远端缺少必要字段。
	// 对外映射：ErrorCodeValidation（HTTP 422，经连接探测结果承载）。
	ErrInvalidGitRemoteURI = errors.New("invalid Git remote URI")
	// ErrUnsupportedGitRemoteScheme 表示 Git 远端的协议仅支持 https/ssh。
	// 对外映射：ErrorCodeValidation（HTTP 422，经连接探测结果承载）。
	ErrUnsupportedGitRemoteScheme = errors.New("unsupported Git remote scheme")
	// ErrInvalidGitRemotePort 表示 Git 远端的端口段非法。
	// 对外映射：ErrorCodeValidation（HTTP 422，经连接探测结果承载）。
	ErrInvalidGitRemotePort = errors.New("invalid Git remote port")
	// ErrInvalidSCPGitRemote 表示 SCP 风格（user@host:path）的 Git 远端非法。
	// 对外映射：ErrorCodeValidation（HTTP 422，经连接探测结果承载）。
	ErrInvalidSCPGitRemote = errors.New("invalid SCP-like Git remote")
	// ErrInvalidRepositoryURL 表示仓库 URL 整体非法（空、超长或含禁止字符）。
	// 对外映射：ErrorCodeValidation（HTTP 422，经调用方包装为 ErrValidation）。
	ErrInvalidRepositoryURL = errors.New("invalid repository URL")
	// ErrInvalidSCPURL 表示 SCP 风格（user@host:path）的仓库 URL 非法。
	// 对外映射：ErrorCodeValidation（HTTP 422，经调用方包装为 ErrValidation）。
	ErrInvalidSCPURL = errors.New("invalid SCP URL")
	// ErrUntrustedHostKeyCandidate 表示 SSH 探测捕获到未受信任的候选主机密钥（内部信号，不对外）。
	ErrUntrustedHostKeyCandidate = errors.New("untrusted host key candidate captured")
	// ErrInvalidShareTokenPayload 与 ErrShareTokenPayloadInvalid 同义，避免多处拼写漂移。
	// 对外映射：ErrorCodeNotFound（HTTP 404）。
	ErrInvalidShareTokenPayload = ErrShareTokenPayloadInvalid
	// ErrProducerTimedOut 表示 AI 生产者执行超时（内部错误，错误码由 errProducerTimedOutCode 承载）。
	// 对外映射：ErrorCodeInternal。
	ErrProducerTimedOut = errors.New("producer timed out")
	// ErrInvalidProducerOutput 表示 AI 生产者输出结构非法（内部错误）。
	// 对外映射：ErrorCodeInternal。
	ErrInvalidProducerOutput = errors.New("invalid producer output")
	// ErrManifestNoFiles 表示生产者完成清单不含任何文件条目（内部错误）。
	// 对外映射：ErrorCodeInternal。
	ErrManifestNoFiles = errors.New("manifest has no files")
	// ErrManifestFileEntryInvalid 表示完成清单的文件条目不是对象（内部错误）。
	// 对外映射：ErrorCodeInternal。
	ErrManifestFileEntryInvalid = errors.New("manifest file entry invalid")
	// ErrManifestFilePathMissing 表示完成清单的文件条目缺少路径（内部错误）。
	// 对外映射：ErrorCodeInternal。
	ErrManifestFilePathMissing = errors.New("manifest file path missing")
)

var (
	// ErrNotificationDeliveryUnsupported 表示所选通道类别尚无外发适配器。
	// 对外映射：ErrorCodeInternal（HTTP 500）。
	ErrNotificationDeliveryUnsupported = errors.New("notification channel kind has no delivery adapter")
	// ErrNotificationDeliveryFailed 表示一次外发投递失败（可重试）。
	ErrNotificationDeliveryFailed = errors.New("notification delivery failed")
)
