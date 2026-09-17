-- 建立 M2 gitops 配置导入所需的 config_import_previews 表。
-- 对齐 contracts/storage.yaml 的字段清单；preview 列承载规范化的配置投影，
-- config_digest 为规范化配置的 SHA-256 摘要，用于 apply 与 preview 的绑定校验。
-- +goose Up

CREATE TABLE config_import_previews (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  repository_id uuid NOT NULL,
  ref_type text NOT NULL,
  ref_name text NOT NULL,
  commit text NOT NULL,
  config_digest text NOT NULL,
  preview jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, id),
  FOREIGN KEY (tenant_id, repository_id) REFERENCES repositories (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE config_import_previews IS '仓库配置导入的一次可应用预览快照。';
COMMENT ON COLUMN config_import_previews.tenant_id IS '拥有该预览的租户。';
COMMENT ON COLUMN config_import_previews.id IS '应用生成的 UUID v7 预览标识。';
COMMENT ON COLUMN config_import_previews.repository_id IS '该预览针对的仓库。';
COMMENT ON COLUMN config_import_previews.ref_type IS '解析配置所用 Git 引用类别：branch 或 tag。';
COMMENT ON COLUMN config_import_previews.ref_name IS '解析配置所用 Git 引用名。';
COMMENT ON COLUMN config_import_previews.commit IS '解析配置时冻结的提交 SHA。';
COMMENT ON COLUMN config_import_previews.config_digest IS '规范化配置的 SHA-256 摘要，apply 必须与之匹配。';
COMMENT ON COLUMN config_import_previews.preview IS '规范化后的服务与源配置投影 JSON。';
COMMENT ON COLUMN config_import_previews.created_at IS '创建预览时的 UTC 事务时间。';
COMMENT ON COLUMN config_import_previews.expires_at IS '预览失效后的 UTC 时间。';

-- +goose Down
DROP TABLE IF EXISTS config_import_previews;
