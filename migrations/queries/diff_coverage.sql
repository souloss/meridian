-- Diff 规则集、快照、分享链接与上传的补齐查询。

-- 返回租户内全部差异规则集，按 kind + name 排序。
-- name: ListDiffRuleSets :many
SELECT *
FROM diff_rule_sets
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY kind, name, version;

-- 返回一条差异规则集。
-- name: GetDiffRuleSet :one
SELECT *
FROM diff_rule_sets
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 创建一条差异规则集。
-- name: CreateDiffRuleSet :one
INSERT INTO diff_rule_sets (tenant_id, id, kind, name, version, rules, enabled)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(version), sqlc.arg(rules)::jsonb, sqlc.arg(enabled))
RETURNING *;

-- 在 If-Match 下更新一条差异规则集并递增 revision。
-- name: UpdateDiffRuleSet :one
UPDATE diff_rule_sets
SET
  name = COALESCE(sqlc.narg(name), name),
  rules = COALESCE(sqlc.narg(rules), rules),
  enabled = COALESCE(sqlc.narg(enabled), enabled),
  revision = revision + 1,
  updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 删除一条差异规则集。
-- name: DeleteDiffRuleSet :execrows
DELETE FROM diff_rule_sets
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision);

-- 返回租户内全部差异快照分页。
-- name: ListDiffSnapshots :many
SELECT *
FROM diff_snapshots
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回租户内差异快照总数。
-- name: CountDiffSnapshots :one
SELECT count(*)::bigint
FROM diff_snapshots
WHERE tenant_id = sqlc.arg(tenant_id);

-- 删除一条差异快照。
-- name: DeleteDiffSnapshot :execrows
DELETE FROM diff_snapshots
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 返回租户内全部分享链接分页。
-- name: ListShareLinks :many
SELECT *
FROM share_links
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回租户内分享链接总数。
-- name: CountShareLinks :one
SELECT count(*)::bigint
FROM share_links
WHERE tenant_id = sqlc.arg(tenant_id);

-- 幂等撤销一条分享链接。
-- name: RevokeShareLink :execrows
UPDATE share_links
SET revoked_at = COALESCE(revoked_at, now())
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);
