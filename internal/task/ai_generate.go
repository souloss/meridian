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
	// stageMessageAssetAiGenerateCompleted 表示资产 AI 生成成功完成。
	stageMessageAssetAiGenerateCompleted = "asset AI generation completed"
	// stageMessageAssetAiGenerateFailed 表示资产 AI 生成失败。
	stageMessageAssetAiGenerateFailed = "asset AI generation failed"
)

// AiGenerateArgs 是 asset.ai_generate River 任务携带的持久化且不含机密信息的参数。
// 生产者配置与租户设置在执行时解析；队列中永不进入机密数据。
type AiGenerateArgs struct {
	// TenantID 标识每次执行查询的租户边界。
	TenantID uuid.UUID `json:"tenantId"`
	// JobID 标识应用自有的持久化任务行。
	JobID uuid.UUID `json:"jobId"`
	// AssetID 标识待生成的缺失资产。
	AssetID uuid.UUID `json:"assetId"`
}

// Kind 返回持久化在 River schema 中的稳定 River 任务类型名。
func (AiGenerateArgs) Kind() string { return "meridian_asset_ai_generate" }

// AiGenerateResult 是一次 AI 生成运行的脱敏结果。
type AiGenerateResult struct {
	// Stage 标识终态流水线阶段（成功时为 normalize，失败时为 extract/merge）。
	Stage string `json:"stage"`
	// ErrorCode 携带稳定的失败分类；成功时为空。
	ErrorCode string `json:"errorCode,omitempty"`
	// RevisionID 标识新建的修订；未创建修订时为空。
	RevisionID uuid.UUID `json:"revisionId,omitempty"`
}

// AiGenerateRunner 在持久化任务被领取后执行一次 AI 生成。
type AiGenerateRunner interface {
	RunAiGeneration(context.Context, AiGenerateArgs) (AiGenerateResult, error)
}

// AiGenerateWorker 围绕一次 asset.ai_generate 尝试推进持久化的 Meridian 状态。
type AiGenerateWorker struct {
	river.WorkerDefaults[AiGenerateArgs]
	store  ExecutionStore
	runner AiGenerateRunner
	now    func() time.Time
}

// NewAiGenerateWorker 构造 M3 的 AI 生成工作器。
func NewAiGenerateWorker(store ExecutionStore, runner AiGenerateRunner) *AiGenerateWorker {
	return &AiGenerateWorker{store: store, runner: runner, now: time.Now}
}

// Work 领取持久化任务，执行生成，并以脱敏的结果或携带失败阶段与错误码的结构化错误收尾。
func (worker *AiGenerateWorker) Work(ctx context.Context, job *river.Job[AiGenerateArgs]) error {
	if worker.store == nil {
		return errors.New("AI generation worker has no execution store")
	}
	if worker.runner == nil {
		return errors.New("AI generation worker has no runner")
	}
	startedAt := worker.now().UTC()
	claim, err := worker.store.StartJob(ctx, StartInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Stage: StageExtract,
		ExpectedAttempt: job.Attempt, StartedAt: startedAt,
	})
	if err != nil {
		return err
	}
	if !claim.Claimed {
		return nil
	}
	result, err := worker.runner.RunAiGeneration(ctx, job.Args)
	if err != nil {
		return worker.finishFailure(ctx, job.Args, job.Attempt, Stage(result.Stage), result.ErrorCode, err)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return worker.store.FinishJob(ctx, FinishInput{
		TenantID: job.Args.TenantID, JobID: job.Args.JobID, Status: jobStatusSucceeded, Result: payload,
		ExpectedAttempt: job.Attempt, Stage: StageIndex, Level: logLevelInfo,
		Message: stageMessageAssetAiGenerateCompleted, Terminal: true, FinishedAt: worker.now().UTC(),
	})
}

// finishFailure 使用失败阶段与稳定错误码持久化终态失败。
// 非终态错误会被刻意重新抛出以便 River 重试；这里所有结果都是终态，
// 因为运行器自行持久化结果快照，因此运行器返回的错误是硬性基础设施故障。
func (worker *AiGenerateWorker) finishFailure(ctx context.Context, args AiGenerateArgs, expectedAttempt int, stage Stage, errorCode string, cause error) error {
	if errorCode == "" {
		errorCode = errorCodeInternal
	}
	errorPayload, err := json.Marshal(struct {
		Code  string `json:"code"`
		Stage string `json:"stage"`
	}{Code: errorCode, Stage: string(stage)})
	if err != nil {
		return err
	}
	finishErr := worker.store.FinishJob(ctx, FinishInput{
		TenantID: args.TenantID, JobID: args.JobID, Status: jobStatusFailed, Error: errorPayload, ErrorCode: errorCode,
		ExpectedAttempt: expectedAttempt,
		Stage:           stage, Level: logLevelError, Message: stageMessageAssetAiGenerateFailed, Terminal: true, FinishedAt: worker.now().UTC(),
	})
	if finishErr != nil {
		return errors.Join(cause, finishErr)
	}
	return nil
}
