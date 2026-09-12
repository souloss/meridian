-- 为一次 River 尝试领取持久化 Meridian 任务。
-- 终态领域记录不会再次领取，因此 worker 已提交终态后发生 River 重试也不会产生副作用。
-- name: StartJobExecution :one
UPDATE jobs
SET status = 'running',
    stage = sqlc.arg(stage)::text,
    attempt = sqlc.arg(expected_attempt)::integer,
    started_at = COALESCE(started_at, sqlc.arg(started_at)::timestamptz),
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND (
    status IN ('pending', 'running')
    AND attempt < sqlc.arg(expected_attempt)::integer
  )
  AND sqlc.arg(expected_attempt)::integer > 0
RETURNING *;

-- 记录当前流水线阶段，不改变持久化生命周期状态。
-- 阶段取值受应用 DDL 约束。
-- name: SetJobExecutionStage :execrows
UPDATE jobs
SET stage = sqlc.arg(stage)::text,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status = 'running'
  AND attempt = sqlc.arg(expected_attempt)::integer;

-- 在调用方事务内串行化单个任务的日志游标。
-- 锁键由租户和任务 UUID 派生，不包含用户内容。
-- name: LockJobStageSequence :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key), 0));

-- 调用方取得任务专属事务 advisory lock 后，返回下一个回放游标。
-- name: NextJobStageSequence :one
SELECT (COALESCE(MAX(sequence), 0::bigint) + 1::bigint)::bigint AS next_sequence
FROM job_stage_logs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND job_id = sqlc.arg(job_id);

-- 使用调用方提供的游标持久化一条脱敏阶段事件。
-- name: AppendJobStageLog :one
INSERT INTO job_stage_logs (tenant_id, job_id, sequence, attempt, stage, level, message, occurred_at)
VALUES (
  sqlc.arg(tenant_id), sqlc.arg(job_id), sqlc.arg(sequence), sqlc.arg(attempt)::integer, sqlc.arg(stage)::text,
  sqlc.arg(level)::text, sqlc.arg(message)::text, sqlc.arg(occurred_at)::timestamptz
)
RETURNING *;

-- 记录终态结果或可重试失败。
-- 可重试失败保持 pending，等待 River 下一次尝试；终态失败会写入完成时间和 failed 状态。
-- name: FinishJobExecution :one
UPDATE jobs
SET status = sqlc.arg(status)::text,
    result = sqlc.arg(result)::jsonb,
    error = sqlc.arg(error)::jsonb,
    finished_at = CASE WHEN sqlc.arg(terminal)::boolean THEN sqlc.arg(finished_at)::timestamptz ELSE NULL END,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status = 'running'
  AND attempt = sqlc.arg(expected_attempt)::integer
RETURNING *;

-- 在插入两行的同一事务中，将应用 UUID 任务关联到内部 River 序号。
-- name: AttachRiverJobID :execrows
UPDATE jobs
SET river_job_id = sqlc.arg(river_job_id),
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND river_job_id IS NULL;
