package task

// 本文件集中放置任务域的错误码与状态常量，避免跨包引入 service 造成循环依赖。
// 值来源：contracts/openapi.yaml 的 ErrorCode 冻结枚举，以及 domain.yaml 的 jobStatus 段。
// 任务包无法导入 service（service 反向依赖 task），因此在此处保留一份同值常量。

// 任务错误码常量（值 = ErrorCode 枚举原样，与 service.ErrorCodeInternal 同口径）。
const (
	// errorCodeInternal 是所有未分类内部失败的兜底错误码。
	errorCodeInternal = "internal_error"
)

// 任务状态常量（值 = jobStatus 枚举原样，与 service.JobStatus* 同口径）。
const (
	// jobStatusSucceeded 表示任务成功完成。
	jobStatusSucceeded = "succeeded"
	// jobStatusFailed 表示任务失败。
	jobStatusFailed = "failed"
)

// 阶段日志级别常量（值 = job_stage_log.level 同口径，与 repository 层写入一致）。
const (
	// logLevelInfo 表示信息级阶段事件。
	logLevelInfo = "info"
	// logLevelError 表示错误级失败事件。
	logLevelError = "error"
)
