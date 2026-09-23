package handler

import (
	"context"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	layer "github.com/meridian-labs/meridian/internal/generated/api/layer"
	view "github.com/meridian-labs/meridian/internal/generated/api/view"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// GetLayer 返回一个层及其头指针。
func (s *Server) GetLayer(ctx context.Context, request layer.GetLayerRequestObject) (layer.GetLayerResponseObject, error) {
	if s.layerEdit == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refType := ""
	refName := ""
	if request.Params.RefType != nil {
		refType = string(*request.Params.RefType)
	}
	if request.Params.Ref != nil {
		refName = string(*request.Params.Ref)
	}
	record, heads, err := s.layerEdit.GetLayer(ctx, principal, string(request.TenantSlug), serviceUUID(request.LayerId), refType, refName)
	if err != nil {
		return nil, err
	}
	body := layerResponse(record)
	body.Heads = make([]api.LayerHead, 0, len(heads))
	for _, head := range heads {
		body.Heads = append(body.Heads, layerHeadResponse(head))
	}
	return layer.GetLayer200JSONResponse{
		Body: body, Headers: layer.GetLayer200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// UpdateLayer 在 If-Match 下更新一个层的角色、方言与启停。
func (s *Server) UpdateLayer(ctx context.Context, request layer.UpdateLayerRequestObject) (layer.UpdateLayerResponseObject, error) {
	if s.layerEdit == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	patch := service.LayerPatchRecord{Enabled: request.Body.Enabled}
	if request.Body.Role != nil {
		role := string(*request.Body.Role)
		patch.Role = &role
	}
	if request.Body.Dialect.IsSpecified() && !request.Body.Dialect.IsNull() {
		dialect := request.Body.Dialect.MustGet()
		patch.Dialect = &dialect
	}
	record, err := s.layerEdit.UpdateLayer(ctx, principal, string(request.TenantSlug), serviceUUID(request.LayerId), request.Params.IfMatch, patch)
	if err != nil {
		return nil, err
	}
	body := layerResponse(record)
	return layer.UpdateLayer200JSONResponse{
		Body: body, Headers: layer.UpdateLayer200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// GetLayerRevision 返回一条不可变层修订。
func (s *Server) GetLayerRevision(ctx context.Context, request layer.GetLayerRevisionRequestObject) (layer.GetLayerRevisionResponseObject, error) {
	if s.layerEdit == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.layerEdit.GetLayerRevision(ctx, principal, string(request.TenantSlug), serviceUUID(request.RevisionId))
	if err != nil {
		return nil, err
	}
	return layer.GetLayerRevision200JSONResponse(layerRevisionResponse(record)), nil
}

// ListLayerRevisions 返回某层某作用域内一页修订。
func (s *Server) ListLayerRevisions(ctx context.Context, request layer.ListLayerRevisionsRequestObject) (layer.ListLayerRevisionsResponseObject, error) {
	if s.layerEdit == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refType := ""
	refName := ""
	if request.Params.RefType != nil {
		refType = string(*request.Params.RefType)
	}
	if request.Params.Ref != nil {
		refName = string(*request.Params.Ref)
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.layerEdit.ListLayerRevisions(ctx, principal, string(request.TenantSlug), serviceUUID(request.LayerId), refType, refName, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.LayerRevision, 0, len(records))
	for _, record := range records {
		items = append(items, layerRevisionResponse(record))
	}
	return layer.ListLayerRevisions200JSONResponse(api.LayerRevisionPage{
		Total: int(total), Page: page, PageSize: pageSize, Items: items,
	}), nil
}

// ListReviews 返回租户内一页待审核修订。
func (s *Server) ListReviews(ctx context.Context, request layer.ListReviewsRequestObject) (layer.ListReviewsResponseObject, error) {
	if s.layerEdit == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.layerEdit.ListReviews(ctx, principal, string(request.TenantSlug), page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.ReviewContext, 0, len(records))
	for _, record := range records {
		items = append(items, api.ReviewContext{
			Revision:                 layerRevisionResponse(record),
			CurrentEffectiveRevision: nullable.NewNullNullable[api.LayerRevision](),
			Diff:                     emptyDiffResult(), Author: nullable.NewNullNullable[api.User](),
		})
	}
	return layer.ListReviews200JSONResponse(api.ReviewPage{
		Total: int(total), Page: page, PageSize: pageSize, Items: items,
	}), nil
}

// ListAssetLayerContents 一次性返回资产每个层的生效内容与元数据。
func (s *Server) ListAssetLayerContents(ctx context.Context, request layer.ListAssetLayerContentsRequestObject) (layer.ListAssetLayerContentsResponseObject, error) {
	if s.layerEdit == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.layerEdit.ListAssetLayerContents(ctx, principal, string(request.TenantSlug), serviceUUID(request.AssetId))
	if err != nil {
		return nil, err
	}
	layers := make([]api.LayerContent, 0, len(records))
	for _, record := range records {
		layers = append(layers, layerContentResponse(record))
	}
	return layer.ListAssetLayerContents200JSONResponse(api.AssetLayerContents{
		AssetId: api.Uuid(serviceUUID(request.AssetId)), Layers: layers,
	}), nil
}

// PreviewAssetView 使用真实合并引擎对启用的层与内联草稿层做不落库合并。
func (s *Server) PreviewAssetView(ctx context.Context, request view.PreviewAssetViewRequestObject) (view.PreviewAssetViewResponseObject, error) {
	if s.layerEdit == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	enabled := make([]uuid.UUID, 0, len(request.Body.EnabledLayerIds))
	for _, id := range request.Body.EnabledLayerIds {
		enabled = append(enabled, serviceUUID(id))
	}
	drafts := make([]service.DraftLayerRecord, 0)
	if request.Body.DraftLayers != nil {
		for _, draft := range *request.Body.DraftLayers {
			var dialect *string
			if draft.Dialect.IsSpecified() && !draft.Dialect.IsNull() {
				value := draft.Dialect.MustGet()
				dialect = &value
			}
			drafts = append(drafts, service.DraftLayerRecord{
				Role: string(draft.Role), Origin: string(draft.Origin), Ord: draft.Ord,
				Dialect: dialect, Content: draft.Content, ContentType: string(draft.ContentType),
			})
		}
	}
	result, err := s.layerEdit.PreviewAssetView(ctx, principal, string(request.TenantSlug), serviceUUID(request.AssetId), service.PreviewAssetViewInput{
		EnabledLayerIDs: enabled, DraftLayers: drafts, ContentType: string(request.Body.ContentType),
	})
	if err != nil {
		return nil, err
	}
	return view.PreviewAssetView200JSONResponse(api.ViewPreviewResult{
		EngineVersion: result.EngineVersion, MediaType: result.MediaType,
		MergedContent: result.MergedContent, Provenance: result.Provenance,
	}), nil
}

// layerHeadResponse 将层头记录投影为 API 形状。
func layerHeadResponse(record service.LayerHeadRecord) api.LayerHead {
	latest := nullable.NewNullNullable[api.Uuid]()
	if record.LatestRevisionID != nil {
		latest = nullable.NewNullableWithValue(api.Uuid(*record.LatestRevisionID))
	}
	effective := nullable.NewNullNullable[api.Uuid]()
	if record.EffectiveRevisionID != nil {
		effective = nullable.NewNullableWithValue(api.Uuid(*record.EffectiveRevisionID))
	}
	candidate := nullable.NewNullNullable[api.Uuid]()
	if record.CandidateRevisionID != nil {
		candidate = nullable.NewNullableWithValue(api.Uuid(*record.CandidateRevisionID))
	}
	return api.LayerHead{
		ScopeType: api.LayerHeadScopeType(record.ScopeType), ScopeKey: record.ScopeKey,
		LatestRevisionId: latest, EffectiveRevisionId: effective, CandidateRevisionId: candidate,
		Generation: int(record.Generation),
	}
}

// layerContentResponse 将层内容记录投影为 API 形状。
func layerContentResponse(record service.LayerContentRecord) api.LayerContent {
	effective := nullable.NewNullNullable[api.LayerRevision]()
	if record.EffectiveRevision != nil {
		effective = nullable.NewNullableWithValue(layerRevisionResponse(*record.EffectiveRevision))
	}
	return api.LayerContent{
		LayerId: api.Uuid(record.LayerID), AssetId: api.Uuid(record.AssetID),
		Role: api.LayerRole(record.Role), Origin: api.LayerOrigin(record.Origin), Ord: record.Ord,
		Content: record.Content, ContentType: api.ContentType(record.ContentType), EffectiveRevision: effective,
	}
}
