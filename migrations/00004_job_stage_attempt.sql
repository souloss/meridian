-- 为任务阶段日志记录一基执行尝试次数，支持重试期间的事件审计和回放。
-- +goose Up
ALTER TABLE job_stage_logs
ADD COLUMN attempt integer NOT NULL DEFAULT 1 CHECK (attempt > 0);

COMMENT ON COLUMN job_stage_logs.attempt IS '写入该持久化阶段事件的 River 执行尝试次数，从一开始计数。';

-- 删除阶段日志的尝试次数列；仅适用于尚未依赖该字段的环境。
-- +goose Down
ALTER TABLE job_stage_logs DROP COLUMN attempt;
