-- M2 层编辑、overlay 修订、回滚与排序的持久化查询。
-- 全部查询保留 tenant_id 谓词。

-- 返回一条活跃层。
-- name: GetLayer :one
SELECT *
FROM layers
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- 返回一个资产下全部活跃层，按 ord 后 id 排序。
-- name: ListLayersForAsset :many
SELECT *
FROM layers
WHERE tenant_id = sqlc.arg(tenant_id)
  AND asset_id = sqlc.arg(asset_id)
  AND deleted_at IS NULL
ORDER BY ord, id;

-- 锁定一条活跃层，供排序与回滚前校验 revision。
-- name: GetLayerForUpdate :one
SELECT *
FROM layers
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
FOR UPDATE;

-- 更新一条层的排序号并递增 revision。
-- name: UpdateLayerOrd :one
UPDATE layers
SET ord = sqlc.arg(ord), revision = revision + 1, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 返回一条层修订，供回滚与溯源读取。
-- name: GetLayerRevision :one
SELECT *
FROM layer_revisions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 列出某层某作用域内的全部修订，按创建时间倒序。
-- name: ListLayerRevisions :many
SELECT *
FROM layer_revisions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND layer_id = sqlc.arg(layer_id)
  AND scope_type = sqlc.arg(scope_type)
  AND scope_key = sqlc.arg(scope_key)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回一个资产下全部层的头指针（用于合并选择有效修订）。
-- name: ListLayerHeadsForAsset :many
SELECT layer_heads.*
FROM layer_heads
JOIN layers AS layer ON layer.id = layer_heads.layer_id AND layer.tenant_id = layer_heads.tenant_id
WHERE layer_heads.tenant_id = sqlc.arg(tenant_id)
  AND layer.asset_id = sqlc.arg(asset_id)
  AND layer.deleted_at IS NULL;

-- 更新或创建（upsert）一个层头的最新/生效/候选修订指针并递增代次。
-- 首次提交插入 generation=1；后续提交在既有行上递增 generation。
-- name: UpdateLayerHeadPointers :one
INSERT INTO layer_heads (tenant_id, layer_id, scope_type, scope_key, latest_revision_id, effective_revision_id, candidate_revision_id, generation)
VALUES (sqlc.arg(tenant_id), sqlc.arg(layer_id), sqlc.arg(scope_type), sqlc.arg(scope_key), sqlc.narg(latest_revision_id), sqlc.narg(effective_revision_id), sqlc.narg(candidate_revision_id), 1)
ON CONFLICT (tenant_id, layer_id, scope_type, scope_key) DO UPDATE SET
  latest_revision_id = EXCLUDED.latest_revision_id,
  effective_revision_id = EXCLUDED.effective_revision_id,
  candidate_revision_id = EXCLUDED.candidate_revision_id,
  generation = layer_heads.generation + 1,
  updated_at = now()
RETURNING *;

-- 按 id 返回一个资产版本，供溯源读取层清单与基准。
-- name: GetAssetVersionForProvenance :one
SELECT *
FROM asset_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);
