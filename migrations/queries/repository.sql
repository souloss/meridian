-- CountRepositories returns active repository count and the tenant's frozen repository quota.
-- The quota is read from the tenant snapshot, never from a mutable platform default.
-- name: CountRepositories :one
SELECT
  (SELECT count(*)::bigint
   FROM repositories
   WHERE tenant_id = sqlc.arg(tenant_id)
     AND deleted_at IS NULL) AS current_count,
  COALESCE((SELECT (quota ->> 'maxRepositories')::bigint
            FROM tenants
            WHERE id = sqlc.arg(tenant_id)), 0)::bigint AS limit_count;

-- LockRepositoryQuota serializes repository creation against the tenant's repository quota.
-- The repository adapter holds this row lock while counting and inserting, so concurrent creates cannot oversubscribe a quota.
-- name: LockRepositoryQuota :one
SELECT COALESCE((quota ->> 'maxRepositories')::bigint, 0)::bigint AS limit_count
FROM tenants
WHERE id = sqlc.arg(tenant_id)
FOR UPDATE;

-- ResolveRepositoryCredential resolves one tenant-visible credential UUID to exactly one owning table.
-- Tenant credentials are filtered by the same visibility predicate as the credential list endpoint;
-- global credentials are selectable by every active member but remain platform-admin managed.
-- name: ResolveRepositoryCredential :one
SELECT c.id AS resolved_id, false AS is_global
FROM credentials AS c
WHERE c.tenant_id = sqlc.arg(tenant_id)
  AND c.id = sqlc.arg(credential_id)
  AND (
    c.created_by = sqlc.arg(user_id)
    OR c.shared_scope = 'tenant'
    OR (
      c.shared_scope = 'team'
      AND EXISTS (
        SELECT 1
        FROM team_members AS members
        JOIN credential_team_shares AS shares
          ON shares.tenant_id = members.tenant_id
         AND shares.team_id = members.team_id
         AND shares.credential_id = c.id
        WHERE members.tenant_id = c.tenant_id
          AND members.user_id = sqlc.arg(user_id)
      )
    )
  )
UNION ALL
SELECT global_credentials.id AS resolved_id, true AS is_global
FROM global_credentials
WHERE global_credentials.id = sqlc.arg(credential_id)
LIMIT 1;

-- ListRepositories returns active repositories in deterministic canonical URL and UUID order.
-- The query and all predicates retain the tenant boundary even when the search string is empty.
-- name: ListRepositories :many
SELECT *
FROM repositories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND deleted_at IS NULL
  AND (
    sqlc.arg(search_query)::text = ''
    OR url ILIKE '%' || sqlc.arg(search_query)::text || '%'
    OR canonical_url ILIKE '%' || sqlc.arg(search_query)::text || '%'
  )
ORDER BY canonical_url, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- CountListedRepositories returns the number of active repositories matching one tenant search.
-- name: CountListedRepositories :one
SELECT count(*)::bigint
FROM repositories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND deleted_at IS NULL
  AND (
    sqlc.arg(search_query)::text = ''
    OR url ILIKE '%' || sqlc.arg(search_query)::text || '%'
    OR canonical_url ILIKE '%' || sqlc.arg(search_query)::text || '%'
  );

-- GetRepository returns one active repository; soft-deleted rows intentionally appear absent.
-- name: GetRepository :one
SELECT *
FROM repositories
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- CreateRepository persists repository configuration and initializes an empty health summary.
-- URL fields are credential-free; credentials are referenced only by UUID foreign keys.
-- name: CreateRepository :one
INSERT INTO repositories (
  tenant_id, id, url, canonical_url, credential_id, global_credential_id,
  default_branch, branch_policy, fetch_config, sync_cron, note
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(url), sqlc.arg(canonical_url),
  sqlc.narg(credential_id), sqlc.narg(global_credential_id), sqlc.arg(default_branch),
  sqlc.arg(branch_policy), sqlc.arg(fetch_config), sqlc.narg(sync_cron), sqlc.narg(note)
)
RETURNING *;

-- UpdateRepository conditionally updates explicit repository fields and advances its revision.
-- Set flags preserve the distinction between omitted fields and explicit JSON null values.
-- name: UpdateRepository :one
UPDATE repositories
SET
  credential_id = CASE WHEN sqlc.arg(set_credential_id)::boolean THEN sqlc.narg(credential_id) ELSE credential_id END,
  global_credential_id = CASE WHEN sqlc.arg(set_global_credential_id)::boolean THEN sqlc.narg(global_credential_id) ELSE global_credential_id END,
  default_branch = COALESCE(sqlc.narg(default_branch), default_branch),
  branch_policy = COALESCE(sqlc.narg(branch_policy), branch_policy),
  fetch_config = COALESCE(sqlc.narg(fetch_config), fetch_config),
  sync_cron = CASE WHEN sqlc.arg(set_sync_cron)::boolean THEN sqlc.narg(sync_cron) ELSE sync_cron END,
  note = CASE WHEN sqlc.arg(set_note)::boolean THEN sqlc.narg(note) ELSE note END,
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- DeleteRepository soft-deletes a repository and makes its URL/branch reusable only per policy.
-- Historical job and audit rows remain tenant-scoped after this update.
-- name: DeleteRepository :execrows
UPDATE repositories
SET deleted_at = sqlc.arg(deleted_at), revision = revision + 1, updated_at = sqlc.arg(updated_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND deleted_at IS NULL
  AND revision = sqlc.arg(expected_revision);
