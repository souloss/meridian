package repository

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// SettingsStore 基于 sqlc 与 pgx 实现 service.SettingsStore 设置持久化端口。
type SettingsStore struct {
	pool    *pgxpool.Pool
	queries *generated.Queries
}

// NewSettingsStore 将设置持久化绑定到原生 pgx 连接池。
func NewSettingsStore(pool *pgxpool.Pool) *SettingsStore {
	return &SettingsStore{pool: pool, queries: generated.New(pool)}
}

// GetPlatformSettings 返回平台默认配置单例。
func (store *SettingsStore) GetPlatformSettings(ctx context.Context) (service.PlatformSettingsValue, error) {
	row, err := store.queries.GetPlatformSettings(ctx)
	if err != nil {
		return service.PlatformSettingsValue{}, normalizeError(err)
	}
	return service.PlatformSettingsValue{Settings: row.Settings, Revision: row.Revision}, nil
}

// UpdatePlatformSettings 在 If-Match 下替换平台默认配置并递增 revision。
func (store *SettingsStore) UpdatePlatformSettings(ctx context.Context, expectedRevision int64, settings jsontext.Value) (service.PlatformSettingsValue, error) {
	row, err := store.queries.UpdatePlatformSettings(ctx, generated.UpdatePlatformSettingsParams{
		Settings: settings, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.PlatformSettingsValue{}, service.ErrPrecondition
		}
		return service.PlatformSettingsValue{}, normalizeError(err)
	}
	return service.PlatformSettingsValue{Settings: row.Settings, Revision: row.Revision}, nil
}

// GetTenantSettings 返回租户运行设置快照。
func (store *SettingsStore) GetTenantSettings(ctx context.Context, tenantID uuid.UUID) (service.TenantSettingsValue, error) {
	row, err := store.queries.GetTenantSettingsForRead(ctx, tenantID)
	if err != nil {
		return service.TenantSettingsValue{}, normalizeError(err)
	}
	return service.TenantSettingsValue{Settings: row.Settings, Revision: row.Revision}, nil
}

// UpdateTenantSettings 在 If-Match 下替换租户设置并递增 revision。
func (store *SettingsStore) UpdateTenantSettings(ctx context.Context, tenantID uuid.UUID, expectedRevision int64, settings jsontext.Value) (service.TenantSettingsValue, error) {
	row, err := store.queries.UpdateTenantSettingsForWrite(ctx, generated.UpdateTenantSettingsForWriteParams{
		Settings: settings, TenantID: tenantID, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.TenantSettingsValue{}, service.ErrPrecondition
		}
		return service.TenantSettingsValue{}, normalizeError(err)
	}
	return service.TenantSettingsValue{Settings: row.Settings, Revision: row.Revision}, nil
}

var _ service.SettingsStore = (*SettingsStore)(nil)
