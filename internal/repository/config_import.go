package repository

import (
	"context"
	"uuid"

	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// CreateConfigImportPreview 持久化一份非权威的预览快照。
func (store *DiscoveryStore) CreateConfigImportPreview(ctx context.Context, input service.NewConfigImportPreview) (service.ConfigImportPreviewRecord, error) {
	row, err := store.queries.CreateConfigImportPreview(ctx, generated.CreateConfigImportPreviewParams{
		TenantID: input.TenantID, ID: input.ID, RepositoryID: input.RepositoryID,
		RefType: input.RefType, RefName: input.RefName, Commit: input.Commit, ConfigDigest: input.ConfigDigest,
		Preview: input.Preview, ExpiresAt: timestamp(input.ExpiresAt),
	})
	if err != nil {
		return service.ConfigImportPreviewRecord{}, normalizeError(err)
	}
	return configImportPreviewFromRow(row), nil
}

// GetConfigImportPreview 返回一份未过期且为应用而锁定的预览。
func (store *DiscoveryStore) GetConfigImportPreview(ctx context.Context, tenantID, repositoryID, previewID uuid.UUID) (service.ConfigImportPreviewRecord, error) {
	row, err := store.queries.GetConfigImportPreview(ctx, generated.GetConfigImportPreviewParams{
		TenantID: tenantID, ID: previewID, RepositoryID: repositoryID,
	})
	if err != nil {
		return service.ConfigImportPreviewRecord{}, normalizeError(err)
	}
	return configImportPreviewFromRow(row), nil
}

// GetConfigImportPreviewByID 按 id 返回一份预览。
func (store *DiscoveryStore) GetConfigImportPreviewByID(ctx context.Context, tenantID, previewID uuid.UUID) (service.ConfigImportPreviewRecord, error) {
	row, err := store.queries.GetConfigImportPreviewByID(ctx, generated.GetConfigImportPreviewByIDParams{
		TenantID: tenantID, ID: previewID,
	})
	if err != nil {
		return service.ConfigImportPreviewRecord{}, normalizeError(err)
	}
	return configImportPreviewFromRow(row), nil
}

// ListServicesByRepository 返回某一仓库的活跃服务。
func (store *DiscoveryStore) ListServicesByRepository(ctx context.Context, tenantID, repositoryID uuid.UUID) ([]service.ServiceRecord, error) {
	rows, err := store.queries.ListServicesByRepository(ctx, generated.ListServicesByRepositoryParams{TenantID: tenantID, RepositoryID: repositoryID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.ServiceRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, serviceRecordFromRow(row))
	}
	return items, nil
}

// GetProducerProfileByName 按其全局唯一名称解析一个平台配置。
func (store *DiscoveryStore) GetProducerProfileByName(ctx context.Context, name string) (service.ProducerProfile, error) {
	row, err := store.queries.GetProducerProfileByName(ctx, name)
	if err != nil {
		return service.ProducerProfile{}, normalizeError(err)
	}
	return producerProfileFromRow(row), nil
}

func configImportPreviewFromRow(row generated.ConfigImportPreview) service.ConfigImportPreviewRecord {
	return service.ConfigImportPreviewRecord{
		ID: row.ID, RepositoryID: row.RepositoryID, RefType: row.RefType, RefName: row.RefName,
		Commit: row.Commit, ConfigDigest: row.ConfigDigest, Preview: row.Preview, ExpiresAt: row.ExpiresAt.Time,
	}
}
