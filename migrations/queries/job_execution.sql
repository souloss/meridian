-- StartJobExecution claims a durable Meridian job for one River attempt.
-- A terminal domain row is intentionally not claimed again; this makes River retries
-- harmless after a worker already committed a terminal result.
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

-- SetJobExecutionStage records the active pipeline stage without changing the
-- durable lifecycle state. Stage values are constrained by the application DDL.
-- name: SetJobExecutionStage :execrows
UPDATE jobs
SET stage = sqlc.arg(stage)::text,
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status = 'running'
  AND attempt = sqlc.arg(expected_attempt)::integer;

-- LockJobStageSequence serializes the per-job log cursor inside the caller's transaction.
-- The lock key is derived from tenant and job UUIDs and never contains user content.
-- name: LockJobStageSequence :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key), 0));

-- NextJobStageSequence returns the next replay cursor after the caller acquires
-- the job-specific advisory transaction lock.
-- name: NextJobStageSequence :one
SELECT (COALESCE(MAX(sequence), 0::bigint) + 1::bigint)::bigint AS next_sequence
FROM job_stage_logs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND job_id = sqlc.arg(job_id);

-- AppendJobStageLog persists one redacted stage event with its caller-supplied cursor.
-- name: AppendJobStageLog :one
INSERT INTO job_stage_logs (tenant_id, job_id, sequence, attempt, stage, level, message, occurred_at)
VALUES (
  sqlc.arg(tenant_id), sqlc.arg(job_id), sqlc.arg(sequence), sqlc.arg(attempt)::integer, sqlc.arg(stage)::text,
  sqlc.arg(level)::text, sqlc.arg(message)::text, sqlc.arg(occurred_at)::timestamptz
)
RETURNING *;

-- FinishJobExecution records either a terminal result or a retryable failure.
-- Retryable failures remain pending for River's next attempt; terminal failures
-- receive a finished timestamp and a durable failed status.
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

-- AttachRiverJobID links the application UUID job to the internal River sequence
-- in the same transaction that inserted both rows.
-- name: AttachRiverJobID :execrows
UPDATE jobs
SET river_job_id = sqlc.arg(river_job_id),
    updated_at = sqlc.arg(updated_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND river_job_id IS NULL;
