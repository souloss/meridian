package service

import (
	"context"
	"time"
	"uuid"
)

// PublicServiceRecord is the anonymous public projection of one service.
type PublicServiceRecord struct {
	Slug        string
	DisplayName string
	Description *string
	Lifecycle   string
	Tags        []string
	Assets      []AssetSummaryRecord
	UpdatedAt   time.Time
}

// ServiceLifecycleStore is the persistence boundary for service lifecycle,
// public read, and soft delete. Every method retains the tenant predicate,
// except public reads that resolve by tenant and service slug.
type ServiceLifecycleStore interface {
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	GetServiceForUpdate(context.Context, uuid.UUID, uuid.UUID) (ServiceRecord, error)
	GetPublicServiceBySlug(context.Context, string, string) (ServiceRecord, error)
	UpdateService(context.Context, ServicePatch) (ServiceRecord, error)
	DeleteService(context.Context, uuid.UUID, uuid.UUID, int64) (ServiceRecord, error)
	DeleteServiceSourceSpecs(context.Context, uuid.UUID, uuid.UUID) error
	DeleteServiceAssets(context.Context, uuid.UUID, uuid.UUID) error
	DeleteServiceLayers(context.Context, uuid.UUID, uuid.UUID) error
	StaleServiceBindings(context.Context, uuid.UUID, uuid.UUID) error
	DeactivateServiceTracks(context.Context, uuid.UUID, uuid.UUID) error
	DeleteRecentServicesForService(context.Context, uuid.UUID, uuid.UUID) error
	LockPendingServiceJobs(context.Context, uuid.UUID, uuid.UUID) ([]PendingServiceJob, error)
	CancelServiceJob(context.Context, uuid.UUID, uuid.UUID) error
}

// PendingServiceJob is one cancellable job scoped to a service being deleted.
type PendingServiceJob struct {
	ID         uuid.UUID
	RiverJobID *int64
}

// ServicePatch carries explicit PATCH fields for one service update.
type ServicePatch struct {
	TenantID        uuid.UUID
	ID              uuid.UUID
	ExpectedRevision int64
	DisplayName     *string
	Description     *string
	SetDescription  bool
	Visibility      *string
	Lifecycle       *string
}
