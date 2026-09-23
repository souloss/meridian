package repository

import (
	"context"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// KindStore 基于 sqlc 与 pgx 实现 service.KindStore 资产类别持久化端口。
type KindStore struct {
	pool    *pgxpool.Pool
	queries *generated.Queries
}

// NewKindStore 将资产类别持久化绑定到原生 pgx 连接池。
func NewKindStore(pool *pgxpool.Pool) *KindStore {
	return &KindStore{pool: pool, queries: generated.New(pool)}
}

// GetAssetKind 返回一个平台级资产类别注册。
func (store *KindStore) GetAssetKind(ctx context.Context, id string) (service.AssetKindRecord, error) {
	row, err := store.queries.GetAssetKind(ctx, id)
	if err != nil {
		return service.AssetKindRecord{}, normalizeError(err)
	}
	return service.AssetKindRecord{
		ID: row.ID, ContractVersion: row.ContractVersion, Enabled: row.Enabled, PluginVersion: row.PluginVersion,
	}, nil
}

// ListAssetKindOverrides 返回租户视角的全部资产类别（含覆盖派生）。
func (store *KindStore) ListAssetKindOverrides(ctx context.Context, tenantID uuid.UUID) ([]service.AssetKindOption, error) {
	rows, err := store.queries.ListTenantKindOverrides(ctx, tenantID)
	if err != nil {
		return nil, normalizeError(err)
	}
	options := make([]service.AssetKindOption, 0, len(rows))
	for _, row := range rows {
		options = append(options, service.AssetKindOption{
			ID: row.KindID, ContractVersion: row.ContractVersion, PluginVersion: row.PluginVersion,
			Enabled: row.Enabled, Revision: row.Revision,
		})
	}
	return options, nil
}

// UpsertAssetKindOverride 插入或更新一个租户级类别开关。
func (store *KindStore) UpsertAssetKindOverride(ctx context.Context, tenantID uuid.UUID, kindID string, enabled bool) (service.AssetKindOverride, error) {
	row, err := store.queries.UpsertTenantKindOverride(ctx, generated.UpsertTenantKindOverrideParams{
		TenantID: tenantID, KindID: kindID, Enabled: enabled,
	})
	if err != nil {
		return service.AssetKindOverride{}, normalizeError(err)
	}
	return service.AssetKindOverride{
		TenantID: row.TenantID, KindID: row.KindID, Enabled: row.Enabled,
		Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

var _ service.KindStore = (*KindStore)(nil)
