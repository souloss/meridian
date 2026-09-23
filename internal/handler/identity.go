package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	auth "github.com/meridian-labs/meridian/internal/generated/api/auth"
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
	tenant "github.com/meridian-labs/meridian/internal/generated/api/tenant"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// Login 在校验本地凭据后创建浏览器会话。
func (s *Server) Login(ctx context.Context, request auth.LoginRequestObject) (auth.LoginResponseObject, error) {
	if s.identity == nil || request.Body == nil || request.Body.Password == "" {
		return nil, service.ErrUnauthenticated
	}
	result, err := s.identity.Login(ctx, request.Body.Username, request.Body.Password)
	if err != nil {
		return nil, err
	}
	cookie := refreshCookie(result.RefreshToken, result.RefreshExpiresAt, s.secureCookies)
	return auth.Login200JSONResponse{
		Body: api.LoginResult{
			AccessToken:      result.AccessToken,
			ExpiresInSeconds: result.ExpiresInSeconds,
			Me:               meResponse(result.Principal.User, result.Memberships),
		},
		Headers: auth.Login200ResponseHeaders{SetCookie: new(cookie.String())},
	}, nil
}

// Refresh 用刷新 Cookie 换取新的访问令牌并轮换刷新令牌。
func (s *Server) Refresh(ctx context.Context, request auth.RefreshRequestObject) (auth.RefreshResponseObject, error) {
	if s.identity == nil {
		return nil, service.ErrUnauthenticated
	}
	token, err := refreshTokenFromContext(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.identity.Refresh(ctx, token)
	if err != nil {
		return nil, err
	}
	cookie := refreshCookie(result.RefreshToken, result.RefreshExpiresAt, s.secureCookies)
	return auth.Refresh200JSONResponse{
		Body: api.RefreshResult{
			AccessToken:      result.AccessToken,
			ExpiresInSeconds: result.ExpiresInSeconds,
		},
		Headers: auth.Refresh200ResponseHeaders{SetCookie: new(cookie.String())},
	}, nil
}

// Logout 吊销刷新 Cookie 家族并清除 Cookie。
func (s *Server) Logout(ctx context.Context, _ auth.LogoutRequestObject) (auth.LogoutResponseObject, error) {
	if s.identity == nil {
		return nil, service.ErrUnauthenticated
	}
	token, err := refreshTokenFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.identity.Logout(ctx, token); err != nil {
		return nil, err
	}
	cookie := expiredRefreshCookie(s.secureCookies)
	return auth.Logout204Response{Headers: auth.Logout204ResponseHeaders{SetCookie: new(cookie.String())}}, nil
}

// GetMe 返回当前浏览器身份与有效租户成员关系。
func (s *Server) GetMe(ctx context.Context, _ auth.GetMeRequestObject) (auth.GetMeResponseObject, error) {
	if s.identity == nil {
		return nil, service.ErrUnauthenticated
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, memberships, err := s.identity.Me(ctx, principal)
	if err != nil {
		return nil, err
	}
	return auth.GetMe200JSONResponse(meResponse(user, memberships)), nil
}

// CreateUser 通过平台管理边界创建本地身份。
func (s *Server) CreateUser(ctx context.Context, request platform.CreateUserRequestObject) (platform.CreateUserResponseObject, error) {
	if s.identity == nil || request.Body == nil || request.Body.Password == "" {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, err := s.identity.CreateUser(ctx, principal, service.CreateUserInput{
		Username:    request.Body.Username,
		Password:    request.Body.Password,
		DisplayName: request.Body.DisplayName,
		Email:       optionalEmail(request.Body.Email),
	})
	if err != nil {
		return nil, err
	}
	body := userResponse(user)
	return platform.CreateUser201JSONResponse{Body: body, Headers: platform.CreateUser201ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// ListUsers 返回分页的平台身份目录（不含秘密字段）。
func (s *Server) ListUsers(ctx context.Context, request platform.ListUsersRequestObject) (platform.ListUsersResponseObject, error) {
	if s.identity == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	search := ""
	if request.Params.Q != nil {
		search = string(*request.Params.Q)
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.identity.ListUsers(ctx, principal, search, page, pageSize)
	if err != nil {
		return nil, err
	}
	users := make([]api.User, 0, len(items))
	for _, item := range items {
		users = append(users, userResponse(item))
	}
	return platform.ListUsers200JSONResponse(api.UserPage{
		Items: users, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// CreateTenant 以平台默认值的原子快照创建租户。
func (s *Server) CreateTenant(ctx context.Context, request platform.CreateTenantRequestObject) (platform.CreateTenantResponseObject, error) {
	if s.identity == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	createdTenant, err := s.identity.CreateTenant(ctx, principal, service.CreateTenantInput{
		Slug:        request.Body.Slug,
		DisplayName: request.Body.DisplayName,
		Quota:       quotaInput(request.Body.Quota),
	})
	if err != nil {
		return nil, err
	}
	body := tenantResponse(createdTenant)
	return platform.CreateTenant201JSONResponse{Body: body, Headers: platform.CreateTenant201ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// ListTenants 返回跨生命周期状态的分页平台租户目录。
func (s *Server) ListTenants(ctx context.Context, request platform.ListTenantsRequestObject) (platform.ListTenantsResponseObject, error) {
	if s.identity == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.identity.ListTenants(ctx, principal, page, pageSize)
	if err != nil {
		return nil, err
	}
	tenants := make([]api.Tenant, 0, len(items))
	for _, item := range items {
		tenants = append(tenants, tenantResponse(item))
	}
	return platform.ListTenants200JSONResponse(api.TenantPage{
		Items: tenants, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// UpdateTenant 在调用者 ETag 下应用平台租户补丁。
func (s *Server) UpdateTenant(ctx context.Context, request platform.UpdateTenantRequestObject) (platform.UpdateTenantResponseObject, error) {
	if s.identity == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	patch := service.TenantPatchInput{
		DisplayName: request.Body.DisplayName,
		Quota:       quotaInput(request.Body.Quota),
	}
	if request.Body.Status != nil {
		status := string(*request.Body.Status)
		patch.Status = &status
	}
	updatedTenant, err := s.identity.UpdateTenant(ctx, principal, request.TenantSlug, request.Params.IfMatch, patch)
	if err != nil {
		return nil, err
	}
	body := tenantResponse(updatedTenant)
	return platform.UpdateTenant200JSONResponse{Body: body, Headers: platform.UpdateTenant200ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// PutTenantMemberAsPlatformAdmin 创建或替换一份租户角色指派。
func (s *Server) PutTenantMemberAsPlatformAdmin(ctx context.Context, request platform.PutTenantMemberAsPlatformAdminRequestObject) (platform.PutTenantMemberAsPlatformAdminResponseObject, error) {
	if s.identity == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, membership, err := s.identity.PutTenantMembership(ctx, principal, request.TenantSlug, serviceUUID(request.UserId), string(request.Body.Role))
	if err != nil {
		return nil, err
	}
	return platform.PutTenantMemberAsPlatformAdmin200JSONResponse(memberResponse(user, membership)), nil
}

// ListTokens 返回一页 PAT 元数据（不含令牌明文）。
func (s *Server) ListTokens(ctx context.Context, request tenant.ListTokensRequestObject) (tenant.ListTokensResponseObject, error) {
	if s.identity == nil {
		return nil, service.ErrUnauthenticated
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	tokens, total, err := s.identity.ListTokens(ctx, principal, request.TenantSlug, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.ApiToken, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, tokenResponse(token))
	}
	return tenant.ListTokens200JSONResponse(api.TokenPage{
		Items: items, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// CreateToken 创建 PAT 并仅此一次返回其明文。
func (s *Server) CreateToken(ctx context.Context, request tenant.CreateTokenRequestObject) (tenant.CreateTokenResponseObject, error) {
	if s.identity == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	scopes := make([]string, len(request.Body.Scopes))
	for index, scope := range request.Body.Scopes {
		scopes[index] = string(scope)
	}
	created, err := s.identity.CreateToken(ctx, principal, request.TenantSlug, service.CreateTokenInput{
		Name:      request.Body.Name,
		Scopes:    scopes,
		ExpiresAt: optionalTime(request.Body.ExpiresAt),
	})
	if err != nil {
		return nil, err
	}
	metadata := tokenResponse(created.Token)
	return tenant.CreateToken201JSONResponse(api.TokenCreated{
		Id: metadata.Id, Name: metadata.Name, Scopes: metadata.Scopes, ExpiresAt: metadata.ExpiresAt,
		LastUsedAt: metadata.LastUsedAt, RevokedAt: metadata.RevokedAt, CreatedAt: metadata.CreatedAt,
		Token: created.Plaintext,
	}), nil
}

// RevokeToken 幂等地吊销当前浏览器用户拥有的 PAT。
func (s *Server) RevokeToken(ctx context.Context, request tenant.RevokeTokenRequestObject) (tenant.RevokeTokenResponseObject, error) {
	if s.identity == nil {
		return nil, service.ErrUnauthenticated
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.identity.RevokeToken(ctx, principal, request.TenantSlug, serviceUUID(request.TokenId)); err != nil {
		return nil, err
	}
	return tenant.RevokeToken204Response{}, nil
}

// UpdateUser 在校验仅平台授权后更新一个身份的非秘密字段。
func (s *Server) UpdateUser(ctx context.Context, request platform.UpdateUserRequestObject) (platform.UpdateUserResponseObject, error) {
	if s.identity == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	input := service.UpdateUserProfileInput{
		DisplayName: request.Body.DisplayName,
		Status:      optionalStringEnum(request.Body.Status),
	}
	if request.Body.Email.IsSpecified() {
		input.SetEmail = true
		input.Email = optionalEmail(request.Body.Email)
	}
	user, err := s.identity.UpdateUser(ctx, principal, serviceUUID(request.UserId), request.Params.IfMatch, input)
	if err != nil {
		return nil, err
	}
	body := userResponse(user)
	return platform.UpdateUser200JSONResponse{Body: body, Headers: platform.UpdateUser200ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// optionalStringEnum 将可空枚举指针投影为字符串指针。
func optionalStringEnum(value *api.UserPatchRequestStatus) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}

// refreshCookie 构造登录/刷新后的刷新令牌 Cookie。
func refreshCookie(token string, expiresAt time.Time, secure bool) http.Cookie {
	return http.Cookie{
		Name: refreshCookieName, Value: token, Path: authCookiePath, HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, Expires: expiresAt, MaxAge: int(time.Until(expiresAt).Seconds()),
	}
}

// expiredRefreshCookie 构造一个立即过期的刷新令牌 Cookie（用于登出清除）。
func expiredRefreshCookie(secure bool) http.Cookie {
	return http.Cookie{
		Name: refreshCookieName, Path: authCookiePath, HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, Expires: time.Unix(1, 0).UTC(), MaxAge: -1,
	}
}

// meResponse 将当前用户及其成员关系投影为 API 形状。
func meResponse(user service.User, memberships []service.Membership) api.Me {
	tenants := make([]api.TenantMembershipRef, 0, len(memberships))
	for _, membership := range memberships {
		tenants = append(tenants, api.TenantMembershipRef{
			Id: api.Uuid(membership.TenantID), Slug: membership.TenantSlug,
			DisplayName: membership.TenantDisplayName, Role: api.TenantRole(membership.Role),
		})
	}
	return api.Me{User: userResponse(user), IsPlatformAdmin: user.IsPlatformAdmin, Tenants: tenants}
}

// userResponse 将用户记录投影为 API 形状。
func userResponse(user service.User) api.User {
	email := nullable.NewNullNullable[string]()
	if user.Email != nil {
		email = nullable.NewNullableWithValue(*user.Email)
	}
	return api.User{
		Id: api.Uuid(user.ID), Username: user.Username, DisplayName: user.DisplayName, Email: email,
		Status: api.UserStatus(user.Status), Etag: revisionETag(etagKindUser, user.ID.String(), user.Revision),
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}
}

// tenantResponse 将租户记录投影为 API 形状。
func tenantResponse(record service.Tenant) api.Tenant {
	return api.Tenant{
		Id: api.Uuid(record.ID), Slug: record.Slug, DisplayName: record.DisplayName, Status: api.TenantStatus(record.Status),
		Quota: api.Quota{
			MaxRepositories: record.Quota.MaxRepositories, MaxServices: record.Quota.MaxServices,
			MaxStorageBytes: record.Quota.MaxStorageBytes, MaxCollectConcurrency: record.Quota.MaxCollectConcurrency,
		},
		Etag: revisionETag(etagKindTenant, record.ID.String(), record.Revision), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// memberResponse 将用户与成员关系投影为 API 形状。
func memberResponse(user service.User, membership service.Membership) api.Member {
	return api.Member{User: userResponse(user), Role: api.TenantRole(membership.Role), JoinedAt: membership.JoinedAt}
}

// tokenResponse 将 PAT 元数据投影为 API 形状。
func tokenResponse(token service.Token) api.ApiToken {
	scopes := make([]api.TokenScope, len(token.Scopes))
	for index, scope := range token.Scopes {
		scopes[index] = api.TokenScope(scope)
	}
	return api.ApiToken{
		Id: api.Uuid(token.ID), Name: token.Name, Scopes: scopes, ExpiresAt: nullableTime(token.ExpiresAt),
		LastUsedAt: nullableTime(token.LastUsedAt), RevokedAt: nullableTime(token.RevokedAt), CreatedAt: token.CreatedAt,
	}
}

// optionalEmail 将可空邮箱包装为指针（未指定或空视为 nil）。
func optionalEmail(value nullable.Nullable[string]) *string {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	email := string(value.MustGet())
	return &email
}

// optionalTime 将可空时间包装为指针（未指定或空视为 nil）。
func optionalTime(value nullable.Nullable[time.Time]) *time.Time {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	return new(value.MustGet())
}

// nullableTime 将可空时间指针包装为可空 API 值。
func nullableTime(value *time.Time) nullable.Nullable[time.Time] {
	if value == nil {
		return nullable.NewNullNullable[time.Time]()
	}
	return nullable.NewNullableWithValue(*value)
}

// quotaInput 将可空配额请求投影为服务层配额（nil 透传）。
func quotaInput(value *api.Quota) *service.Quota {
	if value == nil {
		return nil
	}
	return &service.Quota{
		MaxRepositories: value.MaxRepositories, MaxServices: value.MaxServices,
		MaxStorageBytes: value.MaxStorageBytes, MaxCollectConcurrency: value.MaxCollectConcurrency,
	}
}

// pagination 解析可选分页参数并套用默认页码与页大小。
func pagination(pageValue *api.Page, pageSizeValue *api.PageSize) (int, int) {
	page, pageSize := defaultPage, defaultPageSize
	if pageValue != nil {
		page = *pageValue
	}
	if pageSizeValue != nil {
		pageSize = *pageSizeValue
	}
	return page, pageSize
}

// revisionETag 格式化乐观并发控制用的版本化 ETag。
func revisionETag(kind, id string, revision int64) string {
	return fmt.Sprintf(`"%s:%s:%d"`, kind, id, revision)
}

// serviceUUID 将 API UUID 转换为服务层 UUID。
func serviceUUID(value api.Uuid) uuid.UUID {
	return uuid.UUID(value)
}

const (
	// defaultPage 是分页查询缺省的页码（一基）。
	defaultPage = 1
	// defaultPageSize 是分页查询缺省的页大小。
	defaultPageSize = 20
	// etagKindUser 是用户 ETag 的实体类型令牌。
	etagKindUser = "user"
	// etagKindTenant 是租户 ETag 的实体类型令牌。
	etagKindTenant = "tenant"
)
