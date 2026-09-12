-- 为凭据轮换产生的持久化任务增加 credential-rotated 触发来源。
-- 该变更只扩展约束取值，不改变既有任务数据。
-- +goose Up
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_trigger_check;
ALTER TABLE jobs
ADD CONSTRAINT jobs_trigger_check CHECK (trigger IN (
  'manual', 'schedule', 'webhook', 'api', 'cli', 'system', 'retry', 'credential-rotated'
));
COMMENT ON COLUMN jobs.trigger IS '请求来源：manual、schedule、webhook、api、cli、system、retry 或 credential-rotated。';

-- 回滚时恢复 M0 基线允许的任务触发来源。
-- 回滚前必须确认没有依赖 credential-rotated 的新任务记录。
-- +goose Down
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_trigger_check;
ALTER TABLE jobs
ADD CONSTRAINT jobs_trigger_check CHECK (trigger IN (
  'manual', 'schedule', 'webhook', 'api', 'cli', 'system', 'retry'
));
COMMENT ON COLUMN jobs.trigger IS '请求来源：manual、schedule、webhook、api、cli、system 或 retry。';
