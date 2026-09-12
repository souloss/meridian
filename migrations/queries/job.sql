-- 按最新时间优先返回脱敏的跨租户任务元数据。
-- 结果刻意排除输入、结果、错误、尝试次数、引用、River 标识和日志。
-- 空过滤数组和空字符串表示不限制；只有租户和仓库范围任务暴露作用域标识。
-- name: ListPlatformJobs :many
SELECT
  jobs.id,
  tenants.slug AS tenant_slug,
  jobs.type,
  jobs.trigger,
  jobs.status,
  jobs.stage,
  jobs.scope_type,
  COALESCE(
    CASE
      WHEN jobs.scope_type IN ('tenant', 'repository') THEN jobs.scope_id::text
      ELSE NULL::text
    END,
    ''::text
  )::text AS scope_id,
  jobs.created_at,
  jobs.started_at,
  jobs.finished_at
FROM jobs
JOIN tenants ON tenants.id = jobs.tenant_id
WHERE (
  COALESCE(array_length(sqlc.arg(type_filter)::text[], 1), 0) = 0
  OR jobs.type = ANY(sqlc.arg(type_filter)::text[])
)
  AND (
    COALESCE(array_length(sqlc.arg(status_filter)::text[], 1), 0) = 0
    OR jobs.status = ANY(sqlc.arg(status_filter)::text[])
  )
  AND (sqlc.arg(scope_type)::text = '' OR jobs.scope_type = sqlc.arg(scope_type)::text)
  AND (sqlc.arg(scope_id)::text = '' OR jobs.scope_id::text = sqlc.arg(scope_id)::text)
  AND (sqlc.arg(tenant_slug)::text = '' OR tenants.slug = sqlc.arg(tenant_slug)::text)
ORDER BY jobs.created_at DESC, jobs.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回匹配给定条件的脱敏平台任务总数。
-- 复用 ListPlatformJobs 的完整谓词，避免分页总数与结果集不一致。
-- name: CountPlatformJobs :one
SELECT count(*)::bigint
FROM jobs
JOIN tenants ON tenants.id = jobs.tenant_id
WHERE (
  COALESCE(array_length(sqlc.arg(type_filter)::text[], 1), 0) = 0
  OR jobs.type = ANY(sqlc.arg(type_filter)::text[])
)
  AND (
    COALESCE(array_length(sqlc.arg(status_filter)::text[], 1), 0) = 0
    OR jobs.status = ANY(sqlc.arg(status_filter)::text[])
  )
  AND (sqlc.arg(scope_type)::text = '' OR jobs.scope_type = sqlc.arg(scope_type)::text)
  AND (sqlc.arg(scope_id)::text = '' OR jobs.scope_id::text = sqlc.arg(scope_id)::text)
  AND (sqlc.arg(tenant_slug)::text = '' OR tenants.slug = sqlc.arg(tenant_slug)::text);

-- 返回一条脱敏平台任务，不包含租户拥有的负载或执行详情。
-- 保留租户和仓库范围的作用域标识，与公开 PlatformJob 契约一致。
-- name: GetPlatformJob :one
SELECT
  jobs.id,
  tenants.slug AS tenant_slug,
  jobs.type,
  jobs.trigger,
  jobs.status,
  jobs.stage,
  jobs.scope_type,
  COALESCE(
    CASE
      WHEN jobs.scope_type IN ('tenant', 'repository') THEN jobs.scope_id::text
      ELSE NULL::text
    END,
    ''::text
  )::text AS scope_id,
  jobs.created_at,
  jobs.started_at,
  jobs.finished_at
FROM jobs
JOIN tenants ON tenants.id = jobs.tenant_id
WHERE jobs.id = sqlc.arg(id);

-- 在一个租户标识边界内按最新时间优先返回任务分页。
-- 空过滤数组和空字符串表示不限制；执行输入和 River 标识仍为内部字段。
-- name: ListTenantJobs :many
SELECT jobs.*
FROM jobs
WHERE jobs.tenant_id = sqlc.arg(tenant_id)
  AND (
    COALESCE(array_length(sqlc.arg(type_filter)::text[], 1), 0) = 0
    OR jobs.type = ANY(sqlc.arg(type_filter)::text[])
  )
  AND (
    COALESCE(array_length(sqlc.arg(status_filter)::text[], 1), 0) = 0
    OR jobs.status = ANY(sqlc.arg(status_filter)::text[])
  )
  AND (sqlc.arg(scope_type)::text = '' OR jobs.scope_type = sqlc.arg(scope_type)::text)
  AND (sqlc.arg(scope_id)::text = '' OR jobs.scope_id::text = sqlc.arg(scope_id)::text)
ORDER BY jobs.created_at DESC, jobs.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回 ListTenantJobs 所用谓词对应的准确总数。
-- name: CountTenantJobs :one
SELECT count(*)::bigint
FROM jobs
WHERE jobs.tenant_id = sqlc.arg(tenant_id)
  AND (
    COALESCE(array_length(sqlc.arg(type_filter)::text[], 1), 0) = 0
    OR jobs.type = ANY(sqlc.arg(type_filter)::text[])
  )
  AND (
    COALESCE(array_length(sqlc.arg(status_filter)::text[], 1), 0) = 0
    OR jobs.status = ANY(sqlc.arg(status_filter)::text[])
  )
  AND (sqlc.arg(scope_type)::text = '' OR jobs.scope_type = sqlc.arg(scope_type)::text)
  AND (sqlc.arg(scope_id)::text = '' OR jobs.scope_id::text = sqlc.arg(scope_id)::text);

-- 保留租户条件，返回一条租户可见的完整任务记录。
-- name: GetTenantJob :one
SELECT *
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 在一条 PostgreSQL 语句中返回任务状态和已持久化日志的最大游标，
-- 确保 SSE 不会发送领先于日志记录的状态。
-- name: GetTenantJobStreamState :one
SELECT jobs.*,
  COALESCE((
    SELECT MAX(job_stage_logs.sequence)
    FROM job_stage_logs
    WHERE job_stage_logs.tenant_id = jobs.tenant_id
      AND job_stage_logs.job_id = jobs.id
  ), 0::bigint)::bigint AS log_cursor
FROM jobs
WHERE jobs.tenant_id = sqlc.arg(tenant_id)
  AND jobs.id = sqlc.arg(id);

-- 批量加载一页任务的持久化事件，避免 N+1 查询。
-- 调用方按任务、一基尝试次数和阶段分组，构造 API 尝试投影。
-- name: ListTenantJobAttemptLogs :many
SELECT *
FROM job_stage_logs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND job_id = ANY(sqlc.arg(job_ids)::uuid[])
ORDER BY job_id, attempt, sequence;

-- 返回持久化序号游标之后、数量受限的 SSE 回放记录。
-- name: ListTenantJobLogsAfter :many
SELECT *
FROM job_stage_logs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND job_id = sqlc.arg(job_id)
  AND sequence > sqlc.arg(after_sequence)
ORDER BY sequence
LIMIT sqlc.arg(event_limit);

-- 串行化一个租户任务的取消和手动重试决策。
-- name: LockTenantJobForControl :one
SELECT *
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
FOR UPDATE;

-- 仅将 pending 或 running 的租户任务转为持久化的 cancelled 终态。
-- name: CancelTenantJob :one
UPDATE jobs
SET status = 'cancelled',
    finished_at = sqlc.arg(finished_at)::timestamptz,
    next_attempt_at = NULL,
    updated_at = sqlc.arg(finished_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status IN ('pending', 'running')
RETURNING *;

-- 持有行锁时返回最新的语义代次。
-- name: LockLatestTenantJobGeneration :one
SELECT *
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND dedupe_key = sqlc.arg(dedupe_key)
ORDER BY active_generation DESC
LIMIT 1
FOR UPDATE;

-- 根据不可变的源执行输入创建一个新的 pending 代次。
-- 结果、错误、阶段、尝试次数和时间戳会重置，形成独立重试。
-- name: CreateRetriedTenantJob :one
INSERT INTO jobs (
  tenant_id, id, retry_of_job_id, type, scope_type, scope_id, ref_type, ref_name,
  trigger, input, status, max_attempts, dedupe_key, active_generation, replay_safe
)
SELECT
  source.tenant_id,
  sqlc.arg(new_job_id),
  source.id,
  source.type,
  source.scope_type,
  source.scope_id,
  source.ref_type,
  source.ref_name,
  'retry',
  source.input,
  'pending',
  source.max_attempts,
  source.dedupe_key,
  sqlc.arg(active_generation),
  source.replay_safe
FROM jobs AS source
WHERE source.tenant_id = sqlc.arg(tenant_id)
  AND source.id = sqlc.arg(source_job_id)
  AND source.status IN ('failed', 'cancelled')
RETURNING *;

-- 为已认证的租户主体串行化一个重试幂等键。
-- name: LockRetryJobIdempotency :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key), 0));

-- 返回 retryJob 请求保留的准确响应。
-- name: GetRetryJobIdempotency :one
SELECT request_hash, response_body, expires_at
FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'retryJob'
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- 在重新使用幂等键前删除已过期的重试重放记录。
-- name: DeleteRetryJobIdempotency :exec
DELETE FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'retryJob'
  AND idempotency_key = sqlc.arg(idempotency_key);

-- 保存可重放 24 小时的准确、非敏感 202 响应。
-- name: CreateRetryJobIdempotency :exec
INSERT INTO idempotency_records (
  tenant_id, principal_type, principal_id, operation_id, idempotency_key,
  request_hash, response_status, response_body, expires_at
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(principal_type), sqlc.arg(principal_id), 'retryJob', sqlc.arg(idempotency_key),
  sqlc.arg(request_hash), 202, sqlc.arg(response_body)::jsonb, now() + interval '24 hours'
);
