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

// 运行时配置常量。
const (
	// riverSchema 是 River 队列使用的数据库 schema 名。
	riverSchema = "river"
	// outboxDispatchInterval 是出箱派发的周期扫描间隔。
	outboxDispatchInterval = time.Minute
	// queueDefaultMaxWorkers 是默认队列并发执行的工作器数量上限。
	queueDefaultMaxWorkers = 4
	// outboxDispatchJobID 是周期性出箱派发任务在 River 中的稳定标识。
	outboxDispatchJobID = "meridian_outbox_dispatch"
)

// RuntimeDependencies 聚合工作器持久化与执行端口。
type RuntimeDependencies struct {
	// Executions 持久化仓库同步生命周期迁移。
	Executions ExecutionStore
	// SyncRunner 执行 M1 仓库同步流水线。
	SyncRunner SyncRunner
	// DiscoverRunner 执行 M1 仓库发现流水线。
	DiscoverRunner DiscoverRunner
	// MergeRunner 执行 M2 资产合并流水线。
	MergeRunner MergeRunner
	// AiGenerateRunner 执行 M3 资产 AI 生成流水线。
	AiGenerateRunner AiGenerateRunner
	// Outbox 持久化投递租约、重试与完成。
	Outbox OutboxStore
	// OutboxDeliverer 通过已配置的渠道适配器发送事件。
	OutboxDeliverer OutboxDeliverer
}

// Runtime 拥有 River 客户端与已注册的 Meridian 工作器。
type Runtime struct {
	client *river.Client[pgx.Tx]
}

// NewRuntime 为应用队列构造一个可执行的 River 客户端。
// 调用者必须仅在数据库迁移完成后启动它。
func NewRuntime(pool *pgxpool.Pool, dependencies RuntimeDependencies, logger *slog.Logger) (*Runtime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, NewCredentialSyncWorker(dependencies.Executions, dependencies.SyncRunner))
	river.AddWorker(workers, NewDiscoverWorker(dependencies.Executions, dependencies.DiscoverRunner))
	river.AddWorker(workers, NewMergeWorker(dependencies.Executions, dependencies.MergeRunner))
	if dependencies.AiGenerateRunner != nil {
		river.AddWorker(workers, NewAiGenerateWorker(dependencies.Executions, dependencies.AiGenerateRunner))
	}
	river.AddWorker(workers, NewOutboxDispatchWorker(dependencies.Outbox, dependencies.OutboxDeliverer))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger: logger,
		PeriodicJobs: []*river.PeriodicJob{river.NewPeriodicJob(
			river.PeriodicInterval(outboxDispatchInterval),
			func() (river.JobArgs, *river.InsertOpts) { return OutboxDispatchArgs{}, nil },
			&river.PeriodicJobOpts{ID: outboxDispatchJobID, RunOnStart: true},
		)},
		Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: queueDefaultMaxWorkers}},
		Schema: riverSchema, Workers: workers,
	})
	if err != nil {
		return nil, err
	}
	return &Runtime{client: client}, nil
}

// Client 返回用于事务性插入任务的 River 客户端。
func (runtime *Runtime) Client() *river.Client[pgx.Tx] { return runtime.client }

// Start 启动队列轮询与工作器执行。
func (runtime *Runtime) Start(ctx context.Context) error { return runtime.client.Start(ctx) }

// Stop 等待在途工作器并释放 River 资源。
func (runtime *Runtime) Stop(ctx context.Context) error { return runtime.client.Stop(ctx) }
