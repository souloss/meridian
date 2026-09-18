package task

import (
	"context"
	"time"
	"uuid"
)

// OutboxDispatchArgs 请求对持久化通知出箱执行一次有界扫描。
type OutboxDispatchArgs struct{}

// Kind 返回出箱派发扫描的稳定 River 任务类型名。
func (OutboxDispatchArgs) Kind() string { return "meridian_outbox_dispatch" }

// OutboxDelivery 是一个已租约的事件及其加密投递配置。
type OutboxDelivery struct {
	// TenantID 标识拥有该事件与渠道的租户。
	TenantID uuid.UUID
	// ID 标识渠道特定的出箱行。
	ID uuid.UUID
	// EventID 是事件投递共享的稳定接收端去重键。
	EventID uuid.UUID
	// EventType 标识领域事件 schema。
	EventType string
	// AggregateID 标识发出该事件的聚合。
	AggregateID uuid.UUID
	// AggregateVersion 为聚合发出的事件排序。
	AggregateVersion int64
	// Payload 包含完整脱敏事件信封 JSON。
	Payload []byte
	// ChannelID 标识所选租户渠道。
	ChannelID uuid.UUID
	// ChannelType 选择未来的应用内、webhook 或邮件适配器。
	ChannelType string
	// EncryptedConfig 包含供已授权适配器使用的不透明渠道配置。
	EncryptedConfig []byte
	// RetryCount 是本次租约之前失败投递尝试的次数。
	RetryCount int
	// ClaimedAt 是 PostgreSQL 写入的租约栅栏令牌。
	ClaimedAt time.Time
}

// ClaimDeliveryInput 定义一次出箱扫描的领取资格与租约恢复。
type ClaimDeliveryInput struct {
	// ClaimedAt 是作为新租约栅栏令牌写入的 UTC 时间。
	ClaimedAt time.Time
	// LeaseExpiredAt 使处于投递中且过期的行具备崩溃恢复资格。
	LeaseExpiredAt time.Time
	// MaxAttempts 在事件契约的尝试上限后阻止派发。
	MaxAttempts int
}

// FailDeliveryInput 记录一次失败尝试而不持久化提供者文本。
type FailDeliveryInput struct {
	// Delivery 标识确切已租约的行与栅栏令牌。
	Delivery OutboxDelivery
	// ErrorCode 是稳定脱敏的失败分类。
	ErrorCode string
	// FailedAt 是尝试结束的 UTC 时间。
	FailedAt time.Time
	// NextAttemptAt 是下一次扫描可重新领取该行的 UTC 时间。
	NextAttemptAt time.Time
}

// OutboxStore 为至少一次投递持久化租约与终态结果。
type OutboxStore interface {
	ClaimOutboxDelivery(context.Context, ClaimDeliveryInput) (OutboxDelivery, bool, error)
	MarkOutboxDelivered(context.Context, OutboxDelivery, time.Time) error
	MarkOutboxFailed(context.Context, FailDeliveryInput) error
}

// OutboxDeliverer 通过所选渠道适配器发送一个已租约投递。
// 实现必须保留 EventID，且可能被调用多次。
type OutboxDeliverer interface {
	Deliver(context.Context, OutboxDelivery) error
}
