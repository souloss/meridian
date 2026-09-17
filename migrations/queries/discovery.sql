-- M1 仓库发现、候选、服务、生产者配置与源配置/绑定的持久化查询。
-- 全部查询保留 tenant_id 谓词；producer_profiles 为平台级 global 表。

-- 返回一个活跃服务，供源配置、服务详情等路径按 slug 定位。
-- name: GetServiceBySlug :one
SELECT *
FROM services
WHERE tenant_id = sqlc.arg(tenant_id)
  AND slug = sqlc.arg(slug)
  AND deleted_at IS NULL;

-- 返回一个活跃服务，供按 id 定位。
-- name: GetServiceByID :one
SELECT *
FROM services
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- 返回有效服务数量与租户固定的服务配额。
-- name: CountServices :one
SELECT
  (SELECT count(*)::bigint
   FROM services
   WHERE tenant_id = sqlc.arg(tenant_id)
     AND deleted_at IS NULL) AS current_count,
  COALESCE((SELECT (quota ->> 'maxServices')::bigint
            FROM tenants
            WHERE id = sqlc.arg(tenant_id)), 0)::bigint AS limit_count;

-- 持久化一个候选接受后新建的服务，冲突时静默跳过。
-- name: CreateService :one
INSERT INTO services (
  tenant_id, id, repository_id, slug, display_name, description, root_dir,
  language, framework, owners, maintainers, lifecycle, visibility
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(repository_id), sqlc.arg(slug),
  sqlc.arg(display_name), sqlc.narg(description), sqlc.arg(root_dir),
  sqlc.narg(language), sqlc.narg(framework), sqlc.arg(owners), sqlc.arg(maintainers),
  sqlc.arg(lifecycle), sqlc.arg(visibility)
)
RETURNING *;

-- 列出活跃服务的确定顺序分页。
-- name: ListServices :many
SELECT *
FROM services
WHERE tenant_id = sqlc.arg(tenant_id)
  AND deleted_at IS NULL
ORDER BY slug, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回匹配一次租户搜索的有效服务数量。
-- name: CountListedServices :one
SELECT count(*)::bigint
FROM services
WHERE tenant_id = sqlc.arg(tenant_id)
  AND deleted_at IS NULL;

-- 串行化一个仓库发现请求的去重键。
-- 终态记录会递增 active_generation 以接受新工作。
-- name: LockLatestDiscoveryJob :one
SELECT id, tenant_id, status, active_generation
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND dedupe_key = sqlc.arg(dedupe_key)
ORDER BY active_generation DESC
LIMIT 1
FOR UPDATE;

-- 记录一条持久化的仓库发现请求。
-- name: CreateDiscoveryJob :one
INSERT INTO jobs (
  tenant_id, id, type, scope_type, scope_id, ref_type, ref_name, trigger, input,
  status, max_attempts, dedupe_key, active_generation, replay_safe
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), 'repo.discover', 'repository', sqlc.arg(repository_id),
  sqlc.arg(ref_type), sqlc.narg(ref_name), 'manual', sqlc.arg(job_input)::jsonb,
  'pending', 3, sqlc.arg(dedupe_key), sqlc.arg(active_generation), true
)
ON CONFLICT (tenant_id, dedupe_key, active_generation) DO NOTHING
RETURNING *;

-- 按仓库引用身份解析候选键：repositoryId + rootDir，不含 commit。
-- name: UpsertDiscoveryCandidate :one
INSERT INTO discovery_candidates (tenant_id, id, repository_id, commit_sha, root_dir, detected, status)
VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(repository_id), sqlc.arg(commit_sha),
  sqlc.arg(root_dir), sqlc.arg(detected)::jsonb, 'pending'
)
ON CONFLICT (tenant_id, repository_id, root_dir) DO UPDATE SET
  commit_sha = EXCLUDED.commit_sha,
  detected = EXCLUDED.detected,
  status = CASE
    WHEN discovery_candidates.status = 'accepted' THEN 'accepted'
    WHEN discovery_candidates.commit_sha IS DISTINCT FROM EXCLUDED.commit_sha THEN 'pending'
    ELSE discovery_candidates.status
  END,
  updated_at = now()
RETURNING *;

-- 列出候选，按 rootDir 逐字节升序并分页。
-- name: ListDiscoveryCandidates :many
SELECT *
FROM discovery_candidates
WHERE tenant_id = sqlc.arg(tenant_id)
  AND repository_id = sqlc.arg(repository_id)
ORDER BY root_dir, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回匹配一次仓库的候选总数。
-- name: CountDiscoveryCandidates :one
SELECT count(*)::bigint
FROM discovery_candidates
WHERE tenant_id = sqlc.arg(tenant_id)
  AND repository_id = sqlc.arg(repository_id);

-- 返回一个候选，供接受前校验。
-- name: GetDiscoveryCandidate :one
SELECT *
FROM discovery_candidates
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 将候选标记为已接受；仅在 pending 状态时生效。
-- name: AcceptDiscoveryCandidate :execrows
UPDATE discovery_candidates
SET status = 'accepted', updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status = 'pending';

-- 平台侧持久化生产者配置文件。
-- name: CreateProducerProfile :one
INSERT INTO producer_profiles (
  id, name, kind, executable, args, env_allowlist, supported_kinds, replay_safe,
  network, timeout_sec, memory_mib, cpu_seconds, pids, enabled, dependency_status, unavailable_reason
) VALUES (
  sqlc.arg(id), sqlc.arg(name), sqlc.arg(kind), sqlc.arg(executable), sqlc.arg(args)::jsonb,
  sqlc.arg(env_allowlist), sqlc.arg(supported_kinds), sqlc.arg(replay_safe), sqlc.arg(network),
  sqlc.arg(timeout_sec), sqlc.arg(memory_mib), sqlc.arg(cpu_seconds), sqlc.arg(pids),
  sqlc.arg(enabled), sqlc.arg(dependency_status), sqlc.narg(unavailable_reason)
)
RETURNING *;

-- 返回平台生产者配置文件分页。
-- name: ListProducerProfiles :many
SELECT *
FROM producer_profiles
WHERE deleted_at IS NULL
ORDER BY name, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 返回一个未删除的生产者配置文件。
-- name: GetProducerProfile :one
SELECT *
FROM producer_profiles
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- 列出可被租户选择的可用生产者配置；可选按 kind 过滤。
-- name: ListAvailableProducerProfiles :many
SELECT *
FROM producer_profiles
WHERE deleted_at IS NULL
  AND enabled = true
  AND dependency_status = 'available'
  AND (
    sqlc.arg(kind_filter)::text = ''
    OR sqlc.arg(kind_filter)::text = ANY(supported_kinds)
  )
ORDER BY name, id;

-- 启动时全量重扫并刷新依赖状态。
-- name: UpdateProducerProfileDependencyStatus :execrows
UPDATE producer_profiles
SET dependency_status = sqlc.arg(dependency_status),
    unavailable_reason = sqlc.narg(unavailable_reason),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- 持久化源配置。
-- name: CreateSourceSpec :one
INSERT INTO source_specs (
  tenant_id, id, service_id, kind, asset_name_template, role, origin, mode, path,
  producer_profile_id, ord, timeout_sec, branch_patterns, enabled, config_origin
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(service_id), sqlc.arg(kind),
  sqlc.arg(asset_name_template), sqlc.arg(role), sqlc.arg(origin), sqlc.arg(mode),
  sqlc.narg(path), sqlc.narg(producer_profile_id), sqlc.arg(ord), sqlc.arg(timeout_sec),
  sqlc.arg(branch_patterns), sqlc.arg(enabled), sqlc.arg(config_origin)
)
RETURNING *;

-- 返回一个活跃源配置。
-- name: GetSourceSpec :one
SELECT *
FROM source_specs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- 列出源配置物化出的绑定，按创建顺序。
-- name: ListSourceBindings :many
SELECT *
FROM source_bindings
WHERE tenant_id = sqlc.arg(tenant_id)
  AND source_spec_id = sqlc.arg(source_spec_id)
ORDER BY created_at, id;

-- 统计一个源配置当前的绑定数量。
-- name: CountSourceBindings :one
SELECT count(*)::bigint
FROM source_bindings
WHERE tenant_id = sqlc.arg(tenant_id)
  AND source_spec_id = sqlc.arg(source_spec_id);
