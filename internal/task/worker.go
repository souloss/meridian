package task

import (
	"context"
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/riverqueue/river"
)

// 阶段日志消息常量（值 = job_stage_log.message 同口径，供阶段推进与终态落库使用）。
const (
	// stageMessagePipelineCompleted 表示单个流水线阶段已完成。
	stageMessagePipelineCompleted = "pipeline stage completed"
	// stageMessageRepositorySyncCompleted 表示仓库同步成功完成。
	stageMessageRepositorySyncCompleted = "repository synchronization completed"
	// stageMessageRepositorySyncFailed 表示仓库同步失败。
	stageMessageRepositorySyncFailed = "repository synchronization failed"
	// stageMessageRepositorySyncUnavailable 表示 Git 生产者尚未实现导致同步不可用。
	stageMessageRepositorySyncUnavailable = "repository synchronization producer is unavailable"
)

// ErrRepositorySyncUnavailable 标记 Git 生产者实现之前（M0 边界）同步不可用。
var ErrRepositorySyncUnavailable = errors.New(stageMessageRepositorySyncUnavailable)

// UnsupportedSyncRunner 使 M0 边界显式化，而不是在没有生产者、检出或完成清单时
// 仍报告一次成功的同步。
type UnsupportedSyncRunner struct{}

// Run 在 M1 仓库生产者存在之前返回一个稳定且不含机密信息的错误。
func (UnsupportedSyncRunner) Run(context.Context, CredentialSyncArgs) (SyncResult, error) {
	return SyncResult{}, ErrRepositorySyncUnavailable
}

// CredentialSyncWorker 围绕一次 River 尝试推进持久化的 Meridian 状态。
type CredentialSyncWorker struct {
	river.WorkerDefaults[CredentialSyncArgs]
	store  ExecutionStore
	runner SyncRunner
	now    func() time.Time
}

// NewCredentialSyncWorker 使用显式的持久化与生产者依赖构造 M0 工作器。
func NewCredentialSyncWorker(store ExecutionStore, runner SyncRunner) *CredentialSyncWorker {
	if runner == nil {
		runner = UnsupportedSyncRunner{}
	}
	return &CredentialSyncWorker{store: store, runner: runner, now: time.Now}
}

// Work 领取持久化任务，记录阶段迁移，并以脱敏的结果或错误收尾。
// 已处于终态的领域行被视为幂等的 River 成功，重放时不会再次改动它。
func (worker *CredentialSyncWorker) Work(ctx context.Context, job *river.Job[CredentialSyncArgs]) error {
	if worker.store == nil {
		return errors.New("credential sync worker has no execution store")
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

	syncResult, err := worker.runner.Run(ctx, job.Args)
	if err != nil {
		return worker.finishFailure(ctx, job.Args, job.Attempt, err)
	}
	for _, stage := range [...]Stage{StageDiscover, StageExtract, StageMerge, StageNormalize, StageIndex} {
		if err := worker.store.SetJobStage(ctx, StageInput{
			TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: stage,
			ExpectedAttempt: job.Attempt,
			Level:           logLevelInfo, Message: stageMessagePipelineCompleted, OccurredAt: worker.now().UTC(),
		}); err != nil {
			return err
		}
	}
	result, err := json.Marshal(struct {
		RepositoryID   string `json:"repositoryId"`
		RefName        string `json:"refName"`
		ResolvedCommit string `json:"resolvedCommit"`
	}{RepositoryID: job.Args.RepositoryID.String(), RefName: job.Args.RefName, ResolvedCommit: syncResult.ResolvedCommit})
	if err != nil {
		return err
	}
	return worker.store.FinishJob(ctx, FinishInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Status: jobStatusSucceeded, Result: result,
		ExpectedAttempt: job.Attempt,
		Stage:           StageIndex, Level: logLevelInfo, Message: stageMessageRepositorySyncCompleted,
		Terminal: true, FinishedAt: worker.now().UTC(),
	})
}

// finishFailure 将一次同步失败持久化为终态失败，并写入稳定的错误码。
func (worker *CredentialSyncWorker) finishFailure(ctx context.Context, args CredentialSyncArgs, expectedAttempt int, cause error) error {
	message := stageMessageRepositorySyncFailed
	if errors.Is(cause, ErrRepositorySyncUnavailable) {
		message = stageMessageRepositorySyncUnavailable
	}
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
		Stage:           StageResolve, Level: logLevelError, Message: message, Terminal: true, FinishedAt: worker.now().UTC(),
	})
	if finishErr != nil {
		return errors.Join(cause, finishErr)
	}
	return nil
}
