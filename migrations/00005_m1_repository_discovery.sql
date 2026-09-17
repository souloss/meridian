-- 建立 M1 仓库发现与源配置所需的 services、discovery_candidates、producer_profiles、
-- source_specs、source_bindings 表结构。全部对齐 contracts/storage.yaml 的字段清单与类型覆盖。
-- 所有租户数据表保留 tenant_id 边界；producer_profiles 为平台级（global）表。
-- +goose Up

CREATE TABLE producer_profiles (
  id uuid PRIMARY KEY,
  name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 64),
  kind text NOT NULL CHECK (kind IN ('command', 'ai')),
  executable text NOT NULL CHECK (executable LIKE '/%'),
  args jsonb NOT NULL DEFAULT '[]'::jsonb,
  env_allowlist text[] NOT NULL DEFAULT '{}'::text[],
  supported_kinds text[] NOT NULL CHECK (cardinality(supported_kinds) >= 1),
  replay_safe boolean NOT NULL DEFAULT false,
  network text NOT NULL DEFAULT 'none' CHECK (network IN ('none', 'inherit')),
  timeout_sec integer NOT NULL CHECK (timeout_sec BETWEEN 10 AND 3600),
  memory_mib integer NOT NULL DEFAULT 1024 CHECK (memory_mib BETWEEN 64 AND 16384),
  cpu_seconds integer NOT NULL DEFAULT 600 CHECK (cpu_seconds BETWEEN 1 AND 3600),
  pids integer NOT NULL DEFAULT 128 CHECK (pids BETWEEN 1 AND 1024),
  enabled boolean NOT NULL DEFAULT true,
  dependency_status text NOT NULL DEFAULT 'available' CHECK (dependency_status IN ('available', 'unavailable')),
  unavailable_reason text,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE producer_profiles IS '平台管理员维护的全局生产者配置文件，供租户源配置选择。';
COMMENT ON COLUMN producer_profiles.id IS '应用生成的 UUID v7 生产者配置文件标识。';
COMMENT ON COLUMN producer_profiles.name IS '平台内唯一的配置文件名称。';
COMMENT ON COLUMN producer_profiles.kind IS '生产者类别：command 或 ai。';
COMMENT ON COLUMN producer_profiles.executable IS '必须为可执行文件的绝对路径。';
COMMENT ON COLUMN producer_profiles.args IS '以显式数组传递的命令参数，不做 shell 展开。';
COMMENT ON COLUMN producer_profiles.env_allowlist IS '允许透传的环境变量名列表，绝不保存值。';
COMMENT ON COLUMN producer_profiles.supported_kinds IS '该生产者可产出的资产 kind 标识列表。';
COMMENT ON COLUMN producer_profiles.replay_safe IS '中断后重新执行是否安全。';
COMMENT ON COLUMN producer_profiles.network IS '网络模式：none 隔离或 inherit 继承。';
COMMENT ON COLUMN producer_profiles.timeout_sec IS '生产者默认超时，与源配置超时取较小值生效。';
COMMENT ON COLUMN producer_profiles.memory_mib IS '执行内存上限，单位 MiB。';
COMMENT ON COLUMN producer_profiles.cpu_seconds IS '执行 CPU 时间上限，单位秒。';
COMMENT ON COLUMN producer_profiles.pids IS '执行进程数上限。';
COMMENT ON COLUMN producer_profiles.enabled IS '平台是否允许租户选择该配置文件。';
COMMENT ON COLUMN producer_profiles.dependency_status IS '可执行依赖探测结果：available 或 unavailable。';
COMMENT ON COLUMN producer_profiles.unavailable_reason IS '依赖不可用时的简短脱敏说明，可用时为空。';
COMMENT ON COLUMN producer_profiles.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN producer_profiles.deleted_at IS '软删除时间；配置文件活跃时为空。';
COMMENT ON COLUMN producer_profiles.created_at IS '创建配置文件时的 UTC 事务时间。';
COMMENT ON COLUMN producer_profiles.updated_at IS '最近一次更新配置文件时的 UTC 事务时间。';

CREATE TABLE services (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  repository_id uuid NOT NULL,
  slug text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
  display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 128),
  description text,
  root_dir text NOT NULL DEFAULT '' CHECK (length(root_dir) <= 512),
  language text,
  framework text,
  owners text[] NOT NULL DEFAULT '{}'::text[],
  maintainers text[] NOT NULL DEFAULT '{}'::text[],
  lifecycle text NOT NULL DEFAULT 'draft' CHECK (lifecycle IN ('draft', 'published', 'deprecated', 'retired')),
  visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'internal', 'public')),
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, slug),
  UNIQUE (tenant_id, repository_id, root_dir),
  FOREIGN KEY (tenant_id, repository_id) REFERENCES repositories (tenant_id, id) ON DELETE RESTRICT
);

COMMENT ON TABLE services IS '仓库发现产生的租户服务聚合与元数据。';
COMMENT ON COLUMN services.tenant_id IS '拥有该服务的租户。';
COMMENT ON COLUMN services.id IS '应用生成的 UUID v7 服务标识。';
COMMENT ON COLUMN services.repository_id IS '该服务所属的仓库。';
COMMENT ON COLUMN services.slug IS '租户内唯一的服务 URL 标识。';
COMMENT ON COLUMN services.display_name IS '界面展示的服务名称。';
COMMENT ON COLUMN services.description IS '可选的服务说明。';
COMMENT ON COLUMN services.root_dir IS '仓库内相对根目录；空字符串表示仓库根。';
COMMENT ON COLUMN services.language IS '探测到的主要语言，可为空。';
COMMENT ON COLUMN services.framework IS '探测到的主要框架，可为空。';
COMMENT ON COLUMN services.owners IS '服务负责人的用户 UUID 列表。';
COMMENT ON COLUMN services.maintainers IS '服务维护者的用户 UUID 列表。';
COMMENT ON COLUMN services.lifecycle IS '服务生命周期：draft、published、deprecated 或 retired。';
COMMENT ON COLUMN services.visibility IS '服务可见性：private、internal 或 public，默认 private。';
COMMENT ON COLUMN services.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN services.deleted_at IS '软删除时间；服务活跃时为空。';
COMMENT ON COLUMN services.created_at IS '创建服务记录时的 UTC 事务时间。';
COMMENT ON COLUMN services.updated_at IS '最近一次更新服务时的 UTC 事务时间。';

CREATE TABLE discovery_candidates (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  repository_id uuid NOT NULL,
  commit_sha text NOT NULL,
  root_dir text NOT NULL DEFAULT '',
  detected jsonb NOT NULL DEFAULT '{}'::jsonb,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'dismissed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, repository_id, root_dir),
  FOREIGN KEY (tenant_id, repository_id) REFERENCES repositories (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE discovery_candidates IS '仓库发现流水线产生的候选服务根目录。';
COMMENT ON COLUMN discovery_candidates.tenant_id IS '拥有该候选记录的租户。';
COMMENT ON COLUMN discovery_candidates.id IS '应用生成的 UUID v7 候选标识。';
COMMENT ON COLUMN discovery_candidates.repository_id IS '产生候选的仓库。';
COMMENT ON COLUMN discovery_candidates.commit_sha IS '发现该候选时解析到的提交 SHA。';
COMMENT ON COLUMN discovery_candidates.root_dir IS '仓库相对根目录，空字符串表示仓库根。';
COMMENT ON COLUMN discovery_candidates.detected IS '探测到的 marker 文件与 kind 结构 JSON。';
COMMENT ON COLUMN discovery_candidates.status IS '候选状态：pending、accepted 或 dismissed。';
COMMENT ON COLUMN discovery_candidates.created_at IS '首次发现候选时的 UTC 事务时间。';
COMMENT ON COLUMN discovery_candidates.updated_at IS '最近一次更新候选时的 UTC 事务时间。';

CREATE TABLE source_specs (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  service_id uuid NOT NULL,
  kind text NOT NULL,
  asset_name_template text NOT NULL DEFAULT '{file_stem}',
  role text NOT NULL CHECK (role IN ('base', 'overlay')),
  origin text NOT NULL CHECK (origin IN ('repo', 'third_party', 'manual', 'ai_generated')),
  mode text NOT NULL CHECK (mode IN ('builtin', 'command', 'push', 'manual', 'ai')),
  path text,
  producer_profile_id uuid,
  ord integer NOT NULL DEFAULT 0 CHECK (ord >= 0),
  timeout_sec integer NOT NULL CHECK (timeout_sec BETWEEN 10 AND 3600),
  branch_patterns text[] NOT NULL DEFAULT '{"**"}'::text[],
  enabled boolean NOT NULL DEFAULT true,
  config_origin text NOT NULL DEFAULT 'api',
  last_error text,
  failure_streak integer NOT NULL DEFAULT 0 CHECK (failure_streak >= 0),
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (producer_profile_id) REFERENCES producer_profiles (id) ON DELETE RESTRICT
);

COMMENT ON TABLE source_specs IS '服务的源配置，定义如何从仓库或生产者产出资产内容。';
COMMENT ON COLUMN source_specs.tenant_id IS '拥有该源配置的租户。';
COMMENT ON COLUMN source_specs.id IS '应用生成的 UUID v7 源配置标识。';
COMMENT ON COLUMN source_specs.service_id IS '该源配置所属的服务。';
COMMENT ON COLUMN source_specs.kind IS '源配置产出的资产 kind 标识。';
COMMENT ON COLUMN source_specs.asset_name_template IS '资产命名模板，缺省为 {file_stem}。';
COMMENT ON COLUMN source_specs.role IS '层角色：base 或 overlay。';
COMMENT ON COLUMN source_specs.origin IS '层来源：repo、third_party、manual 或 ai_generated。';
COMMENT ON COLUMN source_specs.mode IS '源模式：builtin、command、push、manual 或 ai。';
COMMENT ON COLUMN source_specs.path IS '仓库相对 glob 路径，manual/push 模式为空。';
COMMENT ON COLUMN source_specs.producer_profile_id IS 'command/ai 模式选中的生产者配置文件，可为空。';
COMMENT ON COLUMN source_specs.ord IS '同资产内的层排序号，从 0 开始。';
COMMENT ON COLUMN source_specs.timeout_sec IS '有效超时为该值与生产者配置超时的较小值。';
COMMENT ON COLUMN source_specs.branch_patterns IS '有序的分支/标签包含 glob 列表。';
COMMENT ON COLUMN source_specs.enabled IS '该源配置是否参与物化。';
COMMENT ON COLUMN source_specs.config_origin IS '配置来源：api、repository-config 或 discovery。';
COMMENT ON COLUMN source_specs.last_error IS '最近一次失败的可选脱敏说明。';
COMMENT ON COLUMN source_specs.failure_streak IS '连续失败次数。';
COMMENT ON COLUMN source_specs.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN source_specs.deleted_at IS '软删除时间；源配置活跃时为空。';
COMMENT ON COLUMN source_specs.created_at IS '创建源配置时的 UTC 事务时间。';
COMMENT ON COLUMN source_specs.updated_at IS '最近一次更新源配置时的 UTC 事务时间。';

CREATE TABLE source_bindings (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  source_spec_id uuid NOT NULL,
  scope_type text NOT NULL CHECK (scope_type IN ('ref', 'global')),
  scope_key text NOT NULL,
  expansion_key text NOT NULL DEFAULT '',
  resolved_path text,
  source_system text,
  asset_id uuid NOT NULL,
  layer_id uuid NOT NULL,
  state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'stale', 'error')),
  last_seen_commit text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, source_spec_id, scope_type, scope_key, expansion_key),
  FOREIGN KEY (tenant_id, source_spec_id) REFERENCES source_specs (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE source_bindings IS '源配置物化出的绑定，指向具体资产与层。';
COMMENT ON COLUMN source_bindings.tenant_id IS '拥有该绑定的租户。';
COMMENT ON COLUMN source_bindings.id IS '应用生成的 UUID v7 绑定标识。';
COMMENT ON COLUMN source_bindings.source_spec_id IS '产生该绑定的源配置。';
COMMENT ON COLUMN source_bindings.scope_type IS '作用域类型：ref 或 global。';
COMMENT ON COLUMN source_bindings.scope_key IS 'global 时为 *，否则为 branch:<name> 或 tag:<name>。';
COMMENT ON COLUMN source_bindings.expansion_key IS '展开维度键，缺省为空。';
COMMENT ON COLUMN source_bindings.resolved_path IS '解析到的仓库路径，文件消失后保留并标记 stale。';
COMMENT ON COLUMN source_bindings.source_system IS 'push/manual 源的来源系统标识，可为空。';
COMMENT ON COLUMN source_bindings.asset_id IS '该绑定关联的资产，M1 阶段尚无资产表故无外键。';
COMMENT ON COLUMN source_bindings.layer_id IS '该绑定关联的层，M1 阶段尚无层表故无外键。';
COMMENT ON COLUMN source_bindings.state IS '绑定状态：active、stale 或 error。';
COMMENT ON COLUMN source_bindings.last_seen_commit IS 'global 非 Git 物化时为空，否则为最近提交 SHA。';
COMMENT ON COLUMN source_bindings.created_at IS '创建绑定时的 UTC 事务时间。';
COMMENT ON COLUMN source_bindings.updated_at IS '最近一次更新绑定时的 UTC 事务时间。';

-- 按外键依赖反向删除 M1 对象，仅供本地或演练环境回滚。
-- +goose Down
DROP TABLE IF EXISTS source_bindings;
DROP TABLE IF EXISTS source_specs;
DROP TABLE IF EXISTS discovery_candidates;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS producer_profiles;
