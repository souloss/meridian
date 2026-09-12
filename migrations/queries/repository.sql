-- 返回有效仓库数量和租户固定的仓库配额。
-- 配额读取租户快照，不读取可变的平台默认值。
-- name: CountRepositories :one
SELECT
  (SELECT count(*)::bigint
   FROM repositories
   WHERE tenant_id = sqlc.arg(tenant_id)
     AND deleted_at IS NULL) AS current_count,
  COALESCE((SELECT (quota ->> 'maxRepositories')::bigint
            FROM tenants
            WHERE id = sqlc.arg(tenant_id)), 0)::bigint AS limit_count;

-- 使用租户配额行锁串行化仓库创建。
-- 适配器在计数和插入期间持有该锁，避免并发创建超过租户配额。
-- name: LockRepositoryQuota :one
SELECT COALESCE((quota ->> 'maxRepositories')::bigint, 0)::bigint AS limit_count
FROM tenants
WHERE id = sqlc.arg(tenant_id)
FOR UPDATE;

-- 将一个租户可见的凭据 UUID 解析到唯一的所属表。
-- 租户凭据使用与凭据列表相同的可见性条件；平台凭据可被有效成员选择，但仍由平台管理员管理。
-- name: ResolveRepositoryCredential :one
SELECT c.id AS resolved_id, false AS is_global
FROM credentials AS c
WHERE c.tenant_id = sqlc.arg(tenant_id)
  AND c.id = sqlc.arg(credential_id)
  AND (
    c.created_by = sqlc.arg(user_id)
    OR c.shared_scope = 'tenant'
    OR (
      c.shared_scope = 'team'
      AND EXISTS (
        SELECT 1
        FROM team_members AS members
        JOIN credential_team_shares AS shares
          ON shares.tenant_id = members.tenant_id
         AND shares.team_id = members.team_id
         AND shares.credential_id = c.id
        WHERE members.tenant_id = c.tenant_id
          AND members.user_id = sqlc.arg(user_id)
      )
    )
  )
UNION ALL
SELECT global_credentials.id AS resolved_id, true AS is_global
FROM global_credentials
WHERE global_credentials.id = sqlc.arg(credential_id)
LIMIT 1;

-- 按规范化 URL 和 UUID 的确定顺序返回有效仓库。
-- 即使搜索字符串为空，查询及全部谓词仍保留租户边界。
-- name: ListRepositories :many
SELECT *
FROM repositories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND deleted_at IS NULL
  AND (
    sqlc.arg(search_query)::text = ''
    OR url ILIKE '%' || sqlc.arg(search_query)::text || '%'
    OR canonical_url ILIKE '%' || sqlc.arg(search_query)::text || '%'
  )
ORDER BY canonical_url, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回匹配一次租户搜索的有效仓库数量。
-- name: CountListedRepositories :one
SELECT count(*)::bigint
FROM repositories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND deleted_at IS NULL
  AND (
    sqlc.arg(search_query)::text = ''
    OR url ILIKE '%' || sqlc.arg(search_query)::text || '%'
    OR canonical_url ILIKE '%' || sqlc.arg(search_query)::text || '%'
  );

-- 返回一条有效仓库；软删除记录按设计视为不存在。
-- name: GetRepository :one
SELECT *
FROM repositories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- 持久化仓库配置，并初始化空的健康状态摘要。
-- 仓库 URL 字段不含凭据，凭据只通过 UUID 外键引用。
-- name: CreateRepository :one
INSERT INTO repositories (
  tenant_id, id, url, canonical_url, credential_id, global_credential_id,
  default_branch, branch_policy, fetch_config, sync_cron, note
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(url), sqlc.arg(canonical_url),
  sqlc.narg(credential_id), sqlc.narg(global_credential_id), sqlc.arg(default_branch),
  sqlc.arg(branch_policy), sqlc.arg(fetch_config), sqlc.narg(sync_cron), sqlc.narg(note)
)
RETURNING *;

-- 有条件地更新明确提供的仓库字段并递增版本号。
-- 字段 set 标志保留字段省略和显式 JSON null 之间的区别。
-- name: UpdateRepository :one
UPDATE repositories
SET
  credential_id = CASE
    WHEN sqlc.arg(set_credential_id)::boolean THEN sqlc.narg(credential_id)
    ELSE credential_id
  END,
  global_credential_id = CASE
    WHEN sqlc.arg(set_global_credential_id)::boolean THEN sqlc.narg(global_credential_id)
    ELSE global_credential_id
  END,
  default_branch = COALESCE(sqlc.narg(default_branch), default_branch),
  branch_policy = COALESCE(sqlc.narg(branch_policy), branch_policy),
  fetch_config = COALESCE(sqlc.narg(fetch_config), fetch_config),
  sync_cron = CASE
    WHEN sqlc.arg(set_sync_cron)::boolean THEN sqlc.narg(sync_cron)
    ELSE sync_cron
  END,
  note = CASE
    WHEN sqlc.arg(set_note)::boolean THEN sqlc.narg(note)
    ELSE note
  END,
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 软删除仓库；URL 和分支能否复用由策略决定。
-- 历史任务和审计记录在更新后仍保持租户范围。
-- name: DeleteRepository :execrows
UPDATE repositories
SET deleted_at = sqlc.arg(deleted_at), revision = revision + 1, updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND revision = sqlc.arg(expected_revision);
