package handler

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"io/fs"
	"log/slog"
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
	ready            atomic.Bool
	assets           fs.FS
	identity         *service.Identity
	credentials      *service.Credentials
	repositories     *service.Repositories
	jobs             *service.Jobs
	audits           *service.Audits
	producers        *service.Producers
	discovery        *service.Discovery
	assetService     *service.Assets
	views            *service.Views
	serviceLifecycle *service.ServiceLifecycle
	layerEdit        *service.LayerEdit
	configImport     *service.ConfigImport
	aiWorkflow       *service.AiWorkflow
	diffService      *service.DiffService
	searchService    *service.Search
	systemGroups     *service.SystemGroups
	notifications    *service.Notifications
	secureCookies    bool
}

// Dependencies 聚合 HTTP 服务暴露的可独立测试用例。
type Dependencies struct {
	// Identity 提供认证、用户、租户、成员与 PAT 用例。
	Identity *service.Identity
	// Credentials 提供租户与平台凭据用例。
	Credentials *service.Credentials
	// Repositories 提供租户仓库配置用例。
	Repositories *service.Repositories
	// Jobs 提供脱敏的平台任务查询用例。
	Jobs *service.Jobs
	// Audits 提供租户与平台审计查询用例。
	Audits *service.Audits
	// Producers 提供平台生产者配置与租户选择。
	Producers *service.Producers
	// Discovery 提供仓库发现、候选接受与源配置。
	Discovery *service.Discovery
	// Assets 提供资产、版本与条目读取用例。
	Assets *service.Assets
	// Views 提供内置视图解析。
	Views *service.Views
	// ServiceLifecycle 提供服务元数据更新、公开读取与删除。
	ServiceLifecycle *service.ServiceLifecycle
	// LayerEdit 提供 overlay 修订、排序、回滚、合并预览与溯源。
	LayerEdit *service.LayerEdit
	// ConfigImport 提供 gitops 仓库配置预览与落库。
	ConfigImport *service.ConfigImport
	// AiWorkflow 提供 AI 生成、修订审核与版本发布。
	AiWorkflow *service.AiWorkflow
	// DiffService 提供差异、快照分享、破坏性待办、上传与推送。
	DiffService *service.DiffService
	// Search 提供跨类别租户搜索工作流。
	Search *service.Search
	// SystemGroups 提供系统分组创建与成员替换。
	SystemGroups *service.SystemGroups
	// Notifications 提供订阅、通知通道与站内通知用例。
	Notifications *service.Notifications
}

func New() *Server {
	s := &Server{assets: staticAssets()}
	s.ready.Store(true)
	return s
}

// NewWithIdentity 构造一个启用 M0 身份用例的服务器。
func NewWithIdentity(identity *service.Identity, secureCookies bool) *Server {
	return NewWithServices(identity, nil, secureCookies)
}

// NewWithServices 用显式装配的 M0 用例构造 HTTP 服务器。
// 为 nil 的用例会使其生成的严格操作返回契约的 501 桩实现。
func NewWithServices(identity *service.Identity, credentials *service.Credentials, secureCookies bool) *Server {
	return NewWithAllServices(identity, credentials, nil, secureCookies)
}

// NewWithAllServices 构造一个启用当前已实现全部 M0 用例的服务器。
func NewWithAllServices(identity *service.Identity, credentials *service.Credentials, repositories *service.Repositories, secureCookies bool) *Server {
	return NewWithRuntimeServices(Dependencies{Identity: identity, Credentials: credentials, Repositories: repositories}, secureCookies)
}

// NewWithRuntimeServices 从显式应用用例依赖构造 HTTP 服务器。
func NewWithRuntimeServices(dependencies Dependencies, secureCookies bool) *Server {
	s := New()
	s.identity = dependencies.Identity
	s.credentials = dependencies.Credentials
	s.repositories = dependencies.Repositories
	s.jobs = dependencies.Jobs
	s.audits = dependencies.Audits
	s.producers = dependencies.Producers
	s.discovery = dependencies.Discovery
	s.assetService = dependencies.Assets
	s.views = dependencies.Views
	s.serviceLifecycle = dependencies.ServiceLifecycle
	s.layerEdit = dependencies.LayerEdit
	s.configImport = dependencies.ConfigImport
	s.aiWorkflow = dependencies.AiWorkflow
	s.diffService = dependencies.DiffService
	s.searchService = dependencies.Search
	s.systemGroups = dependencies.SystemGroups
	s.notifications = dependencies.Notifications
	s.secureCookies = secureCookies
	return s
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestIDHeader)
	r.Use(traceRequests)
	r.Use(logRequests(slog.Default()))
	r.Use(measureRequests)
	r.Use(openAPIRequestValidator())
	registerDomainHandlers(s, r)
	r.NotFound(s.static)
	return r
}

func requestErrorHandler(w http.ResponseWriter, r *http.Request, _ error) {
	writeError(w, r, http.StatusBadRequest, errorCodeValidation, nil)
}

func responseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var overlayErr *service.OverlayInvalidError
	if quotaErr, ok := errors.AsType[*service.QuotaExceededError](err); ok {
		writeErrorDetails(w, r, http.StatusConflict, errorCodeQuotaExceeded, map[string]any{
			"quota": quotaErr.Resource, "current": quotaErr.Current, "limit": quotaErr.Limit,
		}, map[string]any{"Current": quotaErr.Current, "Limit": quotaErr.Limit})
		return
	}
	switch {
	case errors.Is(err, service.ErrUnauthenticated):
		writeError(w, r, http.StatusUnauthorized, errorCodeUnauthenticated, nil)
		return
	case errors.Is(err, service.ErrNotFound):
		writeError(w, r, http.StatusNotFound, errorCodeNotFound, nil)
		return
	case errors.Is(err, service.ErrDuplicate):
		writeError(w, r, http.StatusConflict, errorCodeDuplicate, nil)
		return
	case errors.As(err, &overlayErr):
		writeErrorDetails(w, r, http.StatusUnprocessableEntity, errorCodeOverlayInvalid, overlayErrorDetails(overlayErr), nil)
		return
	case errors.Is(err, service.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, errorCodeValidation, nil)
		return
	case errors.Is(err, service.ErrPrecondition):
		writeError(w, r, http.StatusPreconditionFailed, errorCodePreconditionFailed, nil)
		return
	case errors.Is(err, service.ErrCredentialInUse):
		writeError(w, r, http.StatusConflict, errorCodeCredentialInUse, nil)
		return
	case errors.Is(err, service.ErrIdempotencyConflict):
		writeError(w, r, http.StatusConflict, errorCodeIdempotencyConflict, nil)
		return
	case errors.Is(err, service.ErrJobNotCancellable):
		writeError(w, r, http.StatusConflict, errorCodeJobNotCancellable, nil)
		return
	case errors.Is(err, service.ErrJobNotRetryable):
		writeError(w, r, http.StatusConflict, errorCodeInvalidState, nil)
		return
	case errors.Is(err, service.ErrViewInputMismatch):
		writeError(w, r, http.StatusUnprocessableEntity, errorCodeInputSpecMismatch, nil)
		return
	case errors.Is(err, service.ErrBranchNotIndexed):
		writeError(w, r, http.StatusUnprocessableEntity, errorCodeBranchNotIndexed, nil)
		return
	case errors.Is(err, service.ErrInvalidState):
		writeError(w, r, http.StatusConflict, errorCodeInvalidState, nil)
		return
	case errors.Is(err, service.ErrBaseLayerExists):
		writeError(w, r, http.StatusConflict, errorCodeBaseLayerExists, nil)
		return
	case func() bool { _, ok := errors.AsType[*service.VersionNotPublishableError](err); return ok }():
		writeErrorDetails(w, r, http.StatusConflict, errorCodeVersionNotPublishable, map[string]any{
			"blockingRevisions": []any{}, "layerId": nil, "revisionId": nil, "reviewStatus": nil,
		}, nil)
		return
	case errors.Is(err, api.ErrStrictOperationNotImplemented):
		writeError(w, r, http.StatusNotImplemented, errorCodeInternal, nil)
		return
	}
	if _, ok := errors.AsType[*service.ProducerUnavailableError](err); ok {
		writeError(w, r, http.StatusUnprocessableEntity, errorCodeProducerUnavailable, nil)
		return
	}
	writeError(w, r, http.StatusInternalServerError, errorCodeInternal, nil)
	slog.Error("unhandled response error", "error", err)
}

func openAPIRequestValidator() func(http.Handler) http.Handler {
	specification, err := api.GetSwagger()
	if err != nil {
		panic("load embedded OpenAPI contract: " + err.Error())
	}
	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(specification, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		ErrorHandlerWithOpts: func(_ context.Context, _ error, w http.ResponseWriter, r *http.Request, options nethttpmiddleware.ErrorHandlerOpts) {
			if options.MatchedRoute == nil {
				writeError(w, r, http.StatusNotFound, errorCodeNotFound, nil)
				return
			}
			status := http.StatusBadRequest
			// 已知主机密钥校验可能在领域处理器运行前失败。
			if route := options.MatchedRoute.Route; route != nil && route.Method == http.MethodPost && route.Path == "/api/v1/t/{tenantSlug}/known-hosts" {
				status = http.StatusUnprocessableEntity
			}
			writeError(w, r, status, errorCodeValidation, nil)
		},
		DoNotValidateServers: true,
		Skipper: func(r *http.Request) bool {
			return r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && !strings.HasPrefix(r.URL.Path, "/api/")
		},
	})
	return func(next http.Handler) http.Handler {
		validated := validator(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isKnownHostCreateRequest(r) && hasDerivedKnownHostFields(r) {
				writeError(w, r, http.StatusUnprocessableEntity, errorCodeValidation, nil)
				return
			}
			validated.ServeHTTP(w, r)
		})
	}
}

func isKnownHostCreateRequest(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/t/") && strings.HasSuffix(r.URL.Path, "/known-hosts")
}

func hasDerivedKnownHostFields(r *http.Request) bool {
	if r.Body == nil {
		return false
	}
	body, err := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return false
	}
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(body, &fields); err != nil {
		return false
	}
	_, hasKeyType := fields["keyType"]
	_, hasFingerprint := fields["fingerprint"]
	return hasKeyType || hasFingerprint
}
