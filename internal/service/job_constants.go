package service

// 本文件集中放置任务（job）域的领域常量。
// 值来源：contracts/domain.yaml 的 jobStatus / jobTrigger / pipelineStage 段，
// 以及 migrations 中 jobs.status / jobs.stage 的 CHECK 约束。

// 任务状态常量（jobStatus 枚举）。
const (
	// JobStatusPending 表示任务已入队尚未开始执行。
	JobStatusPending = "pending"
	// JobStatusRunning 表示任务正在执行。
	JobStatusRunning = "running"
	// JobStatusSucceeded 表示任务成功完成。
	JobStatusSucceeded = "succeeded"
	// JobStatusSucceededWithWarnings 表示任务成功但带告警。
	JobStatusSucceededWithWarnings = "succeeded_with_warnings"
	// JobStatusFailed 表示任务失败。
	JobStatusFailed = "failed"
	// JobStatusOutcomeUnknown 表示任务结果无法确定。
	JobStatusOutcomeUnknown = "outcome_unknown"
	// JobStatusCancelled 表示任务被取消。
	JobStatusCancelled = "cancelled"
)

// 任务阶段常量（pipelineStage 枚举）。
const (
	// StageResolve 是解析仓库引用的阶段。
	StageResolve = "resolve"
	// StageDiscover 是发现服务/候选的阶段。
	StageDiscover = "discover"
	// StageExtract 是提取源内容的阶段。
	StageExtract = "extract"
	// StageMerge 是合并层的阶段。
	StageMerge = "merge"
	// StageNormalize 是规范化文档的阶段。
	StageNormalize = "normalize"
	// StageIndex 是建立条目索引的阶段。
	StageIndex = "index"
)

// validJobStatuses 是任务状态的合法取值集合。
var validJobStatuses = [...]string{
	JobStatusPending, JobStatusRunning, JobStatusSucceeded, JobStatusSucceededWithWarnings,
	JobStatusFailed, JobStatusOutcomeUnknown, JobStatusCancelled,
}
