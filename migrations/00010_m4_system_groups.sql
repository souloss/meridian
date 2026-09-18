-- 建立 M4 系统分组（system group）与成员关系所需的持久化表，
-- 并注册 dbschema 与 dependency 两种资产 kind 的基线行。
-- 对齐 contracts/storage.yaml 字段清单；全部为租户维度表。
-- +goose Up

CREATE TABLE system_groups (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  slug text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
  display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 128),
  description text,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, slug)
);

COMMENT ON TABLE system_groups IS '租户内的一组服务聚合，供跨服务依赖图与目录视图使用。';
COMMENT ON COLUMN system_groups.tenant_id IS '拥有该系统分组的租户。';
COMMENT ON COLUMN system_groups.id IS '应用生成的 UUID v7 系统分组标识。';
COMMENT ON COLUMN system_groups.slug IS '租户内唯一的系统分组 URL 标识。';
COMMENT ON COLUMN system_groups.display_name IS '界面展示的系统分组名称。';
COMMENT ON COLUMN system_groups.description IS '可选的系统分组说明。';
COMMENT ON COLUMN system_groups.revision IS '用于生成 HTTP ETag 的单调递增并发版本号。';
COMMENT ON COLUMN system_groups.created_at IS '创建系统分组时的 UTC 事务时间。';
COMMENT ON COLUMN system_groups.updated_at IS '最近一次更新系统分组时的 UTC 事务时间。';

CREATE TABLE system_group_members (
  tenant_id uuid NOT NULL,
  group_id uuid NOT NULL,
  service_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, group_id, service_id),
  FOREIGN KEY (tenant_id, group_id) REFERENCES system_groups (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE system_group_members IS '系统分组与其成员服务的多对多关系。';
COMMENT ON COLUMN system_group_members.tenant_id IS '拥有该成员关系的租户。';
COMMENT ON COLUMN system_group_members.group_id IS '所属的系统分组标识。';
COMMENT ON COLUMN system_group_members.service_id IS '加入分组的服务标识。';
COMMENT ON COLUMN system_group_members.created_at IS '建立成员关系时的 UTC 事务时间。';

-- 为 dbschema 与 dependency kind 落基线注册，ON CONFLICT 幂等。
INSERT INTO asset_kinds (id, contract_version, plugin_version) VALUES ('dbschema', '1', '1')
ON CONFLICT (id) DO NOTHING;
INSERT INTO asset_kinds (id, contract_version, plugin_version) VALUES ('dependency', '1', '1')
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS system_group_members;
DROP TABLE IF EXISTS system_groups;
DELETE FROM asset_kinds WHERE id IN ('dbschema', 'dependency');
