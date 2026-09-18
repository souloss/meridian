package task

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"
)

// 出箱派发域常量。
const (
	// outboxBatchSize 是单次扫描可领取并投递的最大事件数量。
	outboxBatchSize = 50
	// outboxMaxAttempts 是事件契约允许的最大投递尝试次数，超过后不再派发。
	outboxMaxAttempts = 6
	// outboxLease 是投递租约的有效时长，超时的行会被下一次扫描回收。
	outboxLease = 5 * time.Minute
)

// outboxBackoff 是失败重试的退避阶梯，按已失败次数递增取档。
var outboxBackoff = [...]time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute, 2 * time.Hour}

// 出箱投递失败分类常量（值 = outbox_delivery.error_code 同口径）。
const (
	// outboxErrorDeliveryUnavailable 表示渠道适配器尚未启用。
	outboxErrorDeliveryUnavailable = "delivery_unavailable"
	// outboxErrorDeliveryFailed 表示投递执行失败。
	outboxErrorDeliveryFailed = "delivery_failed"
)

// ErrOutboxDeliveryUnavailable 标记 M5 渠道适配器启用之前的边界。
var ErrOutboxDeliveryUnavailable = errors.New("outbox delivery adapter is unavailable")

// UnsupportedOutboxDeliverer 在配置具体 M5 适配器之前拒绝投递。
type UnsupportedOutboxDeliverer struct{}

// Deliver 在不检查或暴露加密配置的情况下返回一个稳定错误。
func (UnsupportedOutboxDeliverer) Deliver(context.Context, OutboxDelivery) error {
	return ErrOutboxDeliveryUnavailable
}

// OutboxDispatchWorker 执行一次有界的持久化出箱扫描。
type OutboxDispatchWorker struct {
	river.WorkerDefaults[OutboxDispatchArgs]
	store     OutboxStore
	deliverer OutboxDeliverer
	now       func() time.Time
}

// NewOutboxDispatchWorker 使用显式的持久化与投递端口构造派发器。
func NewOutboxDispatchWorker(store OutboxStore, deliverer OutboxDeliverer) *OutboxDispatchWorker {
	if deliverer == nil {
		deliverer = UnsupportedOutboxDeliverer{}
	}
	return &OutboxDispatchWorker{store: store, deliverer: deliverer, now: time.Now}
}

// Work 领取并投递至多一个有界批次。提供者失败会被持久化供下一次周期扫描处理，
// 而不是重试 River 扫描本身。
func (worker *OutboxDispatchWorker) Work(ctx context.Context, _ *river.Job[OutboxDispatchArgs]) error {
	if worker.store == nil {
		return errors.New("outbox dispatch worker has no persistence store")
	}
	for range outboxBatchSize {
		claimedAt := worker.now().UTC()
		delivery, claimed, err := worker.store.ClaimOutboxDelivery(ctx, ClaimDeliveryInput{
			ClaimedAt: claimedAt, LeaseExpiredAt: claimedAt.Add(-outboxLease), MaxAttempts: outboxMaxAttempts,
		})
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		if err := worker.deliverer.Deliver(ctx, delivery); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failedAt := worker.now().UTC()
			if err := worker.store.MarkOutboxFailed(ctx, FailDeliveryInput{
				Delivery: delivery, ErrorCode: outboxErrorCode(err), FailedAt: failedAt,
				NextAttemptAt: failedAt.Add(outboxRetryDelay(delivery.RetryCount)),
			}); err != nil {
				return err
			}
			continue
		}
		if err := worker.store.MarkOutboxDelivered(ctx, delivery, worker.now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

// outboxRetryDelay 按已失败次数返回对应的退避时长，越界时钳制到最后一档。
func outboxRetryDelay(previousFailures int) time.Duration {
	index := min(max(previousFailures, 0), len(outboxBackoff)-1)
	return outboxBackoff[index]
}

// outboxErrorCode 将投递错误归类为稳定的失败码。
func outboxErrorCode(err error) string {
	if errors.Is(err, ErrOutboxDeliveryUnavailable) {
		return outboxErrorDeliveryUnavailable
	}
	return outboxErrorDeliveryFailed
}
