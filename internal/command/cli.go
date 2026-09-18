// Package command 实现 meridian 命令树。
package command

// CLI 退出码由 contracts/cli.yaml 的 exitCodes 冻结。
// 未经契约变更不得重新编号。
const (
	// exitCodeSuccess 表示命令成功且未超出任何 diff 失败阈值。
	exitCodeSuccess = 0
	// exitCodeUsage 表示本地用法、输入或契约校验错误。
	exitCodeUsage = 2
	// exitCodeThreshold 表示 diff 失败阈值被触发。
	exitCodeThreshold = 3
	// exitCodeUnauthenticated 表示 HTTP 401。
	exitCodeUnauthenticated = 4
	// exitCodeNotFound 表示 HTTP 404 或无权访问（被隐藏）的资源。
	exitCodeNotFound = 5
	// exitCodeConflict 表示 HTTP 409 或 412。
	exitCodeConflict = 6
	// exitCodeServerFailure 表示网络 5xx 或终态任务失败。
	exitCodeServerFailure = 7
	// exitCodeWaitTimeout 表示客户端等待超时但服务端任务仍在运行。
	exitCodeWaitTimeout = 8
	// exitCodeInterrupted 表示客户端被中断但服务端任务仍在运行。
	exitCodeInterrupted = 130
)

// diff 失败阈值取值常量（值 = contracts/cli.yaml diff.threshold.fail-on 枚举）。
const (
	// failOnBreaking 表示仅当出现破坏性变更时失败。
	failOnBreaking = "breaking"
	// failOnRisky 表示出现破坏性或风险性变更时失败。
	failOnRisky = "risky"
)

// HTTP 状态码常量（值 = IETF RFC 9110 定义的协议状态码）。
const (
	// httpStatusOK 是成功响应状态码。
	httpStatusOK = 200
	// httpStatusMultipleChoices 是 2xx 成功区间的上界哨兵。
	httpStatusMultipleChoices = 300
	// httpStatusUnauthorized 表示未认证。
	httpStatusUnauthorized = 401
	// httpStatusNotFound 表示资源不存在。
	httpStatusNotFound = 404
	// httpStatusConflict 表示请求与当前状态冲突。
	httpStatusConflict = 409
	// httpStatusPreconditionFailed 表示 If-Match 前置条件失败。
	httpStatusPreconditionFailed = 412
	// httpStatusServerError 是 5xx 服务器错误区间的下界。
	httpStatusServerError = 500
)

// CliError 将一个退出码与其脱敏消息耦合在一起。
// 命令通过返回它来控制进程退出码。
type CliError struct {
	code    int
	message string
}

// Error 以脱敏消息满足 error 接口。
func (err *CliError) Error() string { return err.message }

// ExitCode 返回契约定义的进程退出码。
func (err *CliError) ExitCode() int { return err.code }

// usageError 构造一个用法类退出码的 CliError。
func usageError(message string) error { return &CliError{code: exitCodeUsage, message: message} }

// httpError 从 HTTP 状态码构造一个 CliError。
func httpError(status int, message string) error {
	return &CliError{code: httpExitCode(status), message: message}
}

// diffExitCode 依据 contracts/cli.yaml diff.threshold 将 diff 失败阈值
// 与摘要的破坏性/风险性计数映射为进程退出码。
func diffExitCode(failOn string, breaking, risky int) int {
	switch failOn {
	case failOnBreaking:
		if breaking > 0 {
			return exitCodeThreshold
		}
	case failOnRisky:
		if breaking > 0 || risky > 0 {
			return exitCodeThreshold
		}
	}
	return exitCodeSuccess
}

// httpExitCode 依据 contracts/cli.yaml exitCodes 将 HTTP 状态码映射为 CLI 退出码。
// 未知状态码默认归为服务器失败码。
func httpExitCode(status int) int {
	switch {
	case status == httpStatusUnauthorized:
		return exitCodeUnauthenticated
	case status == httpStatusNotFound:
		return exitCodeNotFound
	case status == httpStatusConflict || status == httpStatusPreconditionFailed:
		return exitCodeConflict
	case status >= httpStatusServerError:
		return exitCodeServerFailure
	case status >= httpStatusOK && status < httpStatusMultipleChoices:
		return exitCodeSuccess
	default:
		return exitCodeServerFailure
	}
}
