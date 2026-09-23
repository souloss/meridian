package handler

import (
	"context"
	"encoding/json/v2"

	"github.com/meridian-labs/meridian/internal/generated/api"
	auth "github.com/meridian-labs/meridian/internal/generated/api/auth"
	collaboration "github.com/meridian-labs/meridian/internal/generated/api/collaboration"
	serviceapi "github.com/meridian-labs/meridian/internal/generated/api/service"
	tenant "github.com/meridian-labs/meridian/internal/generated/api/tenant"
	view "github.com/meridian-labs/meridian/internal/generated/api/view"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// StarService 收藏一个服务。
func (s *Server) StarService(ctx context.Context, request serviceapi.StarServiceRequestObject) (serviceapi.StarServiceResponseObject, error) {
	if s.coverage == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	starred, err := s.coverage.StarService(ctx, principal, string(request.TenantSlug), string(request.ServiceSlug))
	if err != nil {
		return nil, err
	}
	return serviceapi.StarService200JSONResponse(api.StarState{Starred: starred}), nil
}

// UnstarService 取消收藏一个服务。
func (s *Server) UnstarService(ctx context.Context, request serviceapi.UnstarServiceRequestObject) (serviceapi.UnstarServiceResponseObject, error) {
	if s.coverage == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	starred, err := s.coverage.UnstarService(ctx, principal, string(request.TenantSlug), string(request.ServiceSlug))
	if err != nil {
		return nil, err
	}
	return serviceapi.UnstarService200JSONResponse(api.StarState{Starred: starred}), nil
}

// GetMyPreferences 返回当前用户的偏好。
func (s *Server) GetMyPreferences(ctx context.Context, _ auth.GetMyPreferencesRequestObject) (auth.GetMyPreferencesResponseObject, error) {
	if s.coverage == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.coverage.GetMyPreferences(ctx, principal)
	if err != nil {
		return nil, err
	}
	body := userPreferencesResponse(record)
	return auth.GetMyPreferences200JSONResponse{
		Body: body, Headers: auth.GetMyPreferences200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// UpdateMyPreferences 在 If-Match 下更新当前用户的偏好。
func (s *Server) UpdateMyPreferences(ctx context.Context, request auth.UpdateMyPreferencesRequestObject) (auth.UpdateMyPreferencesResponseObject, error) {
	if s.coverage == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var defaultViews []byte
	if request.Body.DefaultViews != nil {
		encoded, err := json.Marshal(*request.Body.DefaultViews)
		if err != nil {
			return nil, err
		}
		defaultViews = encoded
	}
	record, err := s.coverage.UpdateMyPreferences(ctx, principal, request.Params.IfMatch, service.UserPreferencesPatch{
		Locale:       localePointer(request.Body.Locale),
		Theme:        themePointer(request.Body.Theme),
		DefaultViews: defaultViews,
	})
	if err != nil {
		return nil, err
	}
	body := userPreferencesResponse(record)
	return auth.UpdateMyPreferences200JSONResponse{
		Body: body, Headers: auth.UpdateMyPreferences200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// ListViewOverrides 返回租户内全部视图覆盖。
func (s *Server) ListViewOverrides(ctx context.Context, request view.ListViewOverridesRequestObject) (view.ListViewOverridesResponseObject, error) {
	if s.coverage == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.coverage.ListViewOverrides(ctx, principal, string(request.TenantSlug))
	if err != nil {
		return nil, err
	}
	items := make([]api.ViewOverride, 0, len(records))
	for _, record := range records {
		items = append(items, viewOverrideResponse(record))
	}
	return view.ListViewOverrides200JSONResponse(api.ViewOverrideList{Items: items}), nil
}

// PutViewOverride 在 If-Match 下创建或替换一条视图覆盖。
func (s *Server) PutViewOverride(ctx context.Context, request view.PutViewOverrideRequestObject) (view.PutViewOverrideResponseObject, error) {
	if s.coverage == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var defaultOptions map[string]any
	if request.Body.DefaultOptions != nil {
		defaultOptions = *request.Body.DefaultOptions
	}
	record, err := s.coverage.PutViewOverride(ctx, principal, string(request.TenantSlug), request.ViewId, request.Params.IfMatch, request.Body.Enabled, defaultOptions)
	if err != nil {
		return nil, err
	}
	body := viewOverrideResponse(record)
	return view.PutViewOverride200JSONResponse{
		Body: body, Headers: view.PutViewOverride200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// DeleteViewOverride 在 If-Match 下删除一条视图覆盖。
func (s *Server) DeleteViewOverride(ctx context.Context, request view.DeleteViewOverrideRequestObject) (view.DeleteViewOverrideResponseObject, error) {
	if s.coverage == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.coverage.DeleteViewOverride(ctx, principal, string(request.TenantSlug), request.ViewId, request.Params.IfMatch); err != nil {
		return nil, err
	}
	return view.DeleteViewOverride204Response{}, nil
}

// ListViews 返回租户可见的全部内置视图定义。
func (s *Server) ListViews(ctx context.Context, request view.ListViewsRequestObject) (view.ListViewsResponseObject, error) {
	if s.views == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	_ = principal
	kindFilter := ""
	if request.Params.Kind != nil {
		kindFilter = string(*request.Params.Kind)
	}
	items := make([]api.ViewDefinition, 0)
	for _, definition := range s.views.Registry() {
		if kindFilter != "" && !viewMatchesKind(definition, kindFilter) {
			continue
		}
		items = append(items, viewDefinitionResponse(definition))
	}
	return view.ListViews200JSONResponse(api.ViewList{Items: items}), nil
}

// ListTags 返回租户内全部标签。
func (s *Server) ListTags(ctx context.Context, request tenant.ListTagsRequestObject) (tenant.ListTagsResponseObject, error) {
	if s.coverage == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.coverage.ListTags(ctx, principal, string(request.TenantSlug))
	if err != nil {
		return nil, err
	}
	items := make([]api.Tag, 0, len(records))
	for _, record := range records {
		items = append(items, tagResponse(record))
	}
	return tenant.ListTags200JSONResponse(items), nil
}

// CreateTag 创建一条标签。
func (s *Server) CreateTag(ctx context.Context, request tenant.CreateTagRequestObject) (tenant.CreateTagResponseObject, error) {
	if s.coverage == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var color *string
	if request.Body.Color.IsSpecified() && !request.Body.Color.IsNull() {
		value := request.Body.Color.MustGet()
		color = &value
	}
	record, err := s.coverage.CreateTag(ctx, principal, string(request.TenantSlug), service.NewTag{
		Name: request.Body.Name, Color: color,
	})
	if err != nil {
		return nil, err
	}
	body := tagResponse(record)
	return tenant.CreateTag201JSONResponse{
		Body: body, Headers: tenant.CreateTag201ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// UpdateTag 在 If-Match 下更新一条标签。
func (s *Server) UpdateTag(ctx context.Context, request tenant.UpdateTagRequestObject) (tenant.UpdateTagResponseObject, error) {
	if s.coverage == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	patch := service.TagPatch{Name: request.Body.Name}
	if request.Body.Color.IsSpecified() {
		value := request.Body.Color.MustGet()
		patch.Color = &value
	}
	record, err := s.coverage.UpdateTag(ctx, principal, string(request.TenantSlug), serviceUUID(request.TagId), request.Params.IfMatch, patch)
	if err != nil {
		return nil, err
	}
	body := tagResponse(record)
	return tenant.UpdateTag200JSONResponse{
		Body: body, Headers: tenant.UpdateTag200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// DeleteTag 在 If-Match 下删除一条标签。
func (s *Server) DeleteTag(ctx context.Context, request tenant.DeleteTagRequestObject) (tenant.DeleteTagResponseObject, error) {
	if s.coverage == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.coverage.DeleteTag(ctx, principal, string(request.TenantSlug), serviceUUID(request.TagId), request.Params.IfMatch); err != nil {
		return nil, err
	}
	return tenant.DeleteTag204Response{}, nil
}

// ListServiceComments 返回一个服务下的评论分页。
func (s *Server) ListServiceComments(ctx context.Context, request collaboration.ListServiceCommentsRequestObject) (collaboration.ListServiceCommentsResponseObject, error) {
	if s.coverage == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.coverage.ListServiceComments(ctx, principal, string(request.TenantSlug), string(request.ServiceSlug), page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.Comment, 0, len(records))
	for _, record := range records {
		items = append(items, commentResponse(record))
	}
	return collaboration.ListServiceComments200JSONResponse(api.CommentPage{
		Total: int(total), Page: page, PageSize: pageSize, Items: items,
	}), nil
}

// CreateServiceComment 创建一条服务评论。
func (s *Server) CreateServiceComment(ctx context.Context, request collaboration.CreateServiceCommentRequestObject) (collaboration.CreateServiceCommentResponseObject, error) {
	if s.coverage == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.coverage.CreateServiceComment(ctx, principal, string(request.TenantSlug), string(request.ServiceSlug), request.Body.Body)
	if err != nil {
		return nil, err
	}
	return collaboration.CreateServiceComment201JSONResponse(commentResponse(record)), nil
}

// userPreferencesResponse 将用户偏好记录投影为 API 形状。
// ETag 的 id 固定为 "self"（单例实体），与 service 层 parseTextEntityTag 对齐。
func userPreferencesResponse(record service.UserPreferencesRecord) api.UserPreferences {
	defaultViews := map[string]any{}
	_ = json.Unmarshal(record.DefaultViews, &defaultViews)
	return api.UserPreferences{
		Locale: api.UserPreferencesLocale(record.Locale), Theme: api.UserPreferencesTheme(record.Theme),
		DefaultViews: defaultViews, Etag: revisionETag(etagKindUserPreferences, userPreferencesETagID, record.Revision),
	}
}

// viewOverrideResponse 将视图覆盖记录投影为 API 形状。
func viewOverrideResponse(record service.ViewOverrideRecord) api.ViewOverride {
	var defaultOptions map[string]any
	_ = json.Unmarshal(record.DefaultOptions, &defaultOptions)
	return api.ViewOverride{
		ViewId: record.ViewID, Override: api.ViewOverrideValue{Enabled: &record.Enabled, DefaultOptions: &defaultOptions},
		Etag: revisionETag(etagKindViewOverride, record.ViewID, record.Revision), UpdatedAt: record.UpdatedAt,
	}
}

// tagResponse 将标签记录投影为 API 形状。
func tagResponse(record service.TagRecord) api.Tag {
	color := nullable.NewNullNullable[string]()
	if record.Color != "" {
		color = nullable.NewNullableWithValue(record.Color)
	}
	return api.Tag{
		Id: api.Uuid(record.ID), Name: record.Name, Color: color,
		Etag:      revisionETag(etagKindTag, record.ID.String(), record.Revision),
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// commentResponse 将评论记录投影为 API 形状。
func commentResponse(record service.CommentRecord) api.Comment {
	return api.Comment{
		Id: api.Uuid(record.ID), ServiceId: api.Uuid(record.ServiceID),
		Author: api.User{Id: api.Uuid(record.AuthorID), DisplayName: record.AuthorDisplayName},
		Body:   record.Body, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// viewMatchesKind 判断视图定义是否支持给定资产类别。
func viewMatchesKind(definition service.ViewDefinition, kind string) bool {
	for _, inputKind := range definition.InputKinds {
		if inputKind == kind || inputKind == "*" {
			return true
		}
	}
	return false
}

// localePointer 将可空语言枚举投影为字符串指针。
func localePointer(value *api.UserPreferencesPatchRequestLocale) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}

// themePointer 将可空主题枚举投影为字符串指针。
func themePointer(value *api.UserPreferencesPatchRequestTheme) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}

const (
	// etagKindUserPreferences 是用户偏好 ETag 的实体类型令牌。
	etagKindUserPreferences = "user-preferences"
	// userPreferencesETagID 是用户偏好单例实体的 ETag id 占位符。
	userPreferencesETagID = "self"
	// etagKindViewOverride 是视图覆盖 ETag 的实体类型令牌。
	etagKindViewOverride = "view-override"
	// etagKindTag 是标签 ETag 的实体类型令牌。
	etagKindTag = "tag"
)
