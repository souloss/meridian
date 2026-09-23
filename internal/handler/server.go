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

// Server 是从 Meridian OpenAPI 契约派生的生成传输代码。
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
	teams            *service.Teams
	settings         *service.Settings
	assetKinds       *service.AssetKinds
	coverage         *service.Coverage
	webhooks         *service.Webhooks
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
	// Teams 提供团队、团队成员与租户成员管理用例。
	Teams *service.Teams
	// Settings 提供平台默认配置与租户运行设置用例。
	Settings *service.Settings
	// AssetKinds 提供资产类别启停与列示用例。
	AssetKinds *service.AssetKinds
	// Coverage 提供服务收藏、用户偏好、视图覆盖、标签与评论用例。
	Coverage *service.Coverage
	// Webhooks 提供入站 Git webhook 校验与同步入队用例。
	Webhooks *service.Webhooks
}

// New 实现 Meridian OpenAPI 契约的生成传输行为。
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
	s.teams = dependencies.Teams
	s.settings = dependencies.Settings
	s.assetKinds = dependencies.AssetKinds
	s.coverage = dependencies.Coverage
	s.webhooks = dependencies.Webhooks
	s.secureCookies = secureCookies
	return s
}

// Handler 实现 Meridian OpenAPI 契约的生成传输行为。
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
	writeError(w, r, http.StatusBadRequest, service.ErrorCodeValidation, nil)
}

func responseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var overlayErr *service.OverlayInvalidError
	if quotaErr, ok := errors.AsType[*service.QuotaExceededError](err); ok {
		writeErrorDetails(w, r, http.StatusConflict, service.ErrorCodeQuotaExceeded, map[string]any{
			"quota": quotaErr.Resource, "current": quotaErr.Current, "limit": quotaErr.Limit,
		}, map[string]any{"Current": quotaErr.Current, "Limit": quotaErr.Limit})
		return
	}
	switch {
	case errors.Is(err, service.ErrUnauthenticated):
		writeError(w, r, http.StatusUnauthorized, service.ErrorCodeUnauthenticated, nil)
		return
	case errors.Is(err, service.ErrNotFound):
		writeError(w, r, http.StatusNotFound, service.ErrorCodeNotFound, nil)
		return
	case errors.Is(err, service.ErrDuplicate):
		writeError(w, r, http.StatusConflict, service.ErrorCodeDuplicate, nil)
		return
	case errors.As(err, &overlayErr):
		writeErrorDetails(w, r, http.StatusUnprocessableEntity, service.ErrorCodeOverlayInvalid, overlayErrorDetails(overlayErr), nil)
		return
	case errors.Is(err, service.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, service.ErrorCodeValidation, nil)
		return
	case errors.Is(err, service.ErrPrecondition):
		writeError(w, r, http.StatusPreconditionFailed, service.ErrorCodePreconditionFailed, nil)
		return
	case errors.Is(err, service.ErrCredentialInUse):
		writeError(w, r, http.StatusConflict, service.ErrorCodeCredentialInUse, nil)
		return
	case errors.Is(err, service.ErrIdempotencyConflict):
		writeError(w, r, http.StatusConflict, service.ErrorCodeIdempotencyConflict, nil)
		return
	case errors.Is(err, service.ErrJobNotCancellable):
		writeError(w, r, http.StatusConflict, service.ErrorCodeJobNotCancellable, nil)
		return
	case errors.Is(err, service.ErrJobNotRetryable):
		writeError(w, r, http.StatusConflict, service.ErrorCodeInvalidState, nil)
		return
	case errors.Is(err, service.ErrViewInputMismatch):
		writeError(w, r, http.StatusUnprocessableEntity, service.ErrorCodeInputSpecMismatch, nil)
		return
	case errors.Is(err, service.ErrBranchNotIndexed):
		writeError(w, r, http.StatusUnprocessableEntity, service.ErrorCodeBranchNotIndexed, nil)
		return
	case errors.Is(err, service.ErrInvalidState):
		writeError(w, r, http.StatusConflict, service.ErrorCodeInvalidState, nil)
		return
	case errors.Is(err, service.ErrBaseLayerExists):
		writeError(w, r, http.StatusConflict, service.ErrorCodeBaseLayerExists, nil)
		return
	case func() bool { _, ok := errors.AsType[*service.VersionNotPublishableError](err); return ok }():
		writeErrorDetails(w, r, http.StatusConflict, service.ErrorCodeVersionNotPublishable, map[string]any{
			"blockingRevisions": []any{}, "layerId": nil, "revisionId": nil, "reviewStatus": nil,
		}, nil)
		return
	case errors.Is(err, api.ErrStrictOperationNotImplemented):
		writeError(w, r, http.StatusNotImplemented, service.ErrorCodeInternal, nil)
		return
	}
	if _, ok := errors.AsType[*service.ProducerUnavailableError](err); ok {
		writeError(w, r, http.StatusUnprocessableEntity, service.ErrorCodeProducerUnavailable, nil)
		return
	}
	writeError(w, r, http.StatusInternalServerError, service.ErrorCodeInternal, nil)
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
				writeError(w, r, http.StatusNotFound, service.ErrorCodeNotFound, nil)
				return
			}
			status := http.StatusBadRequest
			// 已知主机密钥校验可能在领域处理器运行前失败。
			if route := options.MatchedRoute.Route; route != nil && route.Method == http.MethodPost && route.Path == "/api/v1/t/{tenantSlug}/known-hosts" {
				status = http.StatusUnprocessableEntity
			}
			writeError(w, r, status, service.ErrorCodeValidation, nil)
		},
		DoNotValidateServers: true,
		Skipper: func(r *http.Request) bool {
			// 仅对差异上传端点跳过请求体校验：kin-openapi 的 multipart 解码器无法
			// 解析 allOf 包裹的 HttpPart 标量字段（UploadRequest 的 kind/contentType），
			// 会将 part 值解码为 nil 后误报 "Value is not nullable"。该端点的领域校验
			// 由 CreateDiffUpload 服务层承担（kind 非空、contentType 枚举、大小上限）。
			if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/t/") && strings.HasSuffix(r.URL.Path, "/uploads") {
				return true
			}
			return r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && !strings.HasPrefix(r.URL.Path, "/api/")
		},
	})
	return func(next http.Handler) http.Handler {
		validated := validator(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isKnownHostCreateRequest(r) && hasDerivedKnownHostFields(r) {
				writeError(w, r, http.StatusUnprocessableEntity, service.ErrorCodeValidation, nil)
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
