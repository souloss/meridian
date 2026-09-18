package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/meridian-labs/meridian/internal/generated/api"
	"github.com/meridian-labs/meridian/internal/service"
)

const (
	// refreshCookieName 是浏览器刷新令牌 Cookie 的名称。
	refreshCookieName = "meridian_refresh"
	// authCookiePath 是刷新令牌 Cookie 的路径前缀。
	authCookiePath = "/api/v1/auth"
	// patTokenPrefix 是个人访问令牌（PAT）的固定前缀。
	patTokenPrefix = "pat_"
	// bearerScheme 是 Authorization 头的 Bearer 认证方案前缀。
	bearerScheme = "Bearer "
)

// principalContextKey 是承载已认证主体的上下文键类型。
type principalContextKey struct{}

// refreshTokenContextKey 是承载刷新令牌明文（仅 refresh/logout 流程）的上下文键类型。
type refreshTokenContextKey struct{}

// operationAuthPolicy 描述单个操作的认证策略。
type operationAuthPolicy struct {
	// required 表示该操作必须建立主体上下文。
	required bool
	// allowBearer 表示允许通过 Bearer 访问令牌认证。
	allowBearer bool
	// allowRefreshCookie 表示允许通过刷新 Cookie 认证（refresh/logout）。
	allowRefreshCookie bool
}

// authPolicies 是操作 ID 到认证策略的惰性缓存。
var authPolicies = sync.OnceValue(loadOperationAuthPolicies)

type strictHandlerFunc func(context.Context, http.ResponseWriter, *http.Request, any) (any, error)

// authenticateOperation 包装严格处理器：按操作安全策略建立（或跳过）认证上下文。
func (s *Server) authenticateOperation(next strictHandlerFunc, operationID string) strictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		if s.identity == nil {
			return next(ctx, w, r, request)
		}
		policy, declared := authPolicies()[operationID]
		if !declared {
			return nil, errors.New("OpenAPI authentication policy is missing for operation " + operationID)
		}
		if policy.allowRefreshCookie && !policy.allowBearer {
			// refresh/logout 直接读取刷新 Cookie。
			cookie, err := r.Cookie(refreshCookieName)
			if err != nil {
				if policy.required {
					return nil, service.ErrUnauthenticated
				}
				return next(ctx, w, r, request)
			}
			ctx = context.WithValue(ctx, refreshTokenContextKey{}, cookie.Value)
			return next(ctx, w, r, request)
		}
		if !policy.allowBearer {
			// 公共操作不携带主体。
			return next(ctx, w, r, request)
		}
		principal, found, err := s.authenticateBearer(ctx, r)
		if err != nil {
			return nil, err
		}
		if !found {
			if policy.required {
				return nil, service.ErrUnauthenticated
			}
			return next(ctx, w, r, request)
		}
		ctx = context.WithValue(ctx, principalContextKey{}, principal)
		return next(ctx, w, r, request)
	}
}

// authenticateBearer 从 Authorization 头（或 SSE 的 token 查询参数）解析并认证 Bearer 令牌。
func (s *Server) authenticateBearer(ctx context.Context, r *http.Request) (service.Principal, bool, error) {
	bearer := r.Header.Get("Authorization")
	if bearer == "" {
		// EventSource 无法携带 Authorization 头；SSE 日志流改为通过查询参数传递访问令牌。
		bearer = r.URL.Query().Get("token")
	}
	token, ok := strings.CutPrefix(bearer, bearerScheme)
	if !ok || strings.ContainsAny(token, " \t\r\n") {
		return service.Principal{}, false, nil
	}
	if strings.HasPrefix(token, patTokenPrefix) {
		principal, err := s.identity.AuthenticatePAT(ctx, token)
		if err != nil {
			if errors.Is(err, service.ErrUnauthenticated) {
				return service.Principal{}, false, nil
			}
			return service.Principal{}, false, err
		}
		return principal, true, nil
	}
	principal, err := s.identity.AuthenticateJWT(ctx, token)
	if err != nil {
		if errors.Is(err, service.ErrUnauthenticated) {
			return service.Principal{}, false, nil
		}
		return service.Principal{}, false, err
	}
	return principal, true, nil
}

// principalFromContext 从请求上下文取出已认证主体。
func principalFromContext(ctx context.Context) (service.Principal, error) {
	principal, ok := ctx.Value(principalContextKey{}).(service.Principal)
	if !ok {
		return service.Principal{}, service.ErrUnauthenticated
	}
	return principal, nil
}

// refreshTokenFromContext 从请求上下文取出刷新令牌明文。
func refreshTokenFromContext(ctx context.Context) (string, error) {
	token, ok := ctx.Value(refreshTokenContextKey{}).(string)
	if !ok || token == "" {
		return "", service.ErrUnauthenticated
	}
	return token, nil
}

// loadOperationAuthPolicies 从内嵌 OpenAPI 契约构建操作级认证策略表。
func loadOperationAuthPolicies() map[string]operationAuthPolicy {
	specification, err := api.GetSwagger()
	if err != nil {
		panic("load embedded OpenAPI authentication policies: " + err.Error())
	}
	policies := make(map[string]operationAuthPolicy)
	for _, pathItem := range specification.Paths.Map() {
		for _, operation := range pathItem.Operations() {
			security := specification.Security
			if operation.Security != nil {
				security = *operation.Security
			}
			policies[upperFirst(operation.OperationID)] = authPolicy(security)
		}
	}
	return policies
}

// authPolicy 将一组 OpenAPI 安全需求折叠为操作级认证策略。
func authPolicy(requirements openapi3.SecurityRequirements) operationAuthPolicy {
	policy := operationAuthPolicy{required: len(requirements) > 0}
	for _, requirement := range requirements {
		if len(requirement) == 0 {
			policy.required = false
		}
		if _, ok := requirement["bearerAuth"]; ok {
			policy.allowBearer = true
		}
		if _, ok := requirement["refreshCookie"]; ok {
			policy.allowRefreshCookie = true
		}
	}
	return policy
}

// upperFirst 返回首字母大写的字符串（用于将 operationId 对齐到处理函数命名）。
func upperFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
