-- 建立 M1 资产流水线所需的 asset_kinds、tenant_kind_overrides、assets、layers、
-- layer_revisions、layer_heads、asset_ref_tracks、asset_versions、asset_items、
-- recent_services 表结构。全部对齐 contracts/storage.yaml 的字段清单与类型覆盖。
-- +goose Up

CREATE TABLE asset_kinds (
  id text PRIMARY KEY,
  contract_version text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  plugin_version text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE asset_kinds IS '平台级资产 kind 注册表，声明每种 kind 的契约与插件版本。';
COMMENT ON COLUMN asset_kinds.id IS '资产 kind 标识，如 openapi。';
COMMENT ON COLUMN asset_kinds.contract_version IS '该 kind 遵循的契约版本。';
COMMENT ON COLUMN asset_kinds.enabled IS '该 kind 是否启用。';
COMMENT ON COLUMN asset_kinds.plugin_version IS '处理该 kind 的插件版本。';
COMMENT ON COLUMN asset_kinds.created_at IS '注册 kind 时的 UTC 事务时间。';
COMMENT ON COLUMN asset_kinds.updated_at IS '最近一次更新 kind 注册时的 UTC 事务时间。';

-- 为 openapi kind 落一条基线注册，ON CONFLICT 幂等。
INSERT INTO asset_kinds (id, contract_version, plugin_version) VALUES ('openapi', '1', '1')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE tenant_kind_overrides (
  tenant_id uuid NOT NULL,
  kind_id text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, kind_id),
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

COMMENT ON TABLE tenant_kind_overrides IS '租户级别的资产 kind 开关覆盖。';
COMMENT ON COLUMN tenant_kind_overrides.tenant_id IS '拥有该覆盖的租户。';
COMMENT ON COLUMN tenant_kind_overrides.kind_id IS '被覆盖的资产 kind 标识。';
COMMENT ON COLUMN tenant_kind_overrides.enabled IS '该租户是否启用该 kind。';
COMMENT ON COLUMN tenant_kind_overrides.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN tenant_kind_overrides.created_at IS '创建覆盖时的 UTC 事务时间。';
COMMENT ON COLUMN tenant_kind_overrides.updated_at IS '最近一次更新覆盖时的 UTC 事务时间。';

CREATE TABLE assets (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  service_id uuid NOT NULL,
  kind text NOT NULL,
  name text NOT NULL,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, service_id, kind, name),
  FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE RESTRICT
);

COMMENT ON TABLE assets IS '服务内的资产，由 kind 与 name 唯一标识。';
COMMENT ON COLUMN assets.tenant_id IS '拥有该资产的租户。';
COMMENT ON COLUMN assets.id IS '应用生成的 UUID v7 资产标识。';
COMMENT ON COLUMN assets.service_id IS '该资产所属的服务。';
COMMENT ON COLUMN assets.kind IS '资产 kind 标识。';
COMMENT ON COLUMN assets.name IS '资产名称，经资产名模板渲染并规范化。';
COMMENT ON COLUMN assets.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN assets.deleted_at IS '软删除时间；资产活跃时为空。';
COMMENT ON COLUMN assets.created_at IS '创建资产时的 UTC 事务时间。';
COMMENT ON COLUMN assets.updated_at IS '最近一次更新资产时的 UTC 事务时间。';

CREATE TABLE layers (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  asset_id uuid NOT NULL,
  source_spec_id uuid,
  role text NOT NULL CHECK (role IN ('base', 'overlay')),
  origin text NOT NULL CHECK (origin IN ('repo', 'third_party', 'manual', 'ai_generated')),
  ord integer NOT NULL DEFAULT 0 CHECK (ord >= 0),
  dialect text,
  enabled boolean NOT NULL DEFAULT true,
  branch_patterns text[] NOT NULL DEFAULT '{"**"}'::text[],
  display_name text NOT NULL DEFAULT '',
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  FOREIGN KEY (tenant_id, asset_id) REFERENCES assets (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, source_spec_id) REFERENCES source_specs (tenant_id, id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX layers_base_uniq_idx ON layers (tenant_id, asset_id) WHERE role = 'base' AND deleted_at IS NULL;
CREATE UNIQUE INDEX layers_overlay_ord_uniq_idx ON layers (tenant_id, asset_id, ord) WHERE role = 'overlay' AND deleted_at IS NULL;

COMMENT ON TABLE layers IS '资产的内容层，base 唯一，overlay 按 ord 排序。';
COMMENT ON COLUMN layers.tenant_id IS '拥有该层的租户。';
COMMENT ON COLUMN layers.id IS '应用生成的 UUID v7 层标识。';
COMMENT ON COLUMN layers.asset_id IS '该层所属的资产。';
COMMENT ON COLUMN layers.source_spec_id IS '产生该层的源配置，可为空。';
COMMENT ON COLUMN layers.role IS '层角色：base 或 overlay。';
COMMENT ON COLUMN layers.origin IS '层来源：repo、third_party、manual 或 ai_generated。';
COMMENT ON COLUMN layers.ord IS 'overlay 层在资产内的排序号。';
COMMENT ON COLUMN layers.dialect IS '层内容方言，可为空。';
COMMENT ON COLUMN layers.enabled IS '该层是否参与合并。';
COMMENT ON COLUMN layers.branch_patterns IS '该层适用的分支/标签 glob 列表。';
COMMENT ON COLUMN layers.display_name IS '界面展示的层名称。';
COMMENT ON COLUMN layers.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN layers.deleted_at IS '软删除时间；层活跃时为空。';
COMMENT ON COLUMN layers.created_at IS '创建层时的 UTC 事务时间。';
COMMENT ON COLUMN layers.updated_at IS '最近一次更新层时的 UTC 事务时间。';

CREATE TABLE layer_revisions (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  layer_id uuid NOT NULL,
  scope_type text NOT NULL CHECK (scope_type IN ('ref', 'global')),
  scope_key text NOT NULL,
  content_hash text NOT NULL,
  content_ref text NOT NULL,
  content_type text NOT NULL,
  dialect text,
  source_branch text,
  review_status text NOT NULL DEFAULT 'not_required' CHECK (review_status IN ('not_required', 'pending_review', 'approved', 'rejected', 'superseded')),
  review_comment text,
  git_commit text,
  created_by uuid,
  producer_run_id uuid,
  ai_meta jsonb,
  review jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, layer_id, scope_type, scope_key, producer_run_id),
  FOREIGN KEY (tenant_id, layer_id) REFERENCES layers (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX layer_revisions_layer_created_idx ON layer_revisions (tenant_id, layer_id, created_at);
CREATE INDEX layer_revisions_review_status_idx ON layer_revisions (tenant_id, review_status);

COMMENT ON TABLE layer_revisions IS '层内容的一次不可变修订。';
COMMENT ON COLUMN layer_revisions.tenant_id IS '拥有该修订的租户。';
COMMENT ON COLUMN layer_revisions.id IS '应用生成的 UUID v7 修订标识。';
COMMENT ON COLUMN layer_revisions.layer_id IS '该修订所属的层。';
COMMENT ON COLUMN layer_revisions.scope_type IS '作用域类型：ref 或 global。';
COMMENT ON COLUMN layer_revisions.scope_key IS 'global 时为 *，否则为 branch:<name> 或 tag:<name>。';
COMMENT ON COLUMN layer_revisions.content_hash IS '层内容的 SHA-256 摘要。';
COMMENT ON COLUMN layer_revisions.content_ref IS '指向 blob 存储的内容引用。';
COMMENT ON COLUMN layer_revisions.content_type IS '层内容的 IANA 媒体类型。';
COMMENT ON COLUMN layer_revisions.dialect IS '层内容方言，可为空。';
COMMENT ON COLUMN layer_revisions.source_branch IS '来源分支，可为空。';
COMMENT ON COLUMN layer_revisions.review_status IS '修订审核状态。';
COMMENT ON COLUMN layer_revisions.review_comment IS '审核备注，可为空。';
COMMENT ON COLUMN layer_revisions.git_commit IS '产生该修订的提交 SHA，可为空。';
COMMENT ON COLUMN layer_revisions.created_by IS '提交该修订的用户，可为空。';
COMMENT ON COLUMN layer_revisions.producer_run_id IS '生产者运行标识，可为空。';
COMMENT ON COLUMN layer_revisions.ai_meta IS 'AI 生成相关的结构化元数据，可为空。';
COMMENT ON COLUMN layer_revisions.review IS '审核结构化信息，可为空。';
COMMENT ON COLUMN layer_revisions.created_at IS '创建修订时的 UTC 事务时间。';

CREATE TABLE layer_heads (
  tenant_id uuid NOT NULL,
  layer_id uuid NOT NULL,
  scope_type text NOT NULL CHECK (scope_type IN ('ref', 'global')),
  scope_key text NOT NULL,
  latest_revision_id uuid,
  effective_revision_id uuid,
  candidate_revision_id uuid,
  generation bigint NOT NULL DEFAULT 1 CHECK (generation > 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, layer_id, scope_type, scope_key),
  FOREIGN KEY (tenant_id, layer_id) REFERENCES layers (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE layer_heads IS '层在指定作用域内的最新/生效/候选修订指针。';
COMMENT ON COLUMN layer_heads.tenant_id IS '拥有该层头的租户。';
COMMENT ON COLUMN layer_heads.layer_id IS '该层头所属的层。';
COMMENT ON COLUMN layer_heads.scope_type IS '作用域类型：ref 或 global。';
COMMENT ON COLUMN layer_heads.scope_key IS 'global 时为 *，否则为 branch:<name> 或 tag:<name>。';
COMMENT ON COLUMN layer_heads.latest_revision_id IS '最新提交的修订。';
COMMENT ON COLUMN layer_heads.effective_revision_id IS '最新无需或已批准的有效修订。';
COMMENT ON COLUMN layer_heads.candidate_revision_id IS '待审核的候选修订，可为空。';
COMMENT ON COLUMN layer_heads.generation IS '单调递增的变更计数器。';
COMMENT ON COLUMN layer_heads.updated_at IS '最近一次更新层头时的 UTC 事务时间。';

CREATE TABLE asset_ref_tracks (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  asset_id uuid NOT NULL,
  ref_type text NOT NULL CHECK (ref_type IN ('branch', 'tag')),
  ref_name text NOT NULL,
  latest_version_id uuid,
  current_version_id uuid,
  health text NOT NULL DEFAULT 'ok' CHECK (health IN ('ok', 'stale', 'invalid')),
  desired_generation bigint NOT NULL DEFAULT 1 CHECK (desired_generation > 0),
  processed_generation bigint NOT NULL DEFAULT 0,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, asset_id, ref_type, ref_name),
  FOREIGN KEY (tenant_id, asset_id) REFERENCES assets (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE asset_ref_tracks IS '资产在某一 Git 分支或标签上的版本轨迹。';
COMMENT ON COLUMN asset_ref_tracks.tenant_id IS '拥有该轨迹的租户。';
COMMENT ON COLUMN asset_ref_tracks.id IS '应用生成的 UUID v7 轨迹标识。';
COMMENT ON COLUMN asset_ref_tracks.asset_id IS '该轨迹所属的资产。';
COMMENT ON COLUMN asset_ref_tracks.ref_type IS 'Git 引用类别：branch 或 tag。';
COMMENT ON COLUMN asset_ref_tracks.ref_name IS 'Git 引用名。';
COMMENT ON COLUMN asset_ref_tracks.latest_version_id IS '最新创建的版本，可为空。';
COMMENT ON COLUMN asset_ref_tracks.current_version_id IS '当前选中的已发布版本，可为空。';
COMMENT ON COLUMN asset_ref_tracks.health IS '轨迹健康状态：ok、stale 或 invalid。';
COMMENT ON COLUMN asset_ref_tracks.desired_generation IS '期望处理代次。';
COMMENT ON COLUMN asset_ref_tracks.processed_generation IS '已处理代次。';
COMMENT ON COLUMN asset_ref_tracks.active IS '轨迹是否活跃。';
COMMENT ON COLUMN asset_ref_tracks.created_at IS '创建轨迹时的 UTC 事务时间。';
COMMENT ON COLUMN asset_ref_tracks.updated_at IS '最近一次更新轨迹时的 UTC 事务时间。';

CREATE TABLE asset_versions (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  asset_id uuid NOT NULL,
  track_id uuid NOT NULL,
  sequence_no bigint NOT NULL CHECK (sequence_no > 0),
  version text NOT NULL,
  lifecycle text NOT NULL DEFAULT 'draft' CHECK (lifecycle IN ('draft', 'published', 'deprecated', 'retired')),
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  quality_score integer CHECK (quality_score >= 0),
  merge_request_id uuid,
  input_fingerprint text NOT NULL,
  merge_engine_version text NOT NULL,
  overlay_compiler_version text,
  overlay_mode text,
  normalizer_version text,
  kind_plugin_version text,
  layer_manifest jsonb NOT NULL DEFAULT '[]'::jsonb,
  merged_hash text,
  merged_ref text,
  normalized_ref text,
  bundled_ref text,
  provenance_ref text,
  source_commit text,
  baseline_version_id uuid,
  diff_summary jsonb,
  labels jsonb NOT NULL DEFAULT '{}'::jsonb,
  index_complete boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, track_id, sequence_no),
  UNIQUE (tenant_id, track_id, version),
  UNIQUE (tenant_id, merge_request_id),
  FOREIGN KEY (tenant_id, asset_id) REFERENCES assets (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, track_id) REFERENCES asset_ref_tracks (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, baseline_version_id) REFERENCES asset_versions (tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX asset_versions_asset_created_idx ON asset_versions (tenant_id, asset_id, created_at);
CREATE INDEX asset_versions_merged_hash_idx ON asset_versions (tenant_id, merged_hash);

COMMENT ON TABLE asset_versions IS '资产版本，记录一次合并物化的完整快照。';
COMMENT ON COLUMN asset_versions.tenant_id IS '拥有该版本的租户。';
COMMENT ON COLUMN asset_versions.id IS '应用生成的 UUID v7 版本标识。';
COMMENT ON COLUMN asset_versions.asset_id IS '该版本所属的资产。';
COMMENT ON COLUMN asset_versions.track_id IS '该版本所属的引用轨迹。';
COMMENT ON COLUMN asset_versions.sequence_no IS '轨迹内单调递增的序号。';
COMMENT ON COLUMN asset_versions.version IS '语义化版本标签，如 1.0.0。';
COMMENT ON COLUMN asset_versions.lifecycle IS '版本生命周期：draft、published、deprecated 或 retired。';
COMMENT ON COLUMN asset_versions.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN asset_versions.quality_score IS '可选的质量评分。';
COMMENT ON COLUMN asset_versions.merge_request_id IS '产生该版本的合并请求，可为空。';
COMMENT ON COLUMN asset_versions.input_fingerprint IS '构建输入指纹，用于 no-op 判定。';
COMMENT ON COLUMN asset_versions.merge_engine_version IS '合并引擎版本。';
COMMENT ON COLUMN asset_versions.overlay_compiler_version IS 'overlay 编译器版本，可为空。';
COMMENT ON COLUMN asset_versions.overlay_mode IS 'overlay 模式，可为空。';
COMMENT ON COLUMN asset_versions.normalizer_version IS '规范化器版本，可为空。';
COMMENT ON COLUMN asset_versions.kind_plugin_version IS 'kind 插件版本，可为空。';
COMMENT ON COLUMN asset_versions.layer_manifest IS '产生该版本的层修订清单 JSON。';
COMMENT ON COLUMN asset_versions.merged_hash IS '合并内容摘要，可为空。';
COMMENT ON COLUMN asset_versions.merged_ref IS '合并内容引用，可为空。';
COMMENT ON COLUMN asset_versions.normalized_ref IS '规范化内容引用，可为空。';
COMMENT ON COLUMN asset_versions.bundled_ref IS '打包内容引用，可为空。';
COMMENT ON COLUMN asset_versions.provenance_ref IS '溯源引用，可为空。';
COMMENT ON COLUMN asset_versions.source_commit IS '来源提交 SHA，可为空。';
COMMENT ON COLUMN asset_versions.baseline_version_id IS '比较基线版本，可为空。';
COMMENT ON COLUMN asset_versions.diff_summary IS '与基线的 diff 摘要，可为空。';
COMMENT ON COLUMN asset_versions.labels IS '版本标签 JSON。';
COMMENT ON COLUMN asset_versions.index_complete IS '是否已完成 item 索引。';
COMMENT ON COLUMN asset_versions.created_at IS '创建版本时的 UTC 事务时间。';
COMMENT ON COLUMN asset_versions.updated_at IS '最近一次更新版本时的 UTC 事务时间。';

CREATE TABLE asset_items (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  asset_version_id uuid NOT NULL,
  asset_id uuid NOT NULL,
  service_id uuid NOT NULL,
  kind text NOT NULL,
  item_type text NOT NULL,
  key text NOT NULL,
  display jsonb NOT NULL DEFAULT '{}'::jsonb,
  search_text text NOT NULL DEFAULT '',
  search_vector tsvector,
  search_raw jsonb NOT NULL DEFAULT '{}'::jsonb,
  provenance jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, asset_version_id, item_type, key),
  FOREIGN KEY (tenant_id, asset_version_id) REFERENCES asset_versions (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX asset_items_version_item_type_idx ON asset_items (tenant_id, asset_version_id, item_type);
CREATE INDEX asset_items_service_kind_idx ON asset_items (tenant_id, service_id, kind);
CREATE INDEX asset_items_search_vector_idx ON asset_items USING gin (tenant_id, search_vector);
CREATE INDEX asset_items_search_text_idx ON asset_items USING gin (tenant_id, (search_text) gin_trgm_ops);
CREATE INDEX asset_items_search_raw_idx ON asset_items USING gin (tenant_id, (search_raw) jsonb_path_ops);

COMMENT ON TABLE asset_items IS '资产版本的索引条目，如 openapi 的单个 operation。';
COMMENT ON COLUMN asset_items.tenant_id IS '拥有该条目的租户。';
COMMENT ON COLUMN asset_items.id IS '应用生成的 UUID v7 条目标识。';
COMMENT ON COLUMN asset_items.asset_version_id IS '该条目所属的资产版本。';
COMMENT ON COLUMN asset_items.asset_id IS '该条目所属的资产。';
COMMENT ON COLUMN asset_items.service_id IS '该条目所属的服务。';
COMMENT ON COLUMN asset_items.kind IS '资产 kind 标识。';
COMMENT ON COLUMN asset_items.item_type IS '条目类型，如 operation。';
COMMENT ON COLUMN asset_items.key IS '条目在版本内的稳定键，如 UPPER(method) normalizedPath。';
COMMENT ON COLUMN asset_items.display IS '条目展示字段 JSON。';
COMMENT ON COLUMN asset_items.search_text IS '用于三元组搜索的文本。';
COMMENT ON COLUMN asset_items.search_vector IS '用于全文检索的 tsvector。';
COMMENT ON COLUMN asset_items.search_raw IS '原始可搜索 JSON。';
COMMENT ON COLUMN asset_items.provenance IS '条目溯源 JSON。';
COMMENT ON COLUMN asset_items.created_at IS '创建条目时的 UTC 事务时间。';

CREATE TABLE recent_services (
  tenant_id uuid NOT NULL,
  user_id uuid NOT NULL,
  service_id uuid NOT NULL,
  viewed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id, service_id),
  FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE recent_services IS '用户最近访问的服务记录，一人一服务一行。';
COMMENT ON COLUMN recent_services.tenant_id IS '拥有该记录的租户。';
COMMENT ON COLUMN recent_services.user_id IS '访问该服务的用户。';
COMMENT ON COLUMN recent_services.service_id IS '被访问的服务。';
COMMENT ON COLUMN recent_services.viewed_at IS '最近成功访问该服务详情的时间。';

-- 按外键依赖反向删除 M1 资产流水线对象。
-- +goose Down
DROP TABLE IF EXISTS recent_services;
DROP TABLE IF EXISTS asset_items;
DROP TABLE IF EXISTS asset_versions;
DROP TABLE IF EXISTS asset_ref_tracks;
DROP TABLE IF EXISTS layer_heads;
DROP TABLE IF EXISTS layer_revisions;
DROP TABLE IF EXISTS layers;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS tenant_kind_overrides;
DROP TABLE IF EXISTS asset_kinds;
