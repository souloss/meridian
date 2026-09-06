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
	sessionCookieName = "meridian_session"
	csrfHeaderName    = "X-CSRF-Token"
)

type principalContextKey struct{}

type operationAuthPolicy struct {
	required    bool
	allowCookie bool
	allowPAT    bool
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
		if !policy.allowCookie && !policy.allowPAT {
			return next(ctx, w, r, request)
		}
		principal, found, err := s.authenticateRequest(ctx, r)
		if err != nil {
			return nil, err
		}
		if found {
			if (principal.Kind == service.PrincipalSession && !policy.allowCookie) || (principal.Kind == service.PrincipalPAT && !policy.allowPAT) {
				if policy.required {
					return nil, service.ErrUnauthenticated
				}
				return next(ctx, w, r, request)
			}
			ctx = context.WithValue(ctx, principalContextKey{}, principal)
			if principal.Kind == service.PrincipalSession && policy.allowCookie && !safeMethod(r.Method) {
				if err := s.identity.VerifyCSRF(principal, r.Header.Get(csrfHeaderName)); err != nil {
					return nil, err
				}
			}
		} else if policy.required {
			return nil, service.ErrUnauthenticated
		}
		return next(ctx, w, r, request)
	}
}

func (s *Server) authenticateRequest(ctx context.Context, r *http.Request) (service.Principal, bool, error) {
	authorization := r.Header.Get("Authorization")
	if authorization != "" {
		bearer, ok := strings.CutPrefix(authorization, "Bearer ")
		if !ok || strings.ContainsAny(bearer, " \t\r\n") {
			return service.Principal{}, false, nil
		}
		principal, err := s.identity.AuthenticatePAT(ctx, bearer)
		if err != nil {
			if errors.Is(err, service.ErrUnauthenticated) {
				return service.Principal{}, false, nil
			}
			return service.Principal{}, false, err
		}
		return principal, true, nil
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return service.Principal{}, false, nil
		}
		return service.Principal{}, false, err
	}
	principal, err := s.identity.AuthenticateSession(ctx, cookie.Value)
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

func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || method == http.MethodTrace
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
			policy := authPolicy(security)
			policies[upperFirst(operation.OperationID)] = policy
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
		if _, ok := requirement["cookieSession"]; ok {
			policy.allowCookie = true
		}
		if _, ok := requirement["patBearer"]; ok {
			policy.allowPAT = true
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
