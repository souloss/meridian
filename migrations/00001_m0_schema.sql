-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;

CREATE TABLE platform_settings (
  id text PRIMARY KEY CHECK (id = 'default'),
  settings jsonb NOT NULL,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE blobs (
  blob_digest text PRIMARY KEY,
  storage_key text NOT NULL UNIQUE,
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  media_type text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenants (
  id uuid PRIMARY KEY,
  slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
  display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 128),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  quota jsonb NOT NULL,
  settings jsonb NOT NULL,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenant_blob_refs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  blob_digest text NOT NULL REFERENCES blobs(blob_digest) ON DELETE RESTRICT,
  ref_count bigint NOT NULL DEFAULT 0 CHECK (ref_count >= 0),
  last_referenced_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, blob_digest)
);

CREATE TABLE users (
  id uuid PRIMARY KEY,
  username text NOT NULL UNIQUE CHECK (length(username) BETWEEN 1 AND 128),
  password_hash text NOT NULL,
  display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 128),
  email text UNIQUE,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  is_platform_admin boolean NOT NULL DEFAULT false,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  csrf_hash bytea NOT NULL,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  last_seen_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_expiry_idx ON sessions (user_id, expires_at);
CREATE INDEX sessions_expiry_idx ON sessions (expires_at);

CREATE TABLE tenant_members (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  role text NOT NULL CHECK (role IN ('tenant_admin', 'maintainer', 'viewer')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id)
);

CREATE TABLE teams (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  slug text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
  display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 128),
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, slug)
);

CREATE TABLE team_members (
  tenant_id uuid NOT NULL,
  team_id uuid NOT NULL,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, team_id, user_id),
  FOREIGN KEY (tenant_id, team_id) REFERENCES teams (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_members (tenant_id, user_id) ON DELETE CASCADE
);

CREATE TABLE api_tokens (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  user_id uuid NOT NULL,
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  token_hash bytea NOT NULL,
  scopes text[] NOT NULL CHECK (cardinality(scopes) > 0),
  expires_at timestamptz,
  last_used_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (token_hash),
  FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_members (tenant_id, user_id) ON DELETE CASCADE
);

CREATE TABLE idempotency_records (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  principal_type text NOT NULL,
  principal_id uuid NOT NULL,
  operation_id text NOT NULL,
  idempotency_key uuid NOT NULL,
  request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
  response_status integer NOT NULL CHECK (response_status BETWEEN 100 AND 599),
  response_body jsonb NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, principal_type, principal_id, operation_id, idempotency_key)
);

CREATE TABLE global_idempotency_records (
  context_type text NOT NULL CHECK (context_type IN ('identity', 'platform')),
  principal_type text NOT NULL,
  principal_id uuid NOT NULL,
  operation_id text NOT NULL,
  idempotency_key uuid NOT NULL,
  request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
  response_status integer NOT NULL CHECK (response_status BETWEEN 100 AND 599),
  response_body jsonb NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (context_type, principal_type, principal_id, operation_id, idempotency_key)
);

CREATE TABLE user_preferences (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  locale text NOT NULL DEFAULT 'zh-CN' CHECK (locale IN ('zh-CN', 'en')),
  theme text NOT NULL DEFAULT 'system' CHECK (theme IN ('light', 'dark', 'system')),
  default_views jsonb NOT NULL DEFAULT '{}'::jsonb,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE credentials (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  kind text NOT NULL CHECK (kind IN ('ssh_key', 'http_token')),
  ciphertext bytea NOT NULL,
  nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
  key_version integer NOT NULL CHECK (key_version > 0),
  fingerprint text NOT NULL,
  shared_scope text NOT NULL DEFAULT 'private' CHECK (shared_scope IN ('private', 'team', 'tenant')),
  created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  last_used_at timestamptz,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, name)
);

CREATE TABLE credential_team_shares (
  tenant_id uuid NOT NULL,
  credential_id uuid NOT NULL,
  team_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, credential_id, team_id),
  FOREIGN KEY (tenant_id, credential_id) REFERENCES credentials (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, team_id) REFERENCES teams (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE global_credentials (
  id uuid PRIMARY KEY,
  name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 64),
  kind text NOT NULL CHECK (kind IN ('ssh_key', 'http_token')),
  ciphertext bytea NOT NULL,
  nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
  key_version integer NOT NULL CHECK (key_version > 0),
  fingerprint text NOT NULL,
  created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  last_used_at timestamptz,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE known_hosts (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  host text NOT NULL CHECK (length(host) BETWEEN 1 AND 255),
  port integer NOT NULL DEFAULT 22 CHECK (port BETWEEN 1 AND 65535),
  key_type text NOT NULL CHECK (key_type IN ('ssh-ed25519', 'ssh-rsa')),
  public_key bytea NOT NULL,
  fingerprint text NOT NULL,
  source text NOT NULL CHECK (source IN ('manual', 'accept_new')),
  created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, host, port, key_type, fingerprint)
);

CREATE TABLE repositories (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  url text NOT NULL,
  canonical_url text NOT NULL,
  credential_id uuid,
  global_credential_id uuid REFERENCES global_credentials(id) ON DELETE RESTRICT,
  default_branch text NOT NULL DEFAULT 'main',
  branch_policy jsonb NOT NULL,
  fetch_config jsonb NOT NULL,
  sync_cron text,
  note text,
  webhook_secret_hash bytea,
  health jsonb NOT NULL DEFAULT '{}'::jsonb,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, canonical_url, default_branch),
  CHECK (num_nonnulls(credential_id, global_credential_id) <= 1),
  FOREIGN KEY (tenant_id, credential_id) REFERENCES credentials (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE jobs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  retry_of_job_id uuid,
  river_job_id bigint,
  type text NOT NULL CHECK (type IN ('tenant.delete', 'repo.sync', 'repo.discover', 'asset.produce', 'asset.merge', 'asset.index', 'asset.ai_generate', 'asset.reindex', 'diff.run', 'outbox.dispatch', 'workspace.gc', 'blob.gc', 'retention.cleanup')),
  scope_type text NOT NULL CHECK (scope_type IN ('tenant', 'repository', 'service', 'source', 'track', 'version', 'asset', 'diff', 'system')),
  scope_id uuid,
  ref_type text CHECK (ref_type IN ('branch', 'tag')),
  ref_name text,
  trigger text NOT NULL CHECK (trigger IN ('manual', 'schedule', 'webhook', 'api', 'cli', 'system', 'retry', 'credential-rotated')),
  input jsonb NOT NULL DEFAULT '{}'::jsonb,
  result jsonb,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'succeeded', 'succeeded_with_warnings', 'failed', 'outcome_unknown', 'cancelled')),
  stage text CHECK (stage IN ('resolve', 'discover', 'extract', 'merge', 'normalize', 'index')),
  attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
  max_attempts integer NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
  next_attempt_at timestamptz,
  dedupe_key text NOT NULL,
  active_generation bigint NOT NULL DEFAULT 1 CHECK (active_generation > 0),
  dirty boolean NOT NULL DEFAULT false,
  replay_safe boolean NOT NULL DEFAULT false,
  error jsonb,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, dedupe_key, active_generation),
  FOREIGN KEY (tenant_id, retry_of_job_id) REFERENCES jobs (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX jobs_status_created_idx ON jobs (tenant_id, status, created_at);
CREATE INDEX jobs_scope_created_idx ON jobs (tenant_id, scope_type, scope_id, created_at);

CREATE TABLE job_stage_logs (
  tenant_id uuid NOT NULL,
  job_id uuid NOT NULL,
  sequence bigint NOT NULL CHECK (sequence > 0),
  stage text CHECK (stage IN ('resolve', 'discover', 'extract', 'merge', 'normalize', 'index')),
  level text NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error')),
  message text NOT NULL,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, job_id, sequence),
  FOREIGN KEY (tenant_id, job_id) REFERENCES jobs (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE notification_channels (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  type text NOT NULL CHECK (type IN ('in_app', 'webhook', 'email')),
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  encrypted_config bytea NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);

CREATE TABLE notify_outbox (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  event_id uuid NOT NULL,
  event_type text NOT NULL,
  aggregate_id uuid NOT NULL,
  aggregate_version bigint NOT NULL CHECK (aggregate_version > 0),
  payload jsonb NOT NULL,
  channel_id uuid NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivering', 'delivered', 'failed')),
  retry_count integer NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, event_id, channel_id),
  FOREIGN KEY (tenant_id, channel_id) REFERENCES notification_channels (tenant_id, id) ON DELETE RESTRICT
);

CREATE TABLE audit_logs (
  id uuid PRIMARY KEY,
  tenant_id uuid REFERENCES tenants(id) ON DELETE SET NULL,
  actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
  actor_type text NOT NULL CHECK (actor_type IN ('user', 'system', 'anonymous')),
  action text NOT NULL,
  target_type text NOT NULL,
  target_id uuid,
  detail jsonb NOT NULL DEFAULT '{}'::jsonb,
  ip inet,
  request_id uuid,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_tenant_created_idx ON audit_logs (tenant_id, created_at);
CREATE INDEX audit_logs_actor_created_idx ON audit_logs (actor_id, created_at);
CREATE INDEX audit_logs_target_created_idx ON audit_logs (target_type, target_id, created_at);

INSERT INTO platform_settings (id, settings)
VALUES ('default', '{"defaultQuota":{"maxRepositories":50,"maxServices":200,"maxStorageBytes":10737418240,"maxCollectConcurrency":4},"defaultTenantSettings":{"externalRevisionTrustMode":"review_required","autoPublish":false,"defaultLocale":"zh-CN","defaultAiProducerProfileId":null,"retention":{"jobLogsDays":30,"archivedRevisionsDays":90}},"defaultViewOverrides":{},"defaultNotificationChannels":[]}'::jsonb)
ON CONFLICT (id) DO NOTHING;

COMMENT ON TABLE platform_settings IS 'Singleton platform defaults copied into each tenant at creation time.';
COMMENT ON COLUMN platform_settings.id IS 'Stable singleton key; the only allowed value is default.';
COMMENT ON COLUMN platform_settings.settings IS 'JSON object containing default quota, tenant settings, view overrides, and notification channel templates.';
COMMENT ON COLUMN platform_settings.revision IS 'Monotonic optimistic-concurrency revision used to construct the HTTP ETag.';
COMMENT ON COLUMN platform_settings.created_at IS 'UTC transaction timestamp when the singleton row was created.';
COMMENT ON COLUMN platform_settings.updated_at IS 'UTC transaction timestamp of the latest settings update.';

COMMENT ON TABLE blobs IS 'Global content-addressed metadata for immutable binary objects stored outside PostgreSQL.';
COMMENT ON COLUMN blobs.blob_digest IS 'Lowercase SHA-256 digest that uniquely identifies the blob content.';
COMMENT ON COLUMN blobs.storage_key IS 'Unique opaque key used by the configured blob storage driver.';
COMMENT ON COLUMN blobs.size_bytes IS 'Exact uncompressed blob size in bytes.';
COMMENT ON COLUMN blobs.media_type IS 'IANA media type recorded when the blob was accepted.';
COMMENT ON COLUMN blobs.created_at IS 'UTC transaction timestamp when the blob metadata was first inserted.';

COMMENT ON TABLE tenant_blob_refs IS 'Per-tenant reference counts that authorize blob access and support garbage collection.';
COMMENT ON COLUMN tenant_blob_refs.tenant_id IS 'Tenant that owns the logical references to this blob.';
COMMENT ON COLUMN tenant_blob_refs.blob_digest IS 'SHA-256 digest of the referenced global blob.';
COMMENT ON COLUMN tenant_blob_refs.ref_count IS 'Number of live tenant-owned rows that reference the blob.';
COMMENT ON COLUMN tenant_blob_refs.last_referenced_at IS 'UTC timestamp when the tenant most recently added or refreshed a reference.';

COMMENT ON TABLE tenants IS 'Tenant control-plane records and copied platform defaults.';
COMMENT ON COLUMN tenants.id IS 'Application-generated UUID v7 identifying the tenant.';
COMMENT ON COLUMN tenants.slug IS 'Globally unique lowercase slug used in tenant-scoped URLs.';
COMMENT ON COLUMN tenants.display_name IS 'Human-readable tenant name shown in the user interface.';
COMMENT ON COLUMN tenants.status IS 'Tenant access state: active or disabled, sourced from the domain contract.';
COMMENT ON COLUMN tenants.quota IS 'Tenant quota snapshot copied from platform defaults at creation.';
COMMENT ON COLUMN tenants.settings IS 'Tenant runtime settings snapshot copied from platform defaults at creation.';
COMMENT ON COLUMN tenants.revision IS 'Monotonic optimistic-concurrency revision used to construct the HTTP ETag.';
COMMENT ON COLUMN tenants.created_at IS 'UTC transaction timestamp when the tenant was created.';
COMMENT ON COLUMN tenants.updated_at IS 'UTC transaction timestamp of the latest tenant update.';

COMMENT ON TABLE users IS 'Global user identities that may hold memberships in multiple tenants.';
COMMENT ON COLUMN users.id IS 'Application-generated UUID v7 identifying the user.';
COMMENT ON COLUMN users.username IS 'Globally unique login name.';
COMMENT ON COLUMN users.password_hash IS 'Argon2id PHC password verifier; plaintext passwords are never stored.';
COMMENT ON COLUMN users.display_name IS 'Human-readable name displayed to other authorized users.';
COMMENT ON COLUMN users.email IS 'Optional globally unique email address.';
COMMENT ON COLUMN users.status IS 'Authentication state: active or disabled.';
COMMENT ON COLUMN users.is_platform_admin IS 'Whether the user has platform control-plane privileges; it does not grant tenant data access.';
COMMENT ON COLUMN users.revision IS 'Monotonic optimistic-concurrency revision used to construct the HTTP ETag.';
COMMENT ON COLUMN users.created_at IS 'UTC transaction timestamp when the user was created.';
COMMENT ON COLUMN users.updated_at IS 'UTC transaction timestamp of the latest user update.';

COMMENT ON TABLE sessions IS 'Browser authentication sessions storing only keyed token and CSRF digests.';
COMMENT ON COLUMN sessions.id IS 'Application-generated UUID v7 identifying the browser session.';
COMMENT ON COLUMN sessions.user_id IS 'Global user authenticated by the session.';
COMMENT ON COLUMN sessions.token_hash IS 'HMAC-SHA-256 digest of the opaque session token.';
COMMENT ON COLUMN sessions.csrf_hash IS 'HMAC-SHA-256 digest of the CSRF token paired with the session.';
COMMENT ON COLUMN sessions.expires_at IS 'UTC instant after which the session is invalid.';
COMMENT ON COLUMN sessions.revoked_at IS 'UTC instant when the session was explicitly revoked, or null while active.';
COMMENT ON COLUMN sessions.last_seen_at IS 'UTC instant of the latest accepted request, or null before first use.';
COMMENT ON COLUMN sessions.created_at IS 'UTC transaction timestamp when the session was created.';
COMMENT ON COLUMN sessions.updated_at IS 'UTC transaction timestamp of the latest session metadata update.';

COMMENT ON TABLE tenant_members IS 'Active role assignment joining a global user to a tenant.';
COMMENT ON COLUMN tenant_members.tenant_id IS 'Tenant in which the membership grants permissions.';
COMMENT ON COLUMN tenant_members.user_id IS 'Global user receiving the tenant role.';
COMMENT ON COLUMN tenant_members.role IS 'Tenant role: tenant_admin, maintainer, or viewer.';
COMMENT ON COLUMN tenant_members.created_at IS 'UTC transaction timestamp when membership was granted.';
COMMENT ON COLUMN tenant_members.updated_at IS 'UTC transaction timestamp of the latest role change.';

COMMENT ON TABLE teams IS 'Tenant-scoped groups used for grants and credential sharing.';
COMMENT ON COLUMN teams.tenant_id IS 'Owning tenant and first component of every team identity.';
COMMENT ON COLUMN teams.id IS 'Application-generated UUID v7 identifying the team within its tenant.';
COMMENT ON COLUMN teams.slug IS 'Tenant-unique lowercase team slug.';
COMMENT ON COLUMN teams.display_name IS 'Human-readable team name.';
COMMENT ON COLUMN teams.revision IS 'Monotonic optimistic-concurrency revision used to construct the HTTP ETag.';
COMMENT ON COLUMN teams.created_at IS 'UTC transaction timestamp when the team was created.';
COMMENT ON COLUMN teams.updated_at IS 'UTC transaction timestamp of the latest team update.';

COMMENT ON TABLE team_members IS 'Tenant-enforced membership of users in teams.';
COMMENT ON COLUMN team_members.tenant_id IS 'Owning tenant shared by the team and user membership foreign keys.';
COMMENT ON COLUMN team_members.team_id IS 'Tenant-scoped team receiving the member.';
COMMENT ON COLUMN team_members.user_id IS 'User who must already be a member of the same tenant.';
COMMENT ON COLUMN team_members.created_at IS 'UTC transaction timestamp when the user joined the team.';

COMMENT ON TABLE api_tokens IS 'Tenant-scoped personal access tokens stored only as keyed digests.';
COMMENT ON COLUMN api_tokens.tenant_id IS 'Tenant to which the token and all of its scopes are restricted.';
COMMENT ON COLUMN api_tokens.id IS 'Application-generated UUID v7 identifying token metadata.';
COMMENT ON COLUMN api_tokens.user_id IS 'Same-tenant user who owns the token.';
COMMENT ON COLUMN api_tokens.name IS 'User-provided label for identifying the token without revealing it.';
COMMENT ON COLUMN api_tokens.token_hash IS 'Globally unique HMAC-SHA-256 digest of the PAT secret.';
COMMENT ON COLUMN api_tokens.scopes IS 'Deduplicated set of allowed PAT scopes from the domain contract.';
COMMENT ON COLUMN api_tokens.expires_at IS 'Optional UTC expiry instant; null means no scheduled expiry.';
COMMENT ON COLUMN api_tokens.last_used_at IS 'UTC instant of the latest successful authentication, or null if unused.';
COMMENT ON COLUMN api_tokens.revoked_at IS 'UTC instant of explicit revocation, or null while not revoked.';
COMMENT ON COLUMN api_tokens.created_at IS 'UTC transaction timestamp when token metadata was created.';
COMMENT ON COLUMN api_tokens.updated_at IS 'UTC transaction timestamp of the latest token metadata update.';

COMMENT ON TABLE idempotency_records IS 'Completed tenant-scoped idempotent request results retained for exact replay.';
COMMENT ON COLUMN idempotency_records.tenant_id IS 'Tenant forming the outer boundary of the replay identity.';
COMMENT ON COLUMN idempotency_records.principal_type IS 'Authenticated principal category used in the replay identity.';
COMMENT ON COLUMN idempotency_records.principal_id IS 'Identifier of the authenticated principal used in the replay identity.';
COMMENT ON COLUMN idempotency_records.operation_id IS 'Exact OpenAPI operationId protected by the idempotency key.';
COMMENT ON COLUMN idempotency_records.idempotency_key IS 'Client-supplied UUID that identifies one semantic request.';
COMMENT ON COLUMN idempotency_records.request_hash IS 'Raw 32-byte SHA-256 digest of the RFC 8785 canonical request object.';
COMMENT ON COLUMN idempotency_records.response_status IS 'Original HTTP status code returned by the completed request.';
COMMENT ON COLUMN idempotency_records.response_body IS 'Original JSON response value returned during exact replay.';
COMMENT ON COLUMN idempotency_records.expires_at IS 'UTC instant after which the replay record may be removed.';
COMMENT ON COLUMN idempotency_records.created_at IS 'UTC transaction timestamp when the winning request committed.';

COMMENT ON TABLE global_idempotency_records IS 'Completed identity or platform scoped idempotent results retained for exact replay.';
COMMENT ON COLUMN global_idempotency_records.context_type IS 'Replay context boundary: identity or platform.';
COMMENT ON COLUMN global_idempotency_records.principal_type IS 'Authenticated principal category used in the replay identity.';
COMMENT ON COLUMN global_idempotency_records.principal_id IS 'Identifier of the authenticated principal used in the replay identity.';
COMMENT ON COLUMN global_idempotency_records.operation_id IS 'Exact OpenAPI operationId protected by the idempotency key.';
COMMENT ON COLUMN global_idempotency_records.idempotency_key IS 'Client-supplied UUID that identifies one semantic request.';
COMMENT ON COLUMN global_idempotency_records.request_hash IS 'Raw 32-byte SHA-256 digest of the RFC 8785 canonical request object.';
COMMENT ON COLUMN global_idempotency_records.response_status IS 'Original HTTP status code returned by the completed request.';
COMMENT ON COLUMN global_idempotency_records.response_body IS 'Original JSON response value returned during exact replay.';
COMMENT ON COLUMN global_idempotency_records.expires_at IS 'UTC instant after which the replay record may be removed.';
COMMENT ON COLUMN global_idempotency_records.created_at IS 'UTC transaction timestamp when the winning request committed.';

COMMENT ON TABLE user_preferences IS 'Per-user locale, theme, and default-view preferences created atomically with the user.';
COMMENT ON COLUMN user_preferences.user_id IS 'Global user to whom these preferences belong.';
COMMENT ON COLUMN user_preferences.locale IS 'Preferred interface locale: zh-CN or en.';
COMMENT ON COLUMN user_preferences.theme IS 'Preferred color mode: light, dark, or system.';
COMMENT ON COLUMN user_preferences.default_views IS 'JSON mapping from asset kind or context to preferred view identifier.';
COMMENT ON COLUMN user_preferences.revision IS 'Monotonic optimistic-concurrency revision used to construct the HTTP ETag.';
COMMENT ON COLUMN user_preferences.created_at IS 'UTC transaction timestamp when preferences were created.';
COMMENT ON COLUMN user_preferences.updated_at IS 'UTC transaction timestamp of the latest preference update.';

COMMENT ON TABLE credentials IS 'Tenant-owned encrypted SSH or HTTP credentials; secret plaintext is never persisted.';
COMMENT ON COLUMN credentials.tenant_id IS 'Tenant that owns and authorizes access to the credential.';
COMMENT ON COLUMN credentials.id IS 'Application-generated UUID v7 identifying the credential within its tenant.';
COMMENT ON COLUMN credentials.name IS 'Tenant-unique operator label for the credential.';
COMMENT ON COLUMN credentials.kind IS 'Secret representation kind: ssh_key or http_token.';
COMMENT ON COLUMN credentials.ciphertext IS 'AEAD ciphertext including its authentication tag.';
COMMENT ON COLUMN credentials.nonce IS 'Unique 12-byte AEAD nonce used for this encryption.';
COMMENT ON COLUMN credentials.key_version IS 'Master-key version used to derive the row encryption key.';
COMMENT ON COLUMN credentials.fingerprint IS 'Stable non-secret server-derived fingerprint for rotation comparison.';
COMMENT ON COLUMN credentials.shared_scope IS 'Visibility boundary: private, team, or tenant.';
COMMENT ON COLUMN credentials.created_by IS 'Global user who created the credential.';
COMMENT ON COLUMN credentials.last_used_at IS 'UTC instant when the credential was last used successfully, or null if unused.';
COMMENT ON COLUMN credentials.revision IS 'Monotonic optimistic-concurrency revision used to construct the HTTP ETag.';
COMMENT ON COLUMN credentials.created_at IS 'UTC transaction timestamp when credential metadata was created.';
COMMENT ON COLUMN credentials.updated_at IS 'UTC transaction timestamp of the latest credential update or rotation.';

COMMENT ON TABLE credential_team_shares IS 'Explicit team visibility grants for credentials whose shared scope is team.';
COMMENT ON COLUMN credential_team_shares.tenant_id IS 'Owning tenant shared by both referenced resources.';
COMMENT ON COLUMN credential_team_shares.credential_id IS 'Tenant credential made visible to the team.';
COMMENT ON COLUMN credential_team_shares.team_id IS 'Tenant team receiving visibility of the credential.';
COMMENT ON COLUMN credential_team_shares.created_at IS 'UTC transaction timestamp when the share was granted.';

COMMENT ON TABLE global_credentials IS 'Platform-managed encrypted credentials selectable by tenants but mutable only by platform administrators.';
COMMENT ON COLUMN global_credentials.id IS 'Application-generated UUID v7 identifying the global credential.';
COMMENT ON COLUMN global_credentials.name IS 'Globally unique operator label for the credential.';
COMMENT ON COLUMN global_credentials.kind IS 'Secret representation kind: ssh_key or http_token.';
COMMENT ON COLUMN global_credentials.ciphertext IS 'AEAD ciphertext including its authentication tag.';
COMMENT ON COLUMN global_credentials.nonce IS 'Unique 12-byte AEAD nonce used for this encryption.';
COMMENT ON COLUMN global_credentials.key_version IS 'Master-key version used to derive the row encryption key.';
COMMENT ON COLUMN global_credentials.fingerprint IS 'Stable non-secret server-derived fingerprint for rotation comparison.';
COMMENT ON COLUMN global_credentials.created_by IS 'Platform administrator who created the credential.';
COMMENT ON COLUMN global_credentials.last_used_at IS 'UTC instant when the credential was last used successfully, or null if unused.';
COMMENT ON COLUMN global_credentials.revision IS 'Monotonic optimistic-concurrency revision used to construct the HTTP ETag.';
COMMENT ON COLUMN global_credentials.created_at IS 'UTC transaction timestamp when credential metadata was created.';
COMMENT ON COLUMN global_credentials.updated_at IS 'UTC transaction timestamp of the latest credential update or rotation.';

COMMENT ON TABLE known_hosts IS 'Tenant-approved SSH host keys used to prevent repository host impersonation.';
COMMENT ON COLUMN known_hosts.tenant_id IS 'Tenant that trusts this host-key identity.';
COMMENT ON COLUMN known_hosts.id IS 'Application-generated UUID v7 identifying the known-host record.';
COMMENT ON COLUMN known_hosts.host IS 'Normalized lowercase DNS name or IP address without brackets.';
COMMENT ON COLUMN known_hosts.port IS 'SSH TCP port in the inclusive range 1 through 65535.';
COMMENT ON COLUMN known_hosts.key_type IS 'SSH public-key algorithm parsed from the RFC 4253 blob.';
COMMENT ON COLUMN known_hosts.public_key IS 'Decoded RFC 4253 public-key blob; this contains no private key material.';
COMMENT ON COLUMN known_hosts.fingerprint IS 'OpenSSH SHA256 fingerprint derived from the complete public-key blob.';
COMMENT ON COLUMN known_hosts.source IS 'Trust origin: explicit manual entry or accept_new policy.';
COMMENT ON COLUMN known_hosts.created_by IS 'User who accepted the host key, or null for system-created records.';
COMMENT ON COLUMN known_hosts.created_at IS 'UTC transaction timestamp when the host key was trusted.';
COMMENT ON COLUMN known_hosts.updated_at IS 'UTC transaction timestamp of the latest known-host metadata update.';

COMMENT ON TABLE repositories IS 'Tenant-scoped Git repository connections and synchronization configuration.';
COMMENT ON COLUMN repositories.tenant_id IS 'Tenant that owns the repository configuration.';
COMMENT ON COLUMN repositories.id IS 'Application-generated UUID v7 identifying the repository within its tenant.';
COMMENT ON COLUMN repositories.url IS 'Credential-free repository URL as submitted for display.';
COMMENT ON COLUMN repositories.canonical_url IS 'Normalized credential-free URL used for uniqueness comparisons.';
COMMENT ON COLUMN repositories.credential_id IS 'Optional same-tenant credential used to fetch this repository.';
COMMENT ON COLUMN repositories.global_credential_id IS 'Optional platform credential used to fetch this repository.';
COMMENT ON COLUMN repositories.default_branch IS 'Default Git branch or ref name used when a request omits a ref.';
COMMENT ON COLUMN repositories.branch_policy IS 'JSON branch and tag inclusion policy.';
COMMENT ON COLUMN repositories.fetch_config IS 'JSON checkout settings including depth, submodules, paths, and host-key policy.';
COMMENT ON COLUMN repositories.sync_cron IS 'Optional five-field UTC cron expression; null disables scheduled sync.';
COMMENT ON COLUMN repositories.note IS 'Optional operator note with no secret material.';
COMMENT ON COLUMN repositories.webhook_secret_hash IS 'Optional HMAC verification secret digest; plaintext is not stored.';
COMMENT ON COLUMN repositories.health IS 'JSON summary of the latest synchronization health and failure streak.';
COMMENT ON COLUMN repositories.revision IS 'Monotonic optimistic-concurrency revision used to construct the HTTP ETag.';
COMMENT ON COLUMN repositories.deleted_at IS 'UTC soft-deletion instant, or null while the repository is active.';
COMMENT ON COLUMN repositories.created_at IS 'UTC transaction timestamp when the repository was created.';
COMMENT ON COLUMN repositories.updated_at IS 'UTC transaction timestamp of the latest repository update.';

COMMENT ON TABLE jobs IS 'Tenant-visible durable job metadata linked to River execution records.';
COMMENT ON COLUMN jobs.tenant_id IS 'Tenant that owns the job and bounds all job visibility.';
COMMENT ON COLUMN jobs.id IS 'Application-generated UUID v7 exposed by the Meridian API.';
COMMENT ON COLUMN jobs.retry_of_job_id IS 'Optional same-tenant predecessor job that this job retries.';
COMMENT ON COLUMN jobs.river_job_id IS 'Optional internal River job identifier after queue insertion.';
COMMENT ON COLUMN jobs.type IS 'Job behavior identifier from the OpenAPI JobType contract.';
COMMENT ON COLUMN jobs.scope_type IS 'Resource category used for authorization, deduplication, and display.';
COMMENT ON COLUMN jobs.scope_id IS 'Optional UUID of the scoped resource; null for system scope.';
COMMENT ON COLUMN jobs.ref_type IS 'Optional Git reference category: branch or tag.';
COMMENT ON COLUMN jobs.ref_name IS 'Optional normalized Git reference name.';
COMMENT ON COLUMN jobs.trigger IS 'Origin of the request: manual, schedule, webhook, api, cli, system, retry, or credential-rotated.';
COMMENT ON COLUMN jobs.input IS 'Non-secret immutable JSON input needed to execute the job.';
COMMENT ON COLUMN jobs.result IS 'Optional non-secret JSON result identifiers and counters.';
COMMENT ON COLUMN jobs.status IS 'Current durable state from the domain job state machine.';
COMMENT ON COLUMN jobs.stage IS 'Optional active or final pipeline stage.';
COMMENT ON COLUMN jobs.attempt IS 'Zero-based count of job execution attempts already started.';
COMMENT ON COLUMN jobs.max_attempts IS 'Maximum number of permitted execution attempts.';
COMMENT ON COLUMN jobs.next_attempt_at IS 'Optional UTC instant when a retry becomes eligible.';
COMMENT ON COLUMN jobs.dedupe_key IS 'Stable semantic key used to collapse concurrent equivalent work.';
COMMENT ON COLUMN jobs.active_generation IS 'Positive generation participating in the active-job uniqueness constraint.';
COMMENT ON COLUMN jobs.dirty IS 'Whether newer requested work arrived while this generation was running.';
COMMENT ON COLUMN jobs.replay_safe IS 'Whether an interrupted external producer may be safely executed again.';
COMMENT ON COLUMN jobs.error IS 'Optional structured non-secret terminal or retryable error.';
COMMENT ON COLUMN jobs.started_at IS 'UTC instant when execution first entered running state.';
COMMENT ON COLUMN jobs.finished_at IS 'UTC instant when execution reached a terminal state.';
COMMENT ON COLUMN jobs.created_at IS 'UTC transaction timestamp when the job was accepted.';
COMMENT ON COLUMN jobs.updated_at IS 'UTC transaction timestamp of the latest job state update.';

COMMENT ON TABLE job_stage_logs IS 'Ordered persisted log events for replayable job progress streams.';
COMMENT ON COLUMN job_stage_logs.tenant_id IS 'Tenant that owns both the job and this log entry.';
COMMENT ON COLUMN job_stage_logs.job_id IS 'Same-tenant Meridian job producing the log entry.';
COMMENT ON COLUMN job_stage_logs.sequence IS 'Strictly increasing per-job cursor used by SSE Last-Event-ID replay.';
COMMENT ON COLUMN job_stage_logs.stage IS 'Optional pipeline stage active when the message was emitted.';
COMMENT ON COLUMN job_stage_logs.level IS 'Structured severity: debug, info, warn, or error.';
COMMENT ON COLUMN job_stage_logs.message IS 'Human-readable diagnostic with secrets removed.';
COMMENT ON COLUMN job_stage_logs.occurred_at IS 'UTC transaction timestamp when the event was persisted.';

COMMENT ON TABLE notification_channels IS 'Tenant-owned delivery channel configuration with encrypted webhook secrets.';
COMMENT ON COLUMN notification_channels.tenant_id IS 'Tenant that owns and authorizes use of the channel.';
COMMENT ON COLUMN notification_channels.id IS 'Application-generated UUID v7 identifying the channel within its tenant.';
COMMENT ON COLUMN notification_channels.type IS 'Delivery kind: in_app, webhook, or email.';
COMMENT ON COLUMN notification_channels.name IS 'Human-readable tenant-local channel name.';
COMMENT ON COLUMN notification_channels.encrypted_config IS 'Encrypted endpoint and secret configuration; no plaintext secret is stored.';
COMMENT ON COLUMN notification_channels.enabled IS 'Whether future matching events may be delivered through the channel.';
COMMENT ON COLUMN notification_channels.revision IS 'Monotonic optimistic-concurrency revision used to construct the HTTP ETag.';
COMMENT ON COLUMN notification_channels.created_at IS 'UTC transaction timestamp when the channel was created.';
COMMENT ON COLUMN notification_channels.updated_at IS 'UTC transaction timestamp of the latest channel update or rotation.';

COMMENT ON TABLE notify_outbox IS 'Transactional outbox entries for at-least-once external and in-app event delivery.';
COMMENT ON COLUMN notify_outbox.tenant_id IS 'Tenant that owns the event and delivery channel.';
COMMENT ON COLUMN notify_outbox.id IS 'Application-generated UUID v7 identifying this delivery attempt stream.';
COMMENT ON COLUMN notify_outbox.event_id IS 'Stable UUID shared by all deliveries of one domain event.';
COMMENT ON COLUMN notify_outbox.event_type IS 'Domain event type from the event contract.';
COMMENT ON COLUMN notify_outbox.aggregate_id IS 'UUID of the aggregate whose version emitted the event.';
COMMENT ON COLUMN notify_outbox.aggregate_version IS 'Positive aggregate version used for ordering and deduplication.';
COMMENT ON COLUMN notify_outbox.payload IS 'Versioned JSON event payload containing no credential or token secrets.';
COMMENT ON COLUMN notify_outbox.channel_id IS 'Same-tenant notification channel selected for this delivery.';
COMMENT ON COLUMN notify_outbox.status IS 'Delivery state: pending, delivering, delivered, or failed.';
COMMENT ON COLUMN notify_outbox.retry_count IS 'Number of failed delivery attempts already completed.';
COMMENT ON COLUMN notify_outbox.next_attempt_at IS 'UTC instant when the delivery becomes eligible for its next attempt.';
COMMENT ON COLUMN notify_outbox.last_error IS 'Latest redacted delivery error, or an empty string before any failure.';
COMMENT ON COLUMN notify_outbox.created_at IS 'UTC transaction timestamp when the event delivery was enqueued.';
COMMENT ON COLUMN notify_outbox.updated_at IS 'UTC transaction timestamp of the latest delivery state update.';

COMMENT ON TABLE audit_logs IS 'Append-only security and business audit metadata with secret and content values excluded.';
COMMENT ON COLUMN audit_logs.id IS 'Application-generated UUID v7 identifying the audit entry.';
COMMENT ON COLUMN audit_logs.tenant_id IS 'Optional tenant context; null denotes a platform-scoped action.';
COMMENT ON COLUMN audit_logs.actor_id IS 'Optional user identity; null is retained for system or deleted actors.';
COMMENT ON COLUMN audit_logs.actor_type IS 'Actor category: user, system, or anonymous.';
COMMENT ON COLUMN audit_logs.action IS 'Stable action identifier describing what occurred.';
COMMENT ON COLUMN audit_logs.target_type IS 'Resource category affected by the action.';
COMMENT ON COLUMN audit_logs.target_id IS 'Optional UUID of the affected resource.';
COMMENT ON COLUMN audit_logs.detail IS 'Redacted JSON identifiers, counters, and hashes; business content and secrets are forbidden.';
COMMENT ON COLUMN audit_logs.ip IS 'Optional network address associated with the request.';
COMMENT ON COLUMN audit_logs.request_id IS 'Optional request UUID correlating the audit row with logs and API errors.';
COMMENT ON COLUMN audit_logs.created_at IS 'UTC transaction timestamp when the audited action committed.';

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS notify_outbox;
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS job_stage_logs;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS repositories;
DROP TABLE IF EXISTS known_hosts;
DROP TABLE IF EXISTS global_credentials;
DROP TABLE IF EXISTS credential_team_shares;
DROP TABLE IF EXISTS credentials;
DROP TABLE IF EXISTS user_preferences;
DROP TABLE IF EXISTS global_idempotency_records;
DROP TABLE IF EXISTS idempotency_records;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS tenant_members;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS tenant_blob_refs;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
DROP TABLE IF EXISTS blobs;
DROP TABLE IF EXISTS platform_settings;
