-- ListPlatformJobs returns redacted cross-tenant job metadata in newest-first order.
-- Inputs, results, errors, attempts, refs, River identifiers, and logs are intentionally excluded.
-- Empty filter arrays and strings mean no restriction; scope identifiers are exposed only for tenant/repository jobs.
-- name: ListPlatformJobs :many
SELECT
  jobs.id,
  tenants.slug AS tenant_slug,
  jobs.type,
  jobs.trigger,
  jobs.status,
  jobs.stage,
  jobs.scope_type,
  COALESCE(CASE WHEN jobs.scope_type IN ('tenant', 'repository') THEN jobs.scope_id::text ELSE NULL::text END, ''::text)::text AS scope_id,
  jobs.created_at,
  jobs.started_at,
  jobs.finished_at
FROM jobs
JOIN tenants ON tenants.id = jobs.tenant_id
WHERE (COALESCE(array_length(sqlc.arg(type_filter)::text[], 1), 0) = 0 OR jobs.type = ANY(sqlc.arg(type_filter)::text[]))
  AND (COALESCE(array_length(sqlc.arg(status_filter)::text[], 1), 0) = 0 OR jobs.status = ANY(sqlc.arg(status_filter)::text[]))
  AND (sqlc.arg(scope_type)::text = '' OR jobs.scope_type = sqlc.arg(scope_type)::text)
  AND (sqlc.arg(scope_id)::text = '' OR jobs.scope_id::text = sqlc.arg(scope_id)::text)
  AND (sqlc.arg(tenant_slug)::text = '' OR tenants.slug = sqlc.arg(tenant_slug)::text)
ORDER BY jobs.created_at DESC, jobs.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- CountPlatformJobs returns the total redacted platform-job rows matching the supplied filters.
-- It repeats the exact predicates used by ListPlatformJobs so page totals cannot drift from the result set.
-- name: CountPlatformJobs :one
SELECT count(*)::bigint
FROM jobs
JOIN tenants ON tenants.id = jobs.tenant_id
WHERE (COALESCE(array_length(sqlc.arg(type_filter)::text[], 1), 0) = 0 OR jobs.type = ANY(sqlc.arg(type_filter)::text[]))
  AND (COALESCE(array_length(sqlc.arg(status_filter)::text[], 1), 0) = 0 OR jobs.status = ANY(sqlc.arg(status_filter)::text[]))
  AND (sqlc.arg(scope_type)::text = '' OR jobs.scope_type = sqlc.arg(scope_type)::text)
  AND (sqlc.arg(scope_id)::text = '' OR jobs.scope_id::text = sqlc.arg(scope_id)::text)
  AND (sqlc.arg(tenant_slug)::text = '' OR tenants.slug = sqlc.arg(tenant_slug)::text);

-- GetPlatformJob returns one redacted platform job without tenant-owned payload or execution details.
-- Scope identifiers are retained only for tenant and repository scopes, matching the public PlatformJob contract.
-- name: GetPlatformJob :one
SELECT
  jobs.id,
  tenants.slug AS tenant_slug,
  jobs.type,
  jobs.trigger,
  jobs.status,
  jobs.stage,
  jobs.scope_type,
  COALESCE(CASE WHEN jobs.scope_type IN ('tenant', 'repository') THEN jobs.scope_id::text ELSE NULL::text END, ''::text)::text AS scope_id,
  jobs.created_at,
  jobs.started_at,
  jobs.finished_at
FROM jobs
JOIN tenants ON tenants.id = jobs.tenant_id
WHERE jobs.id = sqlc.arg(id);

-- ListTenantJobs returns a newest-first page bounded by one tenant identifier.
-- Empty filter arrays and strings mean no restriction; execution input and River identifiers remain internal.
-- name: ListTenantJobs :many
SELECT jobs.*
FROM jobs
WHERE jobs.tenant_id = sqlc.arg(tenant_id)
  AND (COALESCE(array_length(sqlc.arg(type_filter)::text[], 1), 0) = 0 OR jobs.type = ANY(sqlc.arg(type_filter)::text[]))
  AND (COALESCE(array_length(sqlc.arg(status_filter)::text[], 1), 0) = 0 OR jobs.status = ANY(sqlc.arg(status_filter)::text[]))
  AND (sqlc.arg(scope_type)::text = '' OR jobs.scope_type = sqlc.arg(scope_type)::text)
  AND (sqlc.arg(scope_id)::text = '' OR jobs.scope_id::text = sqlc.arg(scope_id)::text)
ORDER BY jobs.created_at DESC, jobs.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- CountTenantJobs returns the exact total for the predicates used by ListTenantJobs.
-- name: CountTenantJobs :one
SELECT count(*)::bigint
FROM jobs
WHERE jobs.tenant_id = sqlc.arg(tenant_id)
  AND (COALESCE(array_length(sqlc.arg(type_filter)::text[], 1), 0) = 0 OR jobs.type = ANY(sqlc.arg(type_filter)::text[]))
  AND (COALESCE(array_length(sqlc.arg(status_filter)::text[], 1), 0) = 0 OR jobs.status = ANY(sqlc.arg(status_filter)::text[]))
  AND (sqlc.arg(scope_type)::text = '' OR jobs.scope_type = sqlc.arg(scope_type)::text)
  AND (sqlc.arg(scope_id)::text = '' OR jobs.scope_id::text = sqlc.arg(scope_id)::text);

-- GetTenantJob returns one full tenant-visible job row while retaining the tenant predicate.
-- name: GetTenantJob :one
SELECT *
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- GetTenantJobStreamState returns one state row plus the greatest persisted log cursor
-- from a single PostgreSQL statement so SSE never emits a state ahead of its logs.
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

-- ListTenantJobAttemptLogs batch-loads persisted events for a page of jobs without an N+1 query.
-- The caller groups rows by job, one-based attempt, and stage to construct the API attempt projection.
-- name: ListTenantJobAttemptLogs :many
SELECT *
FROM job_stage_logs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND job_id = ANY(sqlc.arg(job_ids)::uuid[])
ORDER BY job_id, attempt, sequence;

-- ListTenantJobLogsAfter returns bounded SSE replay rows strictly after a persisted sequence cursor.
-- name: ListTenantJobLogsAfter :many
SELECT *
FROM job_stage_logs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND job_id = sqlc.arg(job_id)
  AND sequence > sqlc.arg(after_sequence)
ORDER BY sequence
LIMIT sqlc.arg(event_limit);

-- LockTenantJobForControl serializes cancellation and manual retry decisions for one tenant job.
-- name: LockTenantJobForControl :one
SELECT *
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
FOR UPDATE;

-- CancelTenantJob moves only a pending or running tenant job to its durable cancelled terminal state.
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

-- LockLatestTenantJobGeneration returns the newest semantic generation while holding its row lock.
-- name: LockLatestTenantJobGeneration :one
SELECT *
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND dedupe_key = sqlc.arg(dedupe_key)
ORDER BY active_generation DESC
LIMIT 1
FOR UPDATE;

-- CreateRetriedTenantJob creates a new pending generation from immutable source execution inputs.
-- Result, error, stage, attempt, and timestamps are deliberately reset for the independent retry.
-- name: CreateRetriedTenantJob :one
INSERT INTO jobs (
  tenant_id, id, retry_of_job_id, type, scope_type, scope_id, ref_type, ref_name,
  trigger, input, status, max_attempts, dedupe_key, active_generation, replay_safe
)
SELECT
  source.tenant_id, sqlc.arg(new_job_id), source.id, source.type, source.scope_type, source.scope_id, source.ref_type, source.ref_name,
  'retry', source.input, 'pending', source.max_attempts, source.dedupe_key, sqlc.arg(active_generation), source.replay_safe
FROM jobs AS source
WHERE source.tenant_id = sqlc.arg(tenant_id)
  AND source.id = sqlc.arg(source_job_id)
  AND source.status IN ('failed', 'cancelled')
RETURNING *;

-- LockRetryJobIdempotency serializes one retry key for an authenticated tenant principal.
-- name: LockRetryJobIdempotency :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key), 0));

-- GetRetryJobIdempotency returns the retained exact response for a retryJob request.
-- name: GetRetryJobIdempotency :one
SELECT request_hash, response_body, expires_at
FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'retryJob'
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- DeleteRetryJobIdempotency removes an expired retry replay record before key reuse.
-- name: DeleteRetryJobIdempotency :exec
DELETE FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'retryJob'
  AND idempotency_key = sqlc.arg(idempotency_key);

-- CreateRetryJobIdempotency stores the exact non-secret 202 response for 24-hour replay.
-- name: CreateRetryJobIdempotency :exec
INSERT INTO idempotency_records (
  tenant_id, principal_type, principal_id, operation_id, idempotency_key,
  request_hash, response_status, response_body, expires_at
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(principal_type), sqlc.arg(principal_id), 'retryJob', sqlc.arg(idempotency_key),
  sqlc.arg(request_hash), 202, sqlc.arg(response_body)::jsonb, now() + interval '24 hours'
);
