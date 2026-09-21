-- 建立 M5 订阅、订阅-通道关联与站内通知收件箱的持久化表。
-- 字段口径对齐 contracts/storage.yaml 的 subscriptions / subscription_channels / notifications 段；
-- 全部为租户维度表，事件投递沿用 M0 已建的 notify_outbox 事务外发模式。
-- +goose Up

CREATE TABLE subscriptions (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  target_type text NOT NULL CHECK (target_type IN ('tenant', 'service', 'asset', 'system_group', 'asset_kind')),
  target_id text,
  events text[] NOT NULL DEFAULT '{}'::text[],
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);

COMMENT ON TABLE subscriptions IS '用户按作用域订阅领域事件的通知路由规则。';
COMMENT ON COLUMN subscriptions.tenant_id IS '拥有该订阅的租户。';
COMMENT ON COLUMN subscriptions.id IS '应用生成的 UUID v7 订阅标识。';
COMMENT ON COLUMN subscriptions.user_id IS '接收站内通知的订阅用户。';
COMMENT ON COLUMN subscriptions.target_type IS '订阅作用域类型：tenant、service、asset、system_group 或 asset_kind。';
COMMENT ON COLUMN subscriptions.target_id IS '订阅作用域目标标识；tenant 作用域为空。';
COMMENT ON COLUMN subscriptions.events IS '该订阅匹配的领域事件类型列表。';
COMMENT ON COLUMN subscriptions.enabled IS '是否仅为未来事件生成通知；禁用保留历史通知。';
COMMENT ON COLUMN subscriptions.created_at IS '创建订阅时的 UTC 事务时间。';
COMMENT ON COLUMN subscriptions.updated_at IS '最近一次更新订阅时的 UTC 事务时间。';

-- 订阅的自然身份唯一约束：同一用户对同一作用域只保留一条订阅。
CREATE UNIQUE INDEX subscriptions_identity_uq
  ON subscriptions (tenant_id, user_id, target_type, target_id) NULLS NOT DISTINCT;

CREATE TABLE subscription_channels (
  tenant_id uuid NOT NULL,
  subscription_id uuid NOT NULL,
  channel_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, subscription_id, channel_id),
  FOREIGN KEY (tenant_id, subscription_id) REFERENCES subscriptions (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, channel_id) REFERENCES notification_channels (tenant_id, id) ON DELETE RESTRICT
);

COMMENT ON TABLE subscription_channels IS '订阅与其通知投递通道的多对多关系。';
COMMENT ON COLUMN subscription_channels.tenant_id IS '拥有该关联的租户。';
COMMENT ON COLUMN subscription_channels.subscription_id IS '所属订阅标识。';
COMMENT ON COLUMN subscription_channels.channel_id IS '所选通知通道标识。';
COMMENT ON COLUMN subscription_channels.created_at IS '建立通道关联时的 UTC 事务时间。';

CREATE TABLE notifications (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id uuid NOT NULL,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_id uuid NOT NULL,
  event_type text NOT NULL,
  title_key text NOT NULL,
  body_args jsonb NOT NULL DEFAULT '{}'::jsonb,
  link text,
  read_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, user_id, event_id)
);

COMMENT ON TABLE notifications IS '租户内用户可见的站内通知收件箱。';
COMMENT ON COLUMN notifications.tenant_id IS '拥有该通知的租户。';
COMMENT ON COLUMN notifications.id IS '应用生成的 UUID v7 通知标识。';
COMMENT ON COLUMN notifications.user_id IS '接收该通知的用户。';
COMMENT ON COLUMN notifications.event_id IS '产生该通知的领域事件标识，用于按用户去重。';
COMMENT ON COLUMN notifications.event_type IS '领域事件类型。';
COMMENT ON COLUMN notifications.title_key IS '渲染通知标题的 i18n 消息键。';
COMMENT ON COLUMN notifications.body_args IS '渲染通知正文的模板变量 JSON 对象。';
COMMENT ON COLUMN notifications.link IS '可选的资源定位链接。';
COMMENT ON COLUMN notifications.read_at IS '用户标记已读时间；未读为空。';
COMMENT ON COLUMN notifications.created_at IS '创建通知时的 UTC 事务时间。';

CREATE INDEX notifications_user_read_idx
  ON notifications (tenant_id, user_id, read_at, created_at);

-- 按外键依赖反向删除 M5 通知对象，仅供本地或演练环境回滚。
-- +goose Down
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS subscription_channels;
DROP TABLE IF EXISTS subscriptions;
