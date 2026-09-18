package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	asset "github.com/meridian-labs/meridian/internal/generated/api/asset"
	"github.com/meridian-labs/meridian/internal/service"
)

// PushAssetRevision 将第三方推送的修订作为待审核候选接收。
func (s *Server) PushAssetRevision(ctx context.Context, request asset.PushAssetRevisionRequestObject) (asset.PushAssetRevisionResponseObject, error) {
	if s.diffService == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refType := string(api.RefTypeBranch)
	if request.Body.RefType != nil {
		refType = string(*request.Body.RefType)
	}
	role := string(api.Base)
	if request.Body.Role != nil {
		role = string(*request.Body.Role)
	}
	var dialect *string
	if request.Body.Dialect.IsSpecified() && !request.Body.Dialect.IsNull() {
		value := request.Body.Dialect.MustGet()
		dialect = &value
	}
	var sourceCommit *string
	if request.Body.SourceCommit.IsSpecified() && !request.Body.SourceCommit.IsNull() {
		value := request.Body.SourceCommit.MustGet()
		sourceCommit = &value
	}
	requestHash, err := service.RequestDigest(operationPushAssetRevision, map[string]any{
		"tenantSlug": string(request.TenantSlug),
	}, map[string]any{}, request.Body)
	if err != nil {
		return nil, err
	}
	result, err := s.diffService.PushAssetRevision(ctx, principal, string(request.TenantSlug), service.PushRevisionInput{
		ServiceSlug: string(request.Body.ServiceSlug), Kind: string(request.Body.Kind), Name: string(request.Body.Name),
		RefType: refType, Ref: string(request.Body.Ref), SourceSystem: request.Body.SourceSystem,
		CreateIfMissing: request.Body.CreateIfMissing != nil && *request.Body.CreateIfMissing,
		Content:         request.Body.Content, ContentType: string(request.Body.ContentType),
		Role: role, Dialect: dialect, SourceCommit: sourceCommit,
		IdempotencyKey: serviceUUID(api.Uuid(request.Params.IdempotencyKey)),
		PrincipalType:  string(principal.Kind), PrincipalID: rotationPrincipalID(principal), RequestHash: requestHash,
	})
	if err != nil {
		return nil, err
	}
	body := api.AssetPushResult{
		AssetId: api.Uuid(result.AssetID), LayerId: api.Uuid(result.LayerID),
		RevisionId: api.Uuid(result.RevisionID), JobId: api.Uuid(result.JobID), Deduplicated: result.Deduplicated,
	}
	return asset.PushAssetRevision200JSONResponse(body), nil
}

const (
	// operationPushAssetRevision 是资产修订推送的幂等摘要操作名。
	operationPushAssetRevision = "pushAssetRevision"
)
