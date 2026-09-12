-- 写入一条租户拥有的加密凭据，并返回元数据和密文。
-- 查询不会接收秘密明文，服务层只提供加密后的投影。
-- name: CreateCredential :one
INSERT INTO credentials (
  tenant_id, id, name, kind, ciphertext, nonce, key_version, fingerprint,
  shared_scope, created_by
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(name), sqlc.arg(kind),
  sqlc.arg(ciphertext), sqlc.arg(nonce), sqlc.arg(key_version), sqlc.arg(fingerprint),
  sqlc.arg(shared_scope), sqlc.arg(created_by)
)
RETURNING *;

-- 返回一个用户在一个有效租户内可见的凭据。
-- 团队可见性通过同租户团队成员条件判断；平台凭据以 is_global=true 和租户共享投影追加返回。
-- name: ListTenantCredentials :many
SELECT *
FROM (
  SELECT
    c.tenant_id,
    c.id,
    false AS is_global,
    c.name,
    c.kind,
    c.ciphertext,
    c.nonce,
    c.key_version,
    c.fingerprint,
    c.shared_scope,
    c.created_by,
    c.last_used_at,
    c.revision,
    c.created_at,
    c.updated_at,
    COALESCE(
      jsonb_agg(DISTINCT shares.team_id) FILTER (WHERE shares.team_id IS NOT NULL),
      '[]'::jsonb
    )::text AS team_ids
  FROM credentials AS c
  LEFT JOIN credential_team_shares AS shares
    ON shares.tenant_id = c.tenant_id
   AND shares.credential_id = c.id
  WHERE c.tenant_id = sqlc.arg(tenant_id)
    AND (
      c.created_by = sqlc.arg(user_id)
      OR c.shared_scope = 'tenant'
      OR (
        c.shared_scope = 'team'
        AND EXISTS (
          SELECT 1
          FROM team_members AS members
          JOIN credential_team_shares AS visible
            ON visible.tenant_id = members.tenant_id
           AND visible.team_id = members.team_id
           AND visible.credential_id = c.id
          WHERE members.tenant_id = c.tenant_id
            AND members.user_id = sqlc.arg(user_id)
        )
      )
    )
  GROUP BY c.tenant_id, c.id, c.name, c.kind, c.ciphertext, c.nonce, c.key_version,
    c.fingerprint, c.shared_scope, c.created_by, c.last_used_at, c.revision,
    c.created_at, c.updated_at
  UNION ALL
  SELECT
    sqlc.arg(tenant_id),
    g.id,
    true,
    g.name,
    g.kind,
    g.ciphertext,
    g.nonce,
    g.key_version,
    g.fingerprint,
    'tenant',
    g.created_by,
    g.last_used_at,
    g.revision,
    g.created_at,
    g.updated_at,
    '[]'
  FROM global_credentials AS g
) AS visible_credentials
ORDER BY visible_credentials.created_at DESC, visible_credentials.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 统计一个租户成员可见的租户凭据和平台凭据数量。
-- name: CountTenantCredentials :one
SELECT (
  SELECT count(*)
  FROM credentials AS c
  WHERE c.tenant_id = sqlc.arg(tenant_id)
    AND (
      c.created_by = sqlc.arg(user_id)
      OR c.shared_scope = 'tenant'
      OR (
        c.shared_scope = 'team'
        AND EXISTS (
          SELECT 1
          FROM team_members AS members
          JOIN credential_team_shares AS visible
            ON visible.tenant_id = members.tenant_id
           AND visible.team_id = members.team_id
           AND visible.credential_id = c.id
          WHERE members.tenant_id = c.tenant_id
            AND members.user_id = sqlc.arg(user_id)
        )
      )
    )
)::bigint + (SELECT count(*) FROM global_credentials)::bigint;

-- 返回一条可见的租户凭据及其团队共享标识。
-- name: GetTenantCredential :one
SELECT
  c.tenant_id,
  c.id,
  c.name,
  c.kind,
  c.ciphertext,
  c.nonce,
  c.key_version,
  c.fingerprint,
  c.shared_scope,
  c.created_by,
  c.last_used_at,
  c.revision,
  c.created_at,
  c.updated_at,
  COALESCE(
    jsonb_agg(DISTINCT shares.team_id) FILTER (WHERE shares.team_id IS NOT NULL),
    '[]'::jsonb
  )::text AS team_ids
FROM credentials AS c
LEFT JOIN credential_team_shares AS shares
  ON shares.tenant_id = c.tenant_id
 AND shares.credential_id = c.id
WHERE c.tenant_id = sqlc.arg(tenant_id)
  AND c.id = sqlc.arg(id)
  AND (
    c.created_by = sqlc.arg(user_id)
    OR c.shared_scope = 'tenant'
    OR (
      c.shared_scope = 'team'
      AND EXISTS (
        SELECT 1
        FROM team_members AS members
        JOIN credential_team_shares AS visible
          ON visible.tenant_id = members.tenant_id
         AND visible.team_id = members.team_id
         AND visible.credential_id = c.id
        WHERE members.tenant_id = c.tenant_id
          AND members.user_id = sqlc.arg(user_id)
      )
    )
  )
GROUP BY c.tenant_id, c.id, c.name, c.kind, c.ciphertext, c.nonce, c.key_version,
  c.fingerprint, c.shared_scope, c.created_by, c.last_used_at, c.revision,
  c.created_at, c.updated_at;

-- 返回一条不经过可见性过滤的租户凭据。
-- 服务层已完成租户操作授权，本查询保留 404 与 412 的区别。
-- name: GetTenantCredentialForMutation :one
SELECT *
FROM credentials
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 有条件地更新租户凭据元数据并递增版本号。
-- name: UpdateCredentialMetadata :one
UPDATE credentials
SET
  name = COALESCE(sqlc.narg(name), name),
  shared_scope = COALESCE(sqlc.narg(shared_scope), shared_scope),
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 在一个事务内删除并重建完整的凭据团队共享投影。
-- name: ReplaceCredentialTeamShares :exec
DELETE FROM credential_team_shares
WHERE tenant_id = sqlc.arg(tenant_id)
  AND credential_id = sqlc.arg(credential_id);

-- 授予一条同租户团队可见性关系。
-- name: AddCredentialTeamShare :exec
INSERT INTO credential_team_shares (tenant_id, credential_id, team_id)
VALUES (sqlc.arg(tenant_id), sqlc.arg(credential_id), sqlc.arg(team_id));

-- 返回一条租户凭据完整且有序的团队共享集合。
-- name: ListCredentialTeamShares :many
SELECT team_id
FROM credential_team_shares
WHERE tenant_id = sqlc.arg(tenant_id)
  AND credential_id = sqlc.arg(credential_id)
ORDER BY team_id;

-- 统计引用某条租户凭据的有效仓库数量。
-- name: CountCredentialRepositories :one
SELECT count(*)
FROM repositories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND credential_id = sqlc.arg(credential_id)
  AND deleted_at IS NULL;

-- 通过有效引用策略检查后，清除所有仓库引用。
-- 归档仓库保留健康状态和历史，但不能继续持有已删除凭据的外键。
-- name: UnbindCredentialRepositories :exec
UPDATE repositories
SET
  credential_id = NULL,
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at),
  health = CASE
    WHEN deleted_at IS NULL THEN jsonb_set(
      COALESCE(health, '{}'::jsonb),
      '{lastError}',
      '{"class":"auth","message":"credential was deleted"}'::jsonb,
      true
    )
    ELSE health
  END
WHERE tenant_id = sqlc.arg(tenant_id)
  AND credential_id = sqlc.arg(credential_id);

-- 调用方完成引用和 ETag 检查后，删除一条租户凭据。
-- name: DeleteCredential :execrows
DELETE FROM credentials
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision);

-- 有条件地替换加密秘密材料并递增凭据版本号。
-- name: RotateCredentialSecret :one
UPDATE credentials
SET
  ciphertext = sqlc.arg(ciphertext),
  nonce = sqlc.arg(nonce),
  key_version = sqlc.arg(key_version),
  fingerprint = sqlc.arg(fingerprint),
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 按契约响应顺序返回未删除仓库对该凭据的引用。
-- name: ListRepositoriesForCredential :many
SELECT
  repositories.tenant_id,
  tenants.slug AS tenant_slug,
  repositories.id AS repository_id,
  repositories.default_branch
FROM repositories
JOIN tenants ON tenants.id = repositories.tenant_id
WHERE repositories.tenant_id = sqlc.arg(tenant_id)
  AND repositories.credential_id = sqlc.arg(credential_id)
  AND repositories.deleted_at IS NULL
ORDER BY tenants.slug, repositories.id;

-- 串行化一个仓库分支的凭据轮换去重。
-- 状态为 pending 或 running 的记录会复用；终态记录会递增 active_generation 以接受新工作。
-- name: LockLatestCredentialSyncJob :one
SELECT id, tenant_id, status, active_generation
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND dedupe_key = sqlc.arg(dedupe_key)
ORDER BY active_generation DESC
LIMIT 1
FOR UPDATE;

-- 记录一条持久化的默认分支仓库同步请求。
-- 输入只包含非敏感凭据标识和轮换原因。
-- name: CreateCredentialSyncJob :one
INSERT INTO jobs (
  tenant_id, id, type, scope_type, scope_id, ref_type, ref_name, trigger, input,
  status, max_attempts, dedupe_key, active_generation, replay_safe
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), 'repo.sync', 'repository', sqlc.arg(repository_id),
  'branch', sqlc.arg(ref_name), 'credential-rotated', sqlc.arg(job_input)::jsonb,
  'pending', 3, sqlc.arg(dedupe_key), sqlc.arg(active_generation), true
)
ON CONFLICT (tenant_id, dedupe_key, active_generation) DO NOTHING
RETURNING *;

-- 在并发 HTTP 请求之间串行化一个轮换幂等键。
-- 锁键由认证主体和操作派生，绝不来自秘密明文。
-- name: LockCredentialRotationIdempotency :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key), 0));

-- 返回保留的租户凭据轮换重放记录及其过期时间。
-- name: GetCredentialRotationIdempotency :one
SELECT request_hash, response_body, expires_at
FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'rotateCredential'
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- 在重新使用前删除已过期的租户凭据轮换重放记录。
-- name: DeleteCredentialRotationIdempotency :exec
DELETE FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'rotateCredential'
  AND idempotency_key = sqlc.arg(idempotency_key);

-- 保存可安全精确重放 24 小时的租户凭据轮换响应。
-- 适配器会从 response_body 排除密文、nonce 和其他所有敏感字段。
-- name: CreateCredentialRotationIdempotency :exec
INSERT INTO idempotency_records (
  tenant_id, principal_type, principal_id, operation_id, idempotency_key,
  request_hash, response_status, response_body, expires_at
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(principal_type), sqlc.arg(principal_id), 'rotateCredential', sqlc.arg(idempotency_key),
  sqlc.arg(request_hash), 200, sqlc.arg(response_body)::jsonb, now() + interval '24 hours'
);

-- 返回保留的平台凭据轮换重放记录。
-- name: GetGlobalCredentialRotationIdempotency :one
SELECT request_hash, response_body, expires_at
FROM global_idempotency_records
WHERE context_type = 'platform'
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'rotateGlobalCredential'
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- 在重新使用前删除已过期的平台凭据轮换重放记录。
-- name: DeleteGlobalCredentialRotationIdempotency :exec
DELETE FROM global_idempotency_records
WHERE context_type = 'platform'
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'rotateGlobalCredential'
  AND idempotency_key = sqlc.arg(idempotency_key);

-- 保存可安全精确重放 24 小时的平台凭据轮换响应。
-- 适配器会从 response_body 排除密文、nonce 和其他所有敏感字段。
-- name: CreateGlobalCredentialRotationIdempotency :exec
INSERT INTO global_idempotency_records (
  context_type, principal_type, principal_id, operation_id, idempotency_key,
  request_hash, response_status, response_body, expires_at
) VALUES (
  'platform', sqlc.arg(principal_type), sqlc.arg(principal_id), 'rotateGlobalCredential', sqlc.arg(idempotency_key),
  sqlc.arg(request_hash), 200, sqlc.arg(response_body)::jsonb, now() + interval '24 hours'
);

-- 返回引用某条平台凭据的未删除仓库。
-- name: ListRepositoriesForGlobalCredential :many
SELECT
  repositories.tenant_id,
  tenants.slug AS tenant_slug,
  repositories.id AS repository_id,
  repositories.default_branch
FROM repositories
JOIN tenants ON tenants.id = repositories.tenant_id
WHERE repositories.global_credential_id = sqlc.arg(credential_id)
  AND repositories.deleted_at IS NULL
ORDER BY tenants.slug, repositories.id;

-- 写入一条平台拥有的加密凭据。
-- name: CreateGlobalCredential :one
INSERT INTO global_credentials (
  id, name, kind, ciphertext, nonce, key_version, fingerprint, created_by
) VALUES (
  sqlc.arg(id), sqlc.arg(name), sqlc.arg(kind), sqlc.arg(ciphertext),
  sqlc.arg(nonce), sqlc.arg(key_version), sqlc.arg(fingerprint), sqlc.arg(created_by)
)
RETURNING *;

-- 返回平台凭据的稳定分页结果。
-- name: ListGlobalCredentials :many
SELECT *
FROM global_credentials
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 统计全部平台凭据数量。
-- name: CountGlobalCredentials :one
SELECT count(*) FROM global_credentials;

-- 返回一条平台凭据。
-- name: GetGlobalCredential :one
SELECT * FROM global_credentials WHERE id = sqlc.arg(id);

-- 有条件地更新平台凭据名称并递增版本号。
-- name: UpdateGlobalCredentialMetadata :one
UPDATE global_credentials
SET name = sqlc.arg(name), revision = revision + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 统计引用某条平台凭据的有效仓库数量。
-- name: CountGlobalCredentialRepositories :one
SELECT count(*)
FROM repositories
WHERE global_credential_id = sqlc.arg(credential_id)
  AND deleted_at IS NULL;

-- 在删除平台凭据前清除有效和归档仓库的引用。
-- 只有有效仓库会记录需要重新认证的健康错误。
-- name: UnbindGlobalCredentialRepositories :exec
UPDATE repositories
SET
  global_credential_id = NULL,
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at),
  health = CASE
    WHEN deleted_at IS NULL THEN jsonb_set(
      COALESCE(health, '{}'::jsonb),
      '{lastError}',
      '{"class":"auth","message":"global credential was deleted"}'::jsonb,
      true
    )
    ELSE health
  END
WHERE global_credential_id = sqlc.arg(credential_id);

-- 完成引用和 ETag 检查后，删除一条平台凭据。
-- name: DeleteGlobalCredential :execrows
DELETE FROM global_credentials
WHERE id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision);

-- 有条件地替换平台加密秘密材料并递增版本号。
-- name: RotateGlobalCredentialSecret :one
UPDATE global_credentials
SET
  ciphertext = sqlc.arg(ciphertext),
  nonce = sqlc.arg(nonce),
  key_version = sqlc.arg(key_version),
  fingerprint = sqlc.arg(fingerprint),
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 返回租户认可的 SSH 主机身份稳定分页结果。
-- name: ListKnownHosts :many
SELECT *
FROM known_hosts
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY host, port, key_type, fingerprint, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 统计一个租户认可的主机身份数量。
-- name: CountKnownHosts :one
SELECT count(*) FROM known_hosts WHERE tenant_id = sqlc.arg(tenant_id);

-- 写入一条由服务端派生的已认可 SSH 主机身份。
-- name: CreateKnownHost :one
INSERT INTO known_hosts (
  tenant_id, id, host, port, key_type, public_key, fingerprint, source, created_by
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(host), sqlc.arg(port), sqlc.arg(key_type),
  sqlc.arg(public_key), sqlc.arg(fingerprint), sqlc.arg(source), sqlc.arg(created_by)
)
RETURNING *;
