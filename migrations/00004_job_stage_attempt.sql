-- +goose Up
ALTER TABLE job_stage_logs
ADD COLUMN attempt integer NOT NULL DEFAULT 1 CHECK (attempt > 0);

COMMENT ON COLUMN job_stage_logs.attempt IS 'One-based River execution attempt that emitted this persisted stage event.';

-- +goose Down
ALTER TABLE job_stage_logs DROP COLUMN attempt;
