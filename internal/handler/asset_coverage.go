package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	asset "github.com/meridian-labs/meridian/internal/generated/api/asset"
	"github.com/meridian-labs/meridian/internal/service"
)

// GetPublicAsset 返回一个按可见性门控的匿名公开资产视图。
func (s *Server) GetPublicAsset(ctx context.Context, request asset.GetPublicAssetRequestObject) (asset.GetPublicAssetResponseObject, error) {
	if s.assetService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	record, err := s.assetService.GetPublicAsset(ctx, string(request.TenantSlug), string(request.ServiceSlug), string(request.KindId), string(request.AssetName))
	if err != nil {
		return nil, err
	}
	current := api.VersionRef{}
	if record.CurrentVersion != nil {
		current = api.VersionRef{Id: api.Uuid(record.CurrentVersion.ID), Version: record.CurrentVersion.Version, Lifecycle: api.Lifecycle(record.Lifecycle)}
	}
	return asset.GetPublicAsset200JSONResponse(api.PublicAsset{
		Kind: api.KindId(record.Kind), Name: api.AssetName(record.Name), Lifecycle: api.Lifecycle(record.Lifecycle),
		CurrentVersion: current, ContentUrl: record.ContentRef, UpdatedAt: record.UpdatedAt,
	}), nil
}

// ListAssetVersions 返回某资产一页版本历史。
func (s *Server) ListAssetVersions(ctx context.Context, request asset.ListAssetVersionsRequestObject) (asset.ListAssetVersionsResponseObject, error) {
	if s.assetService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.assetService.ListAssetVersions(ctx, principal, string(request.TenantSlug), serviceUUID(request.AssetId), page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.AssetVersion, 0, len(records))
	for _, record := range records {
		items = append(items, assetVersionResponse(record))
	}
	return asset.ListAssetVersions200JSONResponse(api.AssetVersionPage{
		Total: int(total), Page: page, PageSize: pageSize, Items: items,
	}), nil
}

// DeprecateAssetVersion 在 If-Match 下将一个已发布版本置为 deprecated。
func (s *Server) DeprecateAssetVersion(ctx context.Context, request asset.DeprecateAssetVersionRequestObject) (asset.DeprecateAssetVersionResponseObject, error) {
	if s.assetService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.assetService.DeprecateAssetVersion(ctx, principal, string(request.TenantSlug), serviceUUID(request.VersionId), request.Params.IfMatch)
	if err != nil {
		return nil, err
	}
	body := assetVersionResponse(record)
	return asset.DeprecateAssetVersion200JSONResponse{
		Body: body, Headers: asset.DeprecateAssetVersion200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// RetireAssetVersion 在 If-Match 下将一个已发布版本置为 retired。
func (s *Server) RetireAssetVersion(ctx context.Context, request asset.RetireAssetVersionRequestObject) (asset.RetireAssetVersionResponseObject, error) {
	if s.assetService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.assetService.RetireAssetVersion(ctx, principal, string(request.TenantSlug), serviceUUID(request.VersionId), request.Params.IfMatch)
	if err != nil {
		return nil, err
	}
	body := assetVersionResponse(record)
	return asset.RetireAssetVersion200JSONResponse{
		Body: body, Headers: asset.RetireAssetVersion200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// DeleteSourceSpec 在 If-Match 下软删除一条源配置。
func (s *Server) DeleteSourceSpec(ctx context.Context, request asset.DeleteSourceSpecRequestObject) (asset.DeleteSourceSpecResponseObject, error) {
	if s.discovery == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.discovery.DeleteSourceSpec(ctx, principal, string(request.TenantSlug), serviceUUID(request.SourceId), request.Params.IfMatch); err != nil {
		return nil, err
	}
	return asset.DeleteSourceSpec204Response{}, nil
}

// ProduceSource 为一次源物化请求入队一条 asset.produce 任务。
func (s *Server) ProduceSource(ctx context.Context, request asset.ProduceSourceRequestObject) (asset.ProduceSourceResponseObject, error) {
	if s.discovery == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refType := ""
	refName := ""
	force := false
	if request.Body.RefType != nil {
		refType = string(*request.Body.RefType)
	}
	if request.Body.Ref != nil {
		refName = string(*request.Body.Ref)
	}
	if request.Body.Force != nil {
		force = *request.Body.Force
	}
	accepted, err := s.discovery.ProduceSource(ctx, principal, string(request.TenantSlug), serviceUUID(request.SourceId),
		serviceUUID(api.Uuid(request.Params.IdempotencyKey)), refType, refName, force)
	if err != nil {
		return nil, err
	}
	return asset.ProduceSource202JSONResponse(api.JobAccepted{JobId: api.Uuid(accepted.JobID), Deduplicated: accepted.Deduplicated}), nil
}

// GenerateAssetWithAi 为已存在的资产入队一次 AI 生成。
func (s *Server) GenerateAssetWithAi(ctx context.Context, request asset.GenerateAssetWithAiRequestObject) (asset.GenerateAssetWithAiResponseObject, error) {
	if s.aiWorkflow == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	// 资产类别从资产反查，此处仅传递引用与提示。
	refType := ""
	ref := ""
	hint := ""
	if request.Body.RefType != nil {
		refType = string(*request.Body.RefType)
	}
	if request.Body.Ref != nil {
		ref = string(*request.Body.Ref)
	}
	if request.Body.Hint.IsSpecified() && !request.Body.Hint.IsNull() {
		hint = request.Body.Hint.MustGet()
	}
	// 请求摘要用于幂等重放比较；idempotency_records.request_hash 非空约束要求其必填。
	requestHash, err := service.RequestDigest(operationGenerateAssetWithAi, map[string]any{
		"tenantSlug": string(request.TenantSlug), "assetId": request.AssetId.String(),
	}, map[string]any{}, request.Body)
	if err != nil {
		return nil, err
	}
	genInput := service.AiGenerateInput{
		RefType: refType, Ref: ref, Hint: hint,
		IdempotencyKey: serviceUUID(api.Uuid(request.Params.IdempotencyKey)),
		PrincipalType:  string(principal.Kind), PrincipalID: rotationPrincipalID(principal),
		RequestHash: requestHash,
	}
	if request.Body.ProducerProfileId != nil {
		genInput.ProducerProfileID = serviceUUID(*request.Body.ProducerProfileId)
	}
	accepted, err := s.aiWorkflow.GenerateAssetWithAi(ctx, principal, string(request.TenantSlug), serviceUUID(request.AssetId), genInput)
	if err != nil {
		return nil, err
	}
	return asset.GenerateAssetWithAi202JSONResponse(api.AiGenerationAccepted{
		AssetId: api.Uuid(accepted.AssetID), SourceId: api.Uuid(accepted.SourceID),
		JobId: api.Uuid(accepted.JobID), Deduplicated: accepted.Deduplicated,
	}), nil
}

const (
	// operationGenerateAssetWithAi 是资产生成幂等摘要的操作名。
	operationGenerateAssetWithAi = "generateAssetWithAi"
)
