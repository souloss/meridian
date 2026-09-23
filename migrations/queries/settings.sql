-- 平台设置与租户设置、用户偏好的读取与写入查询。
-- platform_settings 为单例 global 行；tenant settings 存于 tenants.settings（jsonb）；user_preferences 全局按 user_id。

-- 返回平台默认配置单例（含 revision 与时间戳）。
-- name: GetPlatformSettings :one
SELECT id, settings, revision, created_at, updated_at
FROM platform_settings
WHERE id = 'default';

-- 在 If-Match 下替换平台默认配置并递增 revision。
-- name: UpdatePlatformSettings :one
UPDATE platform_settings
SET
  settings = sqlc.arg(settings),
  revision = revision + 1,
  updated_at = now()
WHERE id = 'default'
  AND revision = sqlc.arg(expected_revision)
RETURNING id, settings, revision, created_at, updated_at;

-- 返回一个租户的 settings 快照与 revision。
-- name: GetTenantSettingsForRead :one
SELECT settings, revision
FROM tenants
WHERE id = sqlc.arg(tenant_id)
  AND status = 'active';

-- 在 If-Match 下替换租户 settings 并递增 revision。
-- name: UpdateTenantSettingsForWrite :one
UPDATE tenants
SET
  settings = sqlc.arg(settings),
  revision = revision + 1,
  updated_at = now()
WHERE id = sqlc.arg(tenant_id)
  AND status = 'active'
  AND revision = sqlc.arg(expected_revision)
RETURNING settings, revision;
