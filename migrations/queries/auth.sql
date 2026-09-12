-- 创建一个全局身份，保存 Argon2id PHC 校验值，不保存密码明文。
-- name: CreateUser :one
INSERT INTO users (
  id,
  username,
  password_hash,
  display_name,
  email,
  is_platform_admin
) VALUES (
  sqlc.arg(id),
  sqlc.arg(username),
  sqlc.arg(password_hash),
  sqlc.arg(display_name),
  sqlc.narg(email),
  sqlc.arg(is_platform_admin)
)
RETURNING *;

-- 为新身份创建语言、主题和默认视图偏好。
-- name: CreateDefaultUserPreferences :one
INSERT INTO user_preferences (user_id)
VALUES (sqlc.arg(user_id))
RETURNING *;

-- 按准确且唯一的登录名返回全局身份。
-- name: GetUserByUsername :one
SELECT *
FROM users
WHERE username = sqlc.arg(username);

-- 按传入 UUID 返回对应的全局身份。
-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = sqlc.arg(id);

-- 为平台管理员返回稳定分页的身份元数据，不包含密码或会话秘密。
-- 可选搜索值只匹配 username 和 display_name。
-- name: ListUsers :many
SELECT id, username, display_name, email, status, is_platform_admin, revision, created_at, updated_at
FROM users
WHERE (
  sqlc.arg(search_query)::text = ''
  OR username ILIKE '%' || sqlc.arg(search_query)::text || '%'
  OR display_name ILIKE '%' || sqlc.arg(search_query)::text || '%'
)
ORDER BY username, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回匹配一次平台搜索的身份数量。
-- name: CountUsers :one
SELECT count(*)::bigint
FROM users
WHERE (
  sqlc.arg(search_query)::text = ''
  OR username ILIKE '%' || sqlc.arg(search_query)::text || '%'
  OR display_name ILIKE '%' || sqlc.arg(search_query)::text || '%'
);

-- 授予平台控制面权限，并递增用户版本号。
-- name: PromoteUserToPlatformAdmin :one
UPDATE users
SET
  is_platform_admin = true,
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
RETURNING *;

-- 保存会话令牌和 CSRF 摘要，不保存任一令牌的明文。
-- name: CreateSession :one
INSERT INTO sessions (
  id,
  user_id,
  token_hash,
  csrf_hash,
  expires_at
) VALUES (
  sqlc.arg(id),
  sqlc.arg(user_id),
  sqlc.arg(token_hash),
  sqlc.arg(csrf_hash),
  sqlc.arg(expires_at)
)
RETURNING *;

-- 在调用方指定的时间点，根据令牌摘要认证一个有效浏览器会话和有效用户。
-- name: GetSessionPrincipalByTokenHash :one
SELECT
  sessions.id AS session_id,
  sessions.user_id,
  sessions.csrf_hash,
  sessions.expires_at,
  users.username,
  users.display_name,
  users.email,
	users.status AS user_status,
	users.is_platform_admin,
	users.revision AS user_revision,
	users.created_at AS user_created_at,
	users.updated_at AS user_updated_at
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = sqlc.arg(token_hash)
  AND sessions.revoked_at IS NULL
  AND sessions.expires_at > sqlc.arg(authenticated_at)
  AND users.status = 'active';

-- 记录未撤销浏览器会话最近一次被接受请求的时间。
-- name: TouchSession :exec
UPDATE sessions
SET
  last_seen_at = sqlc.arg(seen_at),
  updated_at = sqlc.arg(seen_at)
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL;

-- 替换一个有效浏览器会话的 CSRF 摘要。
-- name: RotateSessionCSRFHash :execrows
UPDATE sessions
SET
  csrf_hash = sqlc.arg(csrf_hash),
  updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg(updated_at);

-- 原子撤销一个有效浏览器会话，并返回是否有记录发生变化。
-- name: RevokeSession :execrows
UPDATE sessions
SET
  revoked_at = sqlc.arg(revoked_at),
  updated_at = sqlc.arg(revoked_at)
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL;

-- 保存租户范围内的 PAT 元数据和令牌摘要，不保存令牌明文。
-- name: CreateAPIToken :one
INSERT INTO api_tokens (
  tenant_id,
  id,
  user_id,
  name,
  token_hash,
  scopes,
  expires_at
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(id),
  sqlc.arg(user_id),
  sqlc.arg(name),
  sqlc.arg(token_hash),
  sqlc.arg(scopes),
  sqlc.narg(expires_at)
)
RETURNING *;

-- 根据令牌摘要认证一个用户、成员关系和租户均有效的 PAT。
-- name: GetAPITokenPrincipalByTokenHash :one
SELECT
  api_tokens.tenant_id,
  api_tokens.id AS token_id,
  api_tokens.user_id,
  api_tokens.scopes,
  api_tokens.expires_at,
  users.username,
  users.display_name,
	users.email,
	users.status AS user_status,
	users.revision AS user_revision,
	users.created_at AS user_created_at,
	users.updated_at AS user_updated_at,
	tenant_members.role,
	tenants.slug AS tenant_slug
FROM api_tokens
JOIN users ON users.id = api_tokens.user_id
JOIN tenant_members
  ON tenant_members.tenant_id = api_tokens.tenant_id
 AND tenant_members.user_id = api_tokens.user_id
JOIN tenants ON tenants.id = api_tokens.tenant_id
WHERE api_tokens.token_hash = sqlc.arg(token_hash)
  AND api_tokens.revoked_at IS NULL
  AND (api_tokens.expires_at IS NULL OR api_tokens.expires_at > sqlc.arg(authenticated_at))
  AND users.status = 'active'
  AND tenants.status = 'active';

-- 记录未撤销租户 PAT 最近一次成功使用的时间。
-- name: TouchAPIToken :exec
UPDATE api_tokens
SET
  last_used_at = sqlc.arg(used_at),
  updated_at = sqlc.arg(used_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revoked_at IS NULL;

-- 返回一个用户在一个租户内拥有的 PAT 元数据总数。
-- name: CountAPITokensByUser :one
SELECT count(*)
FROM api_tokens
WHERE tenant_id = sqlc.arg(tenant_id)
  AND user_id = sqlc.arg(user_id);

-- 在明确的租户边界内，返回一个用户的稳定分页 PAT 元数据。
-- name: ListAPITokensByUser :many
SELECT *
FROM api_tokens
WHERE tenant_id = sqlc.arg(tenant_id)
  AND user_id = sqlc.arg(user_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 幂等撤销一个租户内用户拥有的 PAT，并返回其标识。
-- 返回已经撤销的匹配记录以保持幂等；不存在或属于其他主体的记录仍视为未找到。
-- name: RevokeAPIToken :one
UPDATE api_tokens
SET
  revoked_at = COALESCE(revoked_at, sqlc.arg(revoked_at)),
  updated_at = CASE WHEN revoked_at IS NULL THEN sqlc.arg(revoked_at) ELSE updated_at END
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
RETURNING id;
