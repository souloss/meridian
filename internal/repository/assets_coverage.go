package repository

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// ListAssetVersions 分页返回某资产版本历史。
func (store *AssetStore) ListAssetVersions(ctx context.Context, tenantID, assetID uuid.UUID, limit, offset int32) ([]service.AssetVersionRecord, int64, error) {
	total, err := store.queries.CountAssetVersions(ctx, generated.CountAssetVersionsParams{TenantID: tenantID, AssetID: assetID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListAssetVersions(ctx, generated.ListAssetVersionsParams{TenantID: tenantID, AssetID: assetID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	versions := make([]service.AssetVersionRecord, 0, len(rows))
	for _, row := range rows {
		versions = append(versions, assetVersionFromRow(row))
	}
	return versions, total, nil
}

// DeprecateAssetVersion 在 If-Match 下将一个已发布版本置为 deprecated。
func (store *AssetStore) DeprecateAssetVersion(ctx context.Context, tenantID, versionID uuid.UUID, expectedRevision int64) (service.AssetVersionRecord, error) {
	row, err := store.queries.DeprecateAssetVersion(ctx, generated.DeprecateAssetVersionParams{
		TenantID: tenantID, ID: versionID, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.AssetVersionRecord{}, service.ErrInvalidState
		}
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// RetireAssetVersion 在 If-Match 下将一个已发布版本置为 retired。
func (store *AssetStore) RetireAssetVersion(ctx context.Context, tenantID, versionID uuid.UUID, expectedRevision int64) (service.AssetVersionRecord, error) {
	row, err := store.queries.RetireAssetVersion(ctx, generated.RetireAssetVersionParams{
		TenantID: tenantID, ID: versionID, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.AssetVersionRecord{}, service.ErrInvalidState
		}
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// GetPublicAsset 返回一个匿名公开资产投影。
// 先按 slug 定位公开可见服务的当前资产；未能解析当前版本时，回退到按
// 服务/类别/名称定位其 default 分支轨道的最新版本（无 serviceSlug 的匿名上下文）。
func (store *AssetStore) GetPublicAsset(ctx context.Context, tenantSlug, serviceSlug, kind, assetName string) (service.PublicAssetRecord, error) {
	// 有 serviceSlug 时按 slug 精确解析。
	if serviceSlug != "" {
		row, err := store.queries.GetPublicAssetBySlug(ctx, generated.GetPublicAssetBySlugParams{
			TenantSlug: tenantSlug, ServiceSlug: serviceSlug, Kind: kind, AssetName: assetName,
		})
		if err == nil {
			record := store.publicAssetFromRow(ctx, row.TenantID, row.Kind, row.Name, row.AssetCreatedAt.Time, row.CurrentVersionID)
			if record.CurrentVersion != nil {
				return record, nil
			}
		}
	}
	// 无 serviceSlug（resolvePublicView 契约不含 serviceSlug）：按租户 + kind + name 反查。
	return store.getPublicAssetByName(ctx, tenantSlug, kind, assetName)
}

// getPublicAssetByName 在不提供 serviceSlug 时，按租户 + 类别 + 名称定位公开资产。
// 资产名在 (tenant, service, kind, name) 上唯一，但公开可见性门控仍需服务级校验，
// 因此先定位唯一资产，再校验其服务 visibility/lifecycle 与 default 分支轨道版本。
func (store *AssetStore) getPublicAssetByName(ctx context.Context, tenantSlug, kind, assetName string) (service.PublicAssetRecord, error) {
	row, err := store.queries.GetPublicAssetByName(ctx, generated.GetPublicAssetByNameParams{
		TenantSlug: tenantSlug, Kind: kind, AssetName: assetName,
	})
	if err != nil {
		return service.PublicAssetRecord{}, normalizeError(err)
	}
	return store.publicAssetFromRow(ctx, row.TenantID, row.Kind, row.Name, row.AssetCreatedAt.Time, row.CurrentVersionID), nil
}

// publicAssetFromRow 从一行公开资产定位结果组装投影，并按需加载其已发布当前版本。
func (store *AssetStore) publicAssetFromRow(ctx context.Context, tenantID uuid.UUID, kind, name string, assetCreatedAt time.Time, currentVersionID *uuid.UUID) service.PublicAssetRecord {
	record := service.PublicAssetRecord{
		Kind: kind, Name: name, Lifecycle: lifecycleDraftForPublic, UpdatedAt: assetCreatedAt,
	}
	if currentVersionID != nil {
		version, err := store.queries.GetPublicAssetVersion(ctx, generated.GetPublicAssetVersionParams{
			TenantID: tenantID, VersionID: *currentVersionID,
		})
		if err == nil {
			record.CurrentVersion = &service.VersionRef{ID: version.ID, Version: version.Version}
			if version.MergedRef != nil {
				record.ContentRef = *version.MergedRef
			}
			record.Lifecycle = version.Lifecycle
		}
	}
	return record
}

const lifecycleDraftForPublic = "draft"

var _ = time.Now
