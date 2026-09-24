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
-- 注意：credential-rotated 已由 ed50958 内联进 00001 的 M0 基线 CHECK，故此处回滚
-- 必须恢复含 credential-rotated 的同一取值集合，否则回滚会把既有 credential-rotated
-- 任务行判为违反约束（SQLSTATE 23514），导致 operator up/down/up 冒烟失败。
-- +goose Down
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_trigger_check;
ALTER TABLE jobs
ADD CONSTRAINT jobs_trigger_check CHECK (trigger IN (
  'manual', 'schedule', 'webhook', 'api', 'cli', 'system', 'retry', 'credential-rotated'
));
COMMENT ON COLUMN jobs.trigger IS '请求来源：manual、schedule、webhook、api、cli、system、retry 或 credential-rotated。';
