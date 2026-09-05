package task

import (
	"context"
	"time"
	"uuid"
)

// OutboxDispatchArgs requests one bounded scan of the durable notification outbox.
type OutboxDispatchArgs struct{}

// Kind returns the stable River kind for an outbox dispatch scan.
func (OutboxDispatchArgs) Kind() string { return "meridian_outbox_dispatch" }

// OutboxDelivery is one leased event and its encrypted delivery configuration.
type OutboxDelivery struct {
	// TenantID identifies the tenant that owns the event and channel.
	TenantID uuid.UUID
	// ID identifies the channel-specific outbox row.
	ID uuid.UUID
	// EventID is the stable receiver deduplication key shared by event deliveries.
	EventID uuid.UUID
	// EventType identifies the domain event schema.
	EventType string
	// AggregateID identifies the aggregate that emitted the event.
	AggregateID uuid.UUID
	// AggregateVersion orders events emitted by the aggregate.
	AggregateVersion int64
	// Payload contains the complete secret-free event envelope JSON.
	Payload []byte
	// ChannelID identifies the selected tenant channel.
	ChannelID uuid.UUID
	// ChannelType selects the future in-app, webhook, or email adapter.
	ChannelType string
	// EncryptedConfig contains opaque channel configuration for an authorized adapter.
	EncryptedConfig []byte
	// RetryCount is the number of failed delivery attempts before this lease.
	RetryCount int
	// ClaimedAt is the lease fencing token written by PostgreSQL.
	ClaimedAt time.Time
}

// ClaimDeliveryInput defines eligibility and lease recovery for one outbox scan.
type ClaimDeliveryInput struct {
	// ClaimedAt is the UTC time written as the new lease fencing token.
	ClaimedAt time.Time
	// LeaseExpiredAt makes stale delivering rows eligible for crash recovery.
	LeaseExpiredAt time.Time
	// MaxAttempts prevents dispatch after the event contract's attempt limit.
	MaxAttempts int
}

// FailDeliveryInput records one failed attempt without persisting provider text.
type FailDeliveryInput struct {
	// Delivery identifies the exact leased row and fencing token.
	Delivery OutboxDelivery
	// ErrorCode is a stable secret-free failure classification.
	ErrorCode string
	// FailedAt is the UTC time when the attempt finished.
	FailedAt time.Time
	// NextAttemptAt is the UTC time when the next scan may reclaim the row.
	NextAttemptAt time.Time
}

// OutboxStore persists leases and terminal results for at-least-once delivery.
type OutboxStore interface {
	ClaimOutboxDelivery(context.Context, ClaimDeliveryInput) (OutboxDelivery, bool, error)
	MarkOutboxDelivered(context.Context, OutboxDelivery, time.Time) error
	MarkOutboxFailed(context.Context, FailDeliveryInput) error
}

// OutboxDeliverer sends one leased delivery through its selected channel adapter.
// Implementations must preserve EventID and may be called more than once.
type OutboxDeliverer interface {
	Deliver(context.Context, OutboxDelivery) error
}
