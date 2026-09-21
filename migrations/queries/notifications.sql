-- M5 订阅、通知通道与站内通知收件箱的持久化查询。
-- 全部查询保留 tenant_id 谓词，事件路由按订阅匹配后写入 notify_outbox 或 notifications。

-- 幂等创建或替换一条订阅（自然身份 tenant_id + user_id + target_type + target_id）。
-- name: UpsertSubscription :one
INSERT INTO subscriptions (tenant_id, id, user_id, target_type, target_id, events, enabled)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(target_type), sqlc.narg(target_id), sqlc.arg(events)::text[], sqlc.arg(enabled))
ON CONFLICT (tenant_id, user_id, target_type, target_id)
DO UPDATE SET events = EXCLUDED.events, enabled = EXCLUDED.enabled, updated_at = now()
RETURNING *;

-- 替换一条订阅的通道关联：先删除再按序重建。
-- name: ReplaceSubscriptionChannels :execrows
WITH deleted AS (
  DELETE FROM subscription_channels
  WHERE tenant_id = sqlc.arg(tenant_id)
    AND subscription_id = sqlc.arg(subscription_id)
)
SELECT 1;

-- name: InsertSubscriptionChannel :execrows
INSERT INTO subscription_channels (tenant_id, subscription_id, channel_id)
VALUES (sqlc.arg(tenant_id), sqlc.arg(subscription_id), sqlc.arg(channel_id))
ON CONFLICT (tenant_id, subscription_id, channel_id) DO NOTHING;

-- 列出用户的全部订阅。
-- name: ListSubscriptions :many
SELECT *
FROM subscriptions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND user_id = sqlc.arg(user_id)
ORDER BY created_at, id;

-- name: ListSubscriptionChannels :many
SELECT channel_id
FROM subscription_channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND subscription_id = sqlc.arg(subscription_id)
ORDER BY channel_id;

-- 批量返回多个订阅的通道关联，供列表场景去 N+1。
-- name: ListSubscriptionChannelsForSubscriptions :many
SELECT subscription_id, channel_id
FROM subscription_channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND subscription_id = ANY(sqlc.arg(subscription_ids)::uuid[])
ORDER BY subscription_id, channel_id;

-- 返回匹配某一作用域与事件类型的启用订阅及通道（供事件路由）。
-- tenant 作用域匹配一切；service/asset/asset_kind/system_group 按对应目标匹配。
-- DISTINCT 去重：一个服务同时属于多个匹配分组时，同一 (user, subscription, channel) 只返回一次。
-- name: ListEventRoutes :many
SELECT DISTINCT s.user_id AS user_id, s.id AS subscription_id, c.id AS channel_id, c.type AS channel_type
FROM subscriptions AS s
JOIN subscription_channels AS sc ON sc.tenant_id = s.tenant_id AND sc.subscription_id = s.id
JOIN notification_channels AS c ON c.tenant_id = sc.tenant_id AND c.id = sc.channel_id AND c.enabled = true
WHERE s.tenant_id = sqlc.arg(tenant_id)
  AND s.enabled = true
  AND sqlc.arg(event_type)::text = ANY(s.events)
  AND (
    s.target_type = 'tenant'
    OR (s.target_type = 'service' AND s.target_id = sqlc.narg(service_id)::text)
    OR (s.target_type = 'asset' AND s.target_id = sqlc.narg(asset_id)::text)
    OR (s.target_type = 'asset_kind' AND s.target_id = sqlc.narg(asset_kind)::text)
    OR (s.target_type = 'system_group' AND s.target_id IN (
        SELECT sg.group_id::text FROM system_group_members AS sg
        WHERE sg.tenant_id = s.tenant_id AND sg.service_id = sqlc.narg(service_id)
    ))
  );

-- 列出租户内全部通知通道。
-- name: ListNotificationChannels :many
SELECT *
FROM notification_channels
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY created_at, id;

-- 创建一条通知通道；encrypted_config 已由应用侧加密。
-- name: CreateNotificationChannel :one
INSERT INTO notification_channels (tenant_id, id, type, name, encrypted_config, enabled)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(type), sqlc.arg(name), sqlc.arg(encrypted_config), sqlc.arg(enabled))
RETURNING *;

-- 返回一条通知通道。
-- name: GetNotificationChannel :one
SELECT *
FROM notification_channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id);

-- 更新一条通知通道的可变字段并递增 revision。
-- name: UpdateNotificationChannel :one
UPDATE notification_channels
SET name = sqlc.arg(name),
    enabled = sqlc.arg(enabled),
    revision = revision + 1,
    updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 轮换一条通知通道的加密配置并递增 revision。
-- name: RotateNotificationChannelSecret :one
UPDATE notification_channels
SET encrypted_config = sqlc.arg(encrypted_config),
    revision = revision + 1,
    updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision)
RETURNING *;

-- 校验一组通道 ID 在租户内可见并返回其有效子集。
-- name: ListVisibleChannelIDs :many
SELECT id
FROM notification_channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY id;

-- 分页列出某用户的站内通知（按可选未读过滤），并在单次往返内统计全量 total/unread。
-- name: ListNotifications :many
WITH counts AS (
  SELECT count(*)::bigint AS total,
         count(*) FILTER (WHERE read_at IS NULL)::bigint AS unread
  FROM notifications
  WHERE tenant_id = sqlc.arg(tenant_id)
    AND user_id = sqlc.arg(user_id)
)
SELECT n.*, counts.total, counts.unread
FROM notifications AS n
CROSS JOIN counts
WHERE n.tenant_id = sqlc.arg(tenant_id)
  AND n.user_id = sqlc.arg(user_id)
  AND (NOT sqlc.arg(unread_only)::boolean OR n.read_at IS NULL)
ORDER BY n.created_at DESC, n.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- 统计某用户的通知总数与未读数（独立于分页，未读数不受 unread_only 影响）。
-- 仅当 ListNotifications 分页为空时调用，作为 total/unread 的空页兜底。
-- name: CountNotifications :one
SELECT count(*)::bigint AS total,
       count(*) FILTER (WHERE read_at IS NULL)::bigint AS unread
FROM notifications
WHERE tenant_id = sqlc.arg(tenant_id)
  AND user_id = sqlc.arg(user_id);

-- 创建一条站内通知（按用户与事件去重）。
-- name: CreateNotification :one
INSERT INTO notifications (tenant_id, id, user_id, event_id, event_type, title_key, body_args, link)
VALUES (sqlc.arg(tenant_id), sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(event_id), sqlc.arg(event_type), sqlc.arg(title_key), sqlc.arg(body_args)::jsonb, sqlc.narg(link))
ON CONFLICT (tenant_id, user_id, event_id) DO NOTHING
RETURNING *;

-- 将一条通知标记为已读。
-- name: MarkNotificationRead :execrows
UPDATE notifications
SET read_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
  AND read_at IS NULL;

-- 将某用户全部未读通知标记为已读。
-- name: MarkAllNotificationsRead :execrows
UPDATE notifications
SET read_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND user_id = sqlc.arg(user_id)
  AND read_at IS NULL;

-- 删除一条通知通道（If-Match 乐观并发）。
-- name: DeleteNotificationChannel :execrows
DELETE FROM notification_channels
WHERE tenant_id = sqlc.arg(tenant_id)
  AND id = sqlc.arg(id)
  AND revision = sqlc.arg(expected_revision);
