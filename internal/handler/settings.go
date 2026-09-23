package handler

import (
	"context"
	"encoding/json/v2"

	"github.com/meridian-labs/meridian/internal/generated/api"
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
	tenant "github.com/meridian-labs/meridian/internal/generated/api/tenant"
	"github.com/meridian-labs/meridian/internal/service"
)

// GetPlatformSettings 返回平台默认配置快照。
func (s *Server) GetPlatformSettings(ctx context.Context, _ platform.GetPlatformSettingsRequestObject) (platform.GetPlatformSettingsResponseObject, error) {
	if s.settings == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	value, err := s.settings.GetPlatformSettings(ctx, principal)
	if err != nil {
		return nil, err
	}
	body, err := platformSettingsResponse(value)
	if err != nil {
		return nil, err
	}
	etag := api.ETag(settingsETag(etagKindPlatformSettings, value.Revision))
	return platform.GetPlatformSettings200JSONResponse{
		Body: body, Headers: platform.GetPlatformSettings200ResponseHeaders{Etag: &etag},
	}, nil
}

// UpdatePlatformSettings 在 If-Match 下对平台默认配置应用顶层浅合并补丁。
func (s *Server) UpdatePlatformSettings(ctx context.Context, request platform.UpdatePlatformSettingsRequestObject) (platform.UpdatePlatformSettingsResponseObject, error) {
	if s.settings == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	patch, err := json.Marshal(request.Body)
	if err != nil {
		return nil, err
	}
	value, err := s.settings.UpdatePlatformSettings(ctx, principal, request.Params.IfMatch, patch)
	if err != nil {
		return nil, err
	}
	body, err := platformSettingsResponse(value)
	if err != nil {
		return nil, err
	}
	etag := api.ETag(settingsETag(etagKindPlatformSettings, value.Revision))
	return platform.UpdatePlatformSettings200JSONResponse{
		Body: body, Headers: platform.UpdatePlatformSettings200ResponseHeaders{Etag: &etag},
	}, nil
}

// GetTenantSettings 返回租户运行设置快照。
func (s *Server) GetTenantSettings(ctx context.Context, request tenant.GetTenantSettingsRequestObject) (tenant.GetTenantSettingsResponseObject, error) {
	if s.settings == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	value, err := s.settings.GetTenantSettings(ctx, principal, string(request.TenantSlug))
	if err != nil {
		return nil, err
	}
	body, err := tenantSettingsResponse(value)
	if err != nil {
		return nil, err
	}
	etag := api.ETag(settingsETag(etagKindTenantSettings, value.Revision))
	return tenant.GetTenantSettings200JSONResponse{
		Body: body, Headers: tenant.GetTenantSettings200ResponseHeaders{Etag: &etag},
	}, nil
}

// UpdateTenantSettings 在 If-Match 下对租户设置应用顶层浅合并补丁。
func (s *Server) UpdateTenantSettings(ctx context.Context, request tenant.UpdateTenantSettingsRequestObject) (tenant.UpdateTenantSettingsResponseObject, error) {
	if s.settings == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	patch, err := json.Marshal(request.Body)
	if err != nil {
		return nil, err
	}
	value, err := s.settings.UpdateTenantSettings(ctx, principal, string(request.TenantSlug), request.Params.IfMatch, patch)
	if err != nil {
		return nil, err
	}
	body, err := tenantSettingsResponse(value)
	if err != nil {
		return nil, err
	}
	etag := api.ETag(settingsETag(etagKindTenantSettings, value.Revision))
	return tenant.UpdateTenantSettings200JSONResponse{
		Body: body, Headers: tenant.UpdateTenantSettings200ResponseHeaders{Etag: &etag},
	}, nil
}

// settingsETag 格式化文本 id 实体的 ETag（平台/租户设置单例）。
func settingsETag(kind string, revision int64) string {
	return revisionETag(kind, "default", revision)
}

// platformSettingsResponse 将平台设置值投影为 API 形状。
func platformSettingsResponse(value service.PlatformSettingsValue) (api.PlatformSettings, error) {
	var body api.PlatformSettings
	if err := json.Unmarshal(value.Settings, &body); err != nil {
		return api.PlatformSettings{}, err
	}
	body.Etag = api.ETag(settingsETag(etagKindPlatformSettings, value.Revision))
	return body, nil
}

// tenantSettingsResponse 将租户设置值投影为 API 形状。
func tenantSettingsResponse(value service.TenantSettingsValue) (api.TenantSettings, error) {
	var body api.TenantSettings
	if err := json.Unmarshal(value.Settings, &body); err != nil {
		return api.TenantSettings{}, err
	}
	body.Etag = api.ETag(settingsETag(etagKindTenantSettings, value.Revision))
	return body, nil
}

const (
	// etagKindPlatformSettings 是平台设置 ETag 的实体类型令牌。
	etagKindPlatformSettings = "platform-settings"
	// etagKindTenantSettings 是租户设置 ETag 的实体类型令牌。
	etagKindTenantSettings = "tenant-settings"
)
