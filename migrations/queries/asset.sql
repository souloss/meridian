-- M1 资产流水线的持久化查询：资产、层、修订、版本、条目、轨迹、kind 与最近访问。
-- 全部查询保留 tenant_id 谓词；asset_kinds 为平台级 global 表。

-- 返回一个未删除的资产 kind 注册。
-- name: GetAssetKind :one
SELECT *
FROM asset_kinds
WHERE id = sqlc.arg(id);

-- 返回全部启用的资产 kind 注册。
-- name: ListAssetKinds :many
SELECT *
FROM asset_kinds
WHERE enabled = true
ORDER BY id;

-- 返回一个租户级 kind 覆盖，未显式配置时回退默认启用。
-- name: GetTenantKindOverride :one
SELECT *
FROM tenant_kind_overrides
WHERE tenant_id = sqlc.arg(tenant_id)
  AND kind_id = sqlc.arg(kind_id);

-- 列出全部租户级 kind 覆盖（含未覆盖时由平台默认派生启用状态的左连接）。
-- name: ListTenantKindOverrides :many
SELECT
  kind.id AS kind_id,
  kind.contract_version,
  kind.plugin_version,
  COALESCE(override.enabled, kind.enabled) AS enabled,
  COALESCE(override.revision, 1) AS revision
FROM asset_kinds AS kind
LEFT JOIN tenant_kind_overrides AS override
  ON override.kind_id = kind.id
 AND override.tenant_id = sqlc.arg(tenant_id)
ORDER BY kind.id;

-- 为租户插入一个 kind 开关覆盖，冲突时更新并递增 revision。
-- name: UpsertTenantKindOverride :one
INSERT INTO tenant_kind_overrides (tenant_id, kind_id, enabled, revision)
VALUES (sqlc.arg(tenant_id), sqlc.arg(kind_id), sqlc.arg(enabled), 1)
ON CONFLICT (tenant_id, kind_id) DO UPDATE SET
  enabled = EXCLUDED.enabled,
  revision = tenant_kind_overrides.revision + 1,
  updated_at = now()
RETURNING *;

-- 按服务、kind、名称返回一个活跃资产。
-- name: GetAssetByName :one
SELECT *
FROM assets
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id)
  AND kind = sqlc.arg(kind)
  AND name = sqlc.arg(name)
  AND deleted_at IS NULL;

-- 返回一个活跃资产。
-- name: GetAsset :one
SELECT *
FROM assets
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- 幂等创建资产；唯一键冲突时返回已存在行。
-- name: UpsertAsset :one
INSERT INTO assets (tenant_id, id, service_id, kind, name)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(service_id), sqlc.arg(kind), sqlc.arg(name))
ON CONFLICT (tenant_id, service_id, kind, name) DO UPDATE SET updated_at = now()
RETURNING *;

-- 创建一条资产引用轨迹。
-- name: CreateAssetRefTrack :one
INSERT INTO asset_ref_tracks (tenant_id, id, asset_id, ref_type, ref_name, health)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(asset_id), sqlc.arg(ref_type), sqlc.arg(ref_name), sqlc.arg(health))
ON CONFLICT (tenant_id, asset_id, ref_type, ref_name) DO UPDATE SET active = true, updated_at = now()
RETURNING *;

-- 返回一条资产引用轨迹。
-- name: GetAssetRefTrack :one
SELECT *
FROM asset_ref_tracks
WHERE tenant_id = sqlc.arg(tenant_id)
  AND asset_id = sqlc.arg(asset_id)
  AND ref_type = sqlc.arg(ref_type)
  AND ref_name = sqlc.arg(ref_name);

-- 按轨迹 id 返回一条资产引用轨迹，供 asset.merge 任务按轨迹定位资产。
-- name: GetAssetRefTrackByID :one
SELECT *
FROM asset_ref_tracks
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 创建一条层。
-- name: CreateLayer :one
INSERT INTO layers (tenant_id, id, asset_id, source_spec_id, role, origin, ord, dialect, enabled, branch_patterns, display_name)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(asset_id), sqlc.narg(source_spec_id), sqlc.arg(role), sqlc.arg(origin), sqlc.arg(ord), sqlc.narg(dialect), sqlc.arg(enabled), sqlc.arg(branch_patterns), sqlc.arg(display_name))
RETURNING *;

-- 返回一个资产的 base 层。
-- name: GetBaseLayerForAsset :one
SELECT *
FROM layers
WHERE tenant_id = sqlc.arg(tenant_id)
  AND asset_id = sqlc.arg(asset_id)
  AND role = 'base'
  AND deleted_at IS NULL;

-- 创建一条层修订。
-- name: CreateLayerRevision :one
INSERT INTO layer_revisions (
  tenant_id, id, layer_id, scope_type, scope_key, content_hash, content_ref,
  content_type, dialect, source_branch, review_status, git_commit, created_by, producer_run_id
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(layer_id), sqlc.arg(scope_type), sqlc.arg(scope_key),
  sqlc.arg(content_hash), sqlc.arg(content_ref), sqlc.arg(content_type), sqlc.narg(dialect),
  sqlc.narg(source_branch), sqlc.arg(review_status), sqlc.narg(git_commit), sqlc.narg(created_by), sqlc.narg(producer_run_id)
)
RETURNING *;

-- 返回某层在某作用域内最新创建的一条修订。
-- name: GetLatestLayerRevision :one
SELECT *
FROM layer_revisions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND layer_id = sqlc.arg(layer_id)
  AND scope_type = sqlc.arg(scope_type)
  AND scope_key = sqlc.arg(scope_key)
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- 记录一次源物化的失败说明并递增连续失败次数。
-- name: SetSourceLastError :execrows
UPDATE source_specs
SET last_error = sqlc.arg(last_error),
    failure_streak = failure_streak + 1,
    updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- 源物化成功后清空失败说明并归零连续失败次数。
-- name: ClearSourceLastError :execrows
UPDATE source_specs
SET last_error = NULL,
    failure_streak = 0,
    updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- 将某源配置关联资产的引用轨迹标记为 stale。
-- name: MarkTracksStaleForSourceSpec :execrows
UPDATE asset_ref_tracks AS track
SET health = 'stale', updated_at = now()
WHERE track.tenant_id = sqlc.arg(tenant_id)
  AND track.asset_id IN (
    SELECT layers.asset_id
    FROM layers
    WHERE layers.tenant_id = sqlc.arg(tenant_id)
      AND layers.source_spec_id = sqlc.arg(source_spec_id)::uuid
      AND layers.deleted_at IS NULL
  );

-- 将某源配置关联资产的引用轨迹恢复为 ok。
-- name: MarkTracksHealthyForSourceSpec :execrows
UPDATE asset_ref_tracks AS track
SET health = 'ok', updated_at = now()
WHERE track.tenant_id = sqlc.arg(tenant_id)
  AND track.asset_id IN (
    SELECT layers.asset_id
    FROM layers
    WHERE layers.tenant_id = sqlc.arg(tenant_id)
      AND layers.source_spec_id = sqlc.arg(source_spec_id)::uuid
      AND layers.deleted_at IS NULL
  );

-- 幂等创建层头。
-- name: UpsertLayerHead :one
INSERT INTO layer_heads (tenant_id, layer_id, scope_type, scope_key, latest_revision_id, effective_revision_id, candidate_revision_id, generation)
VALUES (sqlc.arg(tenant_id), sqlc.arg(layer_id), sqlc.arg(scope_type), sqlc.arg(scope_key), sqlc.arg(latest_revision_id), sqlc.arg(effective_revision_id), sqlc.narg(candidate_revision_id), sqlc.arg(generation))
ON CONFLICT (tenant_id, layer_id, scope_type, scope_key) DO UPDATE SET
  latest_revision_id = EXCLUDED.latest_revision_id,
  effective_revision_id = EXCLUDED.effective_revision_id,
  candidate_revision_id = EXCLUDED.candidate_revision_id,
  generation = EXCLUDED.generation,
  updated_at = now()
RETURNING *;

-- 返回一个层头。
-- name: GetLayerHead :one
SELECT *
FROM layer_heads
WHERE tenant_id = sqlc.arg(tenant_id)
  AND layer_id = sqlc.arg(layer_id)
  AND scope_type = sqlc.arg(scope_type)
  AND scope_key = sqlc.arg(scope_key);

-- 创建一条资产版本。
-- name: CreateAssetVersion :one
INSERT INTO asset_versions (
  tenant_id, id, asset_id, track_id, sequence_no, version, lifecycle, revision,
  quality_score, merge_request_id, input_fingerprint, merge_engine_version,
  overlay_compiler_version, overlay_mode, normalizer_version, kind_plugin_version,
  layer_manifest, merged_hash, merged_ref, normalized_ref, bundled_ref, provenance_ref,
  source_commit, baseline_version_id, diff_summary, labels, index_complete
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(asset_id), sqlc.arg(track_id), sqlc.arg(sequence_no), sqlc.arg(version), sqlc.arg(lifecycle), sqlc.arg(revision),
  sqlc.narg(quality_score), sqlc.narg(merge_request_id), sqlc.arg(input_fingerprint), sqlc.arg(merge_engine_version),
  sqlc.narg(overlay_compiler_version), sqlc.narg(overlay_mode), sqlc.narg(normalizer_version), sqlc.narg(kind_plugin_version),
  sqlc.arg(layer_manifest), sqlc.narg(merged_hash), sqlc.narg(merged_ref), sqlc.narg(normalized_ref), sqlc.narg(bundled_ref), sqlc.narg(provenance_ref),
  sqlc.narg(source_commit), sqlc.narg(baseline_version_id), sqlc.narg(diff_summary), sqlc.arg(labels), sqlc.arg(index_complete)
)
RETURNING *;

-- 返回一条资产版本。
-- name: GetAssetVersion :one
SELECT *
FROM asset_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 将一条资产版本标记为已完成 item 索引。
-- name: MarkAssetVersionIndexed :execrows
UPDATE asset_versions
SET index_complete = true, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 返回轨迹内最新创建的版本。
-- name: GetLatestVersionInTrack :one
SELECT *
FROM asset_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND track_id = sqlc.arg(track_id)
ORDER BY sequence_no DESC
LIMIT 1;

-- 返回轨迹内当前已发布的版本。
-- name: GetCurrentVersionInTrack :one
SELECT *
FROM asset_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND track_id = sqlc.arg(track_id)
  AND lifecycle = 'published'
ORDER BY sequence_no DESC
LIMIT 1;

-- 创建一条资产版本条目。
-- name: CreateAssetItem :one
INSERT INTO asset_items (tenant_id, id, asset_version_id, asset_id, service_id, kind, item_type, key, display, search_text, search_raw, provenance)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(asset_version_id), sqlc.arg(asset_id), sqlc.arg(service_id), sqlc.arg(kind), sqlc.arg(item_type), sqlc.arg(key), sqlc.arg(display), sqlc.arg(search_text), sqlc.arg(search_raw), sqlc.arg(provenance))
RETURNING *;

-- 更新一条资产版本条目的 tsvector 全文检索向量。
-- name: UpdateAssetItemSearchVector :execrows
UPDATE asset_items
SET search_vector = to_tsvector('simple', sqlc.arg(search_text))
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 列出资产版本条目分页。
-- name: ListAssetVersionItems :many
SELECT *
FROM asset_items
WHERE tenant_id = sqlc.arg(tenant_id)
  AND asset_version_id = sqlc.arg(asset_version_id)
  AND (
    sqlc.arg(search_query)::text = ''
    OR search_text ILIKE '%' || sqlc.arg(search_query)::text || '%'
  )
ORDER BY item_type, key
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 统计资产版本条目总数。
-- name: CountAssetVersionItems :one
SELECT count(*)::bigint
FROM asset_items
WHERE tenant_id = sqlc.arg(tenant_id)
  AND asset_version_id = sqlc.arg(asset_version_id);

-- 更新一条轨迹的版本头。
-- name: UpdateAssetRefTrackHead :execrows
UPDATE asset_ref_tracks
SET latest_version_id = sqlc.narg(latest_version_id),
    current_version_id = sqlc.narg(current_version_id),
    processed_generation = sqlc.arg(processed_generation),
    updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 列出服务某作用域内当前活跃的源绑定。
-- name: ListActiveBindingsForScope :many
SELECT *
FROM source_bindings
WHERE tenant_id = sqlc.arg(tenant_id)
  AND source_spec_id = sqlc.arg(source_spec_id)
  AND scope_type = sqlc.arg(scope_type)
  AND scope_key = sqlc.arg(scope_key)
  AND state = 'active';

-- 幂等创建源绑定。
-- name: UpsertSourceBinding :one
INSERT INTO source_bindings (
  tenant_id, id, source_spec_id, scope_type, scope_key, expansion_key,
  resolved_path, source_system, asset_id, layer_id, state, last_seen_commit
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(source_spec_id), sqlc.arg(scope_type), sqlc.arg(scope_key),
  sqlc.arg(expansion_key), sqlc.narg(resolved_path), sqlc.narg(source_system), sqlc.arg(asset_id), sqlc.arg(layer_id), sqlc.arg(state), sqlc.narg(last_seen_commit)
)
ON CONFLICT (tenant_id, source_spec_id, scope_type, scope_key, expansion_key) DO UPDATE SET
  resolved_path = EXCLUDED.resolved_path,
  state = EXCLUDED.state,
  last_seen_commit = EXCLUDED.last_seen_commit,
  asset_id = EXCLUDED.asset_id,
  layer_id = EXCLUDED.layer_id,
  updated_at = now()
RETURNING *;

-- 将某作用域内本次未出现的绑定标记为 stale。
-- name: MarkBindingsStaleInScope :execrows
UPDATE source_bindings
SET state = 'stale', updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND source_spec_id = sqlc.arg(source_spec_id)
  AND scope_type = sqlc.arg(scope_type)
  AND scope_key = sqlc.arg(scope_key)
  AND state = 'active'
  AND NOT (id = ANY(sqlc.arg(seen_ids)::uuid[]));

-- 统计一个源配置当前活跃的绑定数量。
-- name: CountActiveBindings :one
SELECT count(*)::bigint
FROM source_bindings
WHERE tenant_id = sqlc.arg(tenant_id)
  AND source_spec_id = sqlc.arg(source_spec_id)
  AND state = 'active';

-- 幂等记录一次用户对服务的成功访问。
-- name: UpsertRecentService :execrows
INSERT INTO recent_services (tenant_id, user_id, service_id, viewed_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(user_id), sqlc.arg(service_id), sqlc.arg(viewed_at))
ON CONFLICT (tenant_id, user_id, service_id) DO UPDATE SET viewed_at = EXCLUDED.viewed_at;

-- 列出用户最近访问的服务。
-- name: ListRecentServices :many
SELECT s.*
FROM recent_services AS r
JOIN services AS s ON s.tenant_id = r.tenant_id AND s.id = r.service_id
WHERE r.tenant_id = sqlc.arg(tenant_id)
  AND r.user_id = sqlc.arg(user_id)
  AND s.deleted_at IS NULL
ORDER BY r.viewed_at DESC, s.id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 统计用户最近访问的服务总数。
-- name: CountRecentServices :one
SELECT count(*)::bigint
FROM recent_services AS r
JOIN services AS s ON s.tenant_id = r.tenant_id AND s.id = r.service_id
WHERE r.tenant_id = sqlc.arg(tenant_id)
  AND r.user_id = sqlc.arg(user_id)
  AND s.deleted_at IS NULL;

-- 返回一个服务下全部活跃源配置。
-- name: ListSourceSpecsForService :many
SELECT *
FROM source_specs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id)
  AND deleted_at IS NULL
ORDER BY ord, id;

-- 返回一个仓库下全部活跃服务。
-- name: ListServicesByRepository :many
SELECT *
FROM services
WHERE tenant_id = sqlc.arg(tenant_id)
  AND repository_id = sqlc.arg(repository_id)
  AND deleted_at IS NULL
ORDER BY root_dir, id;

-- 返回一个服务下全部活跃资产。
-- name: ListAssetsForService :many
SELECT *
FROM assets
WHERE tenant_id = sqlc.arg(tenant_id)
  AND service_id = sqlc.arg(service_id)
  AND deleted_at IS NULL
ORDER BY name, id;

-- 更新一条源配置，应用明确提供的 PATCH 字段并递增版本。
-- name: UpdateSourceSpec :one
UPDATE source_specs
SET
  asset_name_template = COALESCE(sqlc.narg(asset_name_template), asset_name_template),
  role = COALESCE(sqlc.narg(role), role),
  origin = COALESCE(sqlc.narg(origin), origin),
  mode = COALESCE(sqlc.narg(mode), mode),
  path = COALESCE(sqlc.narg(path), path),
  producer_profile_id = COALESCE(sqlc.narg(producer_profile_id), producer_profile_id),
  ord = COALESCE(sqlc.narg(ord), ord),
  timeout_sec = COALESCE(sqlc.narg(timeout_sec), timeout_sec),
  branch_patterns = COALESCE(sqlc.narg(branch_patterns), branch_patterns),
  enabled = COALESCE(sqlc.narg(enabled), enabled),
  revision = revision + 1,
  updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 记录一条持久化的仓库同步请求。
-- name: CreateSyncJob :one
INSERT INTO jobs (
  tenant_id, id, type, scope_type, scope_id, ref_type, ref_name, trigger, input,
  status, max_attempts, dedupe_key, active_generation, replay_safe
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), 'repo.sync', 'repository', sqlc.arg(repository_id),
  sqlc.arg(ref_type), sqlc.narg(ref_name), 'manual', sqlc.arg(job_input)::jsonb,
  'pending', 3, sqlc.arg(dedupe_key), sqlc.arg(active_generation), true
)
ON CONFLICT (tenant_id, dedupe_key, active_generation) DO NOTHING
RETURNING *;

-- 记录一条持久化的资产合并请求。
-- name: CreateMergeJob :one
INSERT INTO jobs (
  tenant_id, id, type, scope_type, scope_id, ref_type, ref_name, trigger, input,
  status, max_attempts, dedupe_key, active_generation, replay_safe
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), 'asset.merge', 'track', sqlc.arg(track_id),
  NULL, NULL, 'manual', sqlc.arg(job_input)::jsonb,
  'pending', 3, sqlc.arg(dedupe_key), sqlc.arg(active_generation), true
)
ON CONFLICT (tenant_id, dedupe_key, active_generation) DO NOTHING
RETURNING *;

-- 返回一个资产所属服务的仓库默认分支。
-- name: GetAssetRepositoryDefaultBranch :one
SELECT r.default_branch
FROM assets AS a
JOIN services AS s ON s.tenant_id = a.tenant_id AND s.id = a.service_id
JOIN repositories AS r ON r.tenant_id = s.tenant_id AND r.id = s.repository_id
WHERE a.tenant_id = sqlc.arg(tenant_id)
  AND a.id = sqlc.arg(asset_id)
  AND a.deleted_at IS NULL;

-- 返回一个资产下全部版本分页（含生命周期状态），供版本历史读取。
-- name: ListAssetVersions :many
SELECT *
FROM asset_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND asset_id = sqlc.arg(asset_id)
ORDER BY sequence_no DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回一个资产下版本总数。
-- name: CountAssetVersions :one
SELECT count(*)::bigint
FROM asset_versions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND asset_id = sqlc.arg(asset_id);

-- 在 If-Match 下将一个资产版本置为 deprecated 并递增 revision。
-- name: DeprecateAssetVersion :one
UPDATE asset_versions
SET lifecycle = 'deprecated', revision = revision + 1, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND lifecycle = 'published'
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 在 If-Match 下将一个资产版本置为 retired 并递增 revision。
-- name: RetireAssetVersion :one
UPDATE asset_versions
SET lifecycle = 'retired', revision = revision + 1, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND lifecycle = 'published'
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 按 slug 返回一个公开可见服务及其当前资产（匿名公开读取）。
-- name: GetPublicAssetBySlug :one
SELECT
  a.tenant_id, a.kind, a.name, a.created_at AS asset_created_at,
  t.current_version_id, t.latest_version_id
FROM assets AS a
JOIN services AS s ON s.tenant_id = a.tenant_id AND s.id = a.service_id
JOIN tenants AS tenant ON tenant.id = s.tenant_id
JOIN asset_ref_tracks AS t ON t.asset_id = a.id AND t.tenant_id = a.tenant_id AND t.ref_type = 'branch'
WHERE tenant.slug = sqlc.arg(tenant_slug)
  AND s.slug = sqlc.arg(service_slug)
  AND s.visibility = 'public'
  AND s.lifecycle IN ('published', 'deprecated')
  AND a.kind = sqlc.arg(kind)
  AND a.name = sqlc.arg(asset_name)
  AND a.deleted_at IS NULL
  AND t.ref_name = (SELECT default_branch FROM repositories WHERE repositories.tenant_id = s.tenant_id AND repositories.id = s.repository_id)
LIMIT 1;

-- 返回一个公开可见资产的当前版本内容引用。
-- name: GetPublicAssetVersion :one
SELECT v.id, v.version, v.lifecycle, v.merged_ref
FROM asset_versions AS v
WHERE v.tenant_id = sqlc.arg(tenant_id)
  AND v.id = sqlc.arg(version_id)
  AND v.lifecycle = 'published';

-- 在不提供 serviceSlug 的匿名上下文中，按租户 + 类别 + 名称定位公开资产的
-- 当前版本指针（resolvePublicView 契约不含 serviceSlug）。
-- 资产名在 (tenant, service, kind, name) 上唯一，但公开可见性仍由服务门控，
-- 因此本查询同时要求其服务 visibility=public 且 lifecycle 允许公开读取。
-- name: GetPublicAssetByName :one
SELECT
  a.tenant_id, a.kind, a.name, a.created_at AS asset_created_at,
  t.current_version_id, t.latest_version_id
FROM assets AS a
JOIN services AS s ON s.tenant_id = a.tenant_id AND s.id = a.service_id
JOIN tenants AS tenant ON tenant.id = s.tenant_id
JOIN asset_ref_tracks AS t ON t.asset_id = a.id AND t.tenant_id = a.tenant_id AND t.ref_type = 'branch'
WHERE tenant.slug = sqlc.arg(tenant_slug)
  AND s.visibility = 'public'
  AND s.lifecycle IN ('published', 'deprecated')
  AND a.kind = sqlc.arg(kind)
  AND a.name = sqlc.arg(asset_name)
  AND a.deleted_at IS NULL
  AND t.ref_name = (SELECT default_branch FROM repositories WHERE repositories.tenant_id = s.tenant_id AND repositories.id = s.repository_id)
LIMIT 1;
