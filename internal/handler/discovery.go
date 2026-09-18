package handler

import (
	"context"
	"strings"
	"unicode"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	asset "github.com/meridian-labs/meridian/internal/generated/api/asset"
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
	repository "github.com/meridian-labs/meridian/internal/generated/api/repository"
	"github.com/meridian-labs/meridian/internal/generated/api/tenant"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// DiscoverRepository 入队一次仓库发现任务。
func (s *Server) DiscoverRepository(ctx context.Context, request repository.DiscoverRepositoryRequestObject) (repository.DiscoverRepositoryResponseObject, error) {
	if s.discovery == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refType, refName := string(api.RefTypeBranch), ""
	if request.Body.RefType != nil {
		refType = string(*request.Body.RefType)
	}
	if request.Body.Ref != nil {
		refName = string(*request.Body.Ref)
	}
	accepted, err := s.discovery.Discover(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.RepositoryId),
		serviceUUID(api.Uuid(request.Params.IdempotencyKey)), refType, refName,
	)
	if err != nil {
		return nil, err
	}
	return repository.DiscoverRepository202JSONResponse(jobAcceptedResponse(accepted)), nil
}

// ListDiscoveryCandidates 返回仓库的发现候选分页。
func (s *Server) ListDiscoveryCandidates(ctx context.Context, request repository.ListDiscoveryCandidatesRequestObject) (repository.ListDiscoveryCandidatesResponseObject, error) {
	if s.discovery == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.discovery.ListCandidates(ctx, principal, string(request.TenantSlug), serviceUUID(request.RepositoryId), page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.DiscoveryCandidate, 0, len(items))
	for _, item := range items {
		responses = append(responses, discoveryCandidateResponse(item))
	}
	return repository.ListDiscoveryCandidates200JSONResponse(api.CandidatePage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// AcceptDiscoveryCandidates 将待处理候选转换为服务。
func (s *Server) AcceptDiscoveryCandidates(ctx context.Context, request repository.AcceptDiscoveryCandidatesRequestObject) (repository.AcceptDiscoveryCandidatesResponseObject, error) {
	if s.discovery == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	candidateIDs := make([]uuid.UUID, 0, len(request.Body.CandidateIds))
	for _, id := range request.Body.CandidateIds {
		candidateIDs = append(candidateIDs, serviceUUID(id))
	}
	var overrides []service.CandidateOverride
	if request.Body.Overrides != nil {
		overrides = make([]service.CandidateOverride, 0, len(*request.Body.Overrides))
		for _, override := range *request.Body.Overrides {
			item := service.CandidateOverride{CandidateID: serviceUUID(override.CandidateId)}
			if override.Slug != nil {
				value := string(*override.Slug)
				item.Slug = &value
			}
			if override.DisplayName != nil {
				item.DisplayName = override.DisplayName
			}
			if override.Visibility != nil {
				value := string(*override.Visibility)
				item.Visibility = &value
			}
			overrides = append(overrides, item)
		}
	}
	services, err := s.discovery.AcceptCandidates(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.RepositoryId),
		serviceUUID(api.Uuid(request.Params.IdempotencyKey)), candidateIDs, overrides,
	)
	if err != nil {
		return nil, err
	}
	responses := make([]api.Service, 0, len(services))
	for _, item := range services {
		responses = append(responses, serviceResponse(item))
	}
	return repository.AcceptDiscoveryCandidates200JSONResponse(api.ServiceList{Items: responses}), nil
}

// ListAvailableProducerProfiles 返回可供租户源选择的生产者配置。
func (s *Server) ListAvailableProducerProfiles(ctx context.Context, request tenant.ListAvailableProducerProfilesRequestObject) (tenant.ListAvailableProducerProfilesResponseObject, error) {
	if s.producers == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	kind := ""
	if request.Params.Kind != nil {
		kind = string(*request.Params.Kind)
	}
	items, err := s.producers.ListAvailable(ctx, principal, string(request.TenantSlug), kind)
	if err != nil {
		return nil, err
	}
	responses := make([]api.ProducerProfileOption, 0, len(items))
	for _, item := range items {
		responses = append(responses, producerProfileOptionResponse(item))
	}
	return tenant.ListAvailableProducerProfiles200JSONResponse(api.ProducerProfileOptionList{Items: responses}), nil
}

// CreateProducerProfile 注册一份平台生产者配置。
func (s *Server) CreateProducerProfile(ctx context.Context, request platform.CreateProducerProfileRequestObject) (platform.CreateProducerProfileResponseObject, error) {
	if s.producers == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	timeoutSec := 0
	if request.Body.TimeoutSec != nil {
		timeoutSec = *request.Body.TimeoutSec
	}
	record, err := s.producers.Create(ctx, principal, service.NewProducerProfile{
		Name: request.Body.Name, Kind: string(request.Body.Kind), Executable: request.Body.Executable,
		Args: append([]string(nil), request.Body.Args...), EnvAllowlist: append([]string(nil), request.Body.EnvAllowlist...),
		SupportedKinds: append([]string(nil), request.Body.SupportedKinds...), ReplaySafe: request.Body.ReplaySafe,
		Network: string(request.Body.Network), TimeoutSec: timeoutSec, MemoryMiB: request.Body.MemoryMiB,
		CPUSeconds: request.Body.CpuSeconds, Pids: request.Body.Pids, Enabled: request.Body.Enabled,
	})
	if err != nil {
		return nil, err
	}
	body := producerProfileResponse(record)
	return platform.CreateProducerProfile201JSONResponse{
		Body: body, Headers: platform.CreateProducerProfile201ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// CreateSourceSpec 为服务注册一份源配置。
func (s *Server) CreateSourceSpec(ctx context.Context, request asset.CreateSourceSpecRequestObject) (asset.CreateSourceSpecResponseObject, error) {
	if s.discovery == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.discovery.CreateSourceSpec(ctx, principal, string(request.TenantSlug), string(request.ServiceSlug), sourceSpecInput(*request.Body))
	if err != nil {
		return nil, err
	}
	body := sourceSpecResponse(record)
	return asset.CreateSourceSpec201JSONResponse{
		Body: body, Headers: asset.CreateSourceSpec201ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// ListSourceSpecs 返回一个服务的有效源配置。
func (s *Server) ListSourceSpecs(ctx context.Context, request asset.ListSourceSpecsRequestObject) (asset.ListSourceSpecsResponseObject, error) {
	if s.discovery == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.discovery.ListSourceSpecs(ctx, principal, string(request.TenantSlug), string(request.ServiceSlug))
	if err != nil {
		return nil, err
	}
	responses := make([]api.SourceSpec, 0, len(items))
	for _, item := range items {
		responses = append(responses, sourceSpecResponse(item))
	}
	return asset.ListSourceSpecs200JSONResponse(api.SourceSpecList{Items: responses}), nil
}

// ListSourceBindings 返回一份源配置的当前绑定。
func (s *Server) ListSourceBindings(ctx context.Context, request asset.ListSourceBindingsRequestObject) (asset.ListSourceBindingsResponseObject, error) {
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.discovery.ListSourceBindings(ctx, principal, string(request.TenantSlug), serviceUUID(request.SourceId))
	if err != nil {
		return nil, err
	}
	responses := make([]api.SourceBinding, 0, len(items))
	for _, item := range items {
		responses = append(responses, sourceBindingResponse(item))
	}
	return asset.ListSourceBindings200JSONResponse(api.SourceBindingList{Items: responses}), nil
}

// discoveryCandidateResponse 将发现候选记录投影为 API 形状。
func discoveryCandidateResponse(record service.DiscoveryCandidateRecord) api.DiscoveryCandidate {
	detectedKinds := make([]api.KindId, 0)
	return api.DiscoveryCandidate{
		Id: api.Uuid(record.ID), Path: record.RootDir, SuggestedSlug: api.Slug(serviceSlug(record.RootDir)),
		DisplayName: serviceDisplayName(record.RootDir), Confidence: detectedCandidateConfidence, DetectedKinds: detectedKinds,
		Status: api.DiscoveryCandidateStatus(record.Status), DiscoveredAt: record.UpdatedAt,
	}
}

// serviceSlug 从仓库根目录推导稳定的小写 slug。
func serviceSlug(rootDir string) string {
	base := serviceDisplayName(rootDir)
	var builder strings.Builder
	lastDash := false
	for _, character := range strings.ToLower(base) {
		switch {
		case character >= 'a' && character <= 'z' || character >= '0' && character <= '9':
			builder.WriteRune(character)
			lastDash = false
		case character == '-' || unicode.IsSpace(character) || character == '_' || character == '.':
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.TrimSuffix(builder.String(), "-")
	if result == "" {
		return defaultServiceName
	}
	if len(result) > maxServiceSlugLength {
		result = strings.TrimSuffix(result[:maxServiceSlugLength], "-")
	}
	return result
}

// serviceDisplayName 从仓库根目录路径提取目录名作为服务显示名。
func serviceDisplayName(rootDir string) string {
	trimmed := strings.TrimSuffix(rootDir, "/")
	if trimmed == "" {
		return defaultServiceName
	}
	if index := strings.LastIndexByte(trimmed, '/'); index >= 0 {
		trimmed = trimmed[index+1:]
	}
	return trimmed
}

// producerProfileOptionResponse 将可选生产者配置投影为 API 形状。
func producerProfileOptionResponse(record service.ProducerProfileOption) api.ProducerProfileOption {
	reason := nullable.NewNullNullable[string]()
	if record.UnavailableReason != nil {
		reason = nullable.NewNullableWithValue(*record.UnavailableReason)
	}
	return api.ProducerProfileOption{
		Id: api.Uuid(record.ID), Name: record.Name, Kind: api.ProducerProfileKind(record.Kind),
		SupportedKinds: append([]api.KindId{}, record.SupportedKinds...), Network: api.ProducerNetworkMode(record.Network),
		DependencyStatus: api.ProducerDependencyStatus(record.DependencyStatus), UnavailableReason: reason,
	}
}

// producerProfileResponse 将生产者配置记录投影为 API 形状。
func producerProfileResponse(record service.ProducerProfile) api.ProducerProfile {
	reason := nullable.NewNullNullable[string]()
	if record.UnavailableReason != nil {
		reason = nullable.NewNullableWithValue(*record.UnavailableReason)
	}
	return api.ProducerProfile{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKindProducerProfile, record.ID.String(), record.Revision),
		Name: record.Name, Kind: api.ProducerProfileKind(record.Kind), Executable: record.Executable,
		Args: append([]string(nil), record.Args...), EnvAllowlist: append([]string(nil), record.EnvAllowlist...),
		SupportedKinds: append([]api.KindId{}, record.SupportedKinds...), ReplaySafe: record.ReplaySafe,
		Network: api.ProducerNetworkMode(record.Network), TimeoutSec: record.TimeoutSec, MemoryMiB: record.MemoryMiB,
		CpuSeconds: record.CPUSeconds, Pids: record.Pids, Enabled: record.Enabled,
		DependencyStatus: api.ProducerDependencyStatus(record.DependencyStatus), UnavailableReason: reason,
		Revision: int(record.Revision), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// sourceSpecResponse 将源配置记录投影为 API 形状。
func sourceSpecResponse(record service.SourceSpecRecord) api.SourceSpec {
	path := nullable.NewNullNullable[string]()
	if record.Path != nil {
		path = nullable.NewNullableWithValue(*record.Path)
	}
	profileID := nullable.NewNullNullable[api.Uuid]()
	if record.ProducerProfileID != nil {
		profileID = nullable.NewNullableWithValue(api.Uuid(*record.ProducerProfileID))
	}
	lastError := nullable.NewNullNullable[string]()
	lastRun := nullable.NewNullNullable[api.SourceRunSummary]()
	initialLayer := nullable.NewNullNullable[api.Uuid]()
	if record.InitialLayerID != nil {
		initialLayer = nullable.NewNullableWithValue(api.Uuid(*record.InitialLayerID))
	}
	return api.SourceSpec{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKindSourceSpec, record.ID.String(), record.Revision),
		ServiceId: api.Uuid(record.ServiceID), Kind: api.KindId(record.Kind), AssetNameTemplate: api.AssetNameTemplate(record.AssetNameTemplate),
		Role: api.LayerRole(record.Role), Origin: api.LayerOrigin(record.Origin), Mode: api.SourceMode(record.Mode),
		Path: path, ProducerProfileId: profileID, Ord: record.Ord, TimeoutSec: record.TimeoutSec,
		BranchPatterns: append([]api.RefGlob{}, record.BranchPatterns...), Enabled: record.Enabled,
		LastError: lastError, LastRun: lastRun, BindingsCount: record.BindingsCount, InitialLayerId: initialLayer,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// sourceBindingResponse 将源绑定记录投影为 API 形状。
func sourceBindingResponse(record service.SourceBindingRecord) api.SourceBinding {
	path := nullable.NewNullNullable[string]()
	if record.ResolvedPath != nil {
		path = nullable.NewNullableWithValue(*record.ResolvedPath)
	}
	commit := nullable.NewNullNullable[string]()
	if record.LastSeenCommit != nil {
		commit = nullable.NewNullableWithValue(*record.LastSeenCommit)
	}
	return api.SourceBinding{
		Id: api.Uuid(record.ID), SourceSpecId: api.Uuid(record.SourceSpecID),
		ScopeType: api.SourceBindingScopeType(record.ScopeType), ScopeKey: record.ScopeKey, ExpansionKey: record.ExpansionKey,
		ResolvedPath: path, AssetId: api.Uuid(record.AssetID), LayerId: api.Uuid(record.LayerID),
		State: api.SourceBindingState(record.State), LastSeenCommit: commit,
	}
}

// serviceResponse 将服务记录投影为 API 形状。
func serviceResponse(record service.ServiceRecord) api.Service {
	description := nullable.NewNullNullable[string]()
	if record.Description != nil {
		description = nullable.NewNullableWithValue(*record.Description)
	}
	language := nullable.NewNullNullable[string]()
	if record.Language != nil {
		language = nullable.NewNullableWithValue(*record.Language)
	}
	framework := nullable.NewNullNullable[string]()
	if record.Framework != nil {
		framework = nullable.NewNullableWithValue(*record.Framework)
	}
	return api.Service{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKindService, record.ID.String(), record.Revision),
		Slug: api.Slug(record.Slug), DisplayName: record.DisplayName, Description: description, RootDir: record.RootDir,
		Language: language, Framework: framework, Visibility: api.ServiceVisibility(record.Visibility),
		Lifecycle:   api.Lifecycle(record.Lifecycle),
		Repository:  api.RepositoryRef{Id: api.Uuid(record.RepositoryID)},
		Owners:      api.OwnerRefs{UserIds: []api.Uuid{}, TeamIds: []api.Uuid{}},
		Maintainers: api.OwnerRefs{UserIds: []api.Uuid{}, TeamIds: []api.Uuid{}},
		Tags:        []api.Tag{}, Assets: []api.AssetSummary{}, MissingKinds: []api.MissingKind{},
		Capabilities: api.CapabilityList{}, Starred: false, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// serviceResponseWithAssets 渲染一个服务响应，并通过资产读取用例补充资产摘要与缺失 kind。
func serviceResponseWithAssets(record service.ServiceRecord, summaries []service.AssetSummaryRecord, missing []service.MissingKindRecord) api.Service {
	response := serviceResponse(record)
	response.Assets = make([]api.AssetSummary, 0, len(summaries))
	for _, summary := range summaries {
		current := nullable.NewNullNullable[api.VersionRef]()
		if summary.CurrentVersion != nil {
			current = nullable.NewNullableWithValue(api.VersionRef{Id: api.Uuid(summary.CurrentVersion.ID), Version: summary.CurrentVersion.Version, Lifecycle: api.Lifecycle(summary.CurrentVersion.Lifecycle)})
		}
		latest := nullable.NewNullNullable[api.VersionRef]()
		if summary.LatestVersion != nil {
			latest = nullable.NewNullableWithValue(api.VersionRef{Id: api.Uuid(summary.LatestVersion.ID), Version: summary.LatestVersion.Version, Lifecycle: api.Lifecycle(summary.LatestVersion.Lifecycle)})
		}
		response.Assets = append(response.Assets, api.AssetSummary{
			Id: api.Uuid(summary.ID), Kind: api.KindId(summary.Kind), Name: api.AssetName(summary.Name),
			Lifecycle: api.Lifecycle(summary.Lifecycle), Health: api.AssetSummaryHealth(summary.Health),
			CurrentVersion: current, LatestVersion: latest, RefType: api.RefType(summary.RefType), Ref: api.RefName(summary.RefName),
		})
	}
	response.MissingKinds = make([]api.MissingKind, 0, len(missing))
	for _, kind := range missing {
		response.MissingKinds = append(response.MissingKinds, api.MissingKind{Kind: api.KindId(kind.Kind), CanConfigure: kind.CanConfigure, CanGenerateWithAi: kind.CanGenerateWithAI})
	}
	return response
}

// sourceSpecInput 将源配置创建请求投影为服务层输入。
func sourceSpecInput(body api.SourceSpecCreateRequest) service.NewSourceSpec {
	assetNameTemplate := defaultSourceAssetNameTemplate
	if body.AssetNameTemplate != nil {
		assetNameTemplate = string(*body.AssetNameTemplate)
	}
	role := string(api.Base)
	if body.Role != "" {
		role = string(body.Role)
	}
	origin := string(api.LayerOriginRepo)
	if body.Origin != "" {
		origin = string(body.Origin)
	}
	mode := string(body.Mode)
	var path *string
	if body.Path.IsSpecified() && !body.Path.IsNull() {
		path = new(body.Path.MustGet())
	}
	var profileID *uuid.UUID
	if body.ProducerProfileId.IsSpecified() && !body.ProducerProfileId.IsNull() {
		profileID = new(serviceUUID(body.ProducerProfileId.MustGet()))
	}
	var targetAssetID *uuid.UUID
	if body.TargetAssetId.IsSpecified() && !body.TargetAssetId.IsNull() {
		targetAssetID = new(serviceUUID(body.TargetAssetId.MustGet()))
	}
	ord := 0
	if body.Ord != nil {
		ord = *body.Ord
	}
	timeoutSec := 0
	if body.TimeoutSec != nil {
		timeoutSec = *body.TimeoutSec
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	branchPatterns := []string{defaultBranchPattern}
	if body.BranchPatterns != nil {
		branchPatterns = append([]string(nil), *body.BranchPatterns...)
	}
	return service.NewSourceSpec{
		Kind: string(body.Kind), AssetNameTemplate: assetNameTemplate, Role: role, Origin: origin, Mode: mode,
		Path: path, ProducerProfileID: profileID, Ord: ord, TimeoutSec: timeoutSec,
		BranchPatterns: branchPatterns, Enabled: enabled, ConfigOrigin: sourceConfigOriginAPI, TargetAssetID: targetAssetID,
		ReplaceAiBase: body.ReplaceAiBase != nil && *body.ReplaceAiBase,
	}
}

const (
	// defaultSourceAssetNameTemplate 是源配置缺省的资产名模板。
	defaultSourceAssetNameTemplate = "{file_stem}"
	// defaultBranchPattern 是源配置缺省的分支匹配模式（匹配所有分支）。
	defaultBranchPattern = "**"
	// sourceConfigOriginAPI 是 API 创建源配置的配置来源标识。
	sourceConfigOriginAPI = "api"
	// etagKindSourceSpec 是源配置 ETag 的实体类型令牌。
	etagKindSourceSpec = "source-spec"
	// etagKindProducerProfile 是生产者配置 ETag 的实体类型令牌。
	etagKindProducerProfile = "producer-profile"
	// etagKindService 是服务 ETag 的实体类型令牌。
	etagKindService = "service"
	// defaultServiceName 是空根目录/空目录名回退到的服务名。
	defaultServiceName = "service"
	// maxServiceSlugLength 是服务 slug 的最大长度（域名标签风格边界）。
	maxServiceSlugLength = 63
	// detectedCandidateConfidence 是发现候选的固定置信度（当前检测器无打分，按契约上限填充）。
	detectedCandidateConfidence = 1
)
