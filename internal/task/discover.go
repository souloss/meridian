package task

import (
	"context"
	"encoding/json/v2"
	"errors"
	"time"
	"uuid"

	"github.com/riverqueue/river"
)

// 阶段日志消息常量（值 = job_stage_log.message 同口径，供阶段推进与终态落库使用）。
const (
	// stageMessageRepositoryDiscoveryStarted 表示仓库发现阶段已开始。
	stageMessageRepositoryDiscoveryStarted = "repository discovery started"
	// stageMessageRepositoryDiscoveryCompleted 表示仓库发现成功完成。
	stageMessageRepositoryDiscoveryCompleted = "repository discovery completed"
	// stageMessageRepositoryDiscoveryFailed 表示仓库发现失败。
	stageMessageRepositoryDiscoveryFailed = "repository discovery failed"
)

// DiscoverArgs 是仓库发现任务携带的持久化且不含机密信息的参数。
// 凭据被刻意排除：运行器在执行时解析当前仓库配置，永不接收机密数据。
type DiscoverArgs struct {
	// TenantID 标识每次执行查询的租户边界。
	TenantID uuid.UUID `json:"tenantId"`
	// JobID 标识应用自有的持久化任务行。
	JobID uuid.UUID `json:"jobId"`
	// RepositoryID 标识发起发现的仓库。
	RepositoryID uuid.UUID `json:"repositoryId"`
	// RefType 标识 Git 引用类别（分支或标签）。
	RefType string `json:"refType"`
	// RefName 标识待发现的规范化 Git 引用。
	RefName string `json:"refName"`
}

// Kind 返回持久化在 River schema 中的稳定 River 任务类型名。
func (DiscoverArgs) Kind() string { return "meridian_repo_discover" }

// DiscoverResult 是一次仓库发现运行的脱敏结果。
type DiscoverResult struct {
	// ResolvedCommit 是发现时解析出的提交 SHA。
	ResolvedCommit string
	// CandidateCount 是持久化的候选根目录数量。
	CandidateCount int
}

// DiscoverRunner 在持久化任务被领取后执行仓库发现。
// 具体实现遍历检出目录并 upsert 候选。
type DiscoverRunner interface {
	RunDiscovery(context.Context, DiscoverArgs) (DiscoverResult, error)
}

// DiscoverWorker 围绕一次仓库发现 River 尝试推进持久化的 Meridian 状态，
// 并将解析出的提交写入任务结果。
type DiscoverWorker struct {
	river.WorkerDefaults[DiscoverArgs]
	store  ExecutionStore
	runner DiscoverRunner
	now    func() time.Time
}

// NewDiscoverWorker 使用显式的持久化与运行器依赖构造发现工作器。
func NewDiscoverWorker(store ExecutionStore, runner DiscoverRunner) *DiscoverWorker {
	return &DiscoverWorker{store: store, runner: runner, now: time.Now}
}

// Work 领取持久化任务，记录 resolve 与 discover 阶段迁移，执行发现，
// 并以脱敏的结果收尾。
func (worker *DiscoverWorker) Work(ctx context.Context, job *river.Job[DiscoverArgs]) error {
	if worker.store == nil {
		return errors.New("discovery worker has no execution store")
	}
	if worker.runner == nil {
		return errors.New("discovery worker has no runner")
	}
	startedAt := worker.now().UTC()
	claim, err := worker.store.StartJob(ctx, StartInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageResolve,
		ExpectedAttempt: job.Attempt, StartedAt: startedAt,
	})
	if err != nil {
		return err
	}
	if !claim.Claimed {
		return nil
	}
	if err := worker.store.SetJobStage(ctx, StageInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageDiscover,
		ExpectedAttempt: job.Attempt, Level: logLevelInfo, Message: stageMessageRepositoryDiscoveryStarted, OccurredAt: worker.now().UTC(),
	}); err != nil {
		return err
	}
	result, err := worker.runner.RunDiscovery(ctx, job.Args)
	if err != nil {
		return worker.finishFailure(ctx, job.Args, job.Attempt, err)
	}
	payload, err := json.Marshal(struct {
		RepositoryID   string `json:"repositoryId"`
		RefType        string `json:"refType"`
		RefName        string `json:"refName"`
		ResolvedCommit string `json:"resolvedCommit"`
		CandidateCount int    `json:"candidateCount"`
	}{
		RepositoryID: job.Args.RepositoryID.String(), RefType: job.Args.RefType, RefName: job.Args.RefName,
		ResolvedCommit: result.ResolvedCommit, CandidateCount: result.CandidateCount,
	})
	if err != nil {
		return err
	}
	return worker.store.FinishJob(ctx, FinishInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Status: jobStatusSucceeded, Result: payload,
		ExpectedAttempt: job.Attempt, Stage: StageDiscover, Level: logLevelInfo,
		Message: stageMessageRepositoryDiscoveryCompleted, Terminal: true, FinishedAt: worker.now().UTC(),
	})
}

// finishFailure 将一次发现失败持久化为终态失败，并写入稳定的错误码。
func (worker *DiscoverWorker) finishFailure(ctx context.Context, args DiscoverArgs, expectedAttempt int, cause error) error {
	errorPayload, err := json.Marshal(struct {
		Code string `json:"code"`
	}{Code: errorCodeInternal})
	if err != nil {
		return err
	}
	finishErr := worker.store.FinishJob(ctx, FinishInput{
		TenantID: args.TenantID, JobID: args.JobID, RepositoryID: args.RepositoryID,
		Status: jobStatusFailed, Error: errorPayload, ErrorCode: errorCodeInternal,
		ExpectedAttempt: expectedAttempt,
		Stage:           StageDiscover, Level: logLevelError, Message: stageMessageRepositoryDiscoveryFailed, Terminal: true, FinishedAt: worker.now().UTC(),
	})
	if finishErr != nil {
		return errors.Join(cause, finishErr)
	}
	return nil
}
