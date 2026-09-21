package service

import (
	"context"
	"time"
	"uuid"
)

// NotificationChannelRecord 是租户可见的通知通道投影。
type NotificationChannelRecord struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是通道标识。
	ID uuid.UUID
	// Kind 是通道类别（in_app/webhook/email）。
	Kind string
	// Name 是租户内展示的通道名称。
	Name string
	// Endpoint 是通道端点（webhook URL 或 email 邮箱，in_app 为空）。
	Endpoint *string
	// EncryptedConfig 是应用侧加密后的端点与秘密配置（仅服务层内部用于解密投影）。
	EncryptedConfig []byte
	// Enabled 表示未来事件是否可通过该通道投递。
	Enabled bool
	// Revision 是乐观并发版本号。
	Revision int64
	// SecretConfigured 表示该通道是否已配置秘密。
	SecretConfigured bool
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// NewNotificationChannel 承载一次通道插入所需的校验后值。
type NewNotificationChannel struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是通道标识。
	ID uuid.UUID
	// Kind 是通道类别。
	Kind string
	// Name 是通道名称。
	Name string
	// Endpoint 是通道端点（可为空）。
	Endpoint *string
	// Secret 是仅写入秘密（webhook），不落明文。
	Secret string
	// EncryptedConfig 是应用侧加密后的端点与秘密配置。
	EncryptedConfig []byte
	// Enabled 表示通道是否启用。
	Enabled bool
}

// NotificationChannelPatch 承载一次通道更新的显式 PATCH 字段。
type NotificationChannelPatch struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是通道标识。
	ID uuid.UUID
	// ExpectedRevision 是乐观并发所需版本号。
	ExpectedRevision int64
	// Name 是新的通道名称（可为空）。
	Name *string
	// Enabled 是新的启用状态（可为空）。
	Enabled *bool
}

// SubscriptionRecord 是租户可见的订阅投影。
type SubscriptionRecord struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是订阅标识。
	ID uuid.UUID
	// UserID 是接收站内通知的订阅用户。
	UserID uuid.UUID
	// ScopeType 是订阅作用域类别。
	ScopeType string
	// ScopeID 是订阅作用域目标标识（tenant 作用域为空）。
	ScopeID *string
	// EventTypes 是匹配的领域事件类型列表。
	EventTypes []string
	// ChannelIDs 是所选通知通道标识列表。
	ChannelIDs []uuid.UUID
	// Enabled 表示是否仅为未来事件生成通知。
	Enabled bool
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// PutSubscriptionInput 承载一次订阅创建或替换请求。
type PutSubscriptionInput struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// UserID 是订阅用户标识。
	UserID uuid.UUID
	// ScopeType 是订阅作用域类别。
	ScopeType string
	// ScopeID 是订阅作用域目标标识（可为空）。
	ScopeID *string
	// EventTypes 是匹配的领域事件类型列表。
	EventTypes []string
	// ChannelIDs 是所选通道标识列表。
	ChannelIDs []uuid.UUID
	// Enabled 表示是否启用。
	Enabled bool
}

// NotificationRecord 是一条站内通知投影。
type NotificationRecord struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是通知标识。
	ID uuid.UUID
	// UserID 是接收通知的用户。
	UserID uuid.UUID
	// EventType 是领域事件类型。
	EventType string
	// TitleKey 是渲染通知标题的 i18n 消息键。
	TitleKey string
	// BodyArgs 是渲染通知正文的模板变量 JSON 对象。
	BodyArgs []byte
	// ResourceURL 是资源定位链接（可为空）。
	ResourceURL *string
	// ReadAt 是标记已读时间（可为空）。
	ReadAt *time.Time
	// CreatedAt 是创建时间。
	CreatedAt time.Time
}

// NotificationPageRecord 是一页站内通知投影。
type NotificationPageRecord struct {
	// Total 是命中总数。
	Total int
	// Page 是当前页码。
	Page int
	// PageSize 是每页条数。
	PageSize int
	// UnreadCount 是未读数量。
	UnreadCount int
	// Items 是通知列表。
	Items []NotificationRecord
}

// NotificationStore 是 M5 订阅、通知通道与站内通知的持久化边界。
type NotificationStore interface {
	UpsertSubscription(context.Context, PutSubscriptionInput) (SubscriptionRecord, error)
	ListSubscriptions(context.Context, uuid.UUID, uuid.UUID) ([]SubscriptionRecord, error)
	ReplaceSubscriptionChannels(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) error
	ListSubscriptionChannels(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error)
	// ListSubscriptionChannelsForSubscriptions 批量返回多个订阅的通道关联，供列表去 N+1。
	ListSubscriptionChannelsForSubscriptions(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	ListNotificationChannels(context.Context, uuid.UUID) ([]NotificationChannelRecord, error)
	CreateNotificationChannel(context.Context, NewNotificationChannel) (NotificationChannelRecord, error)
	GetNotificationChannel(context.Context, uuid.UUID, uuid.UUID) (NotificationChannelRecord, error)
	UpdateNotificationChannel(context.Context, NotificationChannelPatch) (NotificationChannelRecord, error)
	RotateNotificationChannelSecret(context.Context, uuid.UUID, uuid.UUID, int64, []byte) (NotificationChannelRecord, error)
	DeleteNotificationChannel(context.Context, uuid.UUID, uuid.UUID, int64) error
	ListVisibleChannelIDs(context.Context, uuid.UUID, []uuid.UUID) ([]uuid.UUID, error)
	EnqueueNotificationTest(context.Context, uuid.UUID, NotificationChannelRecord) (JobAccepted, error)
	ListNotifications(context.Context, uuid.UUID, uuid.UUID, bool, int32, int32) ([]NotificationRecord, int64, int64, error)
	MarkNotificationRead(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	MarkAllNotificationsRead(context.Context, uuid.UUID, uuid.UUID) error
}
