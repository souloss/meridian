-- 解析领域事件信封中使用的稳定租户标识。
-- name: GetTenantSlugForEvent :one
SELECT slug
FROM tenants
WHERE id = sqlc.arg(tenant_id);

-- 与任务终态和出站事件行在同一事务中记录一条追加式、脱敏的系统事实。
-- name: AppendJobFailureAudit :one
INSERT INTO audit_logs (id, tenant_id, actor_id, actor_type, action, target_type, target_id, detail, ip, request_id)
VALUES (
  sqlc.arg(id), sqlc.arg(tenant_id), NULL, 'system', 'job.failed', 'job', sqlc.arg(job_id),
  sqlc.arg(detail)::jsonb, NULL, NULL
)
RETURNING *;

-- 为 M0 运维事件返回确定性的通道目标；M5 订阅路由会提供更精确的目标集合。
-- name: ListEnabledNotificationChannelIDs :many
SELECT id
FROM notification_channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND enabled = true
ORDER BY id;

-- 写入一条通道专属投递记录，并保留接收方用于至少一次去重的共享事件标识。
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

-- 使用 SKIP LOCKED 原子租约领取一条到期投递。
-- 状态为 delivering 的记录在租约过期后重新变为可领取，以便从崩溃中恢复。
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

-- 只完成准确的当前租约，防止旧 worker 覆盖新调度器重新领取的投递。
-- name: MarkOutboxDelivered :execrows
UPDATE notify_outbox
SET status = 'delivered',
    last_error = '',
    updated_at = sqlc.arg(completed_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND status = 'delivering'
  AND updated_at = sqlc.arg(claimed_at)::timestamptz;

-- 记录一次脱敏失败尝试及下一次可执行时间，并隔离已过期租约的更新。
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
