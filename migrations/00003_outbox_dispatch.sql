-- +goose Up
CREATE INDEX notify_outbox_due_idx ON notify_outbox (next_attempt_at, created_at, tenant_id, id)
WHERE status IN ('pending', 'failed', 'delivering');

COMMENT ON INDEX notify_outbox_due_idx IS 'Orders eligible and stale outbox deliveries for bounded SKIP LOCKED dispatcher scans.';

-- +goose Down
DROP INDEX IF EXISTS notify_outbox_due_idx;
