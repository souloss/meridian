package repository

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// AssetStore implements the M1 asset pipeline persistence boundary on top of
// the generated repository queries. It embeds RepositoryStore to reuse the
// repository read path and keeps the tenant predicate on every read.
type AssetStore struct {
	*RepositoryStore
	queries *generated.Queries
}

// NewAssetStore binds asset pipeline persistence to a native pgx pool.
func NewAssetStore(pool *pgxpool.Pool) *AssetStore {
	return &AssetStore{RepositoryStore: NewRepositoryStore(pool), queries: generated.New(pool)}
}

// GetAssetKind returns one platform asset kind registration.
func (store *AssetStore) GetAssetKind(ctx context.Context, id string) (service.AssetKindRecord, error) {
	row, err := store.queries.GetAssetKind(ctx, id)
	if err != nil {
		return service.AssetKindRecord{}, normalizeError(err)
	}
	return service.AssetKindRecord{ID: row.ID, ContractVersion: row.ContractVersion, Enabled: row.Enabled, PluginVersion: row.PluginVersion}, nil
}

// ListAssetKinds returns every registered asset kind, including disabled ones.
func (store *AssetStore) ListAssetKinds(ctx context.Context) ([]service.AssetKindRecord, error) {
	rows, err := store.queries.ListAssetKinds(ctx)
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.AssetKindRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.AssetKindRecord{ID: row.ID, ContractVersion: row.ContractVersion, Enabled: row.Enabled, PluginVersion: row.PluginVersion})
	}
	return items, nil
}

// GetAssetRepositoryDefaultBranch resolves the default branch of an asset's repository.
func (store *AssetStore) GetAssetRepositoryDefaultBranch(ctx context.Context, tenantID, assetID uuid.UUID) (string, error) {
	return store.queries.GetAssetRepositoryDefaultBranch(ctx, generated.GetAssetRepositoryDefaultBranchParams{TenantID: tenantID, AssetID: assetID})
}

// ListServicesByRepository returns active services for one repository.
func (store *AssetStore) ListServicesByRepository(ctx context.Context, tenantID, repositoryID uuid.UUID) ([]service.ServiceRecord, error) {
	rows, err := store.queries.ListServicesByRepository(ctx, generated.ListServicesByRepositoryParams{TenantID: tenantID, RepositoryID: repositoryID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.ServiceRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.ServiceRecord{
			TenantID: row.TenantID, ID: row.ID, RepositoryID: row.RepositoryID, Slug: row.Slug, DisplayName: row.DisplayName,
			Description: row.Description, RootDir: row.RootDir, Language: row.Language, Framework: row.Framework,
			Visibility: row.Visibility, Lifecycle: row.Lifecycle, Revision: row.Revision,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return items, nil
}

// ListAssetsForService returns active assets for one service.
func (store *AssetStore) ListAssetsForService(ctx context.Context, tenantID, serviceID uuid.UUID) ([]service.AssetRecord, error) {
	rows, err := store.queries.ListAssetsForService(ctx, generated.ListAssetsForServiceParams{TenantID: tenantID, ServiceID: serviceID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.AssetRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.AssetRecord{ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, Name: row.Name, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time})
	}
	return items, nil
}

// ListSourceSpecsForService returns active source specs for one service.
func (store *AssetStore) ListSourceSpecsForService(ctx context.Context, tenantID, serviceID uuid.UUID) ([]service.SourceSpecRecord, error) {
	rows, err := store.queries.ListSourceSpecsForService(ctx, generated.ListSourceSpecsForServiceParams{TenantID: tenantID, ServiceID: serviceID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.SourceSpecRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.SourceSpecRecord{
			ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, AssetNameTemplate: row.AssetNameTemplate,
			Role: row.Role, Origin: row.Origin, Mode: row.Mode, Path: row.Path, ProducerProfileID: row.ProducerProfileID,
			Ord: int(row.Ord), TimeoutSec: int(row.TimeoutSec), BranchPatterns: append([]string(nil), row.BranchPatterns...),
			Enabled: row.Enabled, ConfigOrigin: row.ConfigOrigin, BindingsCount: 0, Revision: row.Revision,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return items, nil
}

// UpsertAsset creates or reuses one asset by its unique (service, kind, name) key.
func (store *AssetStore) UpsertAsset(ctx context.Context, input service.NewAsset) (service.AssetRecord, error) {
	row, err := store.queries.UpsertAsset(ctx, generated.UpsertAssetParams{
		TenantID: input.TenantID, ID: input.ID, ServiceID: input.ServiceID, Kind: input.Kind, Name: input.Name,
	})
	if err != nil {
		return service.AssetRecord{}, normalizeError(err)
	}
	return service.AssetRecord{ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, Name: row.Name, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

// GetAssetByName returns one active asset by its unique key.
func (store *AssetStore) GetAssetByName(ctx context.Context, tenantID, serviceID uuid.UUID, kind, name string) (service.AssetRecord, error) {
	row, err := store.queries.GetAssetByName(ctx, generated.GetAssetByNameParams{TenantID: tenantID, ServiceID: serviceID, Kind: kind, Name: name})
	if err != nil {
		return service.AssetRecord{}, normalizeError(err)
	}
	return service.AssetRecord{ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, Name: row.Name, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

// GetAsset returns one active asset by id.
func (store *AssetStore) GetAsset(ctx context.Context, tenantID, id uuid.UUID) (service.AssetRecord, error) {
	row, err := store.queries.GetAsset(ctx, generated.GetAssetParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.AssetRecord{}, normalizeError(err)
	}
	return service.AssetRecord{ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, Name: row.Name, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

// CreateAssetRefTrack creates or reactivates one asset ref track.
func (store *AssetStore) CreateAssetRefTrack(ctx context.Context, input service.NewAssetRefTrack) (service.AssetRefTrackRecord, error) {
	row, err := store.queries.CreateAssetRefTrack(ctx, generated.CreateAssetRefTrackParams{
		TenantID: input.TenantID, ID: input.ID, AssetID: input.AssetID, RefType: input.RefType, RefName: input.RefName, Health: input.Health,
	})
	if err != nil {
		return service.AssetRefTrackRecord{}, normalizeError(err)
	}
	return service.AssetRefTrackRecord{ID: row.ID, AssetID: row.AssetID, RefType: row.RefType, RefName: row.RefName, LatestVersionID: row.LatestVersionID, CurrentVersionID: row.CurrentVersionID, Health: row.Health}, nil
}

// GetAssetRefTrack returns one asset ref track.
func (store *AssetStore) GetAssetRefTrack(ctx context.Context, tenantID, assetID uuid.UUID, refType, refName string) (service.AssetRefTrackRecord, error) {
	row, err := store.queries.GetAssetRefTrack(ctx, generated.GetAssetRefTrackParams{TenantID: tenantID, AssetID: assetID, RefType: refType, RefName: refName})
	if err != nil {
		return service.AssetRefTrackRecord{}, normalizeError(err)
	}
	return service.AssetRefTrackRecord{ID: row.ID, AssetID: row.AssetID, RefType: row.RefType, RefName: row.RefName, LatestVersionID: row.LatestVersionID, CurrentVersionID: row.CurrentVersionID, Health: row.Health}, nil
}

// CreateLayer inserts one asset layer.
func (store *AssetStore) CreateLayer(ctx context.Context, input service.NewLayer) (service.LayerRecord, error) {
	row, err := store.queries.CreateLayer(ctx, generated.CreateLayerParams{
		TenantID: input.TenantID, ID: input.ID, AssetID: input.AssetID, SourceSpecID: input.SourceSpecID,
		Role: input.Role, Origin: input.Origin, Ord: int32(input.Ord), Dialect: input.Dialect, Enabled: input.Enabled,
		BranchPatterns: input.BranchPatterns, DisplayName: input.DisplayName,
	})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

// GetBaseLayerForAsset returns the base layer of an asset.
func (store *AssetStore) GetBaseLayerForAsset(ctx context.Context, tenantID, assetID uuid.UUID) (service.LayerRecord, error) {
	row, err := store.queries.GetBaseLayerForAsset(ctx, generated.GetBaseLayerForAssetParams{TenantID: tenantID, AssetID: assetID})
	if err != nil {
		return service.LayerRecord{}, normalizeError(err)
	}
	return layerFromRow(row), nil
}

// CreateLayerRevision inserts one immutable layer revision.
func (store *AssetStore) CreateLayerRevision(ctx context.Context, input service.NewLayerRevision) (service.LayerRevisionRecord, error) {
	row, err := store.queries.CreateLayerRevision(ctx, generated.CreateLayerRevisionParams{
		TenantID: input.TenantID, ID: input.ID, LayerID: input.LayerID, ScopeType: input.ScopeType, ScopeKey: input.ScopeKey,
		ContentHash: input.ContentHash, ContentRef: input.ContentRef, ContentType: input.ContentType, Dialect: input.Dialect,
		SourceBranch: input.SourceBranch, ReviewStatus: input.ReviewStatus, GitCommit: input.GitCommit, CreatedBy: input.CreatedBy, ProducerRunID: input.ProducerRunID,
	})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return service.LayerRevisionRecord{
		ID: row.ID, LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		ContentHash: row.ContentHash, ContentRef: row.ContentRef, ContentType: row.ContentType, Dialect: row.Dialect, ReviewStatus: row.ReviewStatus,
		GitCommit: row.GitCommit, SourceBranch: row.SourceBranch, CreatedAt: row.CreatedAt.Time,
	}, nil
}

// GetLatestLayerRevision returns the newest revision in one layer scope.
func (store *AssetStore) GetLatestLayerRevision(ctx context.Context, tenantID, layerID uuid.UUID, scopeType, scopeKey string) (service.LayerRevisionRecord, error) {
	row, err := store.queries.GetLatestLayerRevision(ctx, generated.GetLatestLayerRevisionParams{TenantID: tenantID, LayerID: layerID, ScopeType: scopeType, ScopeKey: scopeKey})
	if err != nil {
		return service.LayerRevisionRecord{}, normalizeError(err)
	}
	return service.LayerRevisionRecord{
		ID: row.ID, LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		ContentHash: row.ContentHash, ContentRef: row.ContentRef, ContentType: row.ContentType, Dialect: row.Dialect, ReviewStatus: row.ReviewStatus,
		GitCommit: row.GitCommit, SourceBranch: row.SourceBranch, CreatedAt: row.CreatedAt.Time,
	}, nil
}

// UpsertLayerHead upserts one layer head pointer.
func (store *AssetStore) UpsertLayerHead(ctx context.Context, input service.NewLayerHead) (service.LayerHeadRecord, error) {
	row, err := store.queries.UpsertLayerHead(ctx, generated.UpsertLayerHeadParams{
		TenantID: input.TenantID, LayerID: input.LayerID, ScopeType: input.ScopeType, ScopeKey: input.ScopeKey,
		LatestRevisionID: input.LatestRevisionID, EffectiveRevisionID: input.EffectiveRevisionID, CandidateRevisionID: input.CandidateRevisionID, Generation: input.Generation,
	})
	if err != nil {
		return service.LayerHeadRecord{}, normalizeError(err)
	}
	return service.LayerHeadRecord{
		LayerID: row.LayerID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		LatestRevisionID: row.LatestRevisionID, EffectiveRevisionID: row.EffectiveRevisionID, CandidateRevisionID: row.CandidateRevisionID, Generation: row.Generation,
	}, nil
}

// CreateAssetVersion inserts one asset version.
func (store *AssetStore) CreateAssetVersion(ctx context.Context, input service.NewAssetVersion) (service.AssetVersionRecord, error) {
	row, err := store.queries.CreateAssetVersion(ctx, generated.CreateAssetVersionParams{
		TenantID: input.TenantID, ID: input.ID, AssetID: input.AssetID, TrackID: input.TrackID, SequenceNo: input.SequenceNo, Version: input.Version, Lifecycle: input.Lifecycle, Revision: input.Revision,
		QualityScore: input.QualityScore, MergeRequestID: input.MergeRequestID, InputFingerprint: input.InputFingerprint, MergeEngineVersion: input.MergeEngineVersion,
		OverlayCompilerVersion: input.OverlayCompilerVersion, OverlayMode: input.OverlayMode, NormalizerVersion: input.NormalizerVersion, KindPluginVersion: input.KindPluginVersion,
		LayerManifest: input.LayerManifest, MergedHash: input.MergedHash, MergedRef: input.MergedRef, NormalizedRef: input.NormalizedRef, BundledRef: input.BundledRef, ProvenanceRef: input.ProvenanceRef,
		SourceCommit: input.SourceCommit, BaselineVersionID: input.BaselineVersionID, DiffSummary: input.DiffSummary, Labels: input.Labels, IndexComplete: input.IndexComplete,
	})
	if err != nil {
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// GetAssetVersion returns one asset version.
func (store *AssetStore) GetAssetVersion(ctx context.Context, tenantID, id uuid.UUID) (service.AssetVersionRecord, error) {
	row, err := store.queries.GetAssetVersion(ctx, generated.GetAssetVersionParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// MarkAssetVersionIndexed marks one asset version as fully item-indexed.
func (store *AssetStore) MarkAssetVersionIndexed(ctx context.Context, tenantID, id uuid.UUID) error {
	changed, err := store.queries.MarkAssetVersionIndexed(ctx, generated.MarkAssetVersionIndexedParams{TenantID: tenantID, ID: id})
	if err != nil {
		return normalizeError(err)
	}
	if changed != 1 {
		return service.ErrNotFound
	}
	return nil
}

// GetLatestVersionInTrack returns the newest version in a track.
func (store *AssetStore) GetLatestVersionInTrack(ctx context.Context, tenantID, trackID uuid.UUID) (service.AssetVersionRecord, error) {
	row, err := store.queries.GetLatestVersionInTrack(ctx, generated.GetLatestVersionInTrackParams{TenantID: tenantID, TrackID: trackID})
	if err != nil {
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// GetCurrentVersionInTrack returns the currently published version in a track.
func (store *AssetStore) GetCurrentVersionInTrack(ctx context.Context, tenantID, trackID uuid.UUID) (service.AssetVersionRecord, error) {
	row, err := store.queries.GetCurrentVersionInTrack(ctx, generated.GetCurrentVersionInTrackParams{TenantID: tenantID, TrackID: trackID})
	if err != nil {
		return service.AssetVersionRecord{}, normalizeError(err)
	}
	return assetVersionFromRow(row), nil
}

// UpdateAssetRefTrackHead updates the version pointers on a track.
func (store *AssetStore) UpdateAssetRefTrackHead(ctx context.Context, tenantID, trackID uuid.UUID, latestVersionID, currentVersionID *uuid.UUID, processedGeneration int64) error {
	changed, err := store.queries.UpdateAssetRefTrackHead(ctx, generated.UpdateAssetRefTrackHeadParams{
		TenantID: tenantID, ID: trackID, LatestVersionID: latestVersionID, CurrentVersionID: currentVersionID, ProcessedGeneration: processedGeneration,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed != 1 {
		return service.ErrPrecondition
	}
	return nil
}

// UpdateSourceSpec applies a validated source spec patch under its revision.
func (store *AssetStore) UpdateSourceSpec(ctx context.Context, input service.SourceSpecPatch) (service.SourceSpecRecord, error) {
	row, err := store.queries.UpdateSourceSpec(ctx, generated.UpdateSourceSpecParams{
		TenantID: input.TenantID, ID: input.ID, ExpectedRevision: input.ExpectedRevision,
		AssetNameTemplate: input.AssetNameTemplate, Role: input.Role, Origin: input.Origin, Mode: input.Mode,
		Path: input.Path, ProducerProfileID: input.ProducerProfileID,
		Ord: int32Pointer(input.Ord), TimeoutSec: int32Pointer(input.TimeoutSec),
		BranchPatterns: input.BranchPatterns, Enabled: input.Enabled,
	})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	return service.SourceSpecRecord{
		ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, AssetNameTemplate: row.AssetNameTemplate,
		Role: row.Role, Origin: row.Origin, Mode: row.Mode, Path: row.Path, ProducerProfileID: row.ProducerProfileID,
		Ord: int(row.Ord), TimeoutSec: int(row.TimeoutSec), BranchPatterns: append([]string(nil), row.BranchPatterns...),
		Enabled: row.Enabled, ConfigOrigin: row.ConfigOrigin, Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func int32Pointer(value *int) *int32 {
	if value == nil {
		return nil
	}
	return new(int32(*value))
}

// CreateAssetItem inserts one indexed asset item.
func (store *AssetStore) CreateAssetItem(ctx context.Context, input service.NewAssetItem) (service.AssetItemRecord, error) {
	row, err := store.queries.CreateAssetItem(ctx, generated.CreateAssetItemParams{
		TenantID: input.TenantID, ID: input.ID, AssetVersionID: input.AssetVersionID, AssetID: input.AssetID, ServiceID: input.ServiceID,
		Kind: input.Kind, ItemType: input.ItemType, Key: input.Key, Display: input.Display, SearchText: input.SearchText, SearchRaw: input.SearchRaw, Provenance: input.Provenance,
	})
	if err != nil {
		return service.AssetItemRecord{}, normalizeError(err)
	}
	return assetItemFromRow(row), nil
}

// UpdateAssetItemSearchVector refreshes the tsvector for one item.
func (store *AssetStore) UpdateAssetItemSearchVector(ctx context.Context, tenantID, id uuid.UUID, searchText string) error {
	changed, err := store.queries.UpdateAssetItemSearchVector(ctx, generated.UpdateAssetItemSearchVectorParams{TenantID: tenantID, ID: id, SearchText: []byte(searchText)})
	if err != nil {
		return normalizeError(err)
	}
	if changed != 1 {
		return service.ErrNotFound
	}
	return nil
}

// ListAssetVersionItems returns one item page.
func (store *AssetStore) ListAssetVersionItems(ctx context.Context, tenantID, versionID uuid.UUID, query string, limit, offset int32) ([]service.AssetItemRecord, int64, error) {
	total, err := store.queries.CountAssetVersionItems(ctx, generated.CountAssetVersionItemsParams{TenantID: tenantID, AssetVersionID: versionID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListAssetVersionItems(ctx, generated.ListAssetVersionItemsParams{
		TenantID: tenantID, AssetVersionID: versionID, SearchQuery: query, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.AssetItemRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, assetItemFromRow(row))
	}
	return items, total, nil
}

// UpsertSourceBinding upserts one source binding for a scope+expansion key.
func (store *AssetStore) UpsertSourceBinding(ctx context.Context, input service.NewSourceBinding) (service.SourceBindingRecord, error) {
	row, err := store.queries.UpsertSourceBinding(ctx, generated.UpsertSourceBindingParams{
		TenantID: input.TenantID, ID: input.ID, SourceSpecID: input.SourceSpecID, ScopeType: input.ScopeType, ScopeKey: input.ScopeKey,
		ExpansionKey: input.ExpansionKey, ResolvedPath: input.ResolvedPath, SourceSystem: input.SourceSystem, AssetID: input.AssetID, LayerID: input.LayerID, State: input.State, LastSeenCommit: input.LastSeenCommit,
	})
	if err != nil {
		return service.SourceBindingRecord{}, normalizeError(err)
	}
	return service.SourceBindingRecord{
		ID: row.ID, SourceSpecID: row.SourceSpecID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
		ExpansionKey: row.ExpansionKey, ResolvedPath: row.ResolvedPath, SourceSystem: row.SourceSystem,
		AssetID: row.AssetID, LayerID: row.LayerID, State: row.State, LastSeenCommit: row.LastSeenCommit,
	}, nil
}

// ListActiveBindingsForScope returns active bindings for one scope.
func (store *AssetStore) ListActiveBindingsForScope(ctx context.Context, tenantID, sourceSpecID uuid.UUID, scopeType, scopeKey string) ([]service.SourceBindingRecord, error) {
	rows, err := store.queries.ListActiveBindingsForScope(ctx, generated.ListActiveBindingsForScopeParams{
		TenantID: tenantID, SourceSpecID: sourceSpecID, ScopeType: scopeType, ScopeKey: scopeKey,
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.SourceBindingRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.SourceBindingRecord{
			ID: row.ID, SourceSpecID: row.SourceSpecID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
			ExpansionKey: row.ExpansionKey, ResolvedPath: row.ResolvedPath, SourceSystem: row.SourceSystem,
			AssetID: row.AssetID, LayerID: row.LayerID, State: row.State, LastSeenCommit: row.LastSeenCommit,
		})
	}
	return items, nil
}

// MarkBindingsStaleInScope marks unseen bindings in one scope stale.
func (store *AssetStore) MarkBindingsStaleInScope(ctx context.Context, tenantID, sourceSpecID uuid.UUID, scopeType, scopeKey string, seenIDs []uuid.UUID) error {
	if _, err := store.queries.MarkBindingsStaleInScope(ctx, generated.MarkBindingsStaleInScopeParams{
		TenantID: tenantID, SourceSpecID: sourceSpecID, ScopeType: scopeType, ScopeKey: scopeKey, SeenIds: seenIDs,
	}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// CountActiveBindings returns the active binding count for one source spec.
func (store *AssetStore) CountActiveBindings(ctx context.Context, tenantID, sourceSpecID uuid.UUID) (int64, error) {
	return store.queries.CountActiveBindings(ctx, generated.CountActiveBindingsParams{TenantID: tenantID, SourceSpecID: sourceSpecID})
}

// SetSourceLastError records one source materialization failure.
func (store *AssetStore) SetSourceLastError(ctx context.Context, tenantID, sourceSpecID uuid.UUID, message string) error {
	if _, err := store.queries.SetSourceLastError(ctx, generated.SetSourceLastErrorParams{TenantID: tenantID, ID: sourceSpecID, LastError: new(message)}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// ClearSourceLastError clears a source failure after a successful materialization.
func (store *AssetStore) ClearSourceLastError(ctx context.Context, tenantID, sourceSpecID uuid.UUID) error {
	if _, err := store.queries.ClearSourceLastError(ctx, generated.ClearSourceLastErrorParams{TenantID: tenantID, ID: sourceSpecID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// MarkTracksStaleForSourceSpec marks every track of a source's assets stale.
func (store *AssetStore) MarkTracksStaleForSourceSpec(ctx context.Context, tenantID, sourceSpecID uuid.UUID) error {
	if _, err := store.queries.MarkTracksStaleForSourceSpec(ctx, generated.MarkTracksStaleForSourceSpecParams{TenantID: tenantID, SourceSpecID: sourceSpecID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// MarkTracksHealthyForSourceSpec restores every track of a source's assets to ok.
func (store *AssetStore) MarkTracksHealthyForSourceSpec(ctx context.Context, tenantID, sourceSpecID uuid.UUID) error {
	if _, err := store.queries.MarkTracksHealthyForSourceSpec(ctx, generated.MarkTracksHealthyForSourceSpecParams{TenantID: tenantID, SourceSpecID: sourceSpecID}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// UpsertRecentService records one successful service detail read.
func (store *AssetStore) UpsertRecentService(ctx context.Context, tenantID, userID, serviceID uuid.UUID, viewedAt time.Time) error {
	if _, err := store.queries.UpsertRecentService(ctx, generated.UpsertRecentServiceParams{TenantID: tenantID, UserID: userID, ServiceID: serviceID, ViewedAt: timestamp(viewedAt)}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// ListRecentServices returns one page of recently viewed services.
func (store *AssetStore) ListRecentServices(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]service.ServiceRecord, int64, error) {
	total, err := store.queries.CountRecentServices(ctx, generated.CountRecentServicesParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListRecentServices(ctx, generated.ListRecentServicesParams{TenantID: tenantID, UserID: userID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.ServiceRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.ServiceRecord{
			TenantID: row.TenantID, ID: row.ID, RepositoryID: row.RepositoryID, Slug: row.Slug, DisplayName: row.DisplayName,
			Description: row.Description, RootDir: row.RootDir, Language: row.Language, Framework: row.Framework,
			Visibility: row.Visibility, Lifecycle: row.Lifecycle, Revision: row.Revision,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return items, total, nil
}

func layerFromRow(row generated.Layer) service.LayerRecord {
	return service.LayerRecord{
		ID: row.ID, AssetID: row.AssetID, SourceSpecID: row.SourceSpecID, Role: row.Role, Origin: row.Origin,
		Ord: int(row.Ord), Dialect: row.Dialect, Enabled: row.Enabled, BranchPatterns: append([]string(nil), row.BranchPatterns...),
		DisplayName: row.DisplayName, Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func assetVersionFromRow(row generated.AssetVersion) service.AssetVersionRecord {
	var manifest []service.LayerManifestEntry
	if len(row.LayerManifest) > 0 {
		_ = json.Unmarshal(row.LayerManifest, &manifest)
	}
	return service.AssetVersionRecord{
		ID: row.ID, AssetID: row.AssetID, TrackID: row.TrackID, SequenceNo: row.SequenceNo, Version: row.Version,
		Lifecycle: row.Lifecycle, Revision: row.Revision, InputFingerprint: row.InputFingerprint,
		MergeEngineVersion: row.MergeEngineVersion, LayerManifest: manifest, SourceCommit: row.SourceCommit,
		IndexComplete: row.IndexComplete, CreatedAt: row.CreatedAt.Time,
	}
}

func assetItemFromRow(row generated.AssetItem) service.AssetItemRecord {
	var display map[string]any
	if len(row.Display) > 0 {
		_ = json.Unmarshal(row.Display, &display)
	}
	if display == nil {
		display = map[string]any{}
	}
	return service.AssetItemRecord{ItemType: row.ItemType, Key: row.Key, Display: display}
}

var _ service.AssetStore = (*AssetStore)(nil)

var _ = fmt.Sprintf
