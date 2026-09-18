package handler

import (
	"context"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	asset "github.com/meridian-labs/meridian/internal/generated/api/asset"
	layer "github.com/meridian-labs/meridian/internal/generated/api/layer"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// PreviewMerge 在资产的层集合上运行合并引擎（不持久化）。
func (s *Server) PreviewMerge(ctx context.Context, request asset.PreviewMergeRequestObject) (asset.PreviewMergeResponseObject, error) {
	if s.layerEdit == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refType := ""
	if request.Body.RefType != nil {
		refType = string(*request.Body.RefType)
	}
	selectors := make([]service.MergeLayerSelector, 0)
	if request.Body.Layers != nil {
		for _, entry := range *request.Body.Layers {
			selectors = append(selectors, service.MergeLayerSelector{
				LayerID: serviceUUID(entry.LayerId), RevisionID: serviceUUID(entry.RevisionId),
			})
		}
	}
	result, err := s.layerEdit.PreviewMerge(ctx, principal, string(request.TenantSlug), service.MergePreviewInput{
		AssetID: serviceUUID(request.Body.AssetId), RefType: refType, RefName: string(request.Body.Ref),
		Layers: selectors,
	})
	if err != nil {
		return nil, err
	}
	return asset.PreviewMerge200JSONResponse(mergePreviewResponse(result)), nil
}

// CreateLayerRevision 校验并持久化一次手动 overlay 修订。
func (s *Server) CreateLayerRevision(ctx context.Context, request layer.CreateLayerRevisionRequestObject) (layer.CreateLayerRevisionResponseObject, error) {
	if s.layerEdit == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	submitForReview := true
	if request.Body.SubmitForReview != nil {
		submitForReview = *request.Body.SubmitForReview
	}
	var dialect *string
	if request.Body.Dialect.IsSpecified() && !request.Body.Dialect.IsNull() {
		dialect = new(request.Body.Dialect.MustGet())
	}
	result, err := s.layerEdit.CreateLayerRevision(ctx, principal, string(request.TenantSlug), serviceUUID(request.LayerId), service.LayerRevisionInput{
		ScopeType:       string(request.Body.ScopeType),
		ScopeKey:        request.Body.ScopeKey,
		Content:         request.Body.Content,
		ContentType:     string(request.Body.ContentType),
		Dialect:         dialect,
		SubmitForReview: submitForReview,
	})
	if err != nil {
		return nil, err
	}
	body := api.LayerRevisionSubmission{
		Revision:     layerRevisionResponse(result.Revision),
		JobId:        api.Uuid(result.JobID),
		Deduplicated: result.Deduplicated,
	}
	return layer.CreateLayerRevision201JSONResponse(body), nil
}

// GetAssetVersionProvenance 返回一个版本的最后写入来源信息。
func (s *Server) GetAssetVersionProvenance(ctx context.Context, request asset.GetAssetVersionProvenanceRequestObject) (asset.GetAssetVersionProvenanceResponseObject, error) {
	if s.layerEdit == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.layerEdit.GetAssetVersionProvenance(ctx, principal, string(request.TenantSlug), serviceUUID(request.VersionId))
	if err != nil {
		return nil, err
	}
	items := make([]api.ProvenanceEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, api.ProvenanceEntry{
			Pointer: entry.Pointer, LayerId: api.Uuid(entry.LayerID), RevisionId: api.Uuid(entry.RevisionID),
		})
	}
	return asset.GetAssetVersionProvenance200JSONResponse(api.ProvenanceList{Items: items}), nil
}

// ReorderAssetLayers 在乐观并发下持久化新的 overlay 顺序。
func (s *Server) ReorderAssetLayers(ctx context.Context, request layer.ReorderAssetLayersRequestObject) (layer.ReorderAssetLayersResponseObject, error) {
	if s.layerEdit == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	layerIDs := make([]uuid.UUID, 0, len(request.Body.LayerIds))
	for _, id := range request.Body.LayerIds {
		layerIDs = append(layerIDs, serviceUUID(id))
	}
	records, err := s.layerEdit.ReorderAssetLayers(ctx, principal, string(request.TenantSlug), serviceUUID(request.AssetId), request.Params.IfMatch, layerIDs)
	if err != nil {
		return nil, err
	}
	items := make([]api.Layer, 0, len(records))
	var maxRevision int64
	for _, record := range records {
		items = append(items, layerResponse(record))
		if record.Revision > maxRevision {
			maxRevision = record.Revision
		}
	}
	etag := revisionETag(etagKindAssetLayers, serviceUUID(request.AssetId).String(), maxRevision)
	return layer.ReorderAssetLayers200JSONResponse{
		Body:    api.LayerList{Items: items, Etag: api.ETag(etag)},
		Headers: layer.ReorderAssetLayers200ResponseHeaders{Etag: new(api.ETag(etag))},
	}, nil
}

// RollbackLayer 将层的有效头回退到一个历史修订。
func (s *Server) RollbackLayer(ctx context.Context, request layer.RollbackLayerRequestObject) (layer.RollbackLayerResponseObject, error) {
	if s.layerEdit == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var expected *uuid.UUID
	if request.Body.ExpectedEffectiveRevisionId.IsSpecified() && !request.Body.ExpectedEffectiveRevisionId.IsNull() {
		expected = new(serviceUUID(request.Body.ExpectedEffectiveRevisionId.MustGet()))
	}
	accepted, err := s.layerEdit.RollbackLayer(ctx, principal, string(request.TenantSlug), serviceUUID(request.LayerId), serviceUUID(api.Uuid(request.Params.IdempotencyKey)), service.LayerRollbackInput{
		ScopeType:                   string(request.Body.ScopeType),
		ScopeKey:                    request.Body.ScopeKey,
		ExpectedEffectiveRevisionID: expected,
		TargetRevisionID:            serviceUUID(request.Body.TargetRevisionId),
	})
	if err != nil {
		return nil, err
	}
	return layer.RollbackLayer202JSONResponse(api.JobAccepted{
		JobId: api.Uuid(accepted.JobID), Deduplicated: accepted.Deduplicated,
	}), nil
}

// mergePreviewResponse 将合并预览结果投影为 API 形状。
func mergePreviewResponse(result service.MergePreviewResult) api.MergePreview {
	issues := make([]api.ValidationIssue, 0, len(result.Validation))
	for _, issue := range result.Validation {
		pointer := nullable.NewNullNullable[string]()
		if issue.Pointer != nil {
			pointer = nullable.NewNullableWithValue(*issue.Pointer)
		}
		issues = append(issues, api.ValidationIssue{
			Severity: api.ValidationIssueSeverity(issue.Severity), Code: issue.Code, Message: issue.Message, Pointer: pointer,
		})
	}
	provenance := make([]api.ProvenanceEntry, 0, len(result.Provenance))
	for _, entry := range result.Provenance {
		provenance = append(provenance, api.ProvenanceEntry{
			Pointer: entry.Pointer, LayerId: api.Uuid(entry.LayerID), RevisionId: api.Uuid(entry.RevisionID),
		})
	}
	return api.MergePreview{
		InputFingerprint: result.InputFingerprint, Content: result.Content,
		ContentType: api.ContentType(result.ContentType), Validation: issues, Provenance: provenance,
	}
}

// layerRevisionResponse 将层修订记录投影为 API 形状。
func layerRevisionResponse(record service.LayerRevisionRecord) api.LayerRevision {
	dialect := nullable.NewNullNullable[string]()
	if record.Dialect != nil {
		dialect = nullable.NewNullableWithValue(*record.Dialect)
	}
	sourceBranch := nullable.NewNullNullable[string]()
	if record.SourceBranch != nil {
		sourceBranch = nullable.NewNullableWithValue(*record.SourceBranch)
	}
	gitCommit := nullable.NewNullNullable[string]()
	if record.GitCommit != nil {
		gitCommit = nullable.NewNullableWithValue(*record.GitCommit)
	}
	createdBy := nullable.NewNullNullable[api.Uuid]()
	aiMeta := nullable.NewNullNullable[map[string]any]()
	reviewComment := nullable.NewNullNullable[string]()
	return api.LayerRevision{
		Id: api.Uuid(record.ID), LayerId: api.Uuid(record.LayerID),
		ScopeType: api.LayerRevisionScopeType(record.ScopeType), ScopeKey: record.ScopeKey,
		ContentHash: record.ContentHash, ContentType: api.ContentType(record.ContentType),
		Dialect: dialect, SourceBranch: sourceBranch, GitCommit: gitCommit, CreatedBy: createdBy, AiMeta: aiMeta,
		ReviewStatus: api.RevisionStatus(record.ReviewStatus), ReviewComment: reviewComment, CreatedAt: record.CreatedAt,
	}
}

// layerResponse 将层记录投影为 API 形状。
func layerResponse(record service.LayerRecord) api.Layer {
	sourceSpec := nullable.NewNullNullable[api.Uuid]()
	if record.SourceSpecID != nil {
		sourceSpec = nullable.NewNullableWithValue(api.Uuid(*record.SourceSpecID))
	}
	dialect := nullable.NewNullNullable[string]()
	if record.Dialect != nil {
		dialect = nullable.NewNullableWithValue(*record.Dialect)
	}
	return api.Layer{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKindLayer, record.ID.String(), record.Revision),
		AssetId: api.Uuid(record.AssetID), SourceSpecId: sourceSpec,
		Role: api.LayerRole(record.Role), Origin: api.LayerOrigin(record.Origin), Ord: record.Ord,
		Dialect: dialect, Enabled: record.Enabled, Heads: []api.LayerHead{}, Capabilities: api.CapabilityList{},
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

const (
	// etagKindLayer 是层 ETag 的实体类型令牌。
	etagKindLayer = "layer"
	// etagKindAssetLayers 是资产层排序 ETag 的实体类型令牌。
	etagKindAssetLayers = "asset-layers"
)
