package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/task"
)

// ClaimOutboxDelivery 租用一行到期投递，并返回其加密渠道投影。
func (store *RepositoryStore) ClaimOutboxDelivery(ctx context.Context, input task.ClaimDeliveryInput) (task.OutboxDelivery, bool, error) {
	row, err := store.queries.ClaimNextOutboxDelivery(ctx, generated.ClaimNextOutboxDeliveryParams{
		ClaimedAt: timestamp(input.ClaimedAt), MaxAttempts: int32(input.MaxAttempts), LeaseExpiredAt: timestamp(input.LeaseExpiredAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return task.OutboxDelivery{}, false, nil
		}
		return task.OutboxDelivery{}, false, normalizeError(err)
	}
	return task.OutboxDelivery{
		TenantID: row.TenantID, ID: row.ID, EventID: row.EventID, EventType: row.EventType,
		AggregateID: row.AggregateID, AggregateVersion: row.AggregateVersion, Payload: row.Payload,
		ChannelID: row.ChannelID, ChannelType: row.ChannelType, EncryptedConfig: row.EncryptedConfig,
		RetryCount: int(row.RetryCount), ClaimedAt: row.ClaimedAt.Time,
	}, true, nil
}

// MarkOutboxDelivered 完成一次活跃租约；过期租约被栅栏为无操作。
func (store *RepositoryStore) MarkOutboxDelivered(ctx context.Context, delivery task.OutboxDelivery, completedAt time.Time) error {
	_, err := store.queries.MarkOutboxDelivered(ctx, generated.MarkOutboxDeliveredParams{
		CompletedAt: timestamp(completedAt), TenantID: delivery.TenantID, ID: delivery.ID, ClaimedAt: timestamp(delivery.ClaimedAt),
	})
	return normalizeError(err)
}

// MarkOutboxFailed 记录一个稳定的失败码；过期租约被栅栏为无操作。
func (store *RepositoryStore) MarkOutboxFailed(ctx context.Context, input task.FailDeliveryInput) error {
	_, err := store.queries.MarkOutboxFailed(ctx, generated.MarkOutboxFailedParams{
		NextAttemptAt: timestamp(input.NextAttemptAt), ErrorCode: input.ErrorCode, FailedAt: timestamp(input.FailedAt),
		TenantID: input.Delivery.TenantID, ID: input.Delivery.ID, ClaimedAt: timestamp(input.Delivery.ClaimedAt),
	})
	return normalizeError(err)
}

var _ task.OutboxStore = (*RepositoryStore)(nil)
