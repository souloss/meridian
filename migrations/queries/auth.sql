-- CreateUser inserts one global identity with an Argon2id PHC verifier and no plaintext password.
-- name: CreateUser :one
INSERT INTO users (
  id,
  username,
  password_hash,
  display_name,
  email,
  is_platform_admin
) VALUES (
  sqlc.arg(id),
  sqlc.arg(username),
  sqlc.arg(password_hash),
  sqlc.arg(display_name),
  sqlc.narg(email),
  sqlc.arg(is_platform_admin)
)
RETURNING *;

-- CreateDefaultUserPreferences creates the locale, theme, and view defaults required for a new identity.
-- name: CreateDefaultUserPreferences :one
INSERT INTO user_preferences (user_id)
VALUES (sqlc.arg(user_id))
RETURNING *;

-- GetUserByUsername returns the global identity matching the exact unique login name.
-- name: GetUserByUsername :one
SELECT *
FROM users
WHERE username = sqlc.arg(username);

-- GetUserByID returns the global identity matching the supplied UUID.
-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = sqlc.arg(id);

-- ListUsers returns a stable platform-admin page of identities without password or session secrets.
-- The optional search value is intentionally limited to username and display name.
-- name: ListUsers :many
SELECT id, username, display_name, email, status, is_platform_admin, revision, created_at, updated_at
FROM users
WHERE (
  sqlc.arg(search_query)::text = ''
  OR username ILIKE '%' || sqlc.arg(search_query)::text || '%'
  OR display_name ILIKE '%' || sqlc.arg(search_query)::text || '%'
)
ORDER BY username, id
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- CountUsers returns the number of identities matching one platform search.
-- name: CountUsers :one
SELECT count(*)::bigint
FROM users
WHERE (
  sqlc.arg(search_query)::text = ''
  OR username ILIKE '%' || sqlc.arg(search_query)::text || '%'
  OR display_name ILIKE '%' || sqlc.arg(search_query)::text || '%'
);

-- PromoteUserToPlatformAdmin grants platform control-plane privileges and advances the user revision.
-- name: PromoteUserToPlatformAdmin :one
UPDATE users
SET
  is_platform_admin = true,
  revision = revision + 1,
  updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
RETURNING *;

-- CreateSession persists keyed session and CSRF digests without storing either plaintext token.
-- name: CreateSession :one
INSERT INTO sessions (
  id,
  user_id,
  token_hash,
  csrf_hash,
  expires_at
) VALUES (
  sqlc.arg(id),
  sqlc.arg(user_id),
  sqlc.arg(token_hash),
  sqlc.arg(csrf_hash),
  sqlc.arg(expires_at)
)
RETURNING *;

-- GetSessionPrincipalByTokenHash authenticates one active browser session and active user at a caller-supplied instant.
-- name: GetSessionPrincipalByTokenHash :one
SELECT
  sessions.id AS session_id,
  sessions.user_id,
  sessions.csrf_hash,
  sessions.expires_at,
  users.username,
  users.display_name,
  users.email,
	users.status AS user_status,
	users.is_platform_admin,
	users.revision AS user_revision,
	users.created_at AS user_created_at,
	users.updated_at AS user_updated_at
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = sqlc.arg(token_hash)
  AND sessions.revoked_at IS NULL
  AND sessions.expires_at > sqlc.arg(authenticated_at)
  AND users.status = 'active';

-- TouchSession records the latest accepted request time for a non-revoked browser session.
-- name: TouchSession :exec
UPDATE sessions
SET
  last_seen_at = sqlc.arg(seen_at),
  updated_at = sqlc.arg(seen_at)
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL;

-- RotateSessionCSRFHash replaces the keyed CSRF digest for one active browser session.
-- name: RotateSessionCSRFHash :execrows
UPDATE sessions
SET
  csrf_hash = sqlc.arg(csrf_hash),
  updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL
  AND expires_at > sqlc.arg(updated_at);

-- RevokeSession atomically revokes one active browser session and reports whether a row changed.
-- name: RevokeSession :execrows
UPDATE sessions
SET
  revoked_at = sqlc.arg(revoked_at),
  updated_at = sqlc.arg(revoked_at)
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL;

-- CreateAPIToken persists tenant-scoped PAT metadata and a keyed token digest without storing plaintext.
-- name: CreateAPIToken :one
INSERT INTO api_tokens (
  tenant_id,
  id,
  user_id,
  name,
  token_hash,
  scopes,
  expires_at
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(id),
  sqlc.arg(user_id),
  sqlc.arg(name),
  sqlc.arg(token_hash),
  sqlc.arg(scopes),
  sqlc.narg(expires_at)
)
RETURNING *;

-- GetAPITokenPrincipalByTokenHash authenticates one active PAT whose user, membership, and tenant remain active.
-- name: GetAPITokenPrincipalByTokenHash :one
SELECT
  api_tokens.tenant_id,
  api_tokens.id AS token_id,
  api_tokens.user_id,
  api_tokens.scopes,
  api_tokens.expires_at,
  users.username,
  users.display_name,
	users.email,
	users.status AS user_status,
	users.revision AS user_revision,
	users.created_at AS user_created_at,
	users.updated_at AS user_updated_at,
	tenant_members.role,
	tenants.slug AS tenant_slug
FROM api_tokens
JOIN users ON users.id = api_tokens.user_id
JOIN tenant_members
  ON tenant_members.tenant_id = api_tokens.tenant_id
 AND tenant_members.user_id = api_tokens.user_id
JOIN tenants ON tenants.id = api_tokens.tenant_id
WHERE api_tokens.token_hash = sqlc.arg(token_hash)
  AND api_tokens.revoked_at IS NULL
  AND (api_tokens.expires_at IS NULL OR api_tokens.expires_at > sqlc.arg(authenticated_at))
  AND users.status = 'active'
  AND tenants.status = 'active';

-- TouchAPIToken records the latest successful use of a non-revoked tenant PAT.
-- name: TouchAPIToken :exec
UPDATE api_tokens
SET
  last_used_at = sqlc.arg(used_at),
  updated_at = sqlc.arg(used_at)
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revoked_at IS NULL;

-- CountAPITokensByUser returns the total PAT metadata rows owned by one user inside one tenant.
-- name: CountAPITokensByUser :one
SELECT count(*)
FROM api_tokens
WHERE tenant_id = sqlc.arg(tenant_id)
  AND user_id = sqlc.arg(user_id);

-- ListAPITokensByUser returns one stable page of PAT metadata for one user inside one explicit tenant boundary.
-- name: ListAPITokensByUser :many
SELECT *
FROM api_tokens
WHERE tenant_id = sqlc.arg(tenant_id)
  AND user_id = sqlc.arg(user_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- RevokeAPIToken idempotently revokes a PAT owned by one user in one tenant and returns its identifier.
-- Returning an already-revoked matching row preserves idempotency while an absent or foreign row remains not found.
-- name: RevokeAPIToken :one
UPDATE api_tokens
SET
  revoked_at = COALESCE(revoked_at, sqlc.arg(revoked_at)),
  updated_at = CASE WHEN revoked_at IS NULL THEN sqlc.arg(revoked_at) ELSE updated_at END
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
RETURNING id;
