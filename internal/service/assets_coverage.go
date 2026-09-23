package service

import (
	"context"
	"time"
	"uuid"
)

// ListAssetVersions 返回某资产一页版本历史。
func (assets *Assets) ListAssetVersions(ctx context.Context, actor Principal, tenantSlug string, assetID uuid.UUID, page, pageSize int) ([]AssetVersionRecord, int64, error) {
	membership, err := assets.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	if _, err := assets.store.GetAsset(ctx, membership.TenantID, assetID); err != nil {
		return nil, 0, err
	}
	return assets.store.ListAssetVersions(ctx, membership.TenantID, assetID, int32(pageSize), int32((page-1)*pageSize))
}

// DeprecateAssetVersion 在 If-Match 下将一个已发布版本置为 deprecated。
func (assets *Assets) DeprecateAssetVersion(ctx context.Context, actor Principal, tenantSlug string, versionID uuid.UUID, etag string) (AssetVersionRecord, error) {
	membership, err := assets.tenantMembership(ctx, actor, tenantSlug, scopeAssetPublish)
	if err != nil {
		return AssetVersionRecord{}, err
	}
	current, err := assets.store.GetAssetVersion(ctx, membership.TenantID, versionID)
	if err != nil {
		return AssetVersionRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "asset-version", versionID)
	if err != nil {
		return AssetVersionRecord{}, ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return AssetVersionRecord{}, ErrPrecondition
	}
	return assets.store.DeprecateAssetVersion(ctx, membership.TenantID, versionID, expectedRevision)
}

// RetireAssetVersion 在 If-Match 下将一个已发布版本置为 retired。
func (assets *Assets) RetireAssetVersion(ctx context.Context, actor Principal, tenantSlug string, versionID uuid.UUID, etag string) (AssetVersionRecord, error) {
	membership, err := assets.tenantMembership(ctx, actor, tenantSlug, scopeAssetPublish)
	if err != nil {
		return AssetVersionRecord{}, err
	}
	current, err := assets.store.GetAssetVersion(ctx, membership.TenantID, versionID)
	if err != nil {
		return AssetVersionRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "asset-version", versionID)
	if err != nil {
		return AssetVersionRecord{}, ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return AssetVersionRecord{}, ErrPrecondition
	}
	return assets.store.RetireAssetVersion(ctx, membership.TenantID, versionID, expectedRevision)
}

// GetPublicAsset 返回一个按可见性门控的匿名公开资产视图。
func (assets *Assets) GetPublicAsset(ctx context.Context, tenantSlug, serviceSlug, kind, assetName string) (PublicAssetRecord, error) {
	return assets.store.GetPublicAsset(ctx, tenantSlug, serviceSlug, kind, assetName)
}

// PublicAssetRecord 是一个匿名公开资产投影。
type PublicAssetRecord struct {
	// Kind 承载 PublicAssetRecord 的生成 Kind 值。
	Kind string
	// Name 承载 PublicAssetRecord 的生成 Name 值。
	Name string
	// Lifecycle 承载 PublicAssetRecord 的生成 Lifecycle 值。
	Lifecycle string
	// CurrentVersion 承载 PublicAssetRecord 的生成 CurrentVersion 值。
	CurrentVersion *VersionRef
	// ContentRef 承载 PublicAssetRecord 的生成 ContentRef 值。
	ContentRef string
	// UpdatedAt 承载 PublicAssetRecord 的生成 UpdatedAt 值。
	UpdatedAt time.Time
}

// VersionRef 是一个版本引用投影。
type VersionRef struct {
	// ID 承载 VersionRef 的生成 ID 值。
	ID uuid.UUID
	// Version 承载 VersionRef 的生成 Version 值。
	Version string
}
