package repository

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/riverqueue/river"
)

// ServiceLifecycleStore implements the service lifecycle, public read, and
// soft-delete persistence boundary. It embeds RepositoryStore to reuse the
// service read path and carries a River client for transactional cancellation.
type ServiceLifecycleStore struct {
	*RepositoryStore
	riverClient *river.Client[pgx.Tx]
}

// NewServiceLifecycleStore binds service lifecycle persistence to a native pool.
func NewServiceLifecycleStore(pool *pgxpool.Pool) *ServiceLifecycleStore {
	return &ServiceLifecycleStore{RepositoryStore: NewRepositoryStore(pool)}
}

// NewServiceLifecycleStoreWithRiver binds service lifecycle persistence to a
// River client so deletion can cancel pending jobs transactionally.
func NewServiceLifecycleStoreWithRiver(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *ServiceLifecycleStore {
	return &ServiceLifecycleStore{RepositoryStore: NewRepositoryStore(pool), riverClient: riverClient}
}

// BindRiver attaches the process River client after runtime construction.
func (store *ServiceLifecycleStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
}

// GetServiceBySlug returns one active service within the tenant boundary.
func (store *ServiceLifecycleStore) GetServiceBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceBySlug(ctx, generated.GetServiceBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// GetServiceForUpdate locks one active service row for a conditional mutation.
func (store *ServiceLifecycleStore) GetServiceForUpdate(ctx context.Context, tenantID, id uuid.UUID) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceForUpdate(ctx, generated.GetServiceForUpdateParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// GetPublicServiceBySlug returns one active service resolved by tenant and service slug.
func (store *ServiceLifecycleStore) GetPublicServiceBySlug(ctx context.Context, tenantSlug, serviceSlug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetPublicServiceBySlug(ctx, generated.GetPublicServiceBySlugParams{TenantSlug: tenantSlug, ServiceSlug: serviceSlug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// UpdateService applies a conditional service patch under its revision.
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

// DeleteService soft-deletes one service and cascades logical state, fenced by
// its revision. Pending service-scoped jobs are cancelled in the same transaction.
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

// DeleteServiceSourceSpecs soft-deletes every active source spec of a service.
func (store *ServiceLifecycleStore) DeleteServiceSourceSpecs(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeleteServiceSourceSpecs(ctx, generated.DeleteServiceSourceSpecsParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// DeleteServiceAssets soft-deletes every active asset of a service.
func (store *ServiceLifecycleStore) DeleteServiceAssets(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeleteServiceAssets(ctx, generated.DeleteServiceAssetsParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// DeleteServiceLayers soft-deletes every active layer of a service's assets.
func (store *ServiceLifecycleStore) DeleteServiceLayers(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeleteServiceLayers(ctx, generated.DeleteServiceLayersParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// StaleServiceBindings marks every active source binding of a service stale.
func (store *ServiceLifecycleStore) StaleServiceBindings(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.StaleServiceBindings(ctx, generated.StaleServiceBindingsParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// DeactivateServiceTracks deactivates every ref track of a service's assets.
func (store *ServiceLifecycleStore) DeactivateServiceTracks(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeactivateServiceTracks(ctx, generated.DeactivateServiceTracksParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// DeleteRecentServicesForService removes recent-service rows for a service.
func (store *ServiceLifecycleStore) DeleteRecentServicesForService(ctx context.Context, tenantID, serviceID uuid.UUID) error {
	_, err := store.queries.DeleteRecentServicesForService(ctx, generated.DeleteRecentServicesForServiceParams{TenantID: tenantID, ServiceID: serviceID})
	return normalizeError(err)
}

// LockPendingServiceJobs locks pending jobs scoped to a service or its children.
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

// CancelServiceJob transitions one pending service-scoped job to cancelled.
func (store *ServiceLifecycleStore) CancelServiceJob(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := store.queries.CancelServiceJob(ctx, generated.CancelServiceJobParams{TenantID: tenantID, ID: id})
	return normalizeError(err)
}

var _ service.ServiceLifecycleStore = (*ServiceLifecycleStore)(nil)

var _ = time.Now
