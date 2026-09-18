// Package task 包含 River 任务参数与执行编排。
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
	// stageMessageMergeCompleted 表示合并阶段已完成。
	stageMessageMergeCompleted = "merge completed"
	// stageMessageAssetMergeCompleted 表示资产合并成功完成。
	stageMessageAssetMergeCompleted = "asset merge completed"
	// stageMessageAssetMergeFailed 表示资产合并失败。
	stageMessageAssetMergeFailed = "asset merge failed"
)

// MergeArgs 是 asset.merge 任务携带的持久化且不含机密信息的参数。
type MergeArgs struct {
	// TenantID 标识每次执行查询的租户边界。
	TenantID uuid.UUID `json:"tenantId"`
	// JobID 标识应用自有的持久化任务行。
	JobID uuid.UUID `json:"jobId"`
	// TrackID 标识需要重新合并层的资产引用轨道。
	TrackID uuid.UUID `json:"trackId"`
}

// Kind 返回持久化在 River schema 中的稳定 River 任务类型名。
func (MergeArgs) Kind() string { return "meridian_asset_merge" }

// MergeResult 携带一次已完成合并的物化版本。
type MergeResult struct {
	// VersionID 标识新建或复用的版本（当产生版本时）。
	VersionID uuid.UUID `json:"versionId,omitempty"`
	// Noop 表示合并是否复用了完全相同的既有版本。
	Noop bool `json:"noop"`
}

// MergeRunner 将一条轨道的生效层重新合并为一个版本。
type MergeRunner interface {
	Run(context.Context, MergeArgs) (MergeResult, error)
}

// MergeWorker 围绕一次 asset.merge 尝试推进持久化的 Meridian 状态。
type MergeWorker struct {
	river.WorkerDefaults[MergeArgs]
	store  ExecutionStore
	runner MergeRunner
	now    func() time.Time
}

// NewMergeWorker 构造 M2 的 asset.merge 工作器。
func NewMergeWorker(store ExecutionStore, runner MergeRunner) *MergeWorker {
	return &MergeWorker{store: store, runner: runner, now: time.Now}
}

// Work 领取持久化任务，执行合并，并以脱敏的结果或错误收尾。
func (worker *MergeWorker) Work(ctx context.Context, job *river.Job[MergeArgs]) error {
	if worker.store == nil {
		return errors.New("merge worker has no execution store")
	}
	if worker.runner == nil {
		return errors.New("merge worker has no runner")
	}
	startedAt := worker.now().UTC()
	claim, err := worker.store.StartJob(ctx, StartInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageMerge,
		ExpectedAttempt: job.Attempt, StartedAt: startedAt,
	})
	if err != nil {
		return err
	}
	if !claim.Claimed {
		return nil
	}
	result, err := worker.runner.Run(ctx, job.Args)
	if err != nil {
		return worker.finishFailure(ctx, job.Args, job.Attempt, err)
	}
	if err := worker.store.SetJobStage(ctx, StageInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageIndex,
		ExpectedAttempt: job.Attempt,
		Level:           logLevelInfo, Message: stageMessageMergeCompleted, OccurredAt: worker.now().UTC(),
	}); err != nil {
		return err
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return worker.store.FinishJob(ctx, FinishInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Status: jobStatusSucceeded, Result: resultBytes,
		ExpectedAttempt: job.Attempt,
		Stage:           StageIndex, Level: logLevelInfo, Message: stageMessageAssetMergeCompleted,
		Terminal: true, FinishedAt: worker.now().UTC(),
	})
}

// finishFailure 将一次合并失败持久化为终态失败，并写入稳定的错误码。
func (worker *MergeWorker) finishFailure(ctx context.Context, args MergeArgs, expectedAttempt int, cause error) error {
	errorPayload, err := json.Marshal(struct {
		Code string `json:"code"`
	}{Code: errorCodeInternal})
	if err != nil {
		return err
	}
	finishErr := worker.store.FinishJob(ctx, FinishInput{
		TenantID: args.TenantID, JobID: args.JobID, Status: jobStatusFailed, Error: errorPayload, ErrorCode: errorCodeInternal,
		ExpectedAttempt: expectedAttempt,
		Stage:           StageMerge, Level: logLevelError, Message: stageMessageAssetMergeFailed, Terminal: true, FinishedAt: worker.now().UTC(),
	})
	if finishErr != nil {
		return errors.Join(cause, finishErr)
	}
	return nil
}
