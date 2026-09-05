-- ListTenantAuditLogs returns one newest-first page of append-only audit metadata
-- inside an explicit tenant boundary. Business content and secret-bearing columns
-- do not exist in this projection.
-- name: ListTenantAuditLogs :many
SELECT
  audit_logs.id,
  tenants.slug AS tenant_slug,
  audit_logs.actor_id,
  audit_logs.action,
  audit_logs.target_type,
  COALESCE(audit_logs.target_id::text, ''::text)::text AS target_id,
  COALESCE(audit_logs.request_id::text, ''::text)::text AS request_id,
  audit_logs.detail,
  audit_logs.created_at
FROM audit_logs
JOIN tenants ON tenants.id = audit_logs.tenant_id
WHERE audit_logs.tenant_id = sqlc.arg(tenant_id)
  AND (NOT sqlc.arg(actor_id_set)::boolean OR audit_logs.actor_id = sqlc.arg(actor_id)::uuid)
  AND (COALESCE(array_length(sqlc.arg(action_filter)::text[], 1), 0) = 0 OR audit_logs.action = ANY(sqlc.arg(action_filter)::text[]))
  AND (sqlc.arg(target_type)::text = '' OR audit_logs.target_type = sqlc.arg(target_type)::text)
  AND (sqlc.arg(target_id)::text = '' OR audit_logs.target_id::text = sqlc.arg(target_id)::text)
  AND (NOT sqlc.arg(from_set)::boolean OR audit_logs.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (NOT sqlc.arg(to_set)::boolean OR audit_logs.created_at <= sqlc.arg(to_time)::timestamptz)
ORDER BY audit_logs.created_at DESC, audit_logs.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- CountTenantAuditLogs returns the exact total for ListTenantAuditLogs by
-- repeating its tenant predicate and every optional filter.
-- name: CountTenantAuditLogs :one
SELECT count(*)::bigint
FROM audit_logs
WHERE audit_logs.tenant_id = sqlc.arg(tenant_id)
  AND (NOT sqlc.arg(actor_id_set)::boolean OR audit_logs.actor_id = sqlc.arg(actor_id)::uuid)
  AND (COALESCE(array_length(sqlc.arg(action_filter)::text[], 1), 0) = 0 OR audit_logs.action = ANY(sqlc.arg(action_filter)::text[]))
  AND (sqlc.arg(target_type)::text = '' OR audit_logs.target_type = sqlc.arg(target_type)::text)
  AND (sqlc.arg(target_id)::text = '' OR audit_logs.target_id::text = sqlc.arg(target_id)::text)
  AND (NOT sqlc.arg(from_set)::boolean OR audit_logs.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (NOT sqlc.arg(to_set)::boolean OR audit_logs.created_at <= sqlc.arg(to_time)::timestamptz);

-- ListPlatformAuditLogs returns one newest-first cross-tenant page for the
-- platform control plane. Nullable tenant ownership is retained for platform facts.
-- name: ListPlatformAuditLogs :many
SELECT
  audit_logs.id,
  COALESCE(tenants.slug, ''::text)::text AS tenant_slug,
  audit_logs.actor_id,
  audit_logs.action,
  audit_logs.target_type,
  COALESCE(audit_logs.target_id::text, ''::text)::text AS target_id,
  COALESCE(audit_logs.request_id::text, ''::text)::text AS request_id,
  audit_logs.detail,
  audit_logs.created_at
FROM audit_logs
LEFT JOIN tenants ON tenants.id = audit_logs.tenant_id
WHERE (NOT sqlc.arg(actor_id_set)::boolean OR audit_logs.actor_id = sqlc.arg(actor_id)::uuid)
  AND (COALESCE(array_length(sqlc.arg(action_filter)::text[], 1), 0) = 0 OR audit_logs.action = ANY(sqlc.arg(action_filter)::text[]))
  AND (sqlc.arg(target_type)::text = '' OR audit_logs.target_type = sqlc.arg(target_type)::text)
  AND (sqlc.arg(target_id)::text = '' OR audit_logs.target_id::text = sqlc.arg(target_id)::text)
  AND (NOT sqlc.arg(from_set)::boolean OR audit_logs.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (NOT sqlc.arg(to_set)::boolean OR audit_logs.created_at <= sqlc.arg(to_time)::timestamptz)
  AND (sqlc.arg(tenant_slug)::text = '' OR tenants.slug = sqlc.arg(tenant_slug)::text)
ORDER BY audit_logs.created_at DESC, audit_logs.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- CountPlatformAuditLogs returns the exact total for ListPlatformAuditLogs by
-- repeating its cross-tenant predicates and every optional filter.
-- name: CountPlatformAuditLogs :one
SELECT count(*)::bigint
FROM audit_logs
LEFT JOIN tenants ON tenants.id = audit_logs.tenant_id
WHERE (NOT sqlc.arg(actor_id_set)::boolean OR audit_logs.actor_id = sqlc.arg(actor_id)::uuid)
  AND (COALESCE(array_length(sqlc.arg(action_filter)::text[], 1), 0) = 0 OR audit_logs.action = ANY(sqlc.arg(action_filter)::text[]))
  AND (sqlc.arg(target_type)::text = '' OR audit_logs.target_type = sqlc.arg(target_type)::text)
  AND (sqlc.arg(target_id)::text = '' OR audit_logs.target_id::text = sqlc.arg(target_id)::text)
  AND (NOT sqlc.arg(from_set)::boolean OR audit_logs.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (NOT sqlc.arg(to_set)::boolean OR audit_logs.created_at <= sqlc.arg(to_time)::timestamptz)
  AND (sqlc.arg(tenant_slug)::text = '' OR tenants.slug = sqlc.arg(tenant_slug)::text);
