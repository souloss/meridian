-- 返回租户范围内按最新时间优先排列的一页追加式审计元数据。
-- 查询始终受 tenant_id 限制，结果不包含业务内容或携带秘密的字段。
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
  AND (
    COALESCE(array_length(sqlc.arg(action_filter)::text[], 1), 0) = 0
    OR audit_logs.action = ANY(sqlc.arg(action_filter)::text[])
  )
  AND (sqlc.arg(target_type)::text = '' OR audit_logs.target_type = sqlc.arg(target_type)::text)
  AND (sqlc.arg(target_id)::text = '' OR audit_logs.target_id::text = sqlc.arg(target_id)::text)
  AND (NOT sqlc.arg(from_set)::boolean OR audit_logs.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (NOT sqlc.arg(to_set)::boolean OR audit_logs.created_at <= sqlc.arg(to_time)::timestamptz)
ORDER BY audit_logs.created_at DESC, audit_logs.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 按照 ListTenantAuditLogs 的租户条件和全部可选过滤条件返回准确总数。
-- name: CountTenantAuditLogs :one
SELECT count(*)::bigint
FROM audit_logs
WHERE audit_logs.tenant_id = sqlc.arg(tenant_id)
  AND (NOT sqlc.arg(actor_id_set)::boolean OR audit_logs.actor_id = sqlc.arg(actor_id)::uuid)
  AND (
    COALESCE(array_length(sqlc.arg(action_filter)::text[], 1), 0) = 0
    OR audit_logs.action = ANY(sqlc.arg(action_filter)::text[])
  )
  AND (sqlc.arg(target_type)::text = '' OR audit_logs.target_type = sqlc.arg(target_type)::text)
  AND (sqlc.arg(target_id)::text = '' OR audit_logs.target_id::text = sqlc.arg(target_id)::text)
  AND (NOT sqlc.arg(from_set)::boolean OR audit_logs.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (NOT sqlc.arg(to_set)::boolean OR audit_logs.created_at <= sqlc.arg(to_time)::timestamptz);

-- 为平台控制面返回一页按最新时间优先排列的跨租户审计元数据。
-- 平台级记录的可空租户归属会保留在结果中。
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
  AND (
    COALESCE(array_length(sqlc.arg(action_filter)::text[], 1), 0) = 0
    OR audit_logs.action = ANY(sqlc.arg(action_filter)::text[])
  )
  AND (sqlc.arg(target_type)::text = '' OR audit_logs.target_type = sqlc.arg(target_type)::text)
  AND (sqlc.arg(target_id)::text = '' OR audit_logs.target_id::text = sqlc.arg(target_id)::text)
  AND (NOT sqlc.arg(from_set)::boolean OR audit_logs.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (NOT sqlc.arg(to_set)::boolean OR audit_logs.created_at <= sqlc.arg(to_time)::timestamptz)
  AND (sqlc.arg(tenant_slug)::text = '' OR tenants.slug = sqlc.arg(tenant_slug)::text)
ORDER BY audit_logs.created_at DESC, audit_logs.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 按照 ListPlatformAuditLogs 的跨租户条件和全部可选过滤条件返回准确总数。
-- name: CountPlatformAuditLogs :one
SELECT count(*)::bigint
FROM audit_logs
LEFT JOIN tenants ON tenants.id = audit_logs.tenant_id
WHERE (NOT sqlc.arg(actor_id_set)::boolean OR audit_logs.actor_id = sqlc.arg(actor_id)::uuid)
  AND (
    COALESCE(array_length(sqlc.arg(action_filter)::text[], 1), 0) = 0
    OR audit_logs.action = ANY(sqlc.arg(action_filter)::text[])
  )
  AND (sqlc.arg(target_type)::text = '' OR audit_logs.target_type = sqlc.arg(target_type)::text)
  AND (sqlc.arg(target_id)::text = '' OR audit_logs.target_id::text = sqlc.arg(target_id)::text)
  AND (NOT sqlc.arg(from_set)::boolean OR audit_logs.created_at >= sqlc.arg(from_time)::timestamptz)
  AND (NOT sqlc.arg(to_set)::boolean OR audit_logs.created_at <= sqlc.arg(to_time)::timestamptz)
  AND (sqlc.arg(tenant_slug)::text = '' OR tenants.slug = sqlc.arg(tenant_slug)::text);
