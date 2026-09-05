-- +goose Up
-- Extend the durable job trigger check for credential rotation side effects introduced after the M0 schema baseline.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_trigger_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_trigger_check CHECK (trigger IN ('manual', 'schedule', 'webhook', 'api', 'cli', 'system', 'retry', 'credential-rotated'));
COMMENT ON COLUMN jobs.trigger IS 'Origin of the request: manual, schedule, webhook, api, cli, system, retry, or credential-rotated.';

-- +goose Down
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_trigger_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_trigger_check CHECK (trigger IN ('manual', 'schedule', 'webhook', 'api', 'cli', 'system', 'retry'));
COMMENT ON COLUMN jobs.trigger IS 'Origin of the request: manual, schedule, webhook, api, cli, system, or retry.';
