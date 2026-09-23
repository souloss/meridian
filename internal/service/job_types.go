package service

import (
	"context"
	"errors"
	"time"
	"uuid"
)

var (
	// ErrJobNotCancellable 表示任务已进入终态，无法接受取消。
	// 对外映射：ErrorCodeJobNotCancellable（HTTP 409）。
	ErrJobNotCancellable = errors.New("job is not cancellable")
	// ErrJobNotRetryable 表示任务状态或活跃代次禁止手动重试。
	// 对外映射：ErrorCodeInvalidState（HTTP 409）。
	ErrJobNotRetryable = errors.New("job is not retryable")
)

// JobFilter 使用契约定义的有限字段筛选租户任务。
type JobFilter struct {
	// Types 将结果限定为已注册的任务行为标识。
	Types []string
	// Statuses 将结果限定为持久化生命周期状态。
	Statuses []string
	// ScopeType 将结果限定为一个资源类别。
	ScopeType string
	// ScopeID 将结果限定为一个资源 UUID 字符串。
	ScopeID string
}

// JobError 是一个不含秘密的异步失败投影。
type JobError struct {
	// Code 是稳定的 API 错误类别。
	Code string
	// Message 对认证过的租户成员安全。
	Message string
	// RequestID 在失败并非由 HTTP 请求创建时，将失败关联到持久化任务。
	RequestID string
	// Details 包含可选的非秘密结构化诊断信息。
	Details map[string]any
}

// JobStageAttempt 总结一次 River 执行尝试中的一个阶段。
type JobStageAttempt struct {
	// Stage 标识管线阶段。
	Stage string
	// Attempt 是基于 1 的 River 执行尝试次数。
	Attempt int
	// Status 是由持久化日志与当前任务状态推导出的阶段结果。
	Status string
	// StartedAt 是该阶段首个持久化事件时间。
	StartedAt *time.Time
	// FinishedAt 是已知时的终态或下一阶段边界时间。
	FinishedAt *time.Time
	// Error 仅在该阶段失败时包含不含秘密的失败信息。
	Error *JobError
}

// JobRecord 是租户可见的持久化任务投影。
type JobRecord struct {
	// ID 标识 Meridian 任务。
	ID uuid.UUID
	// TenantSlug 在 URL 与响应中标识所属租户。
	TenantSlug string
	// RetryOfJobID 标识手动重试的同一租户源任务。
	RetryOfJobID *uuid.UUID
	// Type 标识已注册的任务行为。
	Type string
	// Trigger 标识请求来源。
	Trigger string
	// Status 是当前持久化生命周期状态。
	Status string
	// Stage 是活跃或最终管线阶段（存在时）。
	Stage *string
	// ScopeType 标识用于授权的资源类别。
	ScopeType string
	// ScopeID 标识作用域资源（存在时）。
	ScopeID *string
	// RefType 在任务以引用为作用域时标识 branch 或 tag。
	RefType *string
	// Ref 是规范化 Git 引用名（存在时）。
	Ref *string
	// Result 包含操作特定的非秘密标识与计数。
	Result map[string]any
	// Progress 是由状态与阶段推导出的确定性百分比。
	Progress int
	// Dirty 表示执行期间有更新的等价工作到达。
	Dirty bool
	// Attempt 是已启动的 River 尝试次数。
	Attempt int
	// MaxAttempts 是 River 自动尝试的最大次数。
	MaxAttempts int
	// NextAttemptAt 是下一次自动重试时间（存在时）。
	NextAttemptAt *time.Time
	// Attempts 包含有序持久化的阶段尝试摘要。
	Attempts []JobStageAttempt
	// Error 是当前不含秘密的异步失败信息（存在时）。
	Error *JobError
	// CreatedAt 是任务被接受的时间。
	CreatedAt time.Time
	// StartedAt 是首次开始执行的时间。
	StartedAt *time.Time
	// FinishedAt 是任务到达终态的时间。
	FinishedAt *time.Time
	// UpdatedAt 跟踪实质性状态变化，用于 SSE 快照。
	UpdatedAt time.Time
	// Capabilities 列出当前授权的操作。
	Capabilities []string
}

// JobLogRecord 是一个持久化、可重放且不含秘密的阶段事件。
type JobLogRecord struct {
	// Sequence 是严格递增的按任务重放游标。
	Sequence int64
	// Stage 是事件发生时活跃的管线阶段（存在时）。
	Stage *string
	// Message 是持久化的脱敏诊断信息。
	Message string
	// OccurredAt 是事件持久化的时间。
	OccurredAt time.Time
}

// JobStateEvent 是发给任务流的一次实质性状态快照。
type JobStateEvent struct {
	// Cursor 是快照覆盖的最新持久化日志序号。
	Cursor int64
	// Status 是当前持久化任务生命周期状态。
	Status string
	// Progress 是确定性的完成百分比。
	Progress int
	// At 是最新持久化状态更新时间。
	At time.Time
}

// JobEventSink 接收有序的流状态、日志与心跳事件。
type JobEventSink interface {
	// State 承载 JobEventSink 的生成 State 值。
	State(JobStateEvent) error
	// Log 承载 JobEventSink 的生成 Log 值。
	Log(JobLogRecord) error
	// Heartbeat 承载 JobEventSink 的生成 Heartbeat 值。
	Heartbeat() error
}

// JobAccepted 标识一个刚被接受或精确重放的异步任务。
type JobAccepted struct {
	// JobID 标识持久化 Meridian 任务。
	JobID uuid.UUID `json:"jobId"`
	// Deduplicated 报告语义合并是否选中了现有工作。
	Deduplicated bool `json:"deduplicated"`
}

// RetryJobRequest 将认证后的幂等身份带入一次原子重试事务。
type RetryJobRequest struct {
	// TenantID 标识所属租户。
	TenantID uuid.UUID
	// SourceJobID 标识失败或被取消的源任务。
	SourceJobID uuid.UUID
	// PrincipalType 标识会话或 PAT 认证。
	PrincipalType string
	// PrincipalID 标识确切的认证会话或 PAT。
	PrincipalID uuid.UUID
	// IdempotencyKey 在 24 小时内标识该语义重试请求。
	IdempotencyKey uuid.UUID
	// RequestHash 是规范 32 字节语义请求摘要。
	RequestHash []byte
	// RequestedAt 是重试被接受的 UTC 时间。
	RequestedAt time.Time
}

// PlatformJobRecord 是暴露给平台管理员的脱敏跨租户任务投影。
// 它有意排除载荷、错误、尝试、引用与 River 内部信息。
type PlatformJobRecord struct {
	// ID 标识持久化 Meridian 任务。
	ID uuid.UUID
	// TenantSlug 标识拥有任务所在租户。
	TenantSlug string
	// Type 标识已注册的任务行为。
	Type string
	// Trigger 标识请求来源。
	Trigger string
	// Status 是当前持久化任务状态。
	Status string
	// Stage 是活跃或最终管线阶段（存在时）。
	Stage *string
	// ScopeType 标识用于授权的资源类别。
	ScopeType string
	// ScopeID 仅对租户与仓库作用域暴露。
	ScopeID *string
	// CreatedAt 是任务被接受的 UTC 时刻。
	CreatedAt time.Time
	// StartedAt 是开始执行的 UTC 时刻（存在时）。
	StartedAt *time.Time
	// FinishedAt 是到达终态的 UTC 时刻（存在时）。
	FinishedAt *time.Time
}

// PlatformJobFilter 在不允许任意 SQL 表达式的前提下筛选脱敏任务。
type PlatformJobFilter struct {
	// Types 将结果限定为已知任务行为标识。
	Types []string
	// Statuses 将结果限定为已知持久化任务状态。
	Statuses []string
	// ScopeType 将结果限定为一个已知作用域类别。
	ScopeType string
	// ScopeID 将结果限定为一个作用域标识字符串。
	ScopeID string
	// TenantSlug 将结果限定为一个活跃租户 slug。
	TenantSlug string
}

// JobStore 是租户控制与脱敏平台查询的持久化边界。
type JobStore interface {
	// ListPlatformJobs 承载 JobStore 的生成 ListPlatformJobs 值。
	ListPlatformJobs(context.Context, PlatformJobFilter, int32, int32) ([]PlatformJobRecord, int64, error)
	// GetPlatformJob 承载 JobStore 的生成 GetPlatformJob 值。
	GetPlatformJob(context.Context, uuid.UUID) (PlatformJobRecord, error)
	// ListTenantJobs 承载 JobStore 的生成 ListTenantJobs 值。
	ListTenantJobs(context.Context, uuid.UUID, JobFilter, int32, int32) ([]JobRecord, int64, error)
	// GetTenantJob 承载 JobStore 的生成 GetTenantJob 值。
	GetTenantJob(context.Context, uuid.UUID, uuid.UUID) (JobRecord, error)
	// GetTenantJobStreamState 承载 JobStore 的生成 GetTenantJobStreamState 值。
	GetTenantJobStreamState(context.Context, uuid.UUID, uuid.UUID) (JobRecord, int64, error)
	// ListTenantJobLogsAfter 承载 JobStore 的生成 ListTenantJobLogsAfter 值。
	ListTenantJobLogsAfter(context.Context, uuid.UUID, uuid.UUID, int64, int32) ([]JobLogRecord, error)
	// CancelTenantJob 承载 JobStore 的生成 CancelTenantJob 值。
	CancelTenantJob(context.Context, uuid.UUID, uuid.UUID, time.Time) (JobAccepted, error)
	// RetryTenantJob 承载 JobStore 的生成 RetryTenantJob 值。
	RetryTenantJob(context.Context, RetryJobRequest) (JobAccepted, error)
}
