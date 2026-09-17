-- M2 gitops 配置导入的持久化查询。
-- 全部查询保留 tenant_id 谓词。

-- 插入一次可应用的配置导入预览。
-- name: CreateConfigImportPreview :one
INSERT INTO config_import_previews (
  tenant_id, id, repository_id, ref_type, ref_name, commit, config_digest, preview, expires_at
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(repository_id), sqlc.arg(ref_type), sqlc.arg(ref_name),
  sqlc.arg(commit), sqlc.arg(config_digest), sqlc.arg(preview)::jsonb, sqlc.arg(expires_at)
)
RETURNING *;

-- 返回一次未过期的配置导入预览，锁定供 apply 串行化。
-- name: GetConfigImportPreview :one
SELECT *
FROM config_import_previews
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND repository_id = sqlc.arg(repository_id)
  AND expires_at > now()
FOR UPDATE;

-- 按预览 id 返回一次配置导入预览（跨仓库校验失败时保持只读）。
-- name: GetConfigImportPreviewByID :one
SELECT *
FROM config_import_previews
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);
