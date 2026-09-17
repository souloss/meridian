-- M3 AI 评审与发布的持久化查询。
-- 全部查询保留 tenant_id 谓词；涉及层头（评审事务）的查询带 FOR UPDATE 锁。

-- 幂等保存一次 AI 资产生成结果快照。
-- name: UpsertAiGenerationResult :one
INSERT INTO ai_generation_results (
  tenant_id, id, job_id, stage, status, error_code, content_ref, content_hash, content_type, manifest, revision_id
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(job_id), sqlc.arg(stage), sqlc.arg(status),
  sqlc.arg(error_code), sqlc.narg(content_ref), sqlc.narg(content_hash), sqlc.narg(content_type),
  sqlc.arg(manifest)::jsonb, sqlc.narg(revision_id)
)
ON CONFLICT (tenant_id, job_id) DO UPDATE SET
  stage = EXCLUDED.stage,
  status = EXCLUDED.status,
  error_code = EXCLUDED.error_code,
  content_ref = EXCLUDED.content_ref,
  content_hash = EXCLUDED.content_hash,
  content_type = EXCLUDED.content_type,
  manifest = EXCLUDED.manifest,
  revision_id = EXCLUDED.revision_id,
  created_at = now()
RETURNING *;

-- 返回一次 AI 资产生成结果快照，供终态读取与重放。
-- name: GetAiGenerationResult :one
SELECT *
FROM ai_generation_results
WHERE tenant_id = sqlc.arg(tenant_id)
  AND job_id = sqlc.arg(job_id);

-- 记录一条资产 AI 生成任务。
-- name: CreateAiGenerationJob :one
INSERT INTO jobs (
  tenant_id, id, type, scope_type, scope_id, ref_type, ref_name, trigger, input,
  status, max_attempts, dedupe_key, active_generation, replay_safe
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), 'asset.ai_generate', 'asset', sqlc.arg(asset_id),
  sqlc.narg(ref_type), sqlc.narg(ref_name), 'manual', sqlc.arg(job_input)::jsonb,
  'pending', 3, sqlc.arg(dedupe_key), sqlc.arg(active_generation), true
)
ON CONFLICT (tenant_id, dedupe_key, active_generation) DO NOTHING
RETURNING *;

-- 返回待审核修订及其层的行，供评审事务使用。
-- name: GetLayerRevisionForReview :one
SELECT r.*, l.asset_id, l.role, l.origin
FROM layer_revisions AS r
JOIN layers AS l ON l.tenant_id = r.tenant_id AND l.id = r.layer_id
WHERE r.tenant_id = sqlc.arg(tenant_id)
  AND r.id = sqlc.arg(id)
  AND l.deleted_at IS NULL;

-- 列出某资产某作用域内启用层的头指针，供评审前置校验（无候选）。
-- name: ListEnabledLayerHeadsForAssetScope :many
SELECT lh.*
FROM layer_heads AS lh
JOIN layers AS l ON l.tenant_id = lh.tenant_id AND l.id = lh.layer_id
WHERE lh.tenant_id = sqlc.arg(tenant_id)
  AND l.asset_id = sqlc.arg(asset_id)
  AND lh.scope_type = sqlc.arg(scope_type)
  AND lh.scope_key = sqlc.arg(scope_key)
  AND l.enabled = true
  AND l.deleted_at IS NULL;

-- 将一条已发布版本置为 draft，供历史输入回退时解除既有 published 状态。
-- name: UnpublishAssetVersion :execrows
UPDATE asset_versions
SET lifecycle = 'draft', revision = revision + 1, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND lifecycle = 'published';

-- 锁定轨迹行，供发布事务串行化与递增 desired_generation。
-- name: LockAssetRefTrack :one
SELECT *
FROM asset_ref_tracks
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
FOR UPDATE;

-- 更新轨迹的 desired_generation，表示一次发布（或历史回退）影响了该轨迹。
-- name: BumpAssetRefTrackGeneration :execrows
UPDATE asset_ref_tracks
SET desired_generation = desired_generation + 1,
    updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 返回租户的设置 JSON 快照，供 trust 模式与自动发布判定。
-- name: GetTenantSettings :one
SELECT settings
FROM tenants
WHERE id = sqlc.arg(id);

-- 更新一条资产版本的生命周期与版本标签并递增 revision。
-- name: UpdateAssetVersionPublish :one
UPDATE asset_versions
SET lifecycle = 'published',
    version = sqlc.arg(version),
    revision = revision + 1,
    updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 为已认证的租户主体串行化一个 generateMissingAssetWithAi 幂等键。
-- name: LockAiGenerationIdempotency :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key), 0));

-- 返回 generateMissingAssetWithAi 请求保留的准确响应。
-- name: GetAiGenerationIdempotency :one
SELECT request_hash, response_body, expires_at
FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'generateMissingAssetWithAi'
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- 在重新使用幂等键前删除已过期的 generateMissingAssetWithAi 重放记录。
-- name: DeleteAiGenerationIdempotency :exec
DELETE FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'generateMissingAssetWithAi'
  AND idempotency_key = sqlc.arg(idempotency_key);

-- 保存可重放 24 小时的准确、非敏感 202 响应。
-- name: CreateAiGenerationIdempotency :exec
INSERT INTO idempotency_records (
  tenant_id, principal_type, principal_id, operation_id, idempotency_key,
  request_hash, response_status, response_body, expires_at
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(principal_type), sqlc.arg(principal_id), 'generateMissingAssetWithAi', sqlc.arg(idempotency_key),
  sqlc.arg(request_hash), 202, sqlc.arg(response_body)::jsonb, now() + interval '24 hours'
);

-- 将一条待审核修订置为 approved 或 rejected 并写入审核备注。
-- name: UpdateLayerRevisionReview :one
UPDATE layer_revisions
SET review_status = sqlc.arg(review_status),
    review_comment = sqlc.narg(review_comment),
    created_at = created_at
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND review_status = 'pending_review'
RETURNING *;

-- 将一条修订标记为 superseded（新候选替换旧候选）。
-- name: SupersedeLayerRevision :execrows
UPDATE layer_revisions
SET review_status = 'superseded'
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND review_status = 'pending_review';

-- 返回一条任务不可变、非敏感的输入 JSON，供 worker 恢复执行上下文。
-- name: GetJobInput :one
SELECT input
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 返回某服务某 kind 下启用且未删除的 AI 生成 base 源配置。
-- name: GetAiBaseForService :one
SELECT ss.*
FROM source_specs AS ss
WHERE ss.tenant_id = sqlc.arg(tenant_id)
  AND ss.service_id = sqlc.arg(service_id)
  AND ss.kind = sqlc.arg(kind)
  AND ss.origin = 'ai_generated'
  AND ss.role = 'base'
  AND ss.deleted_at IS NULL
LIMIT 1;

-- 将某服务某 kind 的 AI 生成 base 源配置及其层软删除（历史保留）。
-- name: ArchiveAiBaseForService :execrows
UPDATE source_specs
SET deleted_at = now(), updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id)
  AND kind = sqlc.arg(kind)
  AND origin = 'ai_generated'
  AND role = 'base'
  AND deleted_at IS NULL;

-- 将某服务某 kind 的 AI 生成 base 层软删除（历史保留）。
-- name: ArchiveAiBaseLayersForService :execrows
UPDATE layers
SET deleted_at = now(), updated_at = now()
FROM source_specs AS ss
WHERE layers.tenant_id = ss.tenant_id
  AND layers.source_spec_id = ss.id
  AND ss.tenant_id = sqlc.arg(tenant_id)
  AND ss.service_id = sqlc.arg(service_id)
  AND ss.kind = sqlc.arg(kind)
  AND ss.origin = 'ai_generated'
  AND ss.role = 'base'
  AND layers.deleted_at IS NULL;

-- 返回某服务某 kind 的 AI 生成 base 层（含资产标识），供替换事务复用资产。
-- name: ListAiBaseLayersForService :many
SELECT l.*
FROM layers AS l
JOIN source_specs AS ss ON ss.tenant_id = l.tenant_id AND ss.id = l.source_spec_id
WHERE ss.tenant_id = sqlc.arg(tenant_id)
  AND ss.service_id = sqlc.arg(service_id)
  AND ss.kind = sqlc.arg(kind)
  AND ss.origin = 'ai_generated'
  AND ss.role = 'base'
  AND l.deleted_at IS NULL;
