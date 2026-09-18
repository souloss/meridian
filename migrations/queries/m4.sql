-- M4 系统分组、依赖图与跨 kind 搜索的持久化查询。
-- 全部查询保留 tenant_id 谓词。

-- 幂等创建一条系统分组（唯一键 tenant_id + slug）。
-- name: CreateSystemGroup :one
INSERT INTO system_groups (tenant_id, id, slug, display_name, description)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(slug), sqlc.arg(display_name), sqlc.narg(description))
RETURNING *;

-- 返回一条系统分组。
-- name: GetSystemGroup :one
SELECT *
FROM system_groups
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 按 slug 返回一条系统分组。
-- name: GetSystemGroupBySlug :one
SELECT *
FROM system_groups
WHERE tenant_id = sqlc.arg(tenant_id)
  AND slug = sqlc.arg(slug);

-- 列出租户内全部系统分组。
-- name: ListSystemGroups :many
SELECT *
FROM system_groups
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY slug, id;

-- 返回一条系统分组的成员服务。
-- name: ListSystemGroupMembers :many
SELECT *
FROM system_group_members
WHERE tenant_id = sqlc.arg(tenant_id)
  AND group_id = sqlc.arg(group_id)
ORDER BY service_id;

-- 替换一条系统分组的成员：先删除再按序重建。
-- name: ReplaceSystemGroupMembers :execrows
WITH deleted AS (
  DELETE FROM system_group_members
  WHERE tenant_id = sqlc.arg(tenant_id)
    AND group_id = sqlc.arg(group_id)
)
SELECT 1;

-- name: InsertSystemGroupMember :execrows
INSERT INTO system_group_members (tenant_id, group_id, service_id)
VALUES (sqlc.arg(tenant_id), sqlc.arg(group_id), sqlc.arg(service_id))
ON CONFLICT (tenant_id, group_id, service_id) DO NOTHING;

-- 递增系统分组版本号。
-- name: BumpSystemGroupRevision :execrows
UPDATE system_groups
SET revision = revision + 1, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision);

-- 返回租户内全部活跃资产条目，供跨 kind 搜索（不绑定特定版本）。
-- name: ListSearchableAssetItems :many
SELECT ai.*
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (sqlc.arg(kind_filter)::text = '' OR ai.kind = sqlc.arg(kind_filter)::text)
ORDER BY ai.key, ai.id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 统计租户内全部活跃资产条目数量，供跨 kind 搜索分页。
-- name: CountSearchableAssetItems :one
SELECT count(*)::bigint
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (sqlc.arg(kind_filter)::text = '' OR ai.kind = sqlc.arg(kind_filter)::text);

-- 返回资产版本清单中的 AI 生成层修订标识，供 hasAiLayer 过滤判定。
-- name: ListVersionAiLayerRevisions :many
SELECT av.id AS asset_version_id, layers.id AS layer_id
FROM asset_versions AS av
JOIN layers ON layers.tenant_id = av.tenant_id AND layers.asset_id = av.asset_id
WHERE av.tenant_id = sqlc.arg(tenant_id)
  AND layers.origin = 'ai_generated'
  AND layers.deleted_at IS NULL;

-- 返回服务所属的仓库，供搜索命中投影 owning repository。
-- name: GetRepositoryByService :one
SELECT repositories.*
FROM repositories
JOIN services ON services.tenant_id = repositories.tenant_id AND services.repository_id = repositories.id
WHERE repositories.tenant_id = sqlc.arg(tenant_id)
  AND services.id = sqlc.arg(service_id)
LIMIT 1;
