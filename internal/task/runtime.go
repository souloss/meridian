package task

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

const riverSchema = "river"

// Runtime owns the River client and the registered Meridian workers.
type Runtime struct {
	client *river.Client[pgx.Tx]
}

// NewRuntime constructs an executable River client for the application queue.
// The caller must start it only after database migrations have completed.
func NewRuntime(pool *pgxpool.Pool, store ExecutionStore, runner SyncRunner, logger *slog.Logger) (*Runtime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, NewCredentialSyncWorker(store, runner))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:  logger,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
		Schema:  riverSchema,
		Workers: workers,
	})
	if err != nil {
		return nil, err
	}
	return &Runtime{client: client}, nil
}

// Client returns the River client used for transactionally inserting jobs.
func (runtime *Runtime) Client() *river.Client[pgx.Tx] { return runtime.client }

// Start starts queue polling and worker execution.
func (runtime *Runtime) Start(ctx context.Context) error { return runtime.client.Start(ctx) }

// Stop waits for in-flight workers and releases River resources.
func (runtime *Runtime) Stop(ctx context.Context) error { return runtime.client.Stop(ctx) }
