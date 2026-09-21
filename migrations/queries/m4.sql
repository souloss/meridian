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

-- 批量返回多个分组的成员，供列表场景去 N+1。
-- name: ListSystemGroupMembersForGroups :many
SELECT *
FROM system_group_members
WHERE tenant_id = sqlc.arg(tenant_id)
  AND group_id = ANY(sqlc.arg(group_ids)::uuid[])
ORDER BY group_id, service_id;

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
-- 过滤子句统一口径：数组过滤用空数组哨兵跳过，布尔过滤用 NULL 哨兵跳过三态。
-- name: ListSearchableAssetItems :many
SELECT ai.*
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR ai.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (cardinality(sqlc.arg(service_ids)::uuid[]) = 0 OR ai.service_id = ANY(sqlc.arg(service_ids)::uuid[]))
  AND (cardinality(sqlc.arg(repository_ids)::uuid[]) = 0 OR s.repository_id = ANY(sqlc.arg(repository_ids)::uuid[]))
  AND (cardinality(sqlc.arg(item_types)::text[]) = 0 OR ai.item_type = ANY(sqlc.arg(item_types)::text[]))
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR s.language = ANY(sqlc.arg(languages)::text[]))
  AND (cardinality(sqlc.arg(lifecycles)::text[]) = 0 OR s.lifecycle = ANY(sqlc.arg(lifecycles)::text[]))
  AND (sqlc.narg(has_ai_layer)::boolean IS NULL
       OR (sqlc.narg(has_ai_layer)::boolean AND ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))
       OR (NOT sqlc.narg(has_ai_layer)::boolean AND ai.asset_id NOT IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL)))
  AND (sqlc.narg(has_breaking_changes)::boolean IS NULL
       OR (sqlc.narg(has_breaking_changes)::boolean AND EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id))
       OR (NOT sqlc.narg(has_breaking_changes)::boolean AND NOT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)))
ORDER BY ai.key, ai.id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 统计租户内全部活跃资产条目数量，供跨 kind 搜索分页（过滤口径与上一致）。
-- name: CountSearchableAssetItems :one
SELECT count(*)::bigint
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR ai.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (cardinality(sqlc.arg(service_ids)::uuid[]) = 0 OR ai.service_id = ANY(sqlc.arg(service_ids)::uuid[]))
  AND (cardinality(sqlc.arg(repository_ids)::uuid[]) = 0 OR s.repository_id = ANY(sqlc.arg(repository_ids)::uuid[]))
  AND (cardinality(sqlc.arg(item_types)::text[]) = 0 OR ai.item_type = ANY(sqlc.arg(item_types)::text[]))
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR s.language = ANY(sqlc.arg(languages)::text[]))
  AND (cardinality(sqlc.arg(lifecycles)::text[]) = 0 OR s.lifecycle = ANY(sqlc.arg(lifecycles)::text[]))
  AND (sqlc.narg(has_ai_layer)::boolean IS NULL
       OR (sqlc.narg(has_ai_layer)::boolean AND ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))
       OR (NOT sqlc.narg(has_ai_layer)::boolean AND ai.asset_id NOT IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL)))
  AND (sqlc.narg(has_breaking_changes)::boolean IS NULL
       OR (sqlc.narg(has_breaking_changes)::boolean AND EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id))
       OR (NOT sqlc.narg(has_breaking_changes)::boolean AND NOT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)));

-- 以下 facet 查询共享同一过滤口径，各自剔除自身维度的过滤后按值分组计数。
-- name: SearchFacetKinds :many
SELECT ai.kind AS value, count(*)::bigint AS count
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(service_ids)::uuid[]) = 0 OR ai.service_id = ANY(sqlc.arg(service_ids)::uuid[]))
  AND (cardinality(sqlc.arg(repository_ids)::uuid[]) = 0 OR s.repository_id = ANY(sqlc.arg(repository_ids)::uuid[]))
  AND (cardinality(sqlc.arg(item_types)::text[]) = 0 OR ai.item_type = ANY(sqlc.arg(item_types)::text[]))
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR s.language = ANY(sqlc.arg(languages)::text[]))
  AND (cardinality(sqlc.arg(lifecycles)::text[]) = 0 OR s.lifecycle = ANY(sqlc.arg(lifecycles)::text[]))
  AND (sqlc.narg(has_ai_layer)::boolean IS NULL
       OR (sqlc.narg(has_ai_layer)::boolean AND ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))
       OR (NOT sqlc.narg(has_ai_layer)::boolean AND ai.asset_id NOT IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL)))
  AND (sqlc.narg(has_breaking_changes)::boolean IS NULL
       OR (sqlc.narg(has_breaking_changes)::boolean AND EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id))
       OR (NOT sqlc.narg(has_breaking_changes)::boolean AND NOT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)))
GROUP BY ai.kind
ORDER BY count DESC, value;

-- name: SearchFacetLifecycles :many
SELECT s.lifecycle AS value, count(*)::bigint AS count
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR ai.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (cardinality(sqlc.arg(service_ids)::uuid[]) = 0 OR ai.service_id = ANY(sqlc.arg(service_ids)::uuid[]))
  AND (cardinality(sqlc.arg(repository_ids)::uuid[]) = 0 OR s.repository_id = ANY(sqlc.arg(repository_ids)::uuid[]))
  AND (cardinality(sqlc.arg(item_types)::text[]) = 0 OR ai.item_type = ANY(sqlc.arg(item_types)::text[]))
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR s.language = ANY(sqlc.arg(languages)::text[]))
  AND (sqlc.narg(has_ai_layer)::boolean IS NULL
       OR (sqlc.narg(has_ai_layer)::boolean AND ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))
       OR (NOT sqlc.narg(has_ai_layer)::boolean AND ai.asset_id NOT IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL)))
  AND (sqlc.narg(has_breaking_changes)::boolean IS NULL
       OR (sqlc.narg(has_breaking_changes)::boolean AND EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id))
       OR (NOT sqlc.narg(has_breaking_changes)::boolean AND NOT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)))
GROUP BY s.lifecycle
ORDER BY count DESC, value;

-- name: SearchFacetLanguages :many
SELECT s.language AS value, count(*)::bigint AS count
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND s.language IS NOT NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR ai.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (cardinality(sqlc.arg(service_ids)::uuid[]) = 0 OR ai.service_id = ANY(sqlc.arg(service_ids)::uuid[]))
  AND (cardinality(sqlc.arg(repository_ids)::uuid[]) = 0 OR s.repository_id = ANY(sqlc.arg(repository_ids)::uuid[]))
  AND (cardinality(sqlc.arg(item_types)::text[]) = 0 OR ai.item_type = ANY(sqlc.arg(item_types)::text[]))
  AND (cardinality(sqlc.arg(lifecycles)::text[]) = 0 OR s.lifecycle = ANY(sqlc.arg(lifecycles)::text[]))
  AND (sqlc.narg(has_ai_layer)::boolean IS NULL
       OR (sqlc.narg(has_ai_layer)::boolean AND ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))
       OR (NOT sqlc.narg(has_ai_layer)::boolean AND ai.asset_id NOT IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL)))
  AND (sqlc.narg(has_breaking_changes)::boolean IS NULL
       OR (sqlc.narg(has_breaking_changes)::boolean AND EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id))
       OR (NOT sqlc.narg(has_breaking_changes)::boolean AND NOT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)))
GROUP BY s.language
ORDER BY count DESC, value;

-- name: SearchFacetItemTypes :many
SELECT ai.item_type AS value, count(*)::bigint AS count
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR ai.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (cardinality(sqlc.arg(service_ids)::uuid[]) = 0 OR ai.service_id = ANY(sqlc.arg(service_ids)::uuid[]))
  AND (cardinality(sqlc.arg(repository_ids)::uuid[]) = 0 OR s.repository_id = ANY(sqlc.arg(repository_ids)::uuid[]))
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR s.language = ANY(sqlc.arg(languages)::text[]))
  AND (cardinality(sqlc.arg(lifecycles)::text[]) = 0 OR s.lifecycle = ANY(sqlc.arg(lifecycles)::text[]))
  AND (sqlc.narg(has_ai_layer)::boolean IS NULL
       OR (sqlc.narg(has_ai_layer)::boolean AND ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))
       OR (NOT sqlc.narg(has_ai_layer)::boolean AND ai.asset_id NOT IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL)))
  AND (sqlc.narg(has_breaking_changes)::boolean IS NULL
       OR (sqlc.narg(has_breaking_changes)::boolean AND EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id))
       OR (NOT sqlc.narg(has_breaking_changes)::boolean AND NOT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)))
GROUP BY ai.item_type
ORDER BY count DESC, value;

-- name: SearchFacetRepositories :many
SELECT s.repository_id::text AS value, count(*)::bigint AS count
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR ai.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (cardinality(sqlc.arg(service_ids)::uuid[]) = 0 OR ai.service_id = ANY(sqlc.arg(service_ids)::uuid[]))
  AND (cardinality(sqlc.arg(item_types)::text[]) = 0 OR ai.item_type = ANY(sqlc.arg(item_types)::text[]))
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR s.language = ANY(sqlc.arg(languages)::text[]))
  AND (cardinality(sqlc.arg(lifecycles)::text[]) = 0 OR s.lifecycle = ANY(sqlc.arg(lifecycles)::text[]))
  AND (sqlc.narg(has_ai_layer)::boolean IS NULL
       OR (sqlc.narg(has_ai_layer)::boolean AND ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))
       OR (NOT sqlc.narg(has_ai_layer)::boolean AND ai.asset_id NOT IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL)))
  AND (sqlc.narg(has_breaking_changes)::boolean IS NULL
       OR (sqlc.narg(has_breaking_changes)::boolean AND EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id))
       OR (NOT sqlc.narg(has_breaking_changes)::boolean AND NOT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)))
GROUP BY s.repository_id
ORDER BY count DESC, value;

-- name: SearchFacetGroups :many
SELECT sg.group_id::text AS value, count(*)::bigint AS count
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
JOIN system_group_members AS sg ON sg.tenant_id = ai.tenant_id AND sg.service_id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR ai.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (cardinality(sqlc.arg(repository_ids)::uuid[]) = 0 OR s.repository_id = ANY(sqlc.arg(repository_ids)::uuid[]))
  AND (cardinality(sqlc.arg(item_types)::text[]) = 0 OR ai.item_type = ANY(sqlc.arg(item_types)::text[]))
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR s.language = ANY(sqlc.arg(languages)::text[]))
  AND (cardinality(sqlc.arg(lifecycles)::text[]) = 0 OR s.lifecycle = ANY(sqlc.arg(lifecycles)::text[]))
  AND (sqlc.narg(has_ai_layer)::boolean IS NULL
       OR (sqlc.narg(has_ai_layer)::boolean AND ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))
       OR (NOT sqlc.narg(has_ai_layer)::boolean AND ai.asset_id NOT IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL)))
  AND (sqlc.narg(has_breaking_changes)::boolean IS NULL
       OR (sqlc.narg(has_breaking_changes)::boolean AND EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id))
       OR (NOT sqlc.narg(has_breaking_changes)::boolean AND NOT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)))
GROUP BY sg.group_id
ORDER BY count DESC, value;

-- name: SearchFacetHasAiLayer :many
SELECT (ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))::text AS value,
       count(*)::bigint AS count
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR ai.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (cardinality(sqlc.arg(service_ids)::uuid[]) = 0 OR ai.service_id = ANY(sqlc.arg(service_ids)::uuid[]))
  AND (cardinality(sqlc.arg(repository_ids)::uuid[]) = 0 OR s.repository_id = ANY(sqlc.arg(repository_ids)::uuid[]))
  AND (cardinality(sqlc.arg(item_types)::text[]) = 0 OR ai.item_type = ANY(sqlc.arg(item_types)::text[]))
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR s.language = ANY(sqlc.arg(languages)::text[]))
  AND (cardinality(sqlc.arg(lifecycles)::text[]) = 0 OR s.lifecycle = ANY(sqlc.arg(lifecycles)::text[]))
  AND (sqlc.narg(has_breaking_changes)::boolean IS NULL
       OR (sqlc.narg(has_breaking_changes)::boolean AND EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id))
       OR (NOT sqlc.narg(has_breaking_changes)::boolean AND NOT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)))
GROUP BY 1
ORDER BY count DESC, value;

-- name: SearchFacetHasBreakingChanges :many
SELECT EXISTS (SELECT 1 FROM breaking_todos AS bt WHERE bt.tenant_id = ai.tenant_id AND bt.asset_version_id = ai.asset_version_id)::text AS value,
       count(*)::bigint AS count
FROM asset_items AS ai
JOIN asset_versions AS av ON av.tenant_id = ai.tenant_id AND av.id = ai.asset_version_id
JOIN assets AS asset ON asset.tenant_id = ai.tenant_id AND asset.id = ai.asset_id
JOIN services AS s ON s.tenant_id = ai.tenant_id AND s.id = ai.service_id
WHERE ai.tenant_id = sqlc.arg(tenant_id)
  AND asset.deleted_at IS NULL
  AND ai.search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  AND (cardinality(sqlc.arg(kinds)::text[]) = 0 OR ai.kind = ANY(sqlc.arg(kinds)::text[]))
  AND (cardinality(sqlc.arg(service_ids)::uuid[]) = 0 OR ai.service_id = ANY(sqlc.arg(service_ids)::uuid[]))
  AND (cardinality(sqlc.arg(repository_ids)::uuid[]) = 0 OR s.repository_id = ANY(sqlc.arg(repository_ids)::uuid[]))
  AND (cardinality(sqlc.arg(item_types)::text[]) = 0 OR ai.item_type = ANY(sqlc.arg(item_types)::text[]))
  AND (cardinality(sqlc.arg(languages)::text[]) = 0 OR s.language = ANY(sqlc.arg(languages)::text[]))
  AND (cardinality(sqlc.arg(lifecycles)::text[]) = 0 OR s.lifecycle = ANY(sqlc.arg(lifecycles)::text[]))
  AND (sqlc.narg(has_ai_layer)::boolean IS NULL
       OR (sqlc.narg(has_ai_layer)::boolean AND ai.asset_id IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL))
       OR (NOT sqlc.narg(has_ai_layer)::boolean AND ai.asset_id NOT IN (SELECT l.asset_id FROM layers AS l WHERE l.tenant_id = ai.tenant_id AND l.origin = 'ai_generated' AND l.deleted_at IS NULL)))
GROUP BY 1
ORDER BY count DESC, value;

-- 返回服务所属的仓库，供搜索命中投影 owning repository。
-- name: GetRepositoryByService :one
SELECT repositories.*
FROM repositories
JOIN services ON services.tenant_id = repositories.tenant_id AND services.repository_id = repositories.id
WHERE repositories.tenant_id = sqlc.arg(tenant_id)
  AND services.id = sqlc.arg(service_id)
LIMIT 1;

-- 批量返回租户内指定 id 的活跃服务，供搜索命中与服务校验去 N+1。
-- name: ListServicesByIDs :many
SELECT *
FROM services
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = ANY(sqlc.arg(ids)::uuid[])
  AND deleted_at IS NULL
ORDER BY id;

-- 批量返回拥有指定服务的仓库，供搜索命中投影 owning repository 去 N+1。
-- name: ListRepositoriesByServices :many
SELECT DISTINCT ON (services.id)
       services.id AS service_id, repositories.*
FROM repositories
JOIN services ON services.tenant_id = repositories.tenant_id AND services.repository_id = repositories.id
WHERE repositories.tenant_id = sqlc.arg(tenant_id)
  AND services.id = ANY(sqlc.arg(service_ids)::uuid[])
ORDER BY services.id;
