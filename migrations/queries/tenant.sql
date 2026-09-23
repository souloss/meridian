-- 使用明确的配额和设置快照创建租户，快照来自平台默认配置。
-- name: CreateTenant :one
INSERT INTO tenants (
  id,
  slug,
  display_name,
  quota,
  settings
) VALUES (
  sqlc.arg(id),
  sqlc.arg(slug),
  sqlc.arg(display_name),
  sqlc.arg(quota),
  sqlc.arg(settings)
)
RETURNING *;

-- 返回创建租户时原子复制到新租户的单例 JSON 默认配置。
-- name: GetPlatformSettingsForTenantCreate :one
SELECT settings
FROM platform_settings
WHERE id = 'default';

-- 按 slug 返回任意生命周期状态的租户，供平台管理使用。
-- name: GetTenantBySlug :one
SELECT *
FROM tenants
WHERE slug = sqlc.arg(slug);

-- 有条件地更新平台控制的租户字段并递增版本号。
-- 字段 set 标志保留 PATCH 字段省略状态，同时允许完整替换配额。
-- name: UpdateTenant :one
UPDATE tenants
SET
  display_name = CASE WHEN sqlc.arg(set_display_name)::boolean THEN sqlc.arg(display_name) ELSE display_name END,
  status = CASE WHEN sqlc.arg(set_status)::boolean THEN sqlc.arg(status) ELSE status END,
  quota = CASE WHEN sqlc.arg(set_quota)::boolean THEN sqlc.arg(quota) ELSE quota END,
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at)
WHERE slug = sqlc.arg(slug)
  AND revision = sqlc.arg(expected_revision)
RETURNING id, slug, display_name, status, quota, settings, revision, created_at, updated_at;

-- 按稳定 slug 和 UUID 顺序返回全部租户生命周期记录，供平台管理使用。
-- name: ListTenants :many
SELECT id, slug, display_name, status, quota, revision, created_at, updated_at
FROM tenants
ORDER BY slug, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回平台管理可见的租户生命周期记录数量。
-- name: CountTenants :one
SELECT count(*)::bigint
FROM tenants;

-- 仅按 slug 返回有效租户，供租户范围业务访问使用。
-- name: GetActiveTenantBySlug :one
SELECT *
FROM tenants
WHERE slug = sqlc.arg(slug)
  AND status = 'active';

-- 创建或替换租户角色关系，并记录调用方提供的更新时间。
-- name: UpsertTenantMember :one
INSERT INTO tenant_members (
  tenant_id,
  user_id,
  role
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(user_id),
  sqlc.arg(role)
)
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET
  role = EXCLUDED.role,
  updated_at = sqlc.arg(updated_at)
RETURNING *;

-- 返回一条有效租户成员关系，不泄露已停用租户记录。
-- name: GetActiveTenantMembership :one
SELECT
  tenants.id AS tenant_id,
  tenants.slug AS tenant_slug,
  tenants.display_name AS tenant_display_name,
  tenant_members.role
FROM tenant_members
JOIN tenants ON tenants.id = tenant_members.tenant_id
WHERE tenant_members.user_id = sqlc.arg(user_id)
  AND tenants.slug = sqlc.arg(tenant_slug)
  AND tenants.status = 'active';

-- 按稳定 slug 和 UUID 顺序返回用户的有效租户成员关系。
-- name: ListActiveTenantMemberships :many
SELECT
  tenants.id AS tenant_id,
  tenants.slug AS tenant_slug,
  tenants.display_name AS tenant_display_name,
  tenant_members.role
FROM tenant_members
JOIN tenants ON tenants.id = tenant_members.tenant_id
WHERE tenant_members.user_id = sqlc.arg(user_id)
  AND tenants.status = 'active'
ORDER BY tenants.slug, tenants.id;

-- 在 If-Match 下将租户置为 disabled（删除流程的第一步）。
-- name: DisableTenant :one
UPDATE tenants
SET status = 'disabled', revision = revision + 1, updated_at = now()
WHERE id = sqlc.arg(tenant_id)
  AND status = 'active'
  AND revision = sqlc.arg(expected_revision)
RETURNING id, slug, display_name, status, quota, settings, revision, created_at, updated_at;

-- 为租户删除记录一条 tenant.delete 任务。
-- name: CreateTenantDeleteJob :one
INSERT INTO jobs (
  tenant_id, id, type, scope_type, scope_id, trigger, input,
  status, max_attempts, dedupe_key, active_generation, replay_safe
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), 'tenant.delete', 'tenant', sqlc.arg(tenant_id), 'api',
  '{}'::jsonb, 'pending', 3, sqlc.arg(dedupe_key), 1, true
)
RETURNING *;
