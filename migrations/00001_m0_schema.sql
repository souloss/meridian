-- 建立 M0 基线所需的平台、租户、身份、凭据、仓库、任务、通知和审计表结构。
-- 所有租户数据表保留 tenant_id 边界；该基线发布后只通过新迁移演进。
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

COMMENT ON TABLE platform_settings IS '平台默认配置；创建租户时复制为租户配置快照。';
COMMENT ON COLUMN platform_settings.id IS '稳定的单例键，唯一允许的值为 default。';
COMMENT ON COLUMN platform_settings.settings IS '包含默认配额、租户设置、视图覆盖和通知通道模板的 JSON 对象。';
COMMENT ON COLUMN platform_settings.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN platform_settings.created_at IS '创建单例记录时的 UTC 事务时间。';
COMMENT ON COLUMN platform_settings.updated_at IS '最近一次更新配置时的 UTC 事务时间。';

CREATE TABLE blobs (
  blob_digest text PRIMARY KEY,
  storage_key text NOT NULL UNIQUE,
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  media_type text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE blobs IS '存储在 PostgreSQL 外部的不可变二进制对象的全局内容寻址元数据。';
COMMENT ON COLUMN blobs.blob_digest IS '唯一标识对象内容的小写 SHA-256 摘要。';
COMMENT ON COLUMN blobs.storage_key IS '由对象存储驱动使用的唯一不透明键。';
COMMENT ON COLUMN blobs.size_bytes IS '对象的准确未压缩字节数。';
COMMENT ON COLUMN blobs.media_type IS '接收对象时记录的 IANA 媒体类型。';
COMMENT ON COLUMN blobs.created_at IS '首次写入对象元数据时的 UTC 事务时间。';

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

COMMENT ON TABLE tenants IS '租户控制面记录及创建时复制的平台默认配置。';
COMMENT ON COLUMN tenants.id IS '应用生成的 UUID v7 租户标识。';
COMMENT ON COLUMN tenants.slug IS '用于租户 URL 的全局唯一小写标识。';
COMMENT ON COLUMN tenants.display_name IS '界面展示的租户名称。';
COMMENT ON COLUMN tenants.status IS '租户访问状态：active 或 disabled。';
COMMENT ON COLUMN tenants.quota IS '创建租户时复制的平台配额快照。';
COMMENT ON COLUMN tenants.settings IS '创建租户时复制的平台运行设置快照。';
COMMENT ON COLUMN tenants.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN tenants.created_at IS '创建租户记录时的 UTC 事务时间。';
COMMENT ON COLUMN tenants.updated_at IS '最近一次更新租户时的 UTC 事务时间。';

CREATE TABLE tenant_blob_refs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  blob_digest text NOT NULL REFERENCES blobs(blob_digest) ON DELETE RESTRICT,
  ref_count bigint NOT NULL DEFAULT 0 CHECK (ref_count >= 0),
  last_referenced_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, blob_digest)
);

COMMENT ON TABLE tenant_blob_refs IS '按租户统计的对象引用数，用于访问控制和垃圾回收。';
COMMENT ON COLUMN tenant_blob_refs.tenant_id IS '拥有该对象逻辑引用的租户。';
COMMENT ON COLUMN tenant_blob_refs.blob_digest IS '被引用全局对象的 SHA-256 摘要。';
COMMENT ON COLUMN tenant_blob_refs.ref_count IS '仍存活的租户数据行对该对象的引用数。';
COMMENT ON COLUMN tenant_blob_refs.last_referenced_at IS '租户最近新增或刷新引用时的 UTC 时间。';

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

COMMENT ON TABLE users IS '可加入多个租户的全局用户身份记录。';
COMMENT ON COLUMN users.id IS '应用生成的 UUID v7 用户标识。';
COMMENT ON COLUMN users.username IS '全局唯一的登录名。';
COMMENT ON COLUMN users.password_hash IS '密码的 Argon2id PHC 校验值，绝不保存明文密码。';
COMMENT ON COLUMN users.display_name IS '展示给其他授权用户的名称。';
COMMENT ON COLUMN users.email IS '可选的全局唯一电子邮件地址。';
COMMENT ON COLUMN users.status IS '认证状态：active 或 disabled。';
COMMENT ON COLUMN users.is_platform_admin IS '是否拥有平台控制面权限，不代表拥有租户数据访问权。';
COMMENT ON COLUMN users.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN users.created_at IS '创建用户记录时的 UTC 事务时间。';
COMMENT ON COLUMN users.updated_at IS '最近一次更新用户元数据时的 UTC 事务时间。';

CREATE TABLE refresh_tokens (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  family_id uuid NOT NULL,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  replaced_by uuid,
  created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE refresh_tokens IS '浏览器刷新令牌，只保存 HMAC 摘要并支持轮换家族。';
COMMENT ON COLUMN refresh_tokens.id IS '应用生成的 UUID v7 刷新令牌标识。';
COMMENT ON COLUMN refresh_tokens.user_id IS '通过该刷新令牌续期的全局用户。';
COMMENT ON COLUMN refresh_tokens.token_hash IS '刷新令牌明文的 HMAC-SHA-256 摘要。';
COMMENT ON COLUMN refresh_tokens.family_id IS '轮换家族标识，同一登录会话的多次轮换共享，用于检测重放。';
COMMENT ON COLUMN refresh_tokens.expires_at IS '刷新令牌失效的 UTC 时间点。';
COMMENT ON COLUMN refresh_tokens.revoked_at IS '主动撤销或检测到重放时置为当前时间，未撤销时为空。';
COMMENT ON COLUMN refresh_tokens.replaced_by IS '轮换后新令牌的 id，仅被替换的旧令牌非空。';
COMMENT ON COLUMN refresh_tokens.created_at IS '创建刷新令牌记录时的 UTC 事务时间。';

CREATE INDEX refresh_tokens_user_expiry_idx ON refresh_tokens (user_id, expires_at);
CREATE INDEX refresh_tokens_expiry_idx ON refresh_tokens (expires_at);
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);

CREATE TABLE tenant_members (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  role text NOT NULL CHECK (role IN ('tenant_admin', 'maintainer', 'viewer')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id)
);

COMMENT ON TABLE tenant_members IS '用户加入租户后的有效角色关系。';
COMMENT ON COLUMN tenant_members.tenant_id IS '授予成员权限的租户。';
COMMENT ON COLUMN tenant_members.user_id IS '获得租户角色的全局用户。';
COMMENT ON COLUMN tenant_members.role IS '租户角色：tenant_admin、maintainer 或 viewer。';
COMMENT ON COLUMN tenant_members.created_at IS '授予成员关系时的 UTC 事务时间。';
COMMENT ON COLUMN tenant_members.updated_at IS '最近一次变更成员角色时的 UTC 事务时间。';

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

COMMENT ON TABLE teams IS '租户内用于授权和凭据共享的团队。';
COMMENT ON COLUMN teams.tenant_id IS '团队所属租户，也是团队标识的第一部分。';
COMMENT ON COLUMN teams.id IS '应用生成的 UUID v7 团队标识。';
COMMENT ON COLUMN teams.slug IS '租户内唯一的小写团队标识。';
COMMENT ON COLUMN teams.display_name IS '界面展示的团队名称。';
COMMENT ON COLUMN teams.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN teams.created_at IS '创建团队记录时的 UTC 事务时间。';
COMMENT ON COLUMN teams.updated_at IS '最近一次更新团队元数据时的 UTC 事务时间。';

CREATE TABLE team_members (
  tenant_id uuid NOT NULL,
  team_id uuid NOT NULL,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, team_id, user_id),
  FOREIGN KEY (tenant_id, team_id) REFERENCES teams (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_members (tenant_id, user_id) ON DELETE CASCADE
);

COMMENT ON TABLE team_members IS '由租户约束的用户与团队成员关系。';
COMMENT ON COLUMN team_members.tenant_id IS '团队和成员外键共同所属的租户。';
COMMENT ON COLUMN team_members.team_id IS '接收该成员的租户内团队。';
COMMENT ON COLUMN team_members.user_id IS '必须先属于同一租户的用户。';
COMMENT ON COLUMN team_members.created_at IS '用户加入团队时的 UTC 事务时间。';

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

COMMENT ON TABLE api_tokens IS '租户内个人访问令牌，仅保存加密摘要。';
COMMENT ON COLUMN api_tokens.tenant_id IS '令牌及其全部权限所限制的租户。';
COMMENT ON COLUMN api_tokens.id IS '应用生成的 UUID v7 令牌元数据标识。';
COMMENT ON COLUMN api_tokens.user_id IS '拥有令牌的同租户用户。';
COMMENT ON COLUMN api_tokens.name IS '用于识别令牌且不泄露令牌内容的用户标签。';
COMMENT ON COLUMN api_tokens.token_hash IS '全局唯一的 PAT 令牌 HMAC-SHA-256 摘要。';
COMMENT ON COLUMN api_tokens.scopes IS '来自领域契约且已去重的 PAT 权限集合。';
COMMENT ON COLUMN api_tokens.expires_at IS '可选的 UTC 过期时间；为空表示没有计划过期时间。';
COMMENT ON COLUMN api_tokens.last_used_at IS '最近一次认证成功的 UTC 时间，未使用时为空。';
COMMENT ON COLUMN api_tokens.revoked_at IS '主动撤销令牌的 UTC 时间，未撤销时为空。';
COMMENT ON COLUMN api_tokens.created_at IS '创建令牌元数据时的 UTC 事务时间。';
COMMENT ON COLUMN api_tokens.updated_at IS '最近一次更新令牌元数据时的 UTC 事务时间。';

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

COMMENT ON TABLE idempotency_records IS '按租户保存、用于精确重放的幂等请求结果。';
COMMENT ON COLUMN idempotency_records.tenant_id IS '构成重放身份外层边界的租户。';
COMMENT ON COLUMN idempotency_records.principal_type IS '用于重放身份的认证主体类别。';
COMMENT ON COLUMN idempotency_records.principal_id IS '用于重放身份的认证主体标识。';
COMMENT ON COLUMN idempotency_records.operation_id IS '受幂等键保护的准确 OpenAPI operationId。';
COMMENT ON COLUMN idempotency_records.idempotency_key IS '客户端提供、标识一次语义请求的 UUID。';
COMMENT ON COLUMN idempotency_records.request_hash IS '请求对象按 RFC 8785 规范化后的原始 32 字节 SHA-256 摘要。';
COMMENT ON COLUMN idempotency_records.response_status IS '完成请求首次返回的 HTTP 状态码。';
COMMENT ON COLUMN idempotency_records.response_body IS '精确重放时返回的原始 JSON 响应值。';
COMMENT ON COLUMN idempotency_records.expires_at IS '允许删除重放记录的 UTC 时间点。';
COMMENT ON COLUMN idempotency_records.created_at IS '成功请求提交时的 UTC 事务时间。';

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

COMMENT ON TABLE global_idempotency_records IS '按身份或平台范围保存、用于精确重放的幂等请求结果。';
COMMENT ON COLUMN global_idempotency_records.context_type IS '重放上下文边界：identity 或 platform。';
COMMENT ON COLUMN global_idempotency_records.principal_type IS '用于重放身份的认证主体类别。';
COMMENT ON COLUMN global_idempotency_records.principal_id IS '用于重放身份的认证主体标识。';
COMMENT ON COLUMN global_idempotency_records.operation_id IS '受幂等键保护的准确 OpenAPI operationId。';
COMMENT ON COLUMN global_idempotency_records.idempotency_key IS '客户端提供、标识一次语义请求的 UUID。';
COMMENT ON COLUMN global_idempotency_records.request_hash IS '请求对象按 RFC 8785 规范化后的原始 32 字节 SHA-256 摘要。';
COMMENT ON COLUMN global_idempotency_records.response_status IS '完成请求首次返回的 HTTP 状态码。';
COMMENT ON COLUMN global_idempotency_records.response_body IS '精确重放时返回的原始 JSON 响应值。';
COMMENT ON COLUMN global_idempotency_records.expires_at IS '允许删除重放记录的 UTC 时间点。';
COMMENT ON COLUMN global_idempotency_records.created_at IS '成功请求提交时的 UTC 事务时间。';

CREATE TABLE user_preferences (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  locale text NOT NULL DEFAULT 'zh-CN' CHECK (locale IN ('zh-CN', 'en')),
  theme text NOT NULL DEFAULT 'system' CHECK (theme IN ('light', 'dark', 'system')),
  default_views jsonb NOT NULL DEFAULT '{}'::jsonb,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE user_preferences IS '与用户一同原子创建的语言、主题和默认视图偏好。';
COMMENT ON COLUMN user_preferences.user_id IS '拥有这些偏好的全局用户。';
COMMENT ON COLUMN user_preferences.locale IS '界面语言：zh-CN 或 en。';
COMMENT ON COLUMN user_preferences.theme IS '颜色模式：light、dark 或 system。';
COMMENT ON COLUMN user_preferences.default_views IS '从资产类型或上下文映射到默认视图标识的 JSON。';
COMMENT ON COLUMN user_preferences.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN user_preferences.created_at IS '创建用户偏好时的 UTC 事务时间。';
COMMENT ON COLUMN user_preferences.updated_at IS '最近一次更新用户偏好时的 UTC 事务时间。';

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

COMMENT ON TABLE credentials IS '租户拥有的加密 SSH 或 HTTP 凭据，绝不持久化秘密明文。';
COMMENT ON COLUMN credentials.tenant_id IS '拥有凭据并授权其访问的租户。';
COMMENT ON COLUMN credentials.id IS '应用生成的 UUID v7 租户内凭据标识。';
COMMENT ON COLUMN credentials.name IS '租户内唯一的凭据操作标签。';
COMMENT ON COLUMN credentials.kind IS '秘密表示类型：ssh_key 或 http_token。';
COMMENT ON COLUMN credentials.ciphertext IS '包含认证标签的 AEAD 密文。';
COMMENT ON COLUMN credentials.nonce IS '本次加密使用的唯一 12 字节 AEAD nonce。';
COMMENT ON COLUMN credentials.key_version IS '用于派生行加密密钥的主密钥版本。';
COMMENT ON COLUMN credentials.fingerprint IS '用于轮换比较的稳定、非敏感服务端指纹。';
COMMENT ON COLUMN credentials.shared_scope IS '可见性范围：private、team 或 tenant。';
COMMENT ON COLUMN credentials.created_by IS '创建凭据的全局用户。';
COMMENT ON COLUMN credentials.last_used_at IS '最近一次成功使用凭据的 UTC 时间，未使用时为空。';
COMMENT ON COLUMN credentials.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN credentials.created_at IS '创建凭据元数据时的 UTC 事务时间。';
COMMENT ON COLUMN credentials.updated_at IS '最近一次更新或轮换凭据时的 UTC 事务时间。';

CREATE TABLE credential_team_shares (
  tenant_id uuid NOT NULL,
  credential_id uuid NOT NULL,
  team_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, credential_id, team_id),
  FOREIGN KEY (tenant_id, credential_id) REFERENCES credentials (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, team_id) REFERENCES teams (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE credential_team_shares IS '当凭据共享范围为 team 时授予的明确团队可见性。';
COMMENT ON COLUMN credential_team_shares.tenant_id IS '两个被引用资源共同所属的租户。';
COMMENT ON COLUMN credential_team_shares.credential_id IS '向团队开放可见性的租户凭据。';
COMMENT ON COLUMN credential_team_shares.team_id IS '获得凭据可见性的租户内团队。';
COMMENT ON COLUMN credential_team_shares.created_at IS '授予共享关系时的 UTC 事务时间。';

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

COMMENT ON TABLE global_credentials IS '由平台管理、租户可选择但仅平台管理员可修改的加密凭据。';
COMMENT ON COLUMN global_credentials.id IS '应用生成的 UUID v7 全局凭据标识。';
COMMENT ON COLUMN global_credentials.name IS '全局唯一的凭据操作标签。';
COMMENT ON COLUMN global_credentials.kind IS '秘密表示类型：ssh_key 或 http_token。';
COMMENT ON COLUMN global_credentials.ciphertext IS '包含认证标签的 AEAD 密文。';
COMMENT ON COLUMN global_credentials.nonce IS '本次加密使用的唯一 12 字节 AEAD nonce。';
COMMENT ON COLUMN global_credentials.key_version IS '用于派生行加密密钥的主密钥版本。';
COMMENT ON COLUMN global_credentials.fingerprint IS '用于轮换比较的稳定、非敏感服务端指纹。';
COMMENT ON COLUMN global_credentials.created_by IS '创建凭据的平台管理员。';
COMMENT ON COLUMN global_credentials.last_used_at IS '最近一次成功使用凭据的 UTC 时间，未使用时为空。';
COMMENT ON COLUMN global_credentials.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN global_credentials.created_at IS '创建凭据元数据时的 UTC 事务时间。';
COMMENT ON COLUMN global_credentials.updated_at IS '最近一次更新或轮换凭据时的 UTC 事务时间。';

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

COMMENT ON TABLE known_hosts IS '租户认可的 SSH 主机密钥，用于防止仓库主机冒充。';
COMMENT ON COLUMN known_hosts.tenant_id IS '信任该主机密钥身份的租户。';
COMMENT ON COLUMN known_hosts.id IS '应用生成的 UUID v7 known-host 记录标识。';
COMMENT ON COLUMN known_hosts.host IS '不含方括号的规范化小写 DNS 名称或 IP 地址。';
COMMENT ON COLUMN known_hosts.port IS '取值范围为 1 至 65535 的 SSH TCP 端口。';
COMMENT ON COLUMN known_hosts.key_type IS '从 RFC 4253 公钥数据解析出的 SSH 算法。';
COMMENT ON COLUMN known_hosts.public_key IS '解码后的 RFC 4253 公钥数据，不包含私钥材料。';
COMMENT ON COLUMN known_hosts.fingerprint IS '根据完整公钥数据生成的 OpenSSH SHA256 指纹。';
COMMENT ON COLUMN known_hosts.source IS '信任来源：手动录入或 accept_new 策略。';
COMMENT ON COLUMN known_hosts.created_by IS '接受该主机密钥的用户，系统创建时为空。';
COMMENT ON COLUMN known_hosts.created_at IS '信任主机密钥时的 UTC 事务时间。';
COMMENT ON COLUMN known_hosts.updated_at IS '最近一次更新 known-host 元数据时的 UTC 事务时间。';

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

COMMENT ON TABLE repositories IS '租户范围内的 Git 仓库连接和同步配置。';
COMMENT ON COLUMN repositories.tenant_id IS '拥有仓库配置的租户。';
COMMENT ON COLUMN repositories.id IS '应用生成的 UUID v7 租户内仓库标识。';
COMMENT ON COLUMN repositories.url IS '用于展示的、已去除凭据的仓库 URL。';
COMMENT ON COLUMN repositories.canonical_url IS '用于唯一性比较的规范化、已去除凭据的 URL。';
COMMENT ON COLUMN repositories.credential_id IS '用于抓取该仓库的可选同租户凭据。';
COMMENT ON COLUMN repositories.global_credential_id IS '用于抓取该仓库的可选平台凭据。';
COMMENT ON COLUMN repositories.default_branch IS '请求未指定引用时使用的默认 Git 分支或引用名。';
COMMENT ON COLUMN repositories.branch_policy IS '分支和标签包含规则的 JSON 配置。';
COMMENT ON COLUMN repositories.fetch_config IS '包含深度、子模块、路径和主机密钥策略的 JSON 检出配置。';
COMMENT ON COLUMN repositories.sync_cron IS '可选的五字段 UTC cron 表达式；为空表示关闭定时同步。';
COMMENT ON COLUMN repositories.note IS '不包含秘密材料的可选操作备注。';
COMMENT ON COLUMN repositories.webhook_secret_hash IS '可选的 webhook 校验秘密摘要，绝不保存明文。';
COMMENT ON COLUMN repositories.health IS '最近同步健康状态和连续失败次数的 JSON 摘要。';
COMMENT ON COLUMN repositories.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN repositories.deleted_at IS '软删除时间；仓库活跃时为空。';
COMMENT ON COLUMN repositories.created_at IS '创建仓库记录时的 UTC 事务时间。';
COMMENT ON COLUMN repositories.updated_at IS '最近一次更新仓库时的 UTC 事务时间。';

CREATE TABLE jobs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  retry_of_job_id uuid,
  river_job_id bigint,
  type text NOT NULL CHECK (type IN (
    'tenant.delete', 'repo.sync', 'repo.discover', 'asset.produce',
    'asset.merge', 'asset.index', 'asset.ai_generate', 'asset.reindex',
    'diff.run', 'outbox.dispatch', 'workspace.gc', 'blob.gc', 'retention.cleanup'
  )),
  scope_type text NOT NULL CHECK (scope_type IN (
    'tenant', 'repository', 'service', 'source', 'track', 'version', 'asset', 'diff', 'system'
  )),
  scope_id uuid,
  ref_type text CHECK (ref_type IN ('branch', 'tag')),
  ref_name text,
  trigger text NOT NULL CHECK (trigger IN (
    'manual', 'schedule', 'webhook', 'api', 'cli', 'system', 'retry', 'credential-rotated'
  )),
  input jsonb NOT NULL DEFAULT '{}'::jsonb,
  result jsonb,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN (
    'pending', 'running', 'succeeded', 'succeeded_with_warnings',
    'failed', 'outcome_unknown', 'cancelled'
  )),
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

COMMENT ON TABLE jobs IS '租户可见、与 River 执行记录关联的持久化任务元数据。';
COMMENT ON COLUMN jobs.tenant_id IS '拥有任务并限定全部任务可见性的租户。';
COMMENT ON COLUMN jobs.id IS '对外暴露的应用生成 UUID v7 任务标识。';
COMMENT ON COLUMN jobs.retry_of_job_id IS '该任务重试的同租户前置任务，可为空。';
COMMENT ON COLUMN jobs.river_job_id IS '写入队列后可选的内部 River 任务标识。';
COMMENT ON COLUMN jobs.type IS '来自 OpenAPI JobType 契约的任务行为标识。';
COMMENT ON COLUMN jobs.scope_type IS '用于授权、去重和展示的资源类别。';
COMMENT ON COLUMN jobs.scope_id IS '被作用资源的 UUID；系统范围任务为空。';
COMMENT ON COLUMN jobs.ref_type IS '可选的 Git 引用类别：branch 或 tag。';
COMMENT ON COLUMN jobs.ref_name IS '可选的规范化 Git 引用名。';
COMMENT ON COLUMN jobs.trigger IS '请求来源：manual、schedule、webhook、api、cli、system、retry 或 credential-rotated。';
COMMENT ON COLUMN jobs.input IS '执行任务所需的不可变、非敏感 JSON 输入。';
COMMENT ON COLUMN jobs.result IS '可选的非敏感 JSON 结果标识和计数。';
COMMENT ON COLUMN jobs.status IS '来自领域任务状态机的当前持久化状态。';
COMMENT ON COLUMN jobs.stage IS '当前或最终的可选流水线阶段。';
COMMENT ON COLUMN jobs.attempt IS '已经开始的任务执行次数，从零开始计数。';
COMMENT ON COLUMN jobs.max_attempts IS '允许的最大执行次数。';
COMMENT ON COLUMN jobs.next_attempt_at IS '重试变为可执行状态的可选 UTC 时间。';
COMMENT ON COLUMN jobs.dedupe_key IS '用于合并并发等价任务的稳定语义键。';
COMMENT ON COLUMN jobs.active_generation IS '参与活跃任务唯一约束的正数代次。';
COMMENT ON COLUMN jobs.dirty IS '任务运行期间是否收到更新的工作请求。';
COMMENT ON COLUMN jobs.replay_safe IS '中断的外部生产者是否可以安全再次执行。';
COMMENT ON COLUMN jobs.error IS '可选的结构化、非敏感终态或可重试错误。';
COMMENT ON COLUMN jobs.started_at IS '执行首次进入 running 状态的 UTC 时间。';
COMMENT ON COLUMN jobs.finished_at IS '执行进入终态的 UTC 时间。';
COMMENT ON COLUMN jobs.created_at IS '接受任务时的 UTC 事务时间。';
COMMENT ON COLUMN jobs.updated_at IS '最近一次更新任务状态时的 UTC 事务时间。';

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

COMMENT ON TABLE job_stage_logs IS '用于回放任务进度流的有序持久化日志事件。';
COMMENT ON COLUMN job_stage_logs.tenant_id IS '同时拥有任务和日志记录的租户。';
COMMENT ON COLUMN job_stage_logs.job_id IS '产生该日志记录的同租户 Meridian 任务。';
COMMENT ON COLUMN job_stage_logs.sequence IS '用于 SSE Last-Event-ID 回放的任务内严格递增游标。';
COMMENT ON COLUMN job_stage_logs.stage IS '写入消息时所在的可选流水线阶段。';
COMMENT ON COLUMN job_stage_logs.level IS '结构化日志级别：debug、info、warn 或 error。';
COMMENT ON COLUMN job_stage_logs.message IS '已移除秘密材料的人类可读诊断信息。';
COMMENT ON COLUMN job_stage_logs.occurred_at IS '持久化事件时的 UTC 事务时间。';

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

COMMENT ON TABLE notification_channels IS '租户拥有的通知投递通道配置，webhook 秘密使用加密值。';
COMMENT ON COLUMN notification_channels.tenant_id IS '拥有通道并授权使用的租户。';
COMMENT ON COLUMN notification_channels.id IS '应用生成的 UUID v7 租户内通道标识。';
COMMENT ON COLUMN notification_channels.type IS '投递类型：in_app、webhook 或 email。';
COMMENT ON COLUMN notification_channels.name IS '租户内展示的通道名称。';
COMMENT ON COLUMN notification_channels.encrypted_config IS '加密后的端点和秘密配置，不保存秘密明文。';
COMMENT ON COLUMN notification_channels.enabled IS '未来匹配事件是否可以通过该通道投递。';
COMMENT ON COLUMN notification_channels.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN notification_channels.created_at IS '创建通道时的 UTC 事务时间。';
COMMENT ON COLUMN notification_channels.updated_at IS '最近一次更新或轮换通道时的 UTC 事务时间。';

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

COMMENT ON TABLE notify_outbox IS '用于至少一次外部和站内事件投递的事务 outbox 记录。';
COMMENT ON COLUMN notify_outbox.tenant_id IS '拥有事件和投递通道的租户。';
COMMENT ON COLUMN notify_outbox.id IS '应用生成的 UUID v7 投递流标识。';
COMMENT ON COLUMN notify_outbox.event_id IS '同一领域事件所有投递记录共享的稳定 UUID。';
COMMENT ON COLUMN notify_outbox.event_type IS '来自事件契约的领域事件类型。';
COMMENT ON COLUMN notify_outbox.aggregate_id IS '产生事件的聚合根 UUID。';
COMMENT ON COLUMN notify_outbox.aggregate_version IS '用于排序和去重的正数聚合版本。';
COMMENT ON COLUMN notify_outbox.payload IS '不含凭据或令牌秘密的版本化 JSON 事件负载。';
COMMENT ON COLUMN notify_outbox.channel_id IS '为本次投递选定的同租户通知通道。';
COMMENT ON COLUMN notify_outbox.status IS '投递状态：pending、delivering、delivered 或 failed。';
COMMENT ON COLUMN notify_outbox.retry_count IS '已经完成的失败投递次数。';
COMMENT ON COLUMN notify_outbox.next_attempt_at IS '投递下一次变为可执行状态的 UTC 时间。';
COMMENT ON COLUMN notify_outbox.last_error IS '最近一次脱敏投递错误；未失败前为空字符串。';
COMMENT ON COLUMN notify_outbox.created_at IS '创建事件投递记录时的 UTC 事务时间。';
COMMENT ON COLUMN notify_outbox.updated_at IS '最近一次更新投递状态时的 UTC 事务时间。';

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

COMMENT ON TABLE audit_logs IS '不包含秘密和业务内容的追加式安全及业务审计元数据。';
COMMENT ON COLUMN audit_logs.id IS '应用生成的 UUID v7 审计记录标识。';
COMMENT ON COLUMN audit_logs.tenant_id IS '可选的租户上下文；为空表示平台范围操作。';
COMMENT ON COLUMN audit_logs.actor_id IS '可选的用户身份；系统操作或用户删除后保留为空。';
COMMENT ON COLUMN audit_logs.actor_type IS '主体类别：user、system 或 anonymous。';
COMMENT ON COLUMN audit_logs.action IS '描述发生事件的稳定动作标识。';
COMMENT ON COLUMN audit_logs.target_type IS '受影响资源的类别。';
COMMENT ON COLUMN audit_logs.target_id IS '受影响资源的可选 UUID。';
COMMENT ON COLUMN audit_logs.detail IS '仅允许标识、计数和摘要的脱敏 JSON，禁止业务内容和秘密。';
COMMENT ON COLUMN audit_logs.ip IS '与请求关联的可选网络地址。';
COMMENT ON COLUMN audit_logs.request_id IS '用于关联日志和 API 错误的可选请求 UUID。';
COMMENT ON COLUMN audit_logs.created_at IS '审计动作提交时的 UTC 事务时间。';

CREATE INDEX audit_logs_tenant_created_idx ON audit_logs (tenant_id, created_at);
CREATE INDEX audit_logs_actor_created_idx ON audit_logs (actor_id, created_at);
CREATE INDEX audit_logs_target_created_idx ON audit_logs (target_type, target_id, created_at);

INSERT INTO platform_settings (id, settings)
VALUES (
  'default',
  $json$
  {
    "defaultQuota": {
      "maxRepositories": 50,
      "maxServices": 200,
      "maxStorageBytes": 10737418240,
      "maxCollectConcurrency": 4
    },
    "defaultTenantSettings": {
      "externalRevisionTrustMode": "review_required",
      "autoPublish": false,
      "defaultLocale": "zh-CN",
      "defaultAiProducerProfileId": null,
      "retention": {
        "jobLogsDays": 30,
        "archivedRevisionsDays": 90
      }
    },
    "defaultViewOverrides": {},
    "defaultNotificationChannels": []
  }
  $json$::jsonb
)
ON CONFLICT (id) DO NOTHING;
-- 按外键依赖反向删除 M0 基线对象，仅供本地或演练环境回滚。
-- 生产环境执行前必须确认不会造成业务数据丢失。
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
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS tenant_blob_refs;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
DROP TABLE IF EXISTS blobs;
DROP TABLE IF EXISTS platform_settings;
