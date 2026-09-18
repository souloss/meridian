// Package task 包含 River 任务参数与执行编排。
package task

import (
	"context"
	"time"
	"uuid"
)

// Stage 标识一个持久化的仓库同步流水线阶段。
type Stage string

// 流水线阶段常量（值 = pipelineStage 枚举原样，与 service.Stage* 同口径）。
const (
	// StageResolve 解析仓库、提交、引用与来源绑定。
	StageResolve Stage = "resolve"
	// StageDiscover 发现候选资产而不改变生效配置。
	StageDiscover Stage = "discover"
	// StageExtract 将生产者输出抽取到隔离工作区。
	StageExtract Stage = "extract"
	// StageMerge 将生效层头合并为确定性输入。
	StageMerge Stage = "merge"
	// StageNormalize 规范化合并输入并推导来源。
	StageNormalize Stage = "normalize"
	// StageIndex 写入版本、条目索引与依赖的出箱事件。
	StageIndex Stage = "index"
)

// CredentialSyncArgs 是 River 任务携带的持久化且不含机密信息的参数。
// 凭据标识被刻意排除：工作器在执行时解析当前仓库配置，永不接收机密数据。
type CredentialSyncArgs struct {
	// TenantID 标识每次执行查询的租户边界。
	TenantID uuid.UUID `json:"tenantId"`
	// JobID 标识应用自有的持久化任务行。
	JobID uuid.UUID `json:"jobId"`
	// RepositoryID 标识发起同步的仓库。
	RepositoryID uuid.UUID `json:"repositoryId"`
	// RefName 标识待同步的规范化 Git 分支或标签。
	RefName string `json:"refName"`
}

// Kind 返回持久化在 River schema 中的稳定 River 任务类型名。
func (CredentialSyncArgs) Kind() string { return "meridian_repo_sync" }

// StartInput 描述一次任务尝试的初始持久化状态迁移。
type StartInput struct {
	// TenantID 标识拥有该任务的租户。
	TenantID uuid.UUID
	// JobID 标识应用自有的任务行。
	JobID uuid.UUID
	// Stage 是第一个对外暴露的流水线阶段。
	Stage Stage
	// ExpectedAttempt 是领取该领域行的一次性 River 尝试序号。
	ExpectedAttempt int
	// StartedAt 是首个尝试时间戳使用的 UTC 时间。
	StartedAt time.Time
}

// StageInput 描述一次活动阶段迁移及其脱敏消息。
type StageInput struct {
	// TenantID 标识拥有该任务的租户。
	TenantID uuid.UUID
	// JobID 标识应用自有的任务行。
	JobID uuid.UUID
	// Stage 是正在进入的流水线阶段。
	Stage Stage
	// ExpectedAttempt 是拥有该迁移的一次性 River 尝试序号。
	ExpectedAttempt int
	// Level 是结构化日志级别。
	Level string
	// Message 是不含机密信息的诊断消息。
	Message string
	// OccurredAt 是事件被记录的 UTC 时间。
	OccurredAt time.Time
}

// FinishInput 描述终态或可重试的持久化状态迁移。
type FinishInput struct {
	// TenantID 标识拥有该任务的租户。
	TenantID uuid.UUID
	// JobID 标识应用自有的任务行。
	JobID uuid.UUID
	// RepositoryID 标识包含在 collect.failed 事件中的仓库。
	RepositoryID uuid.UUID
	// Status 是持久化 Meridian 任务状态之一。
	Status string
	// ExpectedAttempt 是拥有该迁移的一次性 River 尝试序号。
	ExpectedAttempt int
	// Result 是脱敏的 JSON 结果元数据，或为 nil。
	Result []byte
	// Error 是脱敏的结构化错误元数据，或为 nil。
	Error []byte
	// ErrorCode 是审计与事件使用的稳定脱敏失败分类。
	ErrorCode string
	// Terminal 指示是否应写入 finishedAt。
	Terminal bool
	// Stage 是与最终日志事件关联的阶段。
	Stage Stage
	// Level 是最终事件的结构化日志级别。
	Level string
	// Message 是脱敏的终态诊断消息。
	Message string
	// FinishedAt 是迁移被记录的 UTC 时间。
	FinishedAt time.Time
}

// ExecutionStore 持久化 River 任务中应用自有的一侧。
type ExecutionStore interface {
	StartJob(context.Context, StartInput) (ClaimResult, error)
	SetJobStage(context.Context, StageInput) error
	FinishJob(context.Context, FinishInput) error
}

// ClaimResult 描述当前 River 尝试是否拥有该领域任务。
type ClaimResult struct {
	// Claimed 为 true 表示该尝试可以执行并收尾该任务。
	Claimed bool
	// Attempt 是领取后的持久化尝试序号。
	Attempt int
}

// SyncRunner 在持久化任务被领取后执行仓库同步。
// M0 提供显式的不支持运行器；M1 以 Git 生产者替换之。
type SyncRunner interface {
	Run(context.Context, CredentialSyncArgs) (SyncResult, error)
}

// SyncResult 携带一次已完成同步记录的解析提交。
type SyncResult struct {
	// ResolvedCommit 是流水线物化出的 Git 提交。
	ResolvedCommit string
}
