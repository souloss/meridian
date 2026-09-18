package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	serviceapi "github.com/meridian-labs/meridian/internal/generated/api/service"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// UpdateService 在其修订号下应用经过校验的服务补丁。
func (s *Server) UpdateService(ctx context.Context, request serviceapi.UpdateServiceRequestObject) (serviceapi.UpdateServiceResponseObject, error) {
	if s.serviceLifecycle == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.serviceLifecycle.Update(
		ctx, principal, string(request.TenantSlug), string(request.ServiceSlug),
		string(request.Params.IfMatch), servicePatchInput(request.Body),
	)
	if err != nil {
		return nil, err
	}
	body := serviceResponse(record)
	return serviceapi.UpdateService200JSONResponse{
		Body: body, Headers: serviceapi.UpdateService200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// DeleteService 在其修订号下软删除一个服务。
func (s *Server) DeleteService(ctx context.Context, request serviceapi.DeleteServiceRequestObject) (serviceapi.DeleteServiceResponseObject, error) {
	if s.serviceLifecycle == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.serviceLifecycle.Delete(ctx, principal, string(request.TenantSlug), string(request.ServiceSlug), string(request.Params.IfMatch)); err != nil {
		return nil, err
	}
	return serviceapi.DeleteService204Response{}, nil
}

// GetPublicService 返回一个按可见性门控的匿名公共服务视图。
func (s *Server) GetPublicService(ctx context.Context, request serviceapi.GetPublicServiceRequestObject) (serviceapi.GetPublicServiceResponseObject, error) {
	if s.serviceLifecycle == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	record, err := s.serviceLifecycle.GetPublic(ctx, string(request.TenantSlug), string(request.ServiceSlug))
	if err != nil {
		return nil, err
	}
	return serviceapi.GetPublicService200JSONResponse(publicServiceResponse(record)), nil
}

// servicePatchInput 将服务补丁请求投影为服务层输入。
func servicePatchInput(body *api.ServicePatchRequest) service.ServicePatchInput {
	input := service.ServicePatchInput{}
	if body.DisplayName != nil {
		input.DisplayName = body.DisplayName
	}
	if body.Description.IsSpecified() {
		value := body.Description.MustGet()
		input.Description = &value
	}
	if body.Visibility != nil {
		value := string(*body.Visibility)
		input.Visibility = &value
	}
	if body.Lifecycle != nil {
		value := string(*body.Lifecycle)
		input.Lifecycle = &value
	}
	return input
}

// publicServiceResponse 将公共服务记录投影为 API 形状。
func publicServiceResponse(record service.PublicServiceRecord) api.PublicService {
	description := nullable.NewNullNullable[string]()
	if record.Description != nil {
		description = nullable.NewNullableWithValue(*record.Description)
	}
	assets := make([]api.AssetSummary, 0, len(record.Assets))
	for _, summary := range record.Assets {
		current := nullable.NewNullNullable[api.VersionRef]()
		if summary.CurrentVersion != nil {
			current = nullable.NewNullableWithValue(api.VersionRef{Id: api.Uuid(summary.CurrentVersion.ID), Version: summary.CurrentVersion.Version, Lifecycle: api.Lifecycle(summary.CurrentVersion.Lifecycle)})
		}
		latest := nullable.NewNullNullable[api.VersionRef]()
		if summary.LatestVersion != nil {
			latest = nullable.NewNullableWithValue(api.VersionRef{Id: api.Uuid(summary.LatestVersion.ID), Version: summary.LatestVersion.Version, Lifecycle: api.Lifecycle(summary.LatestVersion.Lifecycle)})
		}
		assets = append(assets, api.AssetSummary{
			Id: api.Uuid(summary.ID), Kind: api.KindId(summary.Kind), Name: api.AssetName(summary.Name),
			Lifecycle: api.Lifecycle(summary.Lifecycle), Health: api.AssetSummaryHealth(summary.Health),
			CurrentVersion: current, LatestVersion: latest, RefType: api.RefType(summary.RefType), Ref: api.RefName(summary.RefName),
		})
	}
	return api.PublicService{
		Slug: api.Slug(record.Slug), DisplayName: record.DisplayName, Description: description,
		Lifecycle: api.Lifecycle(record.Lifecycle), Tags: append([]string(nil), record.Tags...),
		Assets: assets, UpdatedAt: record.UpdatedAt,
	}
}
