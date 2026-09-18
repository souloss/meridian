-- M3 差异、分享与待办的持久化查询。
-- 全部查询保留 tenant_id 谓词。

-- 返回一条上传，供文件选择器解析。
-- name: GetUpload :one
SELECT *
FROM uploads
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND expires_at > now();

-- 创建一条上传。
-- name: CreateUpload :one
INSERT INTO uploads (tenant_id, id, blob_digest, kind, content_type, size_bytes, expires_at, created_by)
VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(blob_digest), sqlc.arg(kind),
  sqlc.arg(content_type), sqlc.arg(size_bytes), sqlc.arg(expires_at), sqlc.narg(created_by)
)
RETURNING *;

-- 创建一条差异快照，冻结解析后的选择器与产物。
-- name: CreateDiffSnapshot :one
INSERT INTO diff_snapshots (
  tenant_id, id, left_selector, right_selector, left_artifact_ref, right_artifact_ref,
  rule_set_id, result_ref, summary, created_by
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(left_selector)::jsonb, sqlc.arg(right_selector)::jsonb,
  sqlc.arg(left_artifact_ref), sqlc.arg(right_artifact_ref), sqlc.narg(rule_set_id),
  sqlc.arg(result_ref), sqlc.arg(summary)::jsonb, sqlc.arg(created_by)
)
RETURNING *;

-- 返回一条差异快照，供分享描述符冻结。
-- name: GetDiffSnapshot :one
SELECT *
FROM diff_snapshots
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 创建一条分享链接，令牌哈希全局唯一。
-- name: CreateShareLink :one
INSERT INTO share_links (
  tenant_id, id, token_hash, creator_id, resource_type, resource_id, descriptor,
  view_id, options, artifact_allowlist, expires_at
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(token_hash), sqlc.arg(creator_id),
  sqlc.arg(resource_type), sqlc.narg(resource_id), sqlc.arg(descriptor)::jsonb,
  sqlc.narg(view_id), sqlc.arg(options)::jsonb, sqlc.arg(artifact_allowlist)::jsonb,
  sqlc.arg(expires_at)
)
RETURNING *;

-- 按令牌哈希返回一条未撤销且未过期的分享链接。
-- name: GetShareLinkByTokenHash :one
SELECT *
FROM share_links
WHERE token_hash = sqlc.arg(token_hash)
  AND revoked_at IS NULL
  AND expires_at > now();

-- 幂等创建一条破坏性变更待办（唯一键 asset_version_id + service_id）。
-- name: CreateBreakingTodoIfAbsent :one
INSERT INTO breaking_todos (tenant_id, id, asset_version_id, service_id)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(asset_version_id), sqlc.arg(service_id))
ON CONFLICT (tenant_id, asset_version_id, service_id) DO NOTHING
RETURNING *;

-- 列出破坏性变更待办，按状态过滤并分页。
-- name: ListBreakingTodos :many
SELECT *
FROM breaking_todos
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(status_filter)::text = '' OR status = sqlc.arg(status_filter))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 统计破坏性变更待办总数。
-- name: CountBreakingTodos :one
SELECT count(*)::bigint
FROM breaking_todos
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(status_filter)::text = '' OR status = sqlc.arg(status_filter));

-- 确认一条待办（任意授权服务成员确认即关闭）。
-- name: AckBreakingTodo :one
UPDATE breaking_todos
SET status = 'acked',
    acked_by = sqlc.arg(acked_by),
    acked_at = sqlc.arg(acked_at),
    comment = sqlc.narg(comment),
    updated_at = sqlc.arg(acked_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status = 'open'
RETURNING *;

-- 返回资产下按推送身份（kind + 资产名模板）匹配的推送层，供重复推送复用同一 overlay 层。
-- name: GetSourceLayerByPushKey :one
SELECT layers.*
FROM layers
JOIN source_specs AS ss ON ss.tenant_id = layers.tenant_id AND ss.id = layers.source_spec_id
WHERE layers.tenant_id = sqlc.arg(tenant_id)
  AND layers.asset_id = sqlc.arg(asset_id)
  AND ss.origin = 'third_party'
  AND ss.mode = 'push'
  AND layers.deleted_at IS NULL
  AND ss.deleted_at IS NULL
LIMIT 1;
