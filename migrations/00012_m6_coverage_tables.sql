-- 补齐 M6 收口缺口所需的契约表：服务收藏、标签字典与关联、轻量评论、
-- 视图覆盖与租户导出。全部对齐 contracts/storage.yaml 字段清单与 DDL 注释约定。
-- +goose Up

CREATE TABLE service_stars (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  service_id uuid NOT NULL,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, service_id, user_id),
  FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE service_stars IS '租户内用户对服务的收藏，用于目录筛选与个人主页。';
COMMENT ON COLUMN service_stars.tenant_id IS '收藏所属租户。';
COMMENT ON COLUMN service_stars.service_id IS '被收藏的服务。';
COMMENT ON COLUMN service_stars.user_id IS '收藏的用户。';
COMMENT ON COLUMN service_stars.created_at IS '收藏创建时间。';

CREATE TABLE tag_definitions (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  name text NOT NULL,
  color text NOT NULL DEFAULT '#64748b',
  description text,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, name)
);

COMMENT ON TABLE tag_definitions IS '租户维护的标签字典，供服务筛选与分面。';
COMMENT ON COLUMN tag_definitions.tenant_id IS '标签所属租户。';
COMMENT ON COLUMN tag_definitions.id IS '应用生成的 UUID v7 标签标识。';
COMMENT ON COLUMN tag_definitions.name IS '租户内唯一的标签名称。';
COMMENT ON COLUMN tag_definitions.color IS '标签展示颜色。';
COMMENT ON COLUMN tag_definitions.description IS '标签描述（可为空）。';
COMMENT ON COLUMN tag_definitions.revision IS '用于生成 HTTP ETag 的并发版本号。';
COMMENT ON COLUMN tag_definitions.created_at IS '创建时间。';
COMMENT ON COLUMN tag_definitions.updated_at IS '最近更新时间。';

CREATE TABLE service_tags (
  tenant_id uuid NOT NULL,
  service_id uuid NOT NULL,
  tag_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, service_id, tag_id),
  FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, tag_id) REFERENCES tag_definitions (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE service_tags IS '服务与标签的多对多关联。';
COMMENT ON COLUMN service_tags.tenant_id IS '关联所属租户。';
COMMENT ON COLUMN service_tags.service_id IS '被标记的服务。';
COMMENT ON COLUMN service_tags.tag_id IS '关联的标签。';
COMMENT ON COLUMN service_tags.created_at IS '关联创建时间。';

CREATE TABLE service_comments (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  service_id uuid NOT NULL,
  author_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  body text NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE service_comments IS '服务页轻量评论/备注，不做完整讨论区。';
COMMENT ON COLUMN service_comments.tenant_id IS '评论所属租户。';
COMMENT ON COLUMN service_comments.id IS '应用生成的 UUID v7 评论标识。';
COMMENT ON COLUMN service_comments.service_id IS '被评论的服务。';
COMMENT ON COLUMN service_comments.author_id IS '评论作者。';
COMMENT ON COLUMN service_comments.body IS '评论正文。';
COMMENT ON COLUMN service_comments.revision IS '并发版本号。';
COMMENT ON COLUMN service_comments.deleted_at IS '软删除时间（可为空）。';
COMMENT ON COLUMN service_comments.created_at IS '创建时间。';
COMMENT ON COLUMN service_comments.updated_at IS '最近更新时间。';

CREATE TABLE view_overrides (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  view_id text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  ord integer NOT NULL DEFAULT 0,
  default_options jsonb NOT NULL DEFAULT '{}'::jsonb,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, view_id)
);

COMMENT ON TABLE view_overrides IS '租户级视图开关、排序与默认参数覆盖。';
COMMENT ON COLUMN view_overrides.tenant_id IS '覆盖所属租户。';
COMMENT ON COLUMN view_overrides.view_id IS '被覆盖的内置视图标识。';
COMMENT ON COLUMN view_overrides.enabled IS '该租户是否启用该视图。';
COMMENT ON COLUMN view_overrides.ord IS '视图在租户内的展示排序。';
COMMENT ON COLUMN view_overrides.default_options IS '租户级默认参数覆盖 JSON。';
COMMENT ON COLUMN view_overrides.revision IS '用于生成 HTTP ETag 的并发版本号。';
COMMENT ON COLUMN view_overrides.updated_at IS '最近更新时间。';

CREATE TABLE tenant_exports (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  requested_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
  artifact_ref text,
  expires_at timestamptz,
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);

COMMENT ON TABLE tenant_exports IS '租户全量数据导出任务记录。';
COMMENT ON COLUMN tenant_exports.tenant_id IS '导出所属租户。';
COMMENT ON COLUMN tenant_exports.id IS '应用生成的 UUID v7 导出标识。';
COMMENT ON COLUMN tenant_exports.requested_by IS '发起导出的用户。';
COMMENT ON COLUMN tenant_exports.status IS '导出状态：pending/running/succeeded/failed。';
COMMENT ON COLUMN tenant_exports.artifact_ref IS '导出产物的 blob 引用（可为空）。';
COMMENT ON COLUMN tenant_exports.expires_at IS '导出产物有效期（可为空）。';
COMMENT ON COLUMN tenant_exports.error IS '失败原因（可为空）。';
COMMENT ON COLUMN tenant_exports.created_at IS '创建时间。';
COMMENT ON COLUMN tenant_exports.updated_at IS '最近更新时间。';

-- +goose Down

DROP TABLE IF EXISTS tenant_exports;
DROP TABLE IF EXISTS view_overrides;
DROP TABLE IF EXISTS service_comments;
DROP TABLE IF EXISTS service_tags;
DROP TABLE IF EXISTS tag_definitions;
DROP TABLE IF EXISTS service_stars;
