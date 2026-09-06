package handler

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/meridian-labs/meridian/internal/generated/api"
	"github.com/meridian-labs/meridian/internal/service"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

type Server struct {
	ready         atomic.Bool
	assets        fs.FS
	identity      *service.Identity
	credentials   *service.Credentials
	repositories  *service.Repositories
	jobs          *service.Jobs
	audits        *service.Audits
	secureCookies bool
}

// Dependencies groups the independently testable use cases exposed by the HTTP server.
type Dependencies struct {
	// Identity provides authentication, user, tenant, membership, and PAT use cases.
	Identity *service.Identity
	// Credentials provides tenant and platform credential use cases.
	Credentials *service.Credentials
	// Repositories provides tenant repository configuration use cases.
	Repositories *service.Repositories
	// Jobs provides redacted platform job query use cases.
	Jobs *service.Jobs
	// Audits provides tenant and platform audit query use cases.
	Audits *service.Audits
}

func New() *Server {
	s := &Server{assets: staticAssets()}
	s.ready.Store(true)
	return s
}

// NewWithIdentity constructs a server with M0 identity use cases enabled.
func NewWithIdentity(identity *service.Identity, secureCookies bool) *Server {
	return NewWithServices(identity, nil, secureCookies)
}

// NewWithServices constructs an HTTP server with explicitly wired M0 use cases.
// A nil use case leaves its generated strict operations returning the contract's 501 stub.
func NewWithServices(identity *service.Identity, credentials *service.Credentials, secureCookies bool) *Server {
	return NewWithAllServices(identity, credentials, nil, secureCookies)
}

// NewWithAllServices constructs an HTTP server with every currently implemented M0 use case.
func NewWithAllServices(identity *service.Identity, credentials *service.Credentials, repositories *service.Repositories, secureCookies bool) *Server {
	return NewWithRuntimeServices(Dependencies{Identity: identity, Credentials: credentials, Repositories: repositories}, secureCookies)
}

// NewWithRuntimeServices constructs an HTTP server from explicit application use-case dependencies.
func NewWithRuntimeServices(dependencies Dependencies, secureCookies bool) *Server {
	s := New()
	s.identity = dependencies.Identity
	s.credentials = dependencies.Credentials
	s.repositories = dependencies.Repositories
	s.jobs = dependencies.Jobs
	s.audits = dependencies.Audits
	s.secureCookies = secureCookies
	return s
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestIDHeader)
	r.Use(openAPIRequestValidator())
	registerDomainHandlers(s, r)
	r.NotFound(s.static)
	return r
}

func requestErrorHandler(w http.ResponseWriter, r *http.Request, _ error) {
	writeError(w, r, http.StatusBadRequest, "validation_error", "request does not satisfy the API contract")
}

func responseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	if quotaErr, ok := errors.AsType[*service.QuotaExceededError](err); ok {
		writeErrorDetails(w, r, http.StatusConflict, "quota_exceeded", "tenant resource quota would be exceeded", map[string]any{
			"quota": quotaErr.Resource, "current": quotaErr.Current, "limit": quotaErr.Limit,
		})
		return
	}
	switch {
	case errors.Is(err, service.ErrUnauthenticated):
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	case errors.Is(err, service.ErrCSRFInvalid):
		writeError(w, r, http.StatusForbidden, "csrf_invalid", "CSRF token is missing or invalid")
		return
	case errors.Is(err, service.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "resource not found")
		return
	case errors.Is(err, service.ErrDuplicate):
		writeError(w, r, http.StatusConflict, "duplicate", "resource already exists")
		return
	case errors.Is(err, service.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "request violates a domain rule")
		return
	case errors.Is(err, service.ErrPrecondition):
		writeError(w, r, http.StatusPreconditionFailed, "precondition_failed", "resource changed; refresh and retry")
		return
	case errors.Is(err, service.ErrCredentialInUse):
		writeError(w, r, http.StatusConflict, "credential_in_use", "credential is still referenced by a repository")
		return
	case errors.Is(err, service.ErrIdempotencyConflict):
		writeError(w, r, http.StatusConflict, "idempotency_conflict", "idempotency key was already used for a different request")
		return
	case errors.Is(err, service.ErrJobNotCancellable):
		writeError(w, r, http.StatusConflict, "job_not_cancellable", "job has already reached a terminal state")
		return
	case errors.Is(err, service.ErrJobNotRetryable):
		writeError(w, r, http.StatusConflict, "invalid_state", "job cannot be retried from its current state")
		return
	case errors.Is(err, api.ErrStrictOperationNotImplemented):
		writeError(w, r, http.StatusNotImplemented, "internal_error", "operation is not implemented")
		return
	}
	writeError(w, r, http.StatusInternalServerError, "internal_error", "request failed")
}

func openAPIRequestValidator() func(http.Handler) http.Handler {
	specification, err := api.GetSwagger()
	if err != nil {
		panic("load embedded OpenAPI contract: " + err.Error())
	}
	return nethttpmiddleware.OapiRequestValidatorWithOptions(specification, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		ErrorHandlerWithOpts: func(_ context.Context, _ error, w http.ResponseWriter, r *http.Request, options nethttpmiddleware.ErrorHandlerOpts) {
			if options.MatchedRoute == nil {
				writeError(w, r, http.StatusNotFound, "not_found", "resource not found")
				return
			}
			status := http.StatusBadRequest
			// Known Host key validation may fail before the domain handler runs.
			if route := options.MatchedRoute.Route; route != nil && route.Method == http.MethodPost && route.Path == "/api/v1/t/{tenantSlug}/known-hosts" {
				status = http.StatusUnprocessableEntity
			}
			writeError(w, r, status, "validation_error", "request does not satisfy the API contract")
		},
		DoNotValidateServers: true,
		Skipper: func(r *http.Request) bool {
			return r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && !strings.HasPrefix(r.URL.Path, "/api/")
		},
	})
}
