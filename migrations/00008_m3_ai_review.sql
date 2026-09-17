-- 建立 M3 AI 评审与发布所需的资产侧扩展表：
--   ai_generation_results —— 保留生成命令重放（replay）所需的精确产物（completion manifest 或结构化错误）。
--   之所以用独立表而非 jobs.result，是因为 jobs.result 走脱敏聚合口径，
--   而成功重放需要原样字节级 content_ref/content_hash，超时/无效两种失败需要失败阶段与错误码。
-- 全部字段对齐 contracts/storage.yaml 的字段清单与中文注释要求。
-- +goose Up

CREATE TABLE ai_generation_results (
  tenant_id uuid NOT NULL,
  id uuid NOT NULL,
  job_id uuid NOT NULL,
  stage text NOT NULL CHECK (stage IN ('resolve', 'discover', 'extract', 'merge', 'normalize', 'index')),
  status text NOT NULL CHECK (status IN ('succeeded', 'failed', 'cancelled')),
  error_code text NOT NULL,
  content_ref text,
  content_hash text,
  content_type text,
  manifest jsonb NOT NULL DEFAULT '{}'::jsonb,
  revision_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, job_id),
  FOREIGN KEY (tenant_id, job_id) REFERENCES jobs (tenant_id, id) ON DELETE CASCADE
);

COMMENT ON TABLE ai_generation_results IS 'AI 资产生成成功或失败的一次可重放结果快照。';
COMMENT ON COLUMN ai_generation_results.tenant_id IS '拥有该生成结果的租户。';
COMMENT ON COLUMN ai_generation_results.id IS '应用生成的 UUID v7 结果标识。';
COMMENT ON COLUMN ai_generation_results.job_id IS '产生该结果的资产 AI 生成任务。';
COMMENT ON COLUMN ai_generation_results.stage IS '终止所在流水线阶段：resolve、discover、extract、merge、normalize 或 index。';
COMMENT ON COLUMN ai_generation_results.status IS '终态：succeeded、failed 或 cancelled。';
COMMENT ON COLUMN ai_generation_results.error_code IS '稳定且不含敏感信息的失败分类；成功时为空。';
COMMENT ON COLUMN ai_generation_results.content_ref IS '生成内容在 blob 存储中的内容引用，成功时非空。';
COMMENT ON COLUMN ai_generation_results.content_hash IS '生成内容的 SHA-256 摘要，成功时非空。';
COMMENT ON COLUMN ai_generation_results.content_type IS '生成内容的 IANA 媒体类型，成功时非空。';
COMMENT ON COLUMN ai_generation_results.manifest IS '生产者完成清单的规范化投影；超时/无效时可观察到的部分清单。';
COMMENT ON COLUMN ai_generation_results.revision_id IS '依据成功内容创建的层修订标识；失败时为空。';
COMMENT ON COLUMN ai_generation_results.created_at IS '记录该结果时的 UTC 事务时间。';

-- +goose Down
DROP TABLE IF EXISTS ai_generation_results;
