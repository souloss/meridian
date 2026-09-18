package repository

import (
	"context"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// SystemGroupStore 实现 M4 系统分组与搜索持久化边界。
type SystemGroupStore struct {
	queries *generated.Queries
	pool    *pgxpool.Pool
}

// NewSystemGroupStore 将系统分组持久化绑定到原生 pgx 连接池。
func NewSystemGroupStore(pool *pgxpool.Pool) *SystemGroupStore {
	return &SystemGroupStore{queries: generated.New(pool), pool: pool}
}

// CreateSystemGroup 插入一条系统分组。
func (store *SystemGroupStore) CreateSystemGroup(ctx context.Context, input service.NewSystemGroup) (service.SystemGroupRecord, error) {
	row, err := store.queries.CreateSystemGroup(ctx, generated.CreateSystemGroupParams{
		TenantID: input.TenantID, ID: input.ID, Slug: input.Slug, DisplayName: input.DisplayName, Description: input.Description,
	})
	if err != nil {
		return service.SystemGroupRecord{}, normalizeError(err)
	}
	return systemGroupFromRow(row), nil
}

// GetSystemGroup 按 id 返回一条系统分组。
func (store *SystemGroupStore) GetSystemGroup(ctx context.Context, tenantID, id uuid.UUID) (service.SystemGroupRecord, error) {
	row, err := store.queries.GetSystemGroup(ctx, generated.GetSystemGroupParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.SystemGroupRecord{}, normalizeError(err)
	}
	return systemGroupFromRow(row), nil
}

// GetSystemGroupBySlug 按 slug 返回一条系统分组。
func (store *SystemGroupStore) GetSystemGroupBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.SystemGroupRecord, error) {
	row, err := store.queries.GetSystemGroupBySlug(ctx, generated.GetSystemGroupBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.SystemGroupRecord{}, normalizeError(err)
	}
	return systemGroupFromRow(row), nil
}

// ListSystemGroups 列出租户内的全部系统分组。
func (store *SystemGroupStore) ListSystemGroups(ctx context.Context, tenantID uuid.UUID) ([]service.SystemGroupRecord, error) {
	rows, err := store.queries.ListSystemGroups(ctx, tenantID)
	if err != nil {
		return nil, normalizeError(err)
	}
	records := make([]service.SystemGroupRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, systemGroupFromRow(row))
	}
	return records, nil
}

// ListSystemGroupMembers 返回某一分组的成员服务 id。
func (store *SystemGroupStore) ListSystemGroupMembers(ctx context.Context, tenantID, groupID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := store.queries.ListSystemGroupMembers(ctx, generated.ListSystemGroupMembersParams{TenantID: tenantID, GroupID: groupID})
	if err != nil {
		return nil, normalizeError(err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ServiceID)
	}
	return ids, nil
}

// ReplaceSystemGroupMembers 原子地替换某一分组的成员。
func (store *SystemGroupStore) ReplaceSystemGroupMembers(ctx context.Context, tenantID, groupID uuid.UUID, serviceIDs []uuid.UUID) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return normalizeError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	if _, err := queries.ReplaceSystemGroupMembers(ctx, generated.ReplaceSystemGroupMembersParams{TenantID: tenantID, GroupID: groupID}); err != nil {
		return normalizeError(err)
	}
	for _, serviceID := range serviceIDs {
		if _, err := queries.InsertSystemGroupMember(ctx, generated.InsertSystemGroupMemberParams{TenantID: tenantID, GroupID: groupID, ServiceID: serviceID}); err != nil {
			return normalizeError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	return nil
}

// BumpSystemGroupRevision 在乐观并发下递增某一分组的修订号。
func (store *SystemGroupStore) BumpSystemGroupRevision(ctx context.Context, tenantID, groupID uuid.UUID, expectedRevision int64) error {
	changed, err := store.queries.BumpSystemGroupRevision(ctx, generated.BumpSystemGroupRevisionParams{
		TenantID: tenantID, ID: groupID, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrPrecondition
	}
	return nil
}

// ListServicesByIDs 在租户内按 id 返回服务，按 id 排序。
func (store *SystemGroupStore) ListServicesByIDs(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) ([]service.ServiceRecord, error) {
	services := make([]service.ServiceRecord, 0, len(ids))
	for _, id := range ids {
		row, err := store.queries.GetServiceByID(ctx, generated.GetServiceByIDParams{TenantID: tenantID, ID: id})
		if err != nil {
			return nil, normalizeError(err)
		}
		services = append(services, serviceRecordFromRow(row))
	}
	return services, nil
}

func systemGroupFromRow(row generated.SystemGroup) service.SystemGroupRecord {
	return service.SystemGroupRecord{
		TenantID: row.TenantID, ID: row.ID, Slug: row.Slug, DisplayName: row.DisplayName,
		Description: row.Description, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

var _ service.SystemGroupStore = (*SystemGroupStore)(nil)
