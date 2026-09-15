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
	refreshCookieName = "meridian_refresh"
)

type principalContextKey struct{}

type refreshTokenContextKey struct{}

type operationAuthPolicy struct {
	required          bool
	allowBearer       bool
	allowRefreshCookie bool
}

var authPolicies = sync.OnceValue(loadOperationAuthPolicies)

type strictHandlerFunc func(context.Context, http.ResponseWriter, *http.Request, any) (any, error)

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
			// Refresh and logout read the refresh cookie directly.
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
			// Public operations carry no principal.
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

func (s *Server) authenticateBearer(ctx context.Context, r *http.Request) (service.Principal, bool, error) {
	bearer := r.Header.Get("Authorization")
	if bearer == "" {
		// EventSource cannot set an Authorization header; the SSE log stream carries
		// the access token in the query string instead.
		bearer = r.URL.Query().Get("token")
	}
	token, ok := strings.CutPrefix(bearer, "Bearer ")
	if !ok || strings.ContainsAny(token, " \t\r\n") {
		return service.Principal{}, false, nil
	}
	if strings.HasPrefix(token, "pat_") {
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

func principalFromContext(ctx context.Context) (service.Principal, error) {
	principal, ok := ctx.Value(principalContextKey{}).(service.Principal)
	if !ok {
		return service.Principal{}, service.ErrUnauthenticated
	}
	return principal, nil
}

func refreshTokenFromContext(ctx context.Context) (string, error) {
	token, ok := ctx.Value(refreshTokenContextKey{}).(string)
	if !ok || token == "" {
		return "", service.ErrUnauthenticated
	}
	return token, nil
}

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

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
