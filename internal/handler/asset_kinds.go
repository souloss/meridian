package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	asset "github.com/meridian-labs/meridian/internal/generated/api/asset"
	"github.com/meridian-labs/meridian/internal/service"
)

// ListAssetKinds 返回租户视角的全部资产类别。
func (s *Server) ListAssetKinds(ctx context.Context, request asset.ListAssetKindsRequestObject) (asset.ListAssetKindsResponseObject, error) {
	if s.assetKinds == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	options, err := s.assetKinds.List(ctx, principal, string(request.TenantSlug))
	if err != nil {
		return nil, err
	}
	items := make([]api.AssetKind, 0, len(options))
	for _, option := range options {
		items = append(items, assetKindResponse(option))
	}
	return asset.ListAssetKinds200JSONResponse(api.AssetKindList{Items: items}), nil
}

// UpdateAssetKindState 在 If-Match 下更新一个租户级资产类别开关。
func (s *Server) UpdateAssetKindState(ctx context.Context, request asset.UpdateAssetKindStateRequestObject) (asset.UpdateAssetKindStateResponseObject, error) {
	if s.assetKinds == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	option, err := s.assetKinds.UpdateState(ctx, principal, string(request.TenantSlug), string(request.KindId), request.Params.IfMatch, request.Body.Enabled)
	if err != nil {
		return nil, err
	}
	body := assetKindResponse(option)
	return asset.UpdateAssetKindState200JSONResponse{
		Body: body, Headers: asset.UpdateAssetKindState200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// assetKindResponse 将资产类别选项投影为 API 形状。
func assetKindResponse(option service.AssetKindOption) api.AssetKind {
	schemaVersion := 1
	if parsed, err := parseInt(option.ContractVersion); err == nil {
		schemaVersion = parsed
	}
	return api.AssetKind{
		Id: api.KindId(option.ID), Enabled: option.Enabled, Milestone: assetKindMilestone(option.ID),
		SchemaVersion:      schemaVersion,
		Etag:               revisionETag(etagKindAssetKind, option.ID, option.Revision),
		AcceptedMediaTypes: []string{}, DefaultViews: []string{}, Capabilities: api.CapabilityList{},
	}
}

// parseInt 解析非负整数（用于 contract_version 到 schemaVersion 的投影）。
func parseInt(value string) (int, error) {
	var result int
	if value == "" {
		return 0, service.ErrValidation
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, service.ErrValidation
		}
		result = result*10 + int(character-'0')
	}
	return result, nil
}

// assetKindMilestone 返回类别已知的里程碑标签（未知类别回退为空）。
func assetKindMilestone(kind string) string {
	switch kind {
	case "openapi":
		return "M1"
	case "dbschema", "dependency":
		return "M4"
	case "asyncapi":
		return "M5"
	default:
		return ""
	}
}

// etagKindAssetKind 是资产类别 ETag 的实体类型令牌。
const etagKindAssetKind = "asset-kind"
