-- 建立 M3 差异/分享/待办所需的持久化表。
-- 对齐 contracts/storage.yaml 字段清单；全部为租户维度表（share_links 的 token 哈希全局唯一）。
-- +goose Up

CREATE TABLE uploads (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  blob_digest text NOT NULL,
  kind text NOT NULL,
  content_type text NOT NULL,
  size_bytes bigint NOT NULL DEFAULT 0,
  expires_at timestamptz NOT NULL,
  created_by uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX uploads_expires_idx ON uploads (tenant_id, expires_at);

COMMENT ON TABLE uploads IS '用于 CLI 文件选择器做差异比较的一次性上传内容。';
COMMENT ON COLUMN uploads.tenant_id IS '拥有该上传的租户。';
COMMENT ON COLUMN uploads.id IS '应用生成的 UUID v7 上传标识。';
COMMENT ON COLUMN uploads.blob_digest IS '上传内容的内容寻址 blob 摘要。';
COMMENT ON COLUMN uploads.kind IS '上传内容的资产类别。';
COMMENT ON COLUMN uploads.content_type IS '上传内容的 IANA 媒体类型。';
COMMENT ON COLUMN uploads.size_bytes IS '上传内容的字节长度。';
COMMENT ON COLUMN uploads.expires_at IS '上传失效后的 UTC 时间。';
COMMENT ON COLUMN uploads.created_by IS '发起上传的用户，可为空。';
COMMENT ON COLUMN uploads.created_at IS '创建上传时的 UTC 事务时间。';

CREATE TABLE diff_rule_sets (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  kind text NOT NULL,
  name text NOT NULL,
  version integer NOT NULL DEFAULT 1,
  rules jsonb NOT NULL DEFAULT '[]'::jsonb,
  enabled boolean NOT NULL DEFAULT true,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, kind, name, version)
);

COMMENT ON TABLE diff_rule_sets IS '资产类别差异规则的版本化集合。';
COMMENT ON COLUMN diff_rule_sets.tenant_id IS '拥有该规则集的租户。';
COMMENT ON COLUMN diff_rule_sets.id IS '应用生成的 UUID v7 规则集标识。';
COMMENT ON COLUMN diff_rule_sets.kind IS '该规则集适用的资产类别。';
COMMENT ON COLUMN diff_rule_sets.name IS '规则集名称。';
COMMENT ON COLUMN diff_rule_sets.version IS '规则集的版本号。';
COMMENT ON COLUMN diff_rule_sets.rules IS '规则列表 JSON 投影。';
COMMENT ON COLUMN diff_rule_sets.enabled IS '规则集是否启用。';
COMMENT ON COLUMN diff_rule_sets.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN diff_rule_sets.created_at IS '创建规则集时的 UTC 事务时间。';
COMMENT ON COLUMN diff_rule_sets.updated_at IS '最近一次更新规则集时的 UTC 事务时间。';

CREATE TABLE diff_snapshots (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  left_selector jsonb NOT NULL,
  right_selector jsonb NOT NULL,
  left_artifact_ref text NOT NULL,
  right_artifact_ref text NOT NULL,
  rule_set_id uuid,
  result_ref text NOT NULL,
  summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_by uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE RESTRICT
);

COMMENT ON TABLE diff_snapshots IS '一次持久化差异结果，冻结了解析后的选择器与产物。';
COMMENT ON COLUMN diff_snapshots.tenant_id IS '拥有该快照的租户。';
COMMENT ON COLUMN diff_snapshots.id IS '应用生成的 UUID v7 快照标识。';
COMMENT ON COLUMN diff_snapshots.left_selector IS '冻结的左侧已解析选择器 JSON。';
COMMENT ON COLUMN diff_snapshots.right_selector IS '冻结的右侧已解析选择器 JSON。';
COMMENT ON COLUMN diff_snapshots.left_artifact_ref IS '左侧产物的不可变内容引用。';
COMMENT ON COLUMN diff_snapshots.right_artifact_ref IS '右侧产物的不可变内容引用。';
COMMENT ON COLUMN diff_snapshots.rule_set_id IS '使用的差异规则集，可为空。';
COMMENT ON COLUMN diff_snapshots.result_ref IS '差异结果 JSON 的内容引用。';
COMMENT ON COLUMN diff_snapshots.summary IS '差异统计摘要 JSON。';
COMMENT ON COLUMN diff_snapshots.created_by IS '创建快照的用户。';
COMMENT ON COLUMN diff_snapshots.created_at IS '创建快照时的 UTC 事务时间。';

CREATE TABLE share_links (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  token_hash bytea NOT NULL,
  creator_id uuid NOT NULL,
  resource_type text NOT NULL CHECK (resource_type IN ('view', 'diff_snapshot')),
  resource_id uuid,
  descriptor jsonb NOT NULL DEFAULT '{}'::jsonb,
  view_id text,
  options jsonb NOT NULL DEFAULT '{}'::jsonb,
  artifact_allowlist jsonb NOT NULL DEFAULT '[]'::jsonb,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (token_hash),
  FOREIGN KEY (creator_id) REFERENCES users (id) ON DELETE RESTRICT
);

COMMENT ON TABLE share_links IS '视图或差异快照的匿名只读分享链接。';
COMMENT ON COLUMN share_links.tenant_id IS '拥有该分享链接的租户。';
COMMENT ON COLUMN share_links.id IS '应用生成的 UUID v7 分享链接标识。';
COMMENT ON COLUMN share_links.token_hash IS '分享令牌的哈希，令牌明文仅在创建响应返回。';
COMMENT ON COLUMN share_links.creator_id IS '创建分享链接的用户。';
COMMENT ON COLUMN share_links.resource_type IS '分享资源类别：view 或 diff_snapshot。';
COMMENT ON COLUMN share_links.resource_id IS '分享资源标识；view 时可为空。';
COMMENT ON COLUMN share_links.descriptor IS '创建时冻结的资源描述符 JSON。';
COMMENT ON COLUMN share_links.view_id IS 'view 资源对应的视图标识。';
COMMENT ON COLUMN share_links.options IS '创建时冻结的视图选项 JSON。';
COMMENT ON COLUMN share_links.artifact_allowlist IS '不可变的内容引用允许列表。';
COMMENT ON COLUMN share_links.expires_at IS '分享失效后的 UTC 时间。';
COMMENT ON COLUMN share_links.revoked_at IS '分享被撤销的 UTC 时间，可为空。';
COMMENT ON COLUMN share_links.created_at IS '创建分享链接时的 UTC 事务时间。';

CREATE TABLE breaking_todos (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  asset_version_id uuid NOT NULL,
  service_id uuid NOT NULL,
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'acked')),
  acked_by uuid,
  acked_at timestamptz,
  comment text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, asset_version_id, service_id),
  FOREIGN KEY (tenant_id, asset_version_id) REFERENCES asset_versions (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (acked_by) REFERENCES users (id) ON DELETE SET NULL
);

COMMENT ON TABLE breaking_todos IS '按资产版本与服务的破坏性变更待办。';
COMMENT ON COLUMN breaking_todos.tenant_id IS '拥有该待办的租户。';
COMMENT ON COLUMN breaking_todos.id IS '应用生成的 UUID v7 待办标识。';
COMMENT ON COLUMN breaking_todos.asset_version_id IS '触发该待办的资产版本。';
COMMENT ON COLUMN breaking_todos.service_id IS '该待办归属的服务。';
COMMENT ON COLUMN breaking_todos.status IS '待办状态：open 或 acked。';
COMMENT ON COLUMN breaking_todos.acked_by IS '确认该待办的用户，可为空。';
COMMENT ON COLUMN breaking_todos.acked_at IS '确认该待办的 UTC 时间，可为空。';
COMMENT ON COLUMN breaking_todos.comment IS '确认备注，可为空。';
COMMENT ON COLUMN breaking_todos.created_at IS '创建待办时的 UTC 事务时间。';
COMMENT ON COLUMN breaking_todos.updated_at IS '最近一次更新待办时的 UTC 事务时间。';

-- +goose Down
DROP TABLE IF EXISTS breaking_todos;
DROP TABLE IF EXISTS share_links;
DROP TABLE IF EXISTS diff_snapshots;
DROP TABLE IF EXISTS diff_rule_sets;
DROP TABLE IF EXISTS uploads;
