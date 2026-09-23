package service

import (
	"context"
	"time"
	"uuid"
)

// PublicServiceRecord 是一个服务的匿名公开投影。
type PublicServiceRecord struct {
	// Slug 是服务 slug。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
	// Description 是描述（可为空）。
	Description *string
	// Lifecycle 是生命周期状态。
	Lifecycle string
	// Tags 是标签列表。
	Tags []string
	// Assets 是资产摘要列表。
	Assets []AssetSummaryRecord
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// ServiceLifecycleStore 是服务生命周期、公开读取与软删除的持久化边界。
// 每个方法都保留租户谓词，除按租户与服务 slug 解析的公开读取外。
type ServiceLifecycleStore interface {
	// GetServiceBySlug 承载 ServiceLifecycleStore 的生成 GetServiceBySlug 值。
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	// GetServiceForUpdate 承载 ServiceLifecycleStore 的生成 GetServiceForUpdate 值。
	GetServiceForUpdate(context.Context, uuid.UUID, uuid.UUID) (ServiceRecord, error)
	// GetPublicServiceBySlug 承载 ServiceLifecycleStore 的生成 GetPublicServiceBySlug 值。
	GetPublicServiceBySlug(context.Context, string, string) (ServiceRecord, error)
	// UpdateService 承载 ServiceLifecycleStore 的生成 UpdateService 值。
	UpdateService(context.Context, ServicePatch) (ServiceRecord, error)
	// DeleteService 承载 ServiceLifecycleStore 的生成 DeleteService 值。
	DeleteService(context.Context, uuid.UUID, uuid.UUID, int64) (ServiceRecord, error)
	// DeleteServiceSourceSpecs 承载 ServiceLifecycleStore 的生成 DeleteServiceSourceSpecs 值。
	DeleteServiceSourceSpecs(context.Context, uuid.UUID, uuid.UUID) error
	// DeleteServiceAssets 承载 ServiceLifecycleStore 的生成 DeleteServiceAssets 值。
	DeleteServiceAssets(context.Context, uuid.UUID, uuid.UUID) error
	// DeleteServiceLayers 承载 ServiceLifecycleStore 的生成 DeleteServiceLayers 值。
	DeleteServiceLayers(context.Context, uuid.UUID, uuid.UUID) error
	// StaleServiceBindings 承载 ServiceLifecycleStore 的生成 StaleServiceBindings 值。
	StaleServiceBindings(context.Context, uuid.UUID, uuid.UUID) error
	// DeactivateServiceTracks 承载 ServiceLifecycleStore 的生成 DeactivateServiceTracks 值。
	DeactivateServiceTracks(context.Context, uuid.UUID, uuid.UUID) error
	// DeleteRecentServicesForService 承载 ServiceLifecycleStore 的生成 DeleteRecentServicesForService 值。
	DeleteRecentServicesForService(context.Context, uuid.UUID, uuid.UUID) error
	// LockPendingServiceJobs 承载 ServiceLifecycleStore 的生成 LockPendingServiceJobs 值。
	LockPendingServiceJobs(context.Context, uuid.UUID, uuid.UUID) ([]PendingServiceJob, error)
	// CancelServiceJob 承载 ServiceLifecycleStore 的生成 CancelServiceJob 值。
	CancelServiceJob(context.Context, uuid.UUID, uuid.UUID) error
}

// PendingServiceJob 是一个可取消的、以被删除服务为作用域的任务。
type PendingServiceJob struct {
	// ID 是任务标识。
	ID uuid.UUID
	// RiverJobID 是 River 任务标识（可为空）。
	RiverJobID *int64
}

// ServicePatch 承载一次服务更新的显式 PATCH 字段。
type ServicePatch struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是服务的标识。
	ID uuid.UUID
	// ExpectedRevision 是乐观并发所需版本号。
	ExpectedRevision int64
	// DisplayName 是新的展示名（可为空）。
	DisplayName *string
	// Description 是新的描述（可为空）。
	Description *string
	// SetDescription 表示是否显式设置描述。
	SetDescription bool
	// Visibility 是新的可见性（可为空）。
	Visibility *string
	// Lifecycle 是新的生命周期状态（可为空）。
	Lifecycle *string
}
