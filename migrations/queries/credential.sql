-- CreateCredential inserts one tenant-owned encrypted credential and returns metadata plus ciphertext.
-- Secret plaintext is never accepted by SQL; the service supplies the encrypted projection only.
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

-- ListTenantCredentials returns credentials visible to one user inside one active tenant.
-- Team visibility is evaluated by a same-tenant team membership predicate. Global credentials are
-- appended as tenant-visible records with is_global=true and a tenant-wide sharing projection.
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

-- CountTenantCredentials counts visible tenant-owned and global credentials for one tenant member.
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

-- GetTenantCredential returns one visible tenant credential and its team-share identifiers.
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

-- GetTenantCredentialForMutation returns one tenant credential without visibility filtering.
-- The service has already authorized the tenant operation; this query preserves a 404/412 distinction.
-- name: GetTenantCredentialForMutation :one
SELECT *
FROM credentials
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- UpdateCredentialMetadata conditionally updates tenant credential metadata and advances its revision.
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

-- ReplaceCredentialTeamShares removes and recreates the complete team-share projection in one transaction.
-- name: ReplaceCredentialTeamShares :exec
DELETE FROM credential_team_shares
WHERE tenant_id = sqlc.arg(tenant_id)
  AND credential_id = sqlc.arg(credential_id);

-- AddCredentialTeamShare grants one same-tenant team visibility entry.
-- name: AddCredentialTeamShare :exec
INSERT INTO credential_team_shares (tenant_id, credential_id, team_id)
VALUES (sqlc.arg(tenant_id), sqlc.arg(credential_id), sqlc.arg(team_id));

-- ListCredentialTeamShares returns the complete ordered team-share set for one tenant credential.
-- name: ListCredentialTeamShares :many
SELECT team_id
FROM credential_team_shares
WHERE tenant_id = sqlc.arg(tenant_id)
  AND credential_id = sqlc.arg(credential_id)
ORDER BY team_id;

-- CountCredentialRepositories counts active repositories referencing a tenant credential.
-- name: CountCredentialRepositories :one
SELECT count(*)
FROM repositories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND credential_id = sqlc.arg(credential_id)
  AND deleted_at IS NULL;

-- UnbindCredentialRepositories clears tenant credential references for forced deletion.
-- name: UnbindCredentialRepositories :exec
UPDATE repositories
SET
  credential_id = NULL,
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at),
  health = jsonb_set(COALESCE(health, '{}'::jsonb), '{lastError}', '{"class":"auth_required","message":"credential was deleted"}'::jsonb, true)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND credential_id = sqlc.arg(credential_id)
  AND deleted_at IS NULL;

-- DeleteCredential removes a tenant credential after the caller has applied reference and ETag checks.
-- name: DeleteCredential :execrows
DELETE FROM credentials
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision);

-- RotateCredentialSecret conditionally replaces encrypted secret material and advances its revision.
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

-- ListRepositoriesForCredential returns non-deleted repository references in contract response order.
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

-- LockLatestCredentialSyncJob serializes credential-rotation deduplication for one repository branch.
-- A pending or running row is reused; terminal rows advance active_generation for new work.
-- name: LockLatestCredentialSyncJob :one
SELECT id, tenant_id, status, active_generation
FROM jobs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND dedupe_key = sqlc.arg(dedupe_key)
ORDER BY active_generation DESC
LIMIT 1
FOR UPDATE;

-- CreateCredentialSyncJob records one durable default-branch repository sync request.
-- The input contains only the non-secret credential identifier and rotation reason.
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

-- LockCredentialRotationIdempotency serializes one rotation key across concurrent HTTP requests.
-- The lock key is derived from the authenticated principal and operation, never from plaintext secrets.
-- name: LockCredentialRotationIdempotency :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key), 0));

-- GetCredentialRotationIdempotency returns a retained tenant rotation replay record, including its expiry.
-- name: GetCredentialRotationIdempotency :one
SELECT request_hash, response_body, expires_at
FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'rotateCredential'
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- DeleteCredentialRotationIdempotency removes an expired tenant rotation replay before reuse.
-- name: DeleteCredentialRotationIdempotency :exec
DELETE FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'rotateCredential'
  AND idempotency_key = sqlc.arg(idempotency_key);

-- CreateCredentialRotationIdempotency stores a safe tenant rotation response for 24-hour exact replay.
-- Ciphertext, nonces, and every other secret-bearing field are excluded from response_body by the adapter.
-- name: CreateCredentialRotationIdempotency :exec
INSERT INTO idempotency_records (
  tenant_id, principal_type, principal_id, operation_id, idempotency_key,
  request_hash, response_status, response_body, expires_at
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(principal_type), sqlc.arg(principal_id), 'rotateCredential', sqlc.arg(idempotency_key),
  sqlc.arg(request_hash), 200, sqlc.arg(response_body)::jsonb, now() + interval '24 hours'
);

-- GetGlobalCredentialRotationIdempotency returns a retained platform rotation replay record.
-- name: GetGlobalCredentialRotationIdempotency :one
SELECT request_hash, response_body, expires_at
FROM global_idempotency_records
WHERE context_type = 'platform'
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'rotateGlobalCredential'
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- DeleteGlobalCredentialRotationIdempotency removes an expired platform rotation replay before reuse.
-- name: DeleteGlobalCredentialRotationIdempotency :exec
DELETE FROM global_idempotency_records
WHERE context_type = 'platform'
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'rotateGlobalCredential'
  AND idempotency_key = sqlc.arg(idempotency_key);

-- CreateGlobalCredentialRotationIdempotency stores a safe platform rotation response for 24-hour exact replay.
-- Ciphertext, nonces, and every other secret-bearing field are excluded from response_body by the adapter.
-- name: CreateGlobalCredentialRotationIdempotency :exec
INSERT INTO global_idempotency_records (
  context_type, principal_type, principal_id, operation_id, idempotency_key,
  request_hash, response_status, response_body, expires_at
) VALUES (
  'platform', sqlc.arg(principal_type), sqlc.arg(principal_id), 'rotateGlobalCredential', sqlc.arg(idempotency_key),
  sqlc.arg(request_hash), 200, sqlc.arg(response_body)::jsonb, now() + interval '24 hours'
);

-- ListRepositoriesForGlobalCredential returns non-deleted repository references for a platform credential.
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

-- CreateGlobalCredential inserts one platform-owned encrypted credential.
-- name: CreateGlobalCredential :one
INSERT INTO global_credentials (
  id, name, kind, ciphertext, nonce, key_version, fingerprint, created_by
) VALUES (
  sqlc.arg(id), sqlc.arg(name), sqlc.arg(kind), sqlc.arg(ciphertext),
  sqlc.arg(nonce), sqlc.arg(key_version), sqlc.arg(fingerprint), sqlc.arg(created_by)
)
RETURNING *;

-- ListGlobalCredentials returns one stable page of platform-owned credentials.
-- name: ListGlobalCredentials :many
SELECT *
FROM global_credentials
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- CountGlobalCredentials counts all platform-owned credentials.
-- name: CountGlobalCredentials :one
SELECT count(*) FROM global_credentials;

-- GetGlobalCredential returns one platform-owned credential.
-- name: GetGlobalCredential :one
SELECT * FROM global_credentials WHERE id = sqlc.arg(id);

-- UpdateGlobalCredentialMetadata conditionally updates a global credential name and advances its revision.
-- name: UpdateGlobalCredentialMetadata :one
UPDATE global_credentials
SET name = sqlc.arg(name), revision = revision + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- CountGlobalCredentialRepositories counts active repositories referencing a global credential.
-- name: CountGlobalCredentialRepositories :one
SELECT count(*)
FROM repositories
WHERE global_credential_id = sqlc.arg(credential_id)
  AND deleted_at IS NULL;

-- UnbindGlobalCredentialRepositories clears global credential references and marks authentication required.
-- name: UnbindGlobalCredentialRepositories :exec
UPDATE repositories
SET
  global_credential_id = NULL,
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at),
  health = jsonb_set(COALESCE(health, '{}'::jsonb), '{lastError}', '{"class":"auth_required","message":"global credential was deleted"}'::jsonb, true)
WHERE global_credential_id = sqlc.arg(credential_id)
  AND deleted_at IS NULL;

-- DeleteGlobalCredential removes one platform credential after reference and ETag checks.
-- name: DeleteGlobalCredential :execrows
DELETE FROM global_credentials
WHERE id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision);

-- RotateGlobalCredentialSecret conditionally replaces global encrypted secret material and advances revision.
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

-- ListKnownHosts returns one stable page of tenant-approved SSH host identities.
-- name: ListKnownHosts :many
SELECT *
FROM known_hosts
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY host, port, key_type, fingerprint, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- CountKnownHosts counts approved host identities in one tenant.
-- name: CountKnownHosts :one
SELECT count(*) FROM known_hosts WHERE tenant_id = sqlc.arg(tenant_id);

-- CreateKnownHost inserts a server-derived approved SSH host identity.
-- name: CreateKnownHost :one
INSERT INTO known_hosts (
  tenant_id, id, host, port, key_type, public_key, fingerprint, source, created_by
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(host), sqlc.arg(port), sqlc.arg(key_type),
  sqlc.arg(public_key), sqlc.arg(fingerprint), sqlc.arg(source), sqlc.arg(created_by)
)
RETURNING *;
