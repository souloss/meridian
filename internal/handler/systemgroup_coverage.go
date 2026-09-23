package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	view "github.com/meridian-labs/meridian/internal/generated/api/view"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListSystemGroups 列出租户内全部分组并解析其成员。
func (s *Server) ListSystemGroups(ctx context.Context, request view.ListSystemGroupsRequestObject) (view.ListSystemGroupsResponseObject, error) {
	if s.systemGroups == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.systemGroups.ListSystemGroups(ctx, principal, string(request.TenantSlug))
	if err != nil {
		return nil, err
	}
	items := make([]api.SystemGroup, 0, len(records))
	for _, record := range records {
		items = append(items, systemGroupResponse(record))
	}
	return view.ListSystemGroups200JSONResponse(api.SystemGroupList{Items: items}), nil
}

// GetSystemGroup 返回一个系统分组及其成员服务。
func (s *Server) GetSystemGroup(ctx context.Context, request view.GetSystemGroupRequestObject) (view.GetSystemGroupResponseObject, error) {
	if s.systemGroups == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.systemGroups.GetSystemGroup(ctx, principal, string(request.TenantSlug), serviceUUID(request.GroupId))
	if err != nil {
		return nil, err
	}
	body := systemGroupResponse(record)
	return view.GetSystemGroup200JSONResponse{
		Body: body, Headers: view.GetSystemGroup200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// UpdateSystemGroup 在 If-Match 下更新一个系统分组的展示名与描述。
func (s *Server) UpdateSystemGroup(ctx context.Context, request view.UpdateSystemGroupRequestObject) (view.UpdateSystemGroupResponseObject, error) {
	if s.systemGroups == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	patch := service.SystemGroupPatchInput{DisplayName: request.Body.DisplayName}
	if request.Body.Description.IsSpecified() {
		value := request.Body.Description.MustGet()
		patch.Description = &value
		patch.SetDescription = true
	}
	record, err := s.systemGroups.UpdateSystemGroup(ctx, principal, string(request.TenantSlug), serviceUUID(request.GroupId), request.Params.IfMatch, patch)
	if err != nil {
		return nil, err
	}
	body := systemGroupResponse(record)
	return view.UpdateSystemGroup200JSONResponse{
		Body: body, Headers: view.UpdateSystemGroup200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// DeleteSystemGroup 在 If-Match 下删除一个系统分组。
func (s *Server) DeleteSystemGroup(ctx context.Context, request view.DeleteSystemGroupRequestObject) (view.DeleteSystemGroupResponseObject, error) {
	if s.systemGroups == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.systemGroups.DeleteSystemGroup(ctx, principal, string(request.TenantSlug), serviceUUID(request.GroupId), request.Params.IfMatch); err != nil {
		return nil, err
	}
	return view.DeleteSystemGroup204Response{}, nil
}

// ResolvePublicView 解析匿名公开视图。
func (s *Server) ResolvePublicView(ctx context.Context, request view.ResolvePublicViewRequestObject) (view.ResolvePublicViewResponseObject, error) {
	if s.views == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	// 公开视图请求体不含 serviceSlug；按 tenantSlug + 公开资产语义解析。
	var options map[string]any
	if request.Body.Options != nil {
		options = *request.Body.Options
	}
	resolution, err := s.views.ResolvePublicView(ctx, string(request.TenantSlug), "", string(request.Body.Kind), string(request.Body.AssetName), string(request.Body.ViewId), options)
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
	default:
		if err := body.FromDashboardViewResolution(api.DashboardViewResolution{
			Metrics: map[string]any{}, Kind: api.DashboardViewResolutionKind("dashboard"), View: viewDefinitionResponse(resolution.View),
		}); err != nil {
			return nil, err
		}
	}
	return view.ResolvePublicView200JSONResponse(body), nil
}

// CreateShareLink 冻结视图选择器并铸造一个签名匿名分享令牌。
func (s *Server) CreateShareLink(ctx context.Context, request view.CreateShareLinkRequestObject) (view.CreateShareLinkResponseObject, error) {
	if s.diffService == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	inputs := make([]any, 0)
	if request.Body.Inputs != nil {
		for _, selector := range *request.Body.Inputs {
			inputs = append(inputs, selector)
		}
	}
	var options map[string]any
	if request.Body.Options != nil {
		options = *request.Body.Options
	}
	created, err := s.diffService.CreateShareLink(ctx, principal, string(request.TenantSlug), service.NewViewShareLink{
		ViewID: request.Body.ViewId, Inputs: inputs, Options: options, ExpiresInSeconds: request.Body.ExpiresInSeconds,
	})
	if err != nil {
		return nil, err
	}
	return view.CreateShareLink201JSONResponse(shareLinkCreatedResponse(created)), nil
}

var _ = nullable.NewNullNullable[api.Uuid]()
