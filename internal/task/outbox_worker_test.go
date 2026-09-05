package task

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river"
)

type recordingOutboxStore struct {
	deliveries []OutboxDelivery
	claims     []ClaimDeliveryInput
	delivered  []OutboxDelivery
	failed     []FailDeliveryInput
}

func (store *recordingOutboxStore) ClaimOutboxDelivery(_ context.Context, input ClaimDeliveryInput) (OutboxDelivery, bool, error) {
	store.claims = append(store.claims, input)
	if len(store.deliveries) == 0 {
		return OutboxDelivery{}, false, nil
	}
	delivery := store.deliveries[0]
	store.deliveries = store.deliveries[1:]
	delivery.ClaimedAt = input.ClaimedAt
	return delivery, true, nil
}

func (store *recordingOutboxStore) MarkOutboxDelivered(_ context.Context, delivery OutboxDelivery, _ time.Time) error {
	store.delivered = append(store.delivered, delivery)
	return nil
}

func (store *recordingOutboxStore) MarkOutboxFailed(_ context.Context, input FailDeliveryInput) error {
	store.failed = append(store.failed, input)
	return nil
}

type recordingOutboxDeliverer struct {
	deliveries []OutboxDelivery
	err        error
}

func (deliverer *recordingOutboxDeliverer) Deliver(_ context.Context, delivery OutboxDelivery) error {
	deliverer.deliveries = append(deliverer.deliveries, delivery)
	return deliverer.err
}

func TestOutboxDispatchWorkerCompletesLeasedDeliveries(t *testing.T) {
	t.Parallel()
	delivery := OutboxDelivery{TenantID: uuid.NewV7(), ID: uuid.NewV7(), EventID: uuid.NewV7(), RetryCount: 0}
	store := &recordingOutboxStore{deliveries: []OutboxDelivery{delivery}}
	deliverer := &recordingOutboxDeliverer{}
	worker := NewOutboxDispatchWorker(store, deliverer)
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	worker.now = func() time.Time { return now }

	if err := worker.Work(t.Context(), &river.Job[OutboxDispatchArgs]{}); err != nil {
		t.Fatalf("dispatch outbox: %v", err)
	}
	if len(deliverer.deliveries) != 1 || len(store.delivered) != 1 || len(store.failed) != 0 {
		t.Fatalf("delivery calls = send %d success %d failed %d, want 1/1/0", len(deliverer.deliveries), len(store.delivered), len(store.failed))
	}
	if len(store.claims) != 2 || !store.claims[0].LeaseExpiredAt.Equal(now.Add(-outboxLease)) || store.claims[0].MaxAttempts != outboxMaxAttempts {
		t.Fatalf("claim inputs = %#v, want lease and terminal empty scan", store.claims)
	}
}

func TestOutboxDispatchWorkerPersistsRedactedFailureAndBackoff(t *testing.T) {
	t.Parallel()
	delivery := OutboxDelivery{TenantID: uuid.NewV7(), ID: uuid.NewV7(), RetryCount: 1}
	store := &recordingOutboxStore{deliveries: []OutboxDelivery{delivery}}
	deliverer := &recordingOutboxDeliverer{err: errors.New("provider said secret=do-not-store")}
	worker := NewOutboxDispatchWorker(store, deliverer)
	now := time.Date(2026, 9, 5, 11, 0, 0, 0, time.UTC)
	worker.now = func() time.Time { return now }

	if err := worker.Work(t.Context(), &river.Job[OutboxDispatchArgs]{}); err != nil {
		t.Fatalf("dispatch failed outbox: %v", err)
	}
	if len(store.failed) != 1 || len(store.delivered) != 0 {
		t.Fatalf("failure/success calls = %d/%d, want 1/0", len(store.failed), len(store.delivered))
	}
	failure := store.failed[0]
	if failure.ErrorCode != "delivery_failed" || !failure.NextAttemptAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("failure = %#v, want redacted code and second backoff", failure)
	}
}

func TestOutboxDispatchWorkerClassifiesUnavailableAdapter(t *testing.T) {
	t.Parallel()
	store := &recordingOutboxStore{deliveries: []OutboxDelivery{{ID: uuid.NewV7()}}}
	worker := NewOutboxDispatchWorker(store, nil)
	worker.now = func() time.Time { return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC) }

	if err := worker.Work(t.Context(), &river.Job[OutboxDispatchArgs]{}); err != nil {
		t.Fatalf("dispatch unavailable adapter: %v", err)
	}
	if len(store.failed) != 1 || store.failed[0].ErrorCode != "delivery_unavailable" {
		t.Fatalf("unavailable failure = %#v", store.failed)
	}
}
