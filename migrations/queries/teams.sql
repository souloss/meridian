-- 团队、成员关系与租户设置的持久化查询。
-- 全部查询保留 tenant_id 谓词；团队成员要求用户先具备同租户成员关系（外键强制）。

-- 创建一条租户内团队。
-- name: CreateTeam :one
INSERT INTO teams (tenant_id, id, slug, display_name)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(slug), sqlc.arg(display_name))
RETURNING *;

-- 按 id 返回一条团队。
-- name: GetTeam :one
SELECT *
FROM teams
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 列出租户内全部团队。
-- name: ListTeams :many
SELECT *
FROM teams
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY slug, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回租户内团队总数。
-- name: CountTeams :one
SELECT count(*)::bigint
FROM teams
WHERE tenant_id = sqlc.arg(tenant_id);

-- 返回一个团队的成员用户 id。
-- name: ListTeamMembers :many
SELECT user_id
FROM team_members
WHERE tenant_id = sqlc.arg(tenant_id)
  AND team_id = sqlc.arg(team_id)
ORDER BY user_id;

-- 一次批量返回多个团队的成员，避免逐团队查询的 N+1 往返。
-- name: ListTeamMembersForTeams :many
SELECT team_id, user_id
FROM team_members
WHERE tenant_id = sqlc.arg(tenant_id)
  AND team_id = ANY(sqlc.arg(team_ids)::uuid[])
ORDER BY team_id, user_id;

-- 清空一个团队的成员关系。
-- name: DeleteTeamMembers :execrows
DELETE FROM team_members
WHERE tenant_id = sqlc.arg(tenant_id)
  AND team_id = sqlc.arg(team_id);

-- 为团队插入一条成员关系。
-- name: InsertTeamMember :execrows
INSERT INTO team_members (tenant_id, team_id, user_id)
VALUES (sqlc.arg(tenant_id), sqlc.arg(team_id), sqlc.arg(user_id));

-- 校验替换成员是否都属于同一租户；任一所给 id 缺失即返回空集合以触发 404。
-- name: ListTenantMembersByIDs :many
SELECT id
FROM users
WHERE id = ANY(sqlc.arg(user_ids)::uuid[])
  AND id IN (
    SELECT user_id FROM tenant_members WHERE tenant_id = sqlc.arg(tenant_id)
  )
ORDER BY id;

-- 在 If-Match 下更新团队展示名并递增 revision。
-- name: UpdateTeam :one
UPDATE teams
SET
  display_name = sqlc.arg(display_name),
  revision = revision + 1,
  updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 删除一条团队；关联成员关系由外键级联清理。
-- name: DeleteTeam :execrows
DELETE FROM teams
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision);

-- 返回租户内成员（用户 × 角色）的分页投影。
-- name: ListTenantMembers :many
SELECT users.id AS user_id, users.username, users.display_name, users.email,
       users.status, users.is_platform_admin, users.revision AS user_revision,
       users.created_at AS user_created_at, users.updated_at AS user_updated_at,
       tenant_members.role, tenant_members.created_at AS joined_at
FROM tenant_members
JOIN users ON users.id = tenant_members.user_id
WHERE tenant_members.tenant_id = sqlc.arg(tenant_id)
ORDER BY users.username, users.id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回租户内成员总数。
-- name: CountTenantMembers :one
SELECT count(*)::bigint
FROM tenant_members
WHERE tenant_id = sqlc.arg(tenant_id);

-- 删除一条租户成员关系。
-- name: DeleteTenantMember :execrows
DELETE FROM tenant_members
WHERE tenant_id = sqlc.arg(tenant_id)
  AND user_id = sqlc.arg(user_id);

-- 返回租户内匹配查询的成员用户元数据，供成员选择目录使用。
-- name: SearchTenantUsers :many
SELECT users.id AS user_id, users.username, users.display_name, users.email,
       users.status, users.is_platform_admin, users.revision AS user_revision,
       users.created_at AS user_created_at, users.updated_at AS user_updated_at,
       tenant_members.role, tenant_members.created_at AS joined_at
FROM tenant_members
JOIN users ON users.id = tenant_members.user_id
WHERE tenant_members.tenant_id = sqlc.arg(tenant_id)
  AND (
    sqlc.arg(search_query)::text = ''
    OR users.username ILIKE '%' || sqlc.arg(search_query)::text || '%'
    OR users.display_name ILIKE '%' || sqlc.arg(search_query)::text || '%'
  )
ORDER BY users.username, users.id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回匹配一次租户目录搜索的成员总数。
-- name: CountTenantUserSearch :one
SELECT count(*)::bigint
FROM tenant_members
JOIN users ON users.id = tenant_members.user_id
WHERE tenant_members.tenant_id = sqlc.arg(tenant_id)
  AND (
    sqlc.arg(search_query)::text = ''
    OR users.username ILIKE '%' || sqlc.arg(search_query)::text || '%'
    OR users.display_name ILIKE '%' || sqlc.arg(search_query)::text || '%'
  );
