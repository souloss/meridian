package repository

import (
	"context"
	"encoding/json/v2"
	"errors"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// CoverageStore 基于 sqlc 与 pgx 实现 service.CoverageStore 轻量目录协作持久化端口。
type CoverageStore struct {
	queries *generated.Queries
}

// NewCoverageStore 将目录协作持久化绑定到原生 pgx 连接池。
func NewCoverageStore(pool *pgxpool.Pool) *CoverageStore {
	return &CoverageStore{queries: generated.New(pool)}
}

// GetServiceBySlug 按 slug 返回租户内一条活跃服务。
func (store *CoverageStore) GetServiceBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceBySlug(ctx, generated.GetServiceBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// UpsertServiceStar 幂等收藏一个服务。
func (store *CoverageStore) UpsertServiceStar(ctx context.Context, tenantID, serviceID, userID uuid.UUID) error {
	if _, err := store.queries.UpsertServiceStar(ctx, generated.UpsertServiceStarParams{
		TenantID: tenantID, ServiceID: serviceID, UserID: userID,
	}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// DeleteServiceStar 取消收藏一个服务。
func (store *CoverageStore) DeleteServiceStar(ctx context.Context, tenantID, serviceID, userID uuid.UUID) error {
	if _, err := store.queries.DeleteServiceStar(ctx, generated.DeleteServiceStarParams{
		TenantID: tenantID, ServiceID: serviceID, UserID: userID,
	}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// GetServiceStar 返回服务是否被某用户收藏。
func (store *CoverageStore) GetServiceStar(ctx context.Context, tenantID, serviceID, userID uuid.UUID) (bool, error) {
	_, err := store.queries.GetServiceStar(ctx, generated.GetServiceStarParams{
		TenantID: tenantID, ServiceID: serviceID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, normalizeError(err)
	}
	return true, nil
}

// GetUserPreferences 返回某用户偏好。
func (store *CoverageStore) GetUserPreferences(ctx context.Context, userID uuid.UUID) (service.UserPreferencesRecord, error) {
	row, err := store.queries.GetUserPreferences(ctx, userID)
	if err != nil {
		return service.UserPreferencesRecord{}, normalizeError(err)
	}
	return service.UserPreferencesRecord{
		Locale: row.Locale, Theme: row.Theme, DefaultViews: row.DefaultViews, Revision: row.Revision,
	}, nil
}

// UpdateUserPreferences 在 If-Match 下更新用户偏好。
func (store *CoverageStore) UpdateUserPreferences(ctx context.Context, userID uuid.UUID, expectedRevision int64, patch service.UserPreferencesPatch) (service.UserPreferencesRecord, error) {
	row, err := store.queries.UpdateUserPreferences(ctx, generated.UpdateUserPreferencesParams{
		Locale: patch.Locale, Theme: patch.Theme, DefaultViews: patch.DefaultViews,
		UserID: userID, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.UserPreferencesRecord{}, service.ErrPrecondition
		}
		return service.UserPreferencesRecord{}, normalizeError(err)
	}
	return service.UserPreferencesRecord{
		Locale: row.Locale, Theme: row.Theme, DefaultViews: row.DefaultViews, Revision: row.Revision,
	}, nil
}

// ListViewOverrides 返回租户内全部视图覆盖。
func (store *CoverageStore) ListViewOverrides(ctx context.Context, tenantID uuid.UUID) ([]service.ViewOverrideRecord, error) {
	rows, err := store.queries.ListViewOverrides(ctx, tenantID)
	if err != nil {
		return nil, normalizeError(err)
	}
	records := make([]service.ViewOverrideRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, viewOverrideFromRow(row))
	}
	return records, nil
}

// GetViewOverride 返回一条视图覆盖。
func (store *CoverageStore) GetViewOverride(ctx context.Context, tenantID uuid.UUID, viewID string) (service.ViewOverrideRecord, error) {
	row, err := store.queries.GetViewOverride(ctx, generated.GetViewOverrideParams{TenantID: tenantID, ViewID: viewID})
	if err != nil {
		return service.ViewOverrideRecord{}, normalizeError(err)
	}
	return viewOverrideFromRow(row), nil
}

// UpsertViewOverride 幂等设置一条视图覆盖。
func (store *CoverageStore) UpsertViewOverride(ctx context.Context, tenantID uuid.UUID, viewID string, enabled bool, ord int, defaultOptions []byte) (service.ViewOverrideRecord, error) {
	row, err := store.queries.UpsertViewOverride(ctx, generated.UpsertViewOverrideParams{
		TenantID: tenantID, ViewID: viewID, Enabled: enabled, Ord: int32(ord), DefaultOptions: defaultOptions,
	})
	if err != nil {
		return service.ViewOverrideRecord{}, normalizeError(err)
	}
	return viewOverrideFromRow(row), nil
}

// DeleteViewOverride 删除一条视图覆盖。
func (store *CoverageStore) DeleteViewOverride(ctx context.Context, tenantID uuid.UUID, viewID string) error {
	changed, err := store.queries.DeleteViewOverride(ctx, generated.DeleteViewOverrideParams{TenantID: tenantID, ViewID: viewID})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrNotFound
	}
	return nil
}

// ListTags 返回租户内全部标签。
func (store *CoverageStore) ListTags(ctx context.Context, tenantID uuid.UUID) ([]service.TagRecord, error) {
	rows, err := store.queries.ListTags(ctx, tenantID)
	if err != nil {
		return nil, normalizeError(err)
	}
	records := make([]service.TagRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, tagFromRow(row))
	}
	return records, nil
}

// GetTag 返回一条标签。
func (store *CoverageStore) GetTag(ctx context.Context, tenantID, id uuid.UUID) (service.TagRecord, error) {
	row, err := store.queries.GetTag(ctx, generated.GetTagParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.TagRecord{}, normalizeError(err)
	}
	return tagFromRow(row), nil
}

// CreateTag 创建一条标签。
func (store *CoverageStore) CreateTag(ctx context.Context, input service.NewTag) (service.TagRecord, error) {
	row, err := store.queries.CreateTag(ctx, generated.CreateTagParams{
		TenantID: input.TenantID, ID: input.ID, Name: input.Name, Color: input.Color, Description: input.Description,
	})
	if err != nil {
		return service.TagRecord{}, normalizeError(err)
	}
	return tagFromRow(row), nil
}

// UpdateTag 在 If-Match 下更新一条标签。
func (store *CoverageStore) UpdateTag(ctx context.Context, tenantID, id uuid.UUID, expectedRevision int64, patch service.TagPatch) (service.TagRecord, error) {
	row, err := store.queries.UpdateTag(ctx, generated.UpdateTagParams{
		Name: patch.Name, Color: patch.Color, SetDescription: patch.SetDescription, Description: patch.Description,
		TenantID: tenantID, ID: id, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.TagRecord{}, service.ErrPrecondition
		}
		return service.TagRecord{}, normalizeError(err)
	}
	return tagFromRow(row), nil
}

// DeleteTag 在 If-Match 下删除一条标签。
func (store *CoverageStore) DeleteTag(ctx context.Context, tenantID, id uuid.UUID, expectedRevision int64) error {
	changed, err := store.queries.DeleteTag(ctx, generated.DeleteTagParams{TenantID: tenantID, ID: id, ExpectedRevision: expectedRevision})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrPrecondition
	}
	return nil
}

// ListServiceComments 返回一个服务下的评论分页。
func (store *CoverageStore) ListServiceComments(ctx context.Context, tenantID, serviceID uuid.UUID, limit, offset int32) ([]service.CommentRecord, int64, error) {
	rows, err := store.queries.ListServiceComments(ctx, generated.ListServiceCommentsParams{
		TenantID: tenantID, ServiceID: serviceID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	total, err := store.queries.CountServiceComments(ctx, generated.CountServiceCommentsParams{TenantID: tenantID, ServiceID: serviceID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	records := make([]service.CommentRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, service.CommentRecord{
			ID: row.ID, ServiceID: row.ServiceID, AuthorID: row.AuthorID, Body: row.Body,
			AuthorDisplayName: row.AuthorDisplayName, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return records, total, nil
}

// CreateServiceComment 创建一条服务评论。
func (store *CoverageStore) CreateServiceComment(ctx context.Context, tenantID, serviceID, authorID, commentID uuid.UUID, body string) (service.CommentRecord, error) {
	row, err := store.queries.CreateServiceComment(ctx, generated.CreateServiceCommentParams{
		TenantID: tenantID, ID: commentID, ServiceID: serviceID, AuthorID: authorID, Body: body,
	})
	if err != nil {
		return service.CommentRecord{}, normalizeError(err)
	}
	return service.CommentRecord{
		ID: row.ID, ServiceID: row.ServiceID, AuthorID: row.AuthorID, Body: row.Body,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

// CreateTenantExport 创建一条租户导出任务记录。
func (store *CoverageStore) CreateTenantExport(ctx context.Context, tenantID, requestedBy, id uuid.UUID) (service.TenantExportRecord, error) {
	row, err := store.queries.CreateTenantExport(ctx, generated.CreateTenantExportParams{
		TenantID: tenantID, ID: id, RequestedBy: requestedBy,
	})
	if err != nil {
		return service.TenantExportRecord{}, normalizeError(err)
	}
	return service.TenantExportRecord{
		ID: row.ID, Status: row.Status, RequestedBy: row.RequestedBy, CreatedAt: row.CreatedAt.Time,
	}, nil
}

func viewOverrideFromRow(row generated.ViewOverride) service.ViewOverrideRecord {
	return service.ViewOverrideRecord{
		ViewID: row.ViewID, Enabled: row.Enabled, Ord: int(row.Ord), DefaultOptions: row.DefaultOptions,
		Revision: row.Revision, UpdatedAt: row.UpdatedAt.Time,
	}
}

func tagFromRow(row generated.TagDefinition) service.TagRecord {
	return service.TagRecord{
		ID: row.ID, Name: row.Name, Color: row.Color, Description: row.Description,
		Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

var _ service.CoverageStore = (*CoverageStore)(nil)

var _ = json.Marshal
var _ = time.Now
