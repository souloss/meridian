package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	auth "github.com/meridian-labs/meridian/internal/generated/api/auth"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"

	// Login creates a browser session after verifying local credentials.
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
	tenant "github.com/meridian-labs/meridian/internal/generated/api/tenant"
)

func (s *Server) Login(ctx context.Context, request auth.LoginRequestObject) (auth.LoginResponseObject, error) {
	if s.identity == nil || request.Body == nil || request.Body.Password == "" {
		return nil, service.ErrUnauthenticated
	}
	result, err := s.identity.Login(ctx, request.Body.Username, request.Body.Password)
	if err != nil {
		return nil, err
	}
	cookie := sessionCookie(result.SessionToken, result.ExpiresAt, s.secureCookies)
	return auth.Login200JSONResponse{
		Body: api.LoginResult{
			CsrfToken: result.CSRFToken,
			Me:        meResponse(result.Principal.User, result.Memberships),
		},
		Headers: auth.Login200ResponseHeaders{SetCookie: new(cookie.String())},
	}, nil
}

// Logout revokes and clears the current browser session.
func (s *Server) Logout(ctx context.Context, _ auth.LogoutRequestObject) (auth.LogoutResponseObject, error) {
	if s.identity == nil {
		return nil, service.ErrUnauthenticated
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.identity.Logout(ctx, principal); err != nil {
		return nil, err
	}
	cookie := expiredSessionCookie(s.secureCookies)
	return auth.Logout204Response{Headers: auth.Logout204ResponseHeaders{SetCookie: new(cookie.String())}}, nil
}

// GetMe returns the current browser identity and active tenant memberships.
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

// GetCsrfToken rotates and returns the anti-forgery token for a browser session.
func (s *Server) GetCsrfToken(ctx context.Context, _ auth.GetCsrfTokenRequestObject) (auth.GetCsrfTokenResponseObject, error) {
	if s.identity == nil {
		return nil, service.ErrUnauthenticated
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	token, err := s.identity.RotateCSRF(ctx, principal)
	if err != nil {
		return nil, err
	}
	return auth.GetCsrfToken200JSONResponse{CsrfToken: token}, nil
}

// CreateUser creates a local identity through the platform administration boundary.
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

// ListUsers returns a paginated platform identity directory without secret fields.
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

// CreateTenant creates a tenant with an atomic snapshot of platform defaults.
func (s *Server) CreateTenant(ctx context.Context, request platform.CreateTenantRequestObject) (platform.CreateTenantResponseObject, error) {
	if s.identity == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	tenant, err := s.identity.CreateTenant(ctx, principal, service.CreateTenantInput{
		Slug:        request.Body.Slug,
		DisplayName: request.Body.DisplayName,
		Quota:       quotaInput(request.Body.Quota),
	})
	if err != nil {
		return nil, err
	}
	body := tenantResponse(tenant)
	return platform.CreateTenant201JSONResponse{Body: body, Headers: platform.CreateTenant201ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// ListTenants returns a paginated platform tenant directory across lifecycle states.
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

// UpdateTenant applies a platform tenant patch under the caller's ETag.
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
	tenant, err := s.identity.UpdateTenant(ctx, principal, request.TenantSlug, request.Params.IfMatch, patch)
	if err != nil {
		return nil, err
	}
	body := tenantResponse(tenant)
	return platform.UpdateTenant200JSONResponse{Body: body, Headers: platform.UpdateTenant200ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// PutTenantMemberAsPlatformAdmin creates or replaces one tenant role assignment.
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

// ListTokens returns one page of PAT metadata without bearer values.
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

// CreateToken creates a PAT and returns its bearer value exactly once.
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

// RevokeToken idempotently revokes a PAT owned by the current browser user.
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

func sessionCookie(token string, expiresAt time.Time, secure bool) http.Cookie {
	return http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, Expires: expiresAt, MaxAge: int(time.Until(expiresAt).Seconds()),
	}
}

func expiredSessionCookie(secure bool) http.Cookie {
	return http.Cookie{
		Name: sessionCookieName, Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, Expires: time.Unix(1, 0).UTC(), MaxAge: -1,
	}
}

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

func userResponse(user service.User) api.User {
	email := nullable.NewNullNullable[string]()
	if user.Email != nil {
		email = nullable.NewNullableWithValue(*user.Email)
	}
	return api.User{
		Id: api.Uuid(user.ID), Username: user.Username, DisplayName: user.DisplayName, Email: email,
		Status: api.UserStatus(user.Status), Etag: revisionETag("user", user.ID.String(), user.Revision),
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}
}

func tenantResponse(tenant service.Tenant) api.Tenant {
	return api.Tenant{
		Id: api.Uuid(tenant.ID), Slug: tenant.Slug, DisplayName: tenant.DisplayName, Status: api.TenantStatus(tenant.Status),
		Quota: api.Quota{
			MaxRepositories: tenant.Quota.MaxRepositories, MaxServices: tenant.Quota.MaxServices,
			MaxStorageBytes: tenant.Quota.MaxStorageBytes, MaxCollectConcurrency: tenant.Quota.MaxCollectConcurrency,
		},
		Etag: revisionETag("tenant", tenant.ID.String(), tenant.Revision), CreatedAt: tenant.CreatedAt, UpdatedAt: tenant.UpdatedAt,
	}
}

func memberResponse(user service.User, membership service.Membership) api.Member {
	return api.Member{User: userResponse(user), Role: api.TenantRole(membership.Role), JoinedAt: membership.JoinedAt}
}

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

func optionalEmail(value nullable.Nullable[string]) *string {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	email := string(value.MustGet())
	return &email
}

func optionalTime(value nullable.Nullable[time.Time]) *time.Time {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	return new(value.MustGet())
}

func nullableTime(value *time.Time) nullable.Nullable[time.Time] {
	if value == nil {
		return nullable.NewNullNullable[time.Time]()
	}
	return nullable.NewNullableWithValue(*value)
}

func quotaInput(value *api.Quota) *service.Quota {
	if value == nil {
		return nil
	}
	return &service.Quota{
		MaxRepositories: value.MaxRepositories, MaxServices: value.MaxServices,
		MaxStorageBytes: value.MaxStorageBytes, MaxCollectConcurrency: value.MaxCollectConcurrency,
	}
}

func pagination(pageValue *api.Page, pageSizeValue *api.PageSize) (int, int) {
	page, pageSize := 1, 20
	if pageValue != nil {
		page = *pageValue
	}
	if pageSizeValue != nil {
		pageSize = *pageSizeValue
	}
	return page, pageSize
}

func revisionETag(kind, id string, revision int64) string {
	return fmt.Sprintf(`"%s:%s:%d"`, kind, id, revision)
}

func serviceUUID(value api.Uuid) uuid.UUID {
	return uuid.UUID(value)
}
