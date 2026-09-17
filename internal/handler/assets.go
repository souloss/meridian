package handler

import (
	"context"
	"strings"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	asset "github.com/meridian-labs/meridian/internal/generated/api/asset"
	repository "github.com/meridian-labs/meridian/internal/generated/api/repository"
	serviceapi "github.com/meridian-labs/meridian/internal/generated/api/service"
	view "github.com/meridian-labs/meridian/internal/generated/api/view"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// SyncRepository enqueues one repository synchronization job.
func (s *Server) SyncRepository(ctx context.Context, request repository.SyncRepositoryRequestObject) (repository.SyncRepositoryResponseObject, error) {
	if s.discovery == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	accepted, err := s.discovery.Sync(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.RepositoryId),
		serviceUUID(api.Uuid(request.Params.IdempotencyKey)), repositorySyncInput(*request.Body),
	)
	if err != nil {
		return nil, err
	}
	return repository.SyncRepository202JSONResponse(jobAcceptedResponse(accepted)), nil
}

// UpdateSourceSpec applies a validated patch to one source spec.
func (s *Server) UpdateSourceSpec(ctx context.Context, request asset.UpdateSourceSpecRequestObject) (asset.UpdateSourceSpecResponseObject, error) {
	if s.discovery == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.discovery.UpdateSourceSpec(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.SourceId),
		request.Params.IfMatch, sourceSpecPatchInput(*request.Body),
	)
	if err != nil {
		return nil, err
	}
	body := sourceSpecResponse(record)
	return asset.UpdateSourceSpec200JSONResponse{
		Body: body, Headers: asset.UpdateSourceSpec200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// GetService returns one service detail and records a recent read.
func (s *Server) GetService(ctx context.Context, request serviceapi.GetServiceRequestObject) (serviceapi.GetServiceResponseObject, error) {
	if s.discovery == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	detail, err := s.discovery.GetServiceDetail(ctx, principal, string(request.TenantSlug), string(request.ServiceSlug))
	if err != nil {
		return nil, err
	}
	body := serviceResponseWithAssets(detail.Service, detail.AssetSummaries, detail.MissingKinds)
	return serviceapi.GetService200JSONResponse{
		Body: body, Headers: serviceapi.GetService200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// ListRecentServices returns the caller's recently viewed services.
func (s *Server) ListRecentServices(ctx context.Context, request serviceapi.ListRecentServicesRequestObject) (serviceapi.ListRecentServicesResponseObject, error) {
	if s.discovery == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.discovery.ListRecentServices(ctx, principal, string(request.TenantSlug), page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.Service, 0, len(items))
	for _, item := range items {
		responses = append(responses, serviceResponse(item))
	}
	return serviceapi.ListRecentServices200JSONResponse(api.ServicePage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// GetAsset returns one asset projected onto the selected ref track.
func (s *Server) GetAsset(ctx context.Context, request asset.GetAssetRequestObject) (asset.GetAssetResponseObject, error) {
	if s.assetService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refType, refName := "", ""
	if request.Params.RefType != nil {
		refType = string(*request.Params.RefType)
	}
	if request.Params.Ref != nil {
		refName = string(*request.Params.Ref)
	}
	record, projection, err := s.assetService.GetAsset(ctx, principal, string(request.TenantSlug), serviceUUID(request.AssetId), refType, refName)
	if err != nil {
		return nil, err
	}
	body := assetResponse(record, projection)
	body.Ref = api.RefName(refName)
	body.RefType = api.RefType(refType)
	return asset.GetAsset200JSONResponse{
		Body: body, Headers: asset.GetAsset200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// GetAssetVersion returns one asset version with its layer manifest.
func (s *Server) GetAssetVersion(ctx context.Context, request asset.GetAssetVersionRequestObject) (asset.GetAssetVersionResponseObject, error) {
	if s.assetService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.assetService.GetAssetVersion(ctx, principal, string(request.TenantSlug), serviceUUID(request.VersionId))
	if err != nil {
		return nil, err
	}
	body := assetVersionResponse(record)
	return asset.GetAssetVersion200JSONResponse{
		Body: body, Headers: asset.GetAssetVersion200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// ListAssetVersionItems returns one page of indexed items.
func (s *Server) ListAssetVersionItems(ctx context.Context, request asset.ListAssetVersionItemsRequestObject) (asset.ListAssetVersionItemsResponseObject, error) {
	if s.assetService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	query := ""
	if request.Params.Q != nil {
		query = string(*request.Params.Q)
	}
	items, total, err := s.assetService.ListAssetVersionItems(ctx, principal, string(request.TenantSlug), serviceUUID(request.VersionId), query, page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.AssetItem, 0, len(items))
	for _, item := range items {
		responses = append(responses, assetItemResponse(item))
	}
	return asset.ListAssetVersionItems200JSONResponse(api.AssetItemPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// ResolveView resolves a built-in view descriptor from the registry.
func (s *Server) ResolveView(ctx context.Context, request view.ResolveViewRequestObject) (view.ResolveViewResponseObject, error) {
	if s.views == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	versionIDs := make([]uuid.UUID, 0)
	if request.Body.Inputs != nil {
		for _, selector := range *request.Body.Inputs {
			selected, err := selector.AsVersionSelector()
			if err != nil {
				return nil, service.ErrViewInputMismatch
			}
			versionIDs = append(versionIDs, serviceUUID(selected.VersionId))
		}
	}
	var scope *service.ScopeSelector
	if request.Body.Scope.IsSpecified() && !request.Body.Scope.IsNull() {
		value := request.Body.Scope.MustGet()
		scope = &service.ScopeSelector{Type: string(value.Type)}
		if value.Id.IsSpecified() && !value.Id.IsNull() {
			id := serviceUUID(value.Id.MustGet())
			scope.ID = &id
		}
	}
	var options map[string]any
	if request.Body.Options != nil {
		options = *request.Body.Options
	}
	resolution, err := s.views.Resolve(ctx, principal, string(request.TenantSlug), request.Body.ViewId, versionIDs, scope, options)
	if err != nil {
		return nil, err
	}
	var body api.ViewResolution
	switch resolution.Kind {
	case "document":
		document := api.DocumentViewResolution{
			Document: api.ArtifactLink{Url: resolution.Document.URL},
			Kind:     api.DocumentViewResolutionKind(resolution.Kind),
			View:     viewDefinitionResponse(resolution.View),
		}
		if err := body.FromDocumentViewResolution(document); err != nil {
			return nil, err
		}
	case "items":
		items := make([]api.AssetItem, 0, len(resolution.Items))
		for _, item := range resolution.Items {
			items = append(items, assetItemResponse(item))
		}
		if err := body.FromItemsViewResolution(api.ItemsViewResolution{
			Items: items, Kind: api.ItemsViewResolutionKind("items"), View: viewDefinitionResponse(resolution.View),
		}); err != nil {
			return nil, err
		}
	case "graph":
		nodes := make([]api.GraphNode, 0, len(resolution.Nodes))
		for _, node := range resolution.Nodes {
			nodes = append(nodes, api.GraphNode{Id: node.ID})
		}
		edges := make([]api.GraphEdge, 0, len(resolution.Edges))
		for _, edge := range resolution.Edges {
			edges = append(edges, api.GraphEdge{Id: edge.Source + "->" + edge.Target, From: edge.Source, To: edge.Target, Details: map[string]any{}})
		}
		if err := body.FromGraphViewResolution(api.GraphViewResolution{
			Nodes: nodes, Edges: edges, Truncated: resolution.Truncated, Kind: api.GraphViewResolutionKind("graph"), View: viewDefinitionResponse(resolution.View),
		}); err != nil {
			return nil, err
		}
	default:
		if err := body.FromDashboardViewResolution(api.DashboardViewResolution{
			Metrics: resolution.Metrics, Kind: api.DashboardViewResolutionKind("dashboard"), View: viewDefinitionResponse(resolution.View),
		}); err != nil {
			return nil, err
		}
	}
	return view.ResolveView200JSONResponse(body), nil
}

func repositorySyncInput(body api.RepositorySyncRequest) service.RepositorySyncInput {
	input := service.RepositorySyncInput{}
	if body.RefType != nil {
		input.RefType = string(*body.RefType)
	}
	if body.Ref != nil {
		input.RefName = string(*body.Ref)
	}
	if body.Force != nil {
		input.Force = body.Force
	}
	return input
}

func sourceSpecPatchInput(body api.SourceSpecPatchRequest) service.SourceSpecPatchInput {
	input := service.SourceSpecPatchInput{}
	if body.AssetNameTemplate != nil {
		value := string(*body.AssetNameTemplate)
		input.AssetNameTemplate = &value
	}
	if body.Role != nil {
		value := string(*body.Role)
		input.Role = &value
	}
	if body.Origin != nil {
		value := string(*body.Origin)
		input.Origin = &value
	}
	if body.Mode != nil {
		value := string(*body.Mode)
		input.Mode = &value
	}
	if body.Path.IsSpecified() && !body.Path.IsNull() {
		value := body.Path.MustGet()
		input.Path = &value
	}
	if body.ProducerProfileId.IsSpecified() && !body.ProducerProfileId.IsNull() {
		value := serviceUUID(body.ProducerProfileId.MustGet())
		input.ProducerProfileID = &value
	}
	if body.Ord != nil {
		input.Ord = body.Ord
	}
	if body.TimeoutSec != nil {
		input.TimeoutSec = body.TimeoutSec
	}
	if body.BranchPatterns != nil {
		input.BranchPatterns = append([]string(nil), *body.BranchPatterns...)
	}
	if body.Enabled != nil {
		input.Enabled = body.Enabled
	}
	return input
}

func assetResponse(record service.AssetRecord, projection service.AssetTrackProjection) api.Asset {
	latest := nullable.NewNullNullable[api.VersionRef]()
	if projection.LatestVersionID != nil {
		latest = nullable.NewNullableWithValue(api.VersionRef{Id: api.Uuid(*projection.LatestVersionID)})
	}
	current := nullable.NewNullNullable[api.VersionRef]()
	if projection.CurrentVersionID != nil {
		current = nullable.NewNullableWithValue(api.VersionRef{Id: api.Uuid(*projection.CurrentVersionID)})
	}
	quality := nullable.NewNullNullable[int]()
	if projection.QualityScore != nil {
		quality = nullable.NewNullableWithValue(int(*projection.QualityScore))
	}
	return api.Asset{
		Id: api.Uuid(record.ID), Etag: revisionETag("asset", record.ID.String(), record.Revision),
		ServiceId: api.Uuid(record.ServiceID), Kind: api.KindId(record.Kind), Name: api.AssetName(record.Name),
		Lifecycle: api.Lifecycle(projection.Lifecycle), LatestVersion: latest, CurrentVersion: current,
		QualityScore: quality, Health: api.AssetHealth(projection.Health), Layers: []api.LayerSummary{}, Capabilities: api.CapabilityList{},
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func assetVersionResponse(record service.AssetVersionRecord) api.AssetVersion {
	manifest := make([]api.LayerManifestEntry, 0, len(record.LayerManifest))
	for _, entry := range record.LayerManifest {
		manifest = append(manifest, api.LayerManifestEntry{
			LayerId: api.Uuid(entry.LayerID), RevisionId: api.Uuid(entry.RevisionID), Role: api.LayerRole(entry.Role),
			Origin: api.LayerOrigin(entry.Origin), Ord: entry.Ord, ScopeType: api.LayerManifestEntryScopeType(entry.ScopeType),
			ScopeKey: entry.ScopeKey, ReviewStatus: api.RevisionStatus(entry.ReviewStatus), ContentHash: entry.ContentHash,
		})
	}
	sourceCommit := nullable.NewNullNullable[string]()
	if record.SourceCommit != nil {
		sourceCommit = nullable.NewNullableWithValue(*record.SourceCommit)
	}
	baseline := nullable.NewNullNullable[api.Uuid]()
	diff := nullable.NewNullNullable[api.DiffCounts]()
	indexedAt := nullable.NewNullNullable[api.Timestamp]()
	return api.AssetVersion{
		Id: api.Uuid(record.ID), Etag: revisionETag("asset-version", record.ID.String(), record.Revision),
		AssetId: api.Uuid(record.AssetID), SequenceNo: int(record.SequenceNo), Version: record.Version,
		Lifecycle: api.Lifecycle(record.Lifecycle), InputFingerprint: record.InputFingerprint,
		MergeEngineVersion: record.MergeEngineVersion, LayerManifest: manifest, SourceCommit: sourceCommit,
		BaselineVersionId: baseline, DiffSummary: diff, IndexedAt: indexedAt,
		MergedHash: "", KindPluginVersion: "", NormalizerVersion: "", OverlayCompilerVersion: "",
		OverlayMode: "", QualityScore: 0, Labels: map[string]any{}, Downloads: map[string]any{},
		Capabilities: api.CapabilityList{}, CreatedAt: record.CreatedAt,
	}
}

func assetItemResponse(record service.AssetItemRecord) api.AssetItem {
	return api.AssetItem{
		ItemType: record.ItemType, Key: record.Key, Display: record.Display, Provenance: []api.ProvenanceEntry{},
	}
}

func viewDefinitionResponse(definition service.ViewDefinition) api.ViewDefinition {
	entrypoint := definition.Entrypoint
	component := definition.Component
	externalURL := definition.ExternalURL
	columnsSource := definition.ColumnsSource
	return api.ViewDefinition{
		Id: definition.ID, NameKey: definition.NameKey, Milestone: definition.Milestone,
		Mount: api.ViewDefinitionMount(definition.Mount), Component: component, Entrypoint: entrypoint, ExternalUrl: externalURL,
		DefaultOptions: definition.DefaultOptions, OptionsSchema: definition.OptionsSchema, ColumnsSource: columnsSource,
		ItemTypes: stringSlicePointer(definition.ItemTypes), Columns: mapSlicePointer(definition.Columns), FallbackColumns: mapSlicePointer(definition.FallbackColumns),
		Input: api.ViewInputSpec{
			Mode: api.ViewInputSpecMode(definition.InputMode),
			Kinds: kindsUnion(definition.InputKinds),
			MinDocs: intPointer(definition.InputMinDocs), MaxDocs: intPointer(definition.InputMaxDocs),
			SameKind: boolPointer(definition.InputSameKind), SameAsset: boolPointer(definition.InputSameAsset),
			Scopes: scopeStringsPointer(definition.InputScopes),
		},
	}
}

func kindsUnion(kinds []string) api.ViewInputSpec_Kinds {
	var union api.ViewInputSpec_Kinds
	if len(kinds) == 1 && kinds[0] == "*" {
		_ = union.FromViewInputSpecKinds0("*")
		return union
	}
	list := make([]api.KindId, 0, len(kinds))
	for _, kind := range kinds {
		list = append(list, api.KindId(kind))
	}
	_ = union.FromViewInputSpecKinds1(list)
	return union
}

func stringSlicePointer(values []string) *[]string {
	if values == nil {
		return nil
	}
	copied := append([]string(nil), values...)
	return &copied
}

func mapSlicePointer(values []map[string]any) *[]map[string]any {
	if values == nil {
		return nil
	}
	copied := make([]map[string]any, len(values))
	for index, value := range values {
		copied[index] = value
	}
	return &copied
}

func scopeStringsPointer(values []string) *[]api.ViewInputSpecScopes {
	if values == nil {
		return nil
	}
	copied := make([]api.ViewInputSpecScopes, len(values))
	for index, value := range values {
		copied[index] = api.ViewInputSpecScopes(value)
	}
	return &copied
}

func intPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func boolPointer(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}

var _ = strings.TrimSpace
