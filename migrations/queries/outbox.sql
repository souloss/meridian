-- GetTenantSlugForEvent resolves the stable tenant slug embedded in a domain event envelope.
-- name: GetTenantSlugForEvent :one
SELECT slug
FROM tenants
WHERE id = sqlc.arg(tenant_id);

-- AppendJobFailureAudit records one append-only, redacted system fact in the
-- same transaction as the job terminal state and its outbound event rows.
-- name: AppendJobFailureAudit :one
INSERT INTO audit_logs (id, tenant_id, actor_id, actor_type, action, target_type, target_id, detail, ip, request_id)
VALUES (
  sqlc.arg(id), sqlc.arg(tenant_id), NULL, 'system', 'job.failed', 'job', sqlc.arg(job_id),
  sqlc.arg(detail)::jsonb, NULL, NULL
)
RETURNING *;

-- ListEnabledNotificationChannelIDs returns deterministic channel targets for
-- an M0 operational event. M5 subscription routing will provide the narrower target set.
-- name: ListEnabledNotificationChannelIDs :many
SELECT id
FROM notification_channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND enabled = true
ORDER BY id;

-- CreateNotifyOutbox inserts one channel-specific delivery row while preserving
-- the shared event identifier used by receivers for at-least-once deduplication.
-- name: CreateNotifyOutbox :one
INSERT INTO notify_outbox (
  tenant_id, id, event_id, event_type, aggregate_id, aggregate_version,
  payload, channel_id
)
VALUES (
  sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(event_id), sqlc.arg(event_type),
  sqlc.arg(aggregate_id), sqlc.arg(aggregate_version), sqlc.arg(payload)::jsonb,
  sqlc.arg(channel_id)
)
ON CONFLICT (tenant_id, event_id, channel_id) DO NOTHING
RETURNING *;

-- ClaimNextOutboxDelivery atomically leases one due delivery with SKIP LOCKED.
-- A stale delivering row is eligible after its lease expires, providing crash recovery.
-- name: ClaimNextOutboxDelivery :one
WITH candidate AS (
  SELECT outbox.tenant_id, outbox.id
  FROM notify_outbox AS outbox
  WHERE outbox.retry_count < sqlc.arg(max_attempts)::integer
    AND (
      (outbox.status IN ('pending', 'failed') AND outbox.next_attempt_at <= sqlc.arg(claimed_at)::timestamptz)
      OR (outbox.status = 'delivering' AND outbox.updated_at <= sqlc.arg(lease_expired_at)::timestamptz)
    )
  ORDER BY outbox.next_attempt_at, outbox.created_at, outbox.tenant_id, outbox.id
  FOR UPDATE OF outbox SKIP LOCKED
  LIMIT 1
)
UPDATE notify_outbox AS outbox
SET status = 'delivering',
    updated_at = sqlc.arg(claimed_at)::timestamptz
FROM candidate, notification_channels AS channel
WHERE outbox.tenant_id = candidate.tenant_id
  AND outbox.id = candidate.id
  AND channel.tenant_id = outbox.tenant_id
  AND channel.id = outbox.channel_id
RETURNING
  outbox.tenant_id,
  outbox.id,
  outbox.event_id,
  outbox.event_type,
  outbox.aggregate_id,
  outbox.aggregate_version,
  outbox.payload,
  outbox.channel_id,
  channel.type AS channel_type,
  channel.encrypted_config,
  outbox.retry_count,
  outbox.updated_at AS claimed_at;

-- MarkOutboxDelivered completes only the exact active lease so a stale worker
-- cannot overwrite a delivery reclaimed by a newer dispatcher.
-- name: MarkOutboxDelivered :execrows
UPDATE notify_outbox
SET status = 'delivered',
    last_error = '',
    updated_at = sqlc.arg(completed_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status = 'delivering'
  AND updated_at = sqlc.arg(claimed_at)::timestamptz;

-- MarkOutboxFailed records one redacted failed attempt and its next eligible
-- time while fencing updates from expired delivery leases.
-- name: MarkOutboxFailed :execrows
UPDATE notify_outbox
SET status = 'failed',
    retry_count = retry_count + 1,
    next_attempt_at = sqlc.arg(next_attempt_at)::timestamptz,
    last_error = sqlc.arg(error_code)::text,
    updated_at = sqlc.arg(failed_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status = 'delivering'
  AND updated_at = sqlc.arg(claimed_at)::timestamptz;
