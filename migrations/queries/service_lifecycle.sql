-- M1 服务生命周期、公开读取与软删除的持久化查询。
-- 全部查询保留 tenant_id 谓词；公开读取按 tenant_slug + service_slug 跨表解析。

-- 锁定一条活跃服务行，供条件更新与删除前校验 revision。
-- name: GetServiceForUpdate :one
SELECT *
FROM services
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
FOR UPDATE;

-- 按 tenant_slug + service_slug 返回一条活跃服务，供匿名公开读取解析。
-- name: GetPublicServiceBySlug :one
SELECT s.*
FROM services AS s
JOIN tenants AS t ON t.id = s.tenant_id
WHERE t.slug = sqlc.arg(tenant_slug)
  AND s.slug = sqlc.arg(service_slug)
  AND s.deleted_at IS NULL;

-- 有条件地更新服务元数据并递增 revision。
-- description 使用 set 标志保留「未提供」与「显式置空」的区别。
-- name: UpdateService :one
UPDATE services
SET
  display_name = COALESCE(sqlc.narg(display_name), display_name),
  description = CASE WHEN sqlc.arg(set_description)::boolean THEN sqlc.narg(description) ELSE description END,
  visibility = COALESCE(sqlc.narg(visibility), visibility),
  lifecycle = COALESCE(sqlc.narg(lifecycle), lifecycle),
  revision = revision + 1,
  updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 软删除一条服务并递增 revision。
-- name: DeleteService :one
UPDATE services
SET deleted_at = now(), updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 软删除一个服务下全部活跃源配置。
-- name: DeleteServiceSourceSpecs :execrows
UPDATE source_specs
SET deleted_at = now(), updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id)
  AND deleted_at IS NULL;

-- 软删除一个服务下全部活跃资产。
-- name: DeleteServiceAssets :execrows
UPDATE assets
SET deleted_at = now(), updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id)
  AND deleted_at IS NULL;

-- 软删除一个服务下全部活跃层。
-- name: DeleteServiceLayers :execrows
UPDATE layers AS layer
SET deleted_at = now(), updated_at = now()
WHERE layer.tenant_id = sqlc.arg(tenant_id)
  AND layer.asset_id IN (
    SELECT asset.id FROM assets AS asset
    WHERE asset.tenant_id = sqlc.arg(tenant_id) AND asset.service_id = sqlc.arg(service_id)
  )
  AND layer.deleted_at IS NULL;

-- 将一个服务下全部活跃源绑定标记为 stale（保留 resolved_path）。
-- name: StaleServiceBindings :execrows
UPDATE source_bindings AS binding
SET state = 'stale', updated_at = now()
WHERE binding.tenant_id = sqlc.arg(tenant_id)
  AND binding.source_spec_id IN (
    SELECT spec.id FROM source_specs AS spec
    WHERE spec.tenant_id = sqlc.arg(tenant_id) AND spec.service_id = sqlc.arg(service_id)
  )
  AND binding.state = 'active';

-- 停用一个服务下全部引用轨迹并递增期望代次。
-- name: DeactivateServiceTracks :execrows
UPDATE asset_ref_tracks AS track
SET active = false, desired_generation = desired_generation + 1, updated_at = now()
WHERE track.tenant_id = sqlc.arg(tenant_id)
  AND track.asset_id IN (
    SELECT asset.id FROM assets AS asset
    WHERE asset.tenant_id = sqlc.arg(tenant_id) AND asset.service_id = sqlc.arg(service_id)
  );

-- 移除一个服务对应的最近访问记录。
-- name: DeleteRecentServicesForService :execrows
DELETE FROM recent_services
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id);

-- 锁定一个服务及其子资源作用域下仍待执行的任务，供删除事务统一取消。
-- name: LockPendingServiceJobs :many
SELECT job.id, job.river_job_id
FROM jobs AS job
WHERE job.tenant_id = sqlc.arg(tenant_id)
  AND job.status IN ('pending', 'running')
  AND (
    (job.scope_type = 'service' AND job.scope_id = sqlc.arg(service_id)::uuid)
    OR (job.scope_type = 'source' AND job.scope_id IN (
      SELECT spec.id FROM source_specs AS spec
      WHERE spec.tenant_id = sqlc.arg(tenant_id) AND spec.service_id = sqlc.arg(service_id)))
    OR (job.scope_type = 'asset' AND job.scope_id IN (
      SELECT asset.id FROM assets AS asset
      WHERE asset.tenant_id = sqlc.arg(tenant_id) AND asset.service_id = sqlc.arg(service_id)))
    OR (job.scope_type = 'track' AND job.scope_id IN (
      SELECT track.id FROM asset_ref_tracks AS track
      JOIN assets AS asset ON asset.id = track.asset_id AND asset.tenant_id = track.tenant_id
      WHERE asset.tenant_id = sqlc.arg(tenant_id) AND asset.service_id = sqlc.arg(service_id)))
    OR (job.scope_type = 'version' AND job.scope_id IN (
      SELECT version.id FROM asset_versions AS version
      JOIN assets AS asset ON asset.id = version.asset_id AND asset.tenant_id = version.tenant_id
      WHERE asset.tenant_id = sqlc.arg(tenant_id) AND asset.service_id = sqlc.arg(service_id)))
  )
FOR UPDATE;

-- 将一条仍待执行的任务转为 cancelled 终态。
-- name: CancelServiceJob :execrows
UPDATE jobs
SET status = 'cancelled', finished_at = now(), next_attempt_at = NULL, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status IN ('pending', 'running');
