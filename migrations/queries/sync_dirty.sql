-- syncRepository 的 dirty 标记与完成后续任物化查询。
-- dirty 语义对齐 contracts/domain.yaml 的 coalescing：运行中收到重复请求置 dirty，
-- 完成后若 dirty 置位则入队一个使用最新输入的后续任。

-- 运行中的 repo.sync 收到重复请求时置 dirty。
-- name: MarkSyncJobDirty :execrows
UPDATE jobs
SET dirty = true, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND dedupe_key = sqlc.arg(dedupe_key)
  AND status = 'running'
  AND active_generation = sqlc.arg(active_generation);

-- 返回一条 repo.sync 任务，供完成后判断是否需要后续任。
-- name: GetSyncJobForSuccessor :one
SELECT id, status, dirty, scope_id, ref_type, ref_name, active_generation
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND type = 'repo.sync';

-- 清空一条已成功任务的 dirty 标记，保证后续任务只入队一次。
-- name: ClearSyncJobDirty :execrows
UPDATE jobs
SET dirty = false, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND dirty = true
  AND status = 'succeeded';
