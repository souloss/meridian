package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// NotificationStore 实现 M5 订阅、通知通道与站内通知的持久化边界。
type NotificationStore struct {
	queries *generated.Queries
	pool    *pgxpool.Pool
}

// NewNotificationStore 将通知持久化绑定到原生 pgx 连接池。
func NewNotificationStore(pool *pgxpool.Pool) *NotificationStore {
	return &NotificationStore{queries: generated.New(pool), pool: pool}
}

// UpsertSubscription 幂等创建或替换一条订阅。
func (store *NotificationStore) UpsertSubscription(ctx context.Context, input service.PutSubscriptionInput) (service.SubscriptionRecord, error) {
	row, err := store.queries.UpsertSubscription(ctx, generated.UpsertSubscriptionParams{
		TenantID: input.TenantID, ID: uuid.NewV7(), UserID: input.UserID, TargetType: input.ScopeType,
		TargetID: input.ScopeID, Events: input.EventTypes, Enabled: input.Enabled,
	})
	if err != nil {
		return service.SubscriptionRecord{}, normalizeError(err)
	}
	return subscriptionFromRow(row), nil
}

// GetSubscription 按 id 返回一条订阅。
func (store *NotificationStore) GetSubscription(ctx context.Context, tenantID, id uuid.UUID) (service.SubscriptionRecord, error) {
	row, err := store.queries.GetSubscription(ctx, generated.GetSubscriptionParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.SubscriptionRecord{}, normalizeError(err)
	}
	return subscriptionFromRow(row), nil
}

// ListSubscriptions 返回某用户的全部订阅。
func (store *NotificationStore) ListSubscriptions(ctx context.Context, tenantID, userID uuid.UUID) ([]service.SubscriptionRecord, error) {
	rows, err := store.queries.ListSubscriptions(ctx, generated.ListSubscriptionsParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, normalizeError(err)
	}
	records := make([]service.SubscriptionRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, subscriptionFromRow(row))
	}
	return records, nil
}

// ReplaceSubscriptionChannels 原子替换一条订阅的通道关联。
func (store *NotificationStore) ReplaceSubscriptionChannels(ctx context.Context, tenantID, subscriptionID uuid.UUID, channelIDs []uuid.UUID) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return normalizeError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	if _, err := queries.ReplaceSubscriptionChannels(ctx, generated.ReplaceSubscriptionChannelsParams{TenantID: tenantID, SubscriptionID: subscriptionID}); err != nil {
		return normalizeError(err)
	}
	for _, channelID := range channelIDs {
		if _, err := queries.InsertSubscriptionChannel(ctx, generated.InsertSubscriptionChannelParams{TenantID: tenantID, SubscriptionID: subscriptionID, ChannelID: channelID}); err != nil {
			return normalizeError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	return nil
}

// ListSubscriptionChannels 返回一条订阅的通道标识。
func (store *NotificationStore) ListSubscriptionChannels(ctx context.Context, tenantID, subscriptionID uuid.UUID) ([]uuid.UUID, error) {
	return store.queries.ListSubscriptionChannels(ctx, generated.ListSubscriptionChannelsParams{TenantID: tenantID, SubscriptionID: subscriptionID})
}

// ListNotificationChannels 返回租户内全部通知通道。
func (store *NotificationStore) ListNotificationChannels(ctx context.Context, tenantID uuid.UUID) ([]service.NotificationChannelRecord, error) {
	rows, err := store.queries.ListNotificationChannels(ctx, tenantID)
	if err != nil {
		return nil, normalizeError(err)
	}
	records := make([]service.NotificationChannelRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, notificationChannelFromRow(row))
	}
	return records, nil
}

// CreateNotificationChannel 创建一条通知通道。
func (store *NotificationStore) CreateNotificationChannel(ctx context.Context, input service.NewNotificationChannel) (service.NotificationChannelRecord, error) {
	row, err := store.queries.CreateNotificationChannel(ctx, generated.CreateNotificationChannelParams{
		TenantID: input.TenantID, ID: input.ID, Type: input.Kind, Name: input.Name,
		EncryptedConfig: input.EncryptedConfig, Enabled: input.Enabled,
	})
	if err != nil {
		return service.NotificationChannelRecord{}, normalizeError(err)
	}
	return notificationChannelFromRow(row), nil
}

// GetNotificationChannel 按 id 返回一条通知通道。
func (store *NotificationStore) GetNotificationChannel(ctx context.Context, tenantID, id uuid.UUID) (service.NotificationChannelRecord, error) {
	row, err := store.queries.GetNotificationChannel(ctx, generated.GetNotificationChannelParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.NotificationChannelRecord{}, normalizeError(err)
	}
	return notificationChannelFromRow(row), nil
}

// UpdateNotificationChannel 应用通道补丁并递增 revision。
func (store *NotificationStore) UpdateNotificationChannel(ctx context.Context, patch service.NotificationChannelPatch) (service.NotificationChannelRecord, error) {
	row, err := store.queries.UpdateNotificationChannel(ctx, generated.UpdateNotificationChannelParams{
		TenantID: patch.TenantID, ID: patch.ID, Name: *patch.Name, Enabled: *patch.Enabled, ExpectedRevision: patch.ExpectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.NotificationChannelRecord{}, service.ErrPrecondition
		}
		return service.NotificationChannelRecord{}, normalizeError(err)
	}
	return notificationChannelFromRow(row), nil
}

// RotateNotificationChannelSecret 轮换通道加密配置并递增 revision。
func (store *NotificationStore) RotateNotificationChannelSecret(ctx context.Context, tenantID, channelID uuid.UUID, expectedRevision int64, encrypted []byte) (service.NotificationChannelRecord, error) {
	row, err := store.queries.RotateNotificationChannelSecret(ctx, generated.RotateNotificationChannelSecretParams{
		TenantID: tenantID, ID: channelID, ExpectedRevision: expectedRevision, EncryptedConfig: encrypted,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.NotificationChannelRecord{}, service.ErrPrecondition
		}
		return service.NotificationChannelRecord{}, normalizeError(err)
	}
	return notificationChannelFromRow(row), nil
}

// DeleteNotificationChannel 在 If-Match 乐观并发下删除一条通知通道。
func (store *NotificationStore) DeleteNotificationChannel(ctx context.Context, tenantID, channelID uuid.UUID, expectedRevision int64) error {
	changed, err := store.queries.DeleteNotificationChannel(ctx, generated.DeleteNotificationChannelParams{
		TenantID: tenantID, ID: channelID, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed == 0 {
		return service.ErrPrecondition
	}
	return nil
}

// ListVisibleChannelIDs 校验一组通道在租户内可见并返回有效子集。
func (store *NotificationStore) ListVisibleChannelIDs(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	return store.queries.ListVisibleChannelIDs(ctx, generated.ListVisibleChannelIDsParams{TenantID: tenantID, Ids: ids})
}

// ListNotifications 分页返回某用户的站内通知。
func (store *NotificationStore) ListNotifications(ctx context.Context, tenantID, userID uuid.UUID, unreadOnly bool, pageSize, pageOffset int32) ([]service.NotificationRecord, int64, int64, error) {
	rows, err := store.queries.ListNotifications(ctx, generated.ListNotificationsParams{
		TenantID: tenantID, UserID: userID, UnreadOnly: unreadOnly, PageLimit: pageSize, PageOffset: pageOffset,
	})
	if err != nil {
		return nil, 0, 0, normalizeError(err)
	}
	counts, err := store.queries.CountNotifications(ctx, generated.CountNotificationsParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, 0, 0, normalizeError(err)
	}
	records := make([]service.NotificationRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, notificationFromRow(row))
	}
	return records, counts.Total, counts.Unread, nil
}

// MarkNotificationRead 将一条通知标记为已读。
func (store *NotificationStore) MarkNotificationRead(ctx context.Context, tenantID, userID, notificationID uuid.UUID) error {
	changed, err := store.queries.MarkNotificationRead(ctx, generated.MarkNotificationReadParams{
		TenantID: tenantID, UserID: userID, ID: notificationID,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed == 0 {
		return service.ErrNotFound
	}
	return nil
}

// MarkAllNotificationsRead 将某用户全部未读通知标记为已读。
func (store *NotificationStore) MarkAllNotificationsRead(ctx context.Context, tenantID, userID uuid.UUID) error {
	_, err := store.queries.MarkAllNotificationsRead(ctx, generated.MarkAllNotificationsReadParams{TenantID: tenantID, UserID: userID})
	return normalizeError(err)
}

// EnqueueNotificationTest 将一条通道专属测试投递写入 notify_outbox 并返回已受理任务。
// 测试事件负载为固定脱敏信封，派发器按通道类型执行真实投递（webhook/email/in_app）。
func (store *NotificationStore) EnqueueNotificationTest(ctx context.Context, tenantID uuid.UUID, channel service.NotificationChannelRecord) (service.JobAccepted, error) {
	eventID := uuid.NewV7()
	payload, err := json.Marshal(channelTestEnvelope{
		EventID: eventID, EventType: channelTestEventType, ChannelID: channel.ID, Kind: channel.Kind,
	})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("encode channel test envelope: %w", err)
	}
	row, err := store.queries.CreateNotifyOutbox(ctx, generated.CreateNotifyOutboxParams{
		TenantID: tenantID, ID: uuid.NewV7(), EventID: eventID, EventType: channelTestEventType,
		AggregateID: channel.ID, AggregateVersion: 1, Payload: payload, ChannelID: channel.ID,
	})
	if err != nil {
		return service.JobAccepted{}, normalizeError(err)
	}
	return service.JobAccepted{JobID: row.ID, Deduplicated: false}, nil
}

// channelTestEventType 是测试通知通道投递的稳定事件类型（不注册在领域事件枚举，仅供测试通道）。
const channelTestEventType = "channel.test"

// channelTestEnvelope 是测试通道投递的脱敏事件信封。
type channelTestEnvelope struct {
	EventID   uuid.UUID `json:"eventId"`
	EventType string    `json:"eventType"`
	ChannelID uuid.UUID `json:"channelId"`
	Kind      string    `json:"kind"`
}

func subscriptionFromRow(row generated.Subscription) service.SubscriptionRecord {
	return service.SubscriptionRecord{
		TenantID: row.TenantID, ID: row.ID, UserID: row.UserID, ScopeType: row.TargetType,
		ScopeID: row.TargetID, EventTypes: row.Events, Enabled: row.Enabled,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

// notificationChannelTypeWebhook 是 Webhook 通知通道的列值（notification_channels.type）。
// 仅 webhook 通道允许配置秘密，故用它推导 SecretConfigured 投影。
const notificationChannelTypeWebhook = "webhook"

func notificationChannelFromRow(row generated.NotificationChannel) service.NotificationChannelRecord {
	return service.NotificationChannelRecord{
		TenantID: row.TenantID, ID: row.ID, Kind: row.Type, Name: row.Name, Enabled: row.Enabled,
		EncryptedConfig: row.EncryptedConfig, Revision: row.Revision,
		SecretConfigured: row.Type == notificationChannelTypeWebhook,
		CreatedAt:        row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func notificationFromRow(row generated.Notification) service.NotificationRecord {
	var readAt *time.Time
	if row.ReadAt.Valid {
		readAt = &row.ReadAt.Time
	}
	return service.NotificationRecord{
		TenantID: row.TenantID, ID: row.ID, UserID: row.UserID, EventType: row.EventType,
		TitleKey: row.TitleKey, BodyArgs: row.BodyArgs, ResourceURL: row.Link,
		ReadAt: readAt, CreatedAt: row.CreatedAt.Time,
	}
}

var _ service.NotificationStore = (*NotificationStore)(nil)
