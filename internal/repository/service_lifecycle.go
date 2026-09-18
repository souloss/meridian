package repository

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/riverqueue/river"
)

// ServiceLifecycleStore 实现服务生命周期、公开读取与软删除持久化边界。
// 它内嵌 RepositoryStore 复用服务读取路径，并携带 River 客户端用于事务内取消。
type ServiceLifecycleStore struct {
	*RepositoryStore
	riverClient *river.Client[pgx.Tx]
}

// NewServiceLifecycleStore 将服务生命周期持久化绑定到原生连接池。
func NewServiceLifecycleStore(pool *pgxpool.Pool) *ServiceLifecycleStore {
	return &ServiceLifecycleStore{RepositoryStore: NewRepositoryStore(pool)}
}

// NewServiceLifecycleStoreWithRiver 将服务生命周期持久化绑定到 River 客户端，
// 使删除操作可事务内取消待处理任务。
func NewServiceLifecycleStoreWithRiver(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *ServiceLifecycleStore {
	return &ServiceLifecycleStore{RepositoryStore: NewRepositoryStore(pool), riverClient: riverClient}
}

// BindRiver 在运行时构造后挂接进程 River 客户端。
func (store *ServiceLifecycleStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
}

// GetServiceBySlug 返回租户边界内的一条活跃服务。
func (store *ServiceLifecycleStore) GetServiceBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceBySlug(ctx, generated.GetServiceBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// GetServiceForUpdate 锁定一条活跃服务行以供条件变更。
func (store *ServiceLifecycleStore) GetServiceForUpdate(ctx context.Context, tenantID, id uuid.UUID) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceForUpdate(ctx, generated.GetServiceForUpdateParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// GetPublicServiceBySlug 按租户与服务 slug 解析一条活跃服务。
func (store *ServiceLifecycleStore) GetPublicServiceBySlug(ctx context.Context, tenantSlug, serviceSlug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetPublicServiceBySlug(ctx, generated.GetPublicServiceBySlugParams{TenantSlug: tenantSlug, ServiceSlug: serviceSlug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// UpdateService 在其修订号下应用一次条件服务补丁。
func (store *ServiceLifecycleStore) UpdateService(ctx context.Context, patch service.ServicePatch) (service.ServiceRecord, error) {
	row, err := store.queries.UpdateService(ctx, generated.UpdateServiceParams{
		TenantID: patch.TenantID, ID: patch.ID, ExpectedRevision: patch.ExpectedRevision,
		DisplayName: patch.DisplayName, Description: patch.Description, SetDescription: patch.SetDescription,
		Visibility: patch.Visibility, Lifecycle: patch.Lifecycle,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ServiceRecord{}, service.ErrPrecondition
		}
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// DeleteService 软删除一条服务并级联逻辑状态，由其修订号栅栏保护。
// 待处理的服务作用域任务在同一事务内被取消。
func (store *ServiceLifecycleStore) DeleteService(ctx context.Context, tenantID, id uuid.UUID, expectedRevision int64) (service.ServiceRecord, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.ServiceRecord{}, fmt.Errorf("begin delete service transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)

	row, err := queries.DeleteService(ctx, generated.DeleteServiceParams{TenantID: tenantID, ID: id, ExpectedRevision: expectedRevision})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.ServiceRecord{}, service.ErrPrecondition
		}
		return service.ServiceRecord{}, normalizeError(err)
	}
	if _, err := queries.DeleteServiceSourceSpecs(ctx, generated.DeleteServiceSourceSpecsParams{TenantID: tenantID, ServiceID: id}); err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	if _, err := queries.DeleteServiceLayers(ctx, generated.DeleteServiceLayersParams{TenantID: tenantID, ServiceID: id}); err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	if _, err := queries.DeleteServiceAssets(ctx, generated.DeleteServiceAssetsParams{TenantID: tenantID, ServiceID: id}); err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	if _, err := queries.StaleServiceBindings(ctx, generated.StaleServiceBindingsParams{TenantID: tenantID, ServiceID: id}); err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	if _, err := queries.DeactivateServiceTracks(ctx, generated.DeactivateServiceTracksParams{TenantID: tenantID, ServiceID: id}); err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	if _, err := queries.DeleteRecentServicesForService(ctx, generated.DeleteRecentServicesForServiceParams{TenantID: tenantID, ServiceID: id}); err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	pending, err := queries.LockPendingServiceJobs(ctx, generated.LockPendingServiceJobsParams{TenantID: tenantID, ServiceID: id})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	for _, job := range pending {
		if job.RiverJobID != nil {
			if store.riverClient == nil {
				return service.ServiceRecord{}, errors.New("service lifecycle has no River client to cancel pending work")
			}
			if _, err := store.riverClient.JobCancelTx(ctx, tx, *job.RiverJobID); err != nil {
				return service.ServiceRecord{}, fmt.Errorf("cancel pending service job: %w", err)
			}
		}
		if _, err := queries.CancelServiceJob(ctx, generated.CancelServiceJobParams{TenantID: tenantID, ID: job.ID}); err != nil {
			return service.ServiceRecord{}, normalizeError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// DeleteServiceSourceSpecs 软删除某一服务的全部活跃源配置。
func (store *ServiceLifecycleStore) DeleteServiceSourceSpecs(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeleteServiceSourceSpecs(ctx, generated.DeleteServiceSourceSpecsParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// DeleteServiceAssets 软删除某一服务的全部活跃资产。
func (store *ServiceLifecycleStore) DeleteServiceAssets(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeleteServiceAssets(ctx, generated.DeleteServiceAssetsParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// DeleteServiceLayers 软删除服务资产的全部活跃层。
func (store *ServiceLifecycleStore) DeleteServiceLayers(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeleteServiceLayers(ctx, generated.DeleteServiceLayersParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// StaleServiceBindings 将某一服务的全部活跃源绑定标记为失效。
func (store *ServiceLifecycleStore) StaleServiceBindings(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.StaleServiceBindings(ctx, generated.StaleServiceBindingsParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// DeactivateServiceTracks 停用服务资产的全部 ref track。
func (store *ServiceLifecycleStore) DeactivateServiceTracks(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeactivateServiceTracks(ctx, generated.DeactivateServiceTracksParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// DeleteRecentServicesForService 删除某服务的最近访问记录。
func (store *ServiceLifecycleStore) DeleteRecentServicesForService(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeleteRecentServicesForService(ctx, generated.DeleteRecentServicesForServiceParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// LockPendingServiceJobs 锁定作用域为某服务或其子级的待处理任务。
func (store *ServiceLifecycleStore) LockPendingServiceJobs(ctx context.Context, tenantID, serviceID uuid.UUID) ([]service.PendingServiceJob, error) {
	rows, err := store.queries.LockPendingServiceJobs(ctx, generated.LockPendingServiceJobsParams{TenantID: tenantID, ServiceID: serviceID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.PendingServiceJob, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.PendingServiceJob{ID: row.ID, RiverJobID: row.RiverJobID})
	}
	return items, nil
}

// CancelServiceJob 将一条待处理的服务作用域任务转为已取消。
func (store *ServiceLifecycleStore) CancelServiceJob(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := store.queries.CancelServiceJob(ctx, generated.CancelServiceJobParams{TenantID: tenantID, ID: id})
	return normalizeError(err)
}

var _ service.ServiceLifecycleStore = (*ServiceLifecycleStore)(nil)
