package repository

import (
	"context"
	"uuid"

	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// ListPlatformJobs 仅返回平台任务契约批准的已脱敏字段。
func (store *RepositoryStore) ListPlatformJobs(ctx context.Context, filter service.PlatformJobFilter, limit, offset int32) ([]service.PlatformJobRecord, int64, error) {
	params := generated.ListPlatformJobsParams{
		TypeFilter: filter.Types, StatusFilter: filter.Statuses, ScopeType: filter.ScopeType,
		ScopeID: filter.ScopeID, TenantSlug: filter.TenantSlug, PageLimit: limit, PageOffset: offset,
	}
	total, err := store.queries.CountPlatformJobs(ctx, generated.CountPlatformJobsParams{
		TypeFilter: filter.Types, StatusFilter: filter.Statuses, ScopeType: filter.ScopeType,
		ScopeID: filter.ScopeID, TenantSlug: filter.TenantSlug,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListPlatformJobs(ctx, params)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.PlatformJobRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, platformJobFromRow(row))
	}
	return items, total, nil
}

// GetPlatformJob 按其全局 UUID 返回一条已脱敏的任务投影。
func (store *RepositoryStore) GetPlatformJob(ctx context.Context, id uuid.UUID) (service.PlatformJobRecord, error) {
	row, err := store.queries.GetPlatformJob(ctx, id)
	if err != nil {
		return service.PlatformJobRecord{}, normalizeError(err)
	}
	return service.PlatformJobRecord{
		ID: row.ID, TenantSlug: row.TenantSlug, Type: row.Type, Trigger: row.Trigger,
		Status: row.Status, Stage: row.Stage, ScopeType: row.ScopeType, ScopeID: optionalText(row.ScopeID),
		CreatedAt: row.CreatedAt.Time, StartedAt: timePointer(row.StartedAt), FinishedAt: timePointer(row.FinishedAt),
	}, nil
}

func platformJobFromRow(row generated.ListPlatformJobsRow) service.PlatformJobRecord {
	return service.PlatformJobRecord{
		ID: row.ID, TenantSlug: row.TenantSlug, Type: row.Type, Trigger: row.Trigger,
		Status: row.Status, Stage: row.Stage, ScopeType: row.ScopeType, ScopeID: optionalText(row.ScopeID),
		CreatedAt: row.CreatedAt.Time, StartedAt: timePointer(row.StartedAt), FinishedAt: timePointer(row.FinishedAt),
	}
}

func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return new(value)
}
