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
