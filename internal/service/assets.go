package service

import (
	"context"
	"sort"
	"slices"
	"time"
	"uuid"
)

// Assets coordinates asset, version, and item read use cases for tenant members.
type Assets struct {
	store      AssetStore
	identities IdentityStore
	now        func() time.Time
}

// NewAssets constructs asset read use cases with caller-owned persistence.
func NewAssets(store AssetStore, identities IdentityStore) *Assets {
	return &Assets{store: store, identities: identities, now: time.Now}
}

// GetAsset returns one asset projected onto the selected ref track.
func (assets *Assets) GetAsset(ctx context.Context, actor Principal, tenantSlug string, assetID uuid.UUID, refType, refName string) (AssetRecord, AssetTrackProjection, error) {
	membership, err := assets.tenantMembership(ctx, actor, tenantSlug, "asset:read")
	if err != nil {
		return AssetRecord{}, AssetTrackProjection{}, err
	}
	asset, err := assets.store.GetAsset(ctx, membership.TenantID, assetID)
	if err != nil {
		return AssetRecord{}, AssetTrackProjection{}, err
	}
	projection := AssetTrackProjection{Lifecycle: "draft"}
	if refName == "" {
		if defaultBranch, err := assets.store.GetAssetRepositoryDefaultBranch(ctx, membership.TenantID, assetID); err == nil && defaultBranch != "" {
			refName = defaultBranch
		}
	}
	if refType == "" {
		refType = "branch"
	}
	track, err := assets.store.GetAssetRefTrack(ctx, membership.TenantID, asset.ID, refType, refName)
	if err == nil {
		projection = assets.trackProjection(ctx, membership.TenantID, track)
	}
	return asset, projection, nil
}

// GetAssetVersion returns one asset version with its layer manifest.
func (assets *Assets) GetAssetVersion(ctx context.Context, actor Principal, tenantSlug string, versionID uuid.UUID) (AssetVersionRecord, error) {
	membership, err := assets.tenantMembership(ctx, actor, tenantSlug, "asset:read")
	if err != nil {
		return AssetVersionRecord{}, err
	}
	return assets.store.GetAssetVersion(ctx, membership.TenantID, versionID)
}

// ListAssetVersionItems returns one page of indexed items for a version.
func (assets *Assets) ListAssetVersionItems(ctx context.Context, actor Principal, tenantSlug string, versionID uuid.UUID, query string, page, pageSize int) ([]AssetItemRecord, int64, error) {
	membership, err := assets.tenantMembership(ctx, actor, tenantSlug, "asset:read")
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	if _, err := assets.store.GetAssetVersion(ctx, membership.TenantID, versionID); err != nil {
		return nil, 0, err
	}
	return assets.store.ListAssetVersionItems(ctx, membership.TenantID, versionID, query, int32(pageSize), int32((page-1)*pageSize))
}

// AssetSummaries returns the service-level asset summary list and the missing
// kind list for a service. Every registered kind without an asset is reported
// as a missing kind, deterministically ordered by kind id. Each summary is
// projected onto the repository default branch track.
func (assets *Assets) AssetSummaries(ctx context.Context, tenantID uuid.UUID, serviceID uuid.UUID) ([]AssetSummaryRecord, []MissingKindRecord, error) {
	records, err := assets.store.ListAssetsForService(ctx, tenantID, serviceID)
	if err != nil {
		return nil, nil, err
	}
	owned := make(map[string]bool, len(records))
	for _, record := range records {
		owned[record.Kind] = true
	}
	kinds, err := assets.store.ListAssetKinds(ctx)
	if err != nil {
		return nil, nil, err
	}
	summaries := make([]AssetSummaryRecord, 0, len(records))
	for _, record := range records {
		summary := AssetSummaryRecord{ID: record.ID, Kind: record.Kind, Name: record.Name, Lifecycle: "draft", Health: "ok", RefType: "branch"}
		if defaultBranch, err := assets.store.GetAssetRepositoryDefaultBranch(ctx, tenantID, record.ID); err == nil && defaultBranch != "" {
			summary.RefName = defaultBranch
			if track, trackErr := assets.store.GetAssetRefTrack(ctx, tenantID, record.ID, "branch", defaultBranch); trackErr == nil {
				summary.Health = track.Health
				if track.LatestVersionID != nil {
					if version, versionErr := assets.store.GetAssetVersion(ctx, tenantID, *track.LatestVersionID); versionErr == nil {
						summary.LatestVersion = &VersionRefRecord{ID: version.ID, Version: version.Version, Lifecycle: version.Lifecycle}
						summary.Lifecycle = version.Lifecycle
					}
				}
				if track.CurrentVersionID != nil {
					if version, versionErr := assets.store.GetAssetVersion(ctx, tenantID, *track.CurrentVersionID); versionErr == nil {
						summary.CurrentVersion = &VersionRefRecord{ID: version.ID, Version: version.Version, Lifecycle: version.Lifecycle}
						summary.Lifecycle = version.Lifecycle
					}
				}
			}
		}
		summaries = append(summaries, summary)
	}
	missing := make([]MissingKindRecord, 0)
	for _, kind := range kinds {
		if kind.Enabled && !owned[kind.ID] {
			missing = append(missing, MissingKindRecord{Kind: kind.ID, CanConfigure: true})
		}
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	sort.Slice(missing, func(i, j int) bool { return missing[i].Kind < missing[j].Kind })
	return summaries, missing, nil
}

// AssetTrackProjection carries the derived asset-level fields from a ref track.
type AssetTrackProjection struct {
	Lifecycle        string
	QualityScore     *int32
	Health           string
	LatestVersionID  *uuid.UUID
	CurrentVersionID *uuid.UUID
}

func (assets *Assets) trackProjection(ctx context.Context, tenantID uuid.UUID, track AssetRefTrackRecord) AssetTrackProjection {
	projection := AssetTrackProjection{LatestVersionID: track.LatestVersionID, CurrentVersionID: track.CurrentVersionID, Lifecycle: "draft", Health: track.Health}
	if projection.Health == "" {
		projection.Health = "ok"
	}
	if track.LatestVersionID != nil {
		latest, err := assets.store.GetAssetVersion(ctx, tenantID, *track.LatestVersionID)
		if err == nil {
			projection.Lifecycle = latest.Lifecycle
		}
	}
	if track.CurrentVersionID != nil {
		current, err := assets.store.GetAssetVersion(ctx, tenantID, *track.CurrentVersionID)
		if err == nil {
			projection.Lifecycle = current.Lifecycle
			if current.Revision > 0 {
				score := int32(0)
				projection.QualityScore = &score
			}
		}
	}
	return projection
}

// assetRepository is intentionally unused; the default branch is resolved
// directly via the store query, keeping the asset service free of repository
// graph traversal. Kept as a no-op seam for future multi-repo assets.
func assetRepository(ctx context.Context, store AssetStore, tenantID uuid.UUID, asset AssetRecord) uuid.UUID {
	return uuid.Nil()
}

func (assets *Assets) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, "*")) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || assets.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := assets.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}
