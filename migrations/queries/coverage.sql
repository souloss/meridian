-- 服务收藏、用户偏好、视图覆盖、标签与轻量评论的持久化查询。
-- 全部查询保留 tenant_id 谓词；user_preferences 为全局按 user_id。

-- 收藏一个服务（幂等）。
-- name: UpsertServiceStar :execrows
INSERT INTO service_stars (tenant_id, service_id, user_id)
VALUES (sqlc.arg(tenant_id), sqlc.arg(service_id), sqlc.arg(user_id))
ON CONFLICT (tenant_id, service_id, user_id) DO NOTHING;

-- 取消收藏一个服务。
-- name: DeleteServiceStar :execrows
DELETE FROM service_stars
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id)
  AND user_id = sqlc.arg(user_id);

-- 返回一个服务是否被当前用户收藏。
-- name: GetServiceStar :one
SELECT service_id
FROM service_stars
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id)
  AND user_id = sqlc.arg(user_id);

-- 返回一个用户的偏好，供读取。
-- name: GetUserPreferences :one
SELECT *
FROM user_preferences
WHERE user_id = sqlc.arg(user_id);

-- 在 If-Match 下更新用户偏好并递增 revision。
-- name: UpdateUserPreferences :one
UPDATE user_preferences
SET
  locale = COALESCE(sqlc.narg(locale), locale),
  theme = COALESCE(sqlc.narg(theme), theme),
  default_views = COALESCE(sqlc.narg(default_views), default_views),
  revision = revision + 1,
  updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 返回租户内全部视图覆盖。
-- name: ListViewOverrides :many
SELECT *
FROM view_overrides
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY view_id;

-- 返回一条视图覆盖。
-- name: GetViewOverride :one
SELECT *
FROM view_overrides
WHERE tenant_id = sqlc.arg(tenant_id)
  AND view_id = sqlc.arg(view_id);

-- 幂等设置一条视图覆盖（创建或替换并递增 revision）。
-- name: UpsertViewOverride :one
INSERT INTO view_overrides (tenant_id, view_id, enabled, ord, default_options, revision)
VALUES (sqlc.arg(tenant_id), sqlc.arg(view_id), sqlc.arg(enabled), sqlc.arg(ord), sqlc.arg(default_options)::jsonb, 1)
ON CONFLICT (tenant_id, view_id) DO UPDATE SET
  enabled = EXCLUDED.enabled,
  ord = EXCLUDED.ord,
  default_options = EXCLUDED.default_options,
  revision = view_overrides.revision + 1,
  updated_at = now()
RETURNING *;

-- 删除一条视图覆盖。
-- name: DeleteViewOverride :execrows
DELETE FROM view_overrides
WHERE tenant_id = sqlc.arg(tenant_id)
  AND view_id = sqlc.arg(view_id);

-- 返回租户内全部标签，按名称排序。
-- name: ListTags :many
SELECT *
FROM tag_definitions
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY name, id;

-- 返回一条标签。
-- name: GetTag :one
SELECT *
FROM tag_definitions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 创建一条标签。颜色由服务层缺省填充（见 service.Coverage.CreateTag 的 tagColorDefault），
-- 此处按非空列值直接落库；SQL 侧不再依赖列 DEFAULT 兜底 NULL。
-- name: CreateTag :one
INSERT INTO tag_definitions (tenant_id, id, name, color, description)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(name), sqlc.narg(color), sqlc.narg(description))
RETURNING *;

-- 在 If-Match 下更新一条标签并递增 revision。
-- name: UpdateTag :one
UPDATE tag_definitions
SET
  name = COALESCE(sqlc.narg(name), name),
  color = COALESCE(sqlc.narg(color), color),
  description = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.narg(description) ELSE description END,
  revision = revision + 1,
  updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 删除一条标签（关联由外键级联清理）。
-- name: DeleteTag :execrows
DELETE FROM tag_definitions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision);

-- 返回一个服务下的评论分页。
-- name: ListServiceComments :many
SELECT comments.*, users.display_name AS author_display_name
FROM service_comments AS comments
JOIN users ON users.id = comments.author_id
WHERE comments.tenant_id = sqlc.arg(tenant_id)
  AND comments.service_id = sqlc.arg(service_id)
  AND comments.deleted_at IS NULL
ORDER BY comments.created_at, comments.id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回一个服务下的评论总数。
-- name: CountServiceComments :one
SELECT count(*)::bigint
FROM service_comments
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id)
  AND deleted_at IS NULL;

-- 创建一条服务评论。
-- name: CreateServiceComment :one
INSERT INTO service_comments (tenant_id, id, service_id, author_id, body)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(service_id), sqlc.arg(author_id), sqlc.arg(body))
RETURNING *;

-- 创建一条租户导出任务记录（pending）。
-- name: CreateTenantExport :one
INSERT INTO tenant_exports (tenant_id, id, requested_by, status)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(requested_by), 'pending')
RETURNING *;
