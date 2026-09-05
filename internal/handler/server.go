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
	api.UnimplementedStrictServer
	ready         atomic.Bool
	assets        fs.FS
	identity      *service.Identity
	credentials   *service.Credentials
	secureCookies bool
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
	s := New()
	s.identity = identity
	s.credentials = credentials
	s.secureCookies = secureCookies
	return s
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestIDHeader)
	r.Use(openAPIRequestValidator())
	strict := api.NewStrictHandlerWithOptions(s, []api.StrictMiddlewareFunc{s.authenticate}, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			writeError(w, r, http.StatusBadRequest, "validation_error", "request does not satisfy the API contract")
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
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
			}
			if errors.Is(err, api.ErrStrictOperationNotImplemented) {
				writeError(w, r, http.StatusNotImplemented, "internal_error", "operation is not implemented")
				return
			}
			writeError(w, r, http.StatusInternalServerError, "internal_error", "request failed")
		},
	})
	api.HandlerFromMux(strict, r)
	r.NotFound(s.static)
	return r
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
			writeError(w, r, http.StatusBadRequest, "validation_error", "request does not satisfy the API contract")
		},
		DoNotValidateServers: true,
		Skipper: func(r *http.Request) bool {
			return r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && !strings.HasPrefix(r.URL.Path, "/api/")
		},
	})
}
