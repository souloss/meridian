-- CreateTenant inserts one tenant with explicit quota and settings snapshots copied from platform defaults.
-- name: CreateTenant :one
INSERT INTO tenants (
  id,
  slug,
  display_name,
  quota,
  settings
) VALUES (
  sqlc.arg(id),
  sqlc.arg(slug),
  sqlc.arg(display_name),
  sqlc.arg(quota),
  sqlc.arg(settings)
)
RETURNING *;

-- GetPlatformSettingsForTenantCreate returns the singleton JSON defaults copied atomically into a new tenant.
-- name: GetPlatformSettingsForTenantCreate :one
SELECT settings
FROM platform_settings
WHERE id = 'default';

-- GetTenantBySlug returns a tenant in any lifecycle state for platform administration.
-- name: GetTenantBySlug :one
SELECT *
FROM tenants
WHERE slug = sqlc.arg(slug);

-- GetActiveTenantBySlug returns only an active tenant for tenant-scoped business access.
-- name: GetActiveTenantBySlug :one
SELECT *
FROM tenants
WHERE slug = sqlc.arg(slug)
  AND status = 'active';

-- UpsertTenantMember creates or replaces a tenant role assignment and records the caller-supplied update time.
-- name: UpsertTenantMember :one
INSERT INTO tenant_members (
  tenant_id,
  user_id,
  role
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(user_id),
  sqlc.arg(role)
)
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET
  role = EXCLUDED.role,
  updated_at = sqlc.arg(updated_at)
RETURNING *;

-- GetActiveTenantMembership returns one active tenant membership without revealing disabled tenant records.
-- name: GetActiveTenantMembership :one
SELECT
  tenants.id AS tenant_id,
  tenants.slug AS tenant_slug,
  tenants.display_name AS tenant_display_name,
  tenant_members.role
FROM tenant_members
JOIN tenants ON tenants.id = tenant_members.tenant_id
WHERE tenant_members.user_id = sqlc.arg(user_id)
  AND tenants.slug = sqlc.arg(tenant_slug)
  AND tenants.status = 'active';

-- ListActiveTenantMemberships returns a user's active tenant memberships in stable slug and UUID order.
-- name: ListActiveTenantMemberships :many
SELECT
  tenants.id AS tenant_id,
  tenants.slug AS tenant_slug,
  tenants.display_name AS tenant_display_name,
  tenant_members.role
FROM tenant_members
JOIN tenants ON tenants.id = tenant_members.tenant_id
WHERE tenant_members.user_id = sqlc.arg(user_id)
  AND tenants.status = 'active'
ORDER BY tenants.slug, tenants.id;
