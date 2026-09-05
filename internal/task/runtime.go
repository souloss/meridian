package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

const riverSchema = "river"
const outboxDispatchInterval = time.Minute

// RuntimeDependencies groups worker persistence and execution ports.
type RuntimeDependencies struct {
	// Executions persists repository synchronization lifecycle transitions.
	Executions ExecutionStore
	// SyncRunner executes the M1 repository synchronization pipeline.
	SyncRunner SyncRunner
	// Outbox persists delivery leases, retries, and completions.
	Outbox OutboxStore
	// OutboxDeliverer sends events through configured channel adapters.
	OutboxDeliverer OutboxDeliverer
}

// Runtime owns the River client and the registered Meridian workers.
type Runtime struct {
	client *river.Client[pgx.Tx]
}

// NewRuntime constructs an executable River client for the application queue.
// The caller must start it only after database migrations have completed.
func NewRuntime(pool *pgxpool.Pool, dependencies RuntimeDependencies, logger *slog.Logger) (*Runtime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, NewCredentialSyncWorker(dependencies.Executions, dependencies.SyncRunner))
	river.AddWorker(workers, NewOutboxDispatchWorker(dependencies.Outbox, dependencies.OutboxDeliverer))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger: logger,
		PeriodicJobs: []*river.PeriodicJob{river.NewPeriodicJob(
			river.PeriodicInterval(outboxDispatchInterval),
			func() (river.JobArgs, *river.InsertOpts) { return OutboxDispatchArgs{}, nil },
			&river.PeriodicJobOpts{ID: "meridian_outbox_dispatch", RunOnStart: true},
		)},
		Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
		Schema: riverSchema, Workers: workers,
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
