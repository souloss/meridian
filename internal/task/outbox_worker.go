package task

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"
)

const (
	outboxBatchSize   = 50
	outboxMaxAttempts = 6
	outboxLease       = 5 * time.Minute
)

var outboxBackoff = [...]time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute, 2 * time.Hour}

// ErrOutboxDeliveryUnavailable marks the boundary before M5 channel adapters are enabled.
var ErrOutboxDeliveryUnavailable = errors.New("outbox delivery adapter is unavailable")

// UnsupportedOutboxDeliverer refuses delivery until a concrete M5 adapter is configured.
type UnsupportedOutboxDeliverer struct{}

// Deliver returns a stable error without inspecting or exposing encrypted configuration.
func (UnsupportedOutboxDeliverer) Deliver(context.Context, OutboxDelivery) error {
	return ErrOutboxDeliveryUnavailable
}

// OutboxDispatchWorker executes one bounded durable outbox scan.
type OutboxDispatchWorker struct {
	river.WorkerDefaults[OutboxDispatchArgs]
	store     OutboxStore
	deliverer OutboxDeliverer
	now       func() time.Time
}

// NewOutboxDispatchWorker constructs a dispatcher with explicit persistence and delivery ports.
func NewOutboxDispatchWorker(store OutboxStore, deliverer OutboxDeliverer) *OutboxDispatchWorker {
	if deliverer == nil {
		deliverer = UnsupportedOutboxDeliverer{}
	}
	return &OutboxDispatchWorker{store: store, deliverer: deliverer, now: time.Now}
}

// Work claims and delivers at most one bounded batch. Provider failures are
// persisted for the next periodic scan instead of retrying the River scan itself.
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

func outboxRetryDelay(previousFailures int) time.Duration {
	index := min(max(previousFailures, 0), len(outboxBackoff)-1)
	return outboxBackoff[index]
}

func outboxErrorCode(err error) string {
	if errors.Is(err, ErrOutboxDeliveryUnavailable) {
		return "delivery_unavailable"
	}
	return "delivery_failed"
}
