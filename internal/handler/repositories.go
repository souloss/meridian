package handler

import (
	"context"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	repository "github.com/meridian-labs/meridian/internal/generated/api/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListRepositories 返回租户范围的仓库分页与逐项能力。
func (s *Server) ListRepositories(ctx context.Context, request repository.ListRepositoriesRequestObject) (repository.ListRepositoriesResponseObject, error) {
	if s.repositories == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	query := ""
	if request.Params.Q != nil {
		query = *request.Params.Q
	}
	items, total, err := s.repositories.List(ctx, principal, request.TenantSlug, query, page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.Repository, 0, len(items))
	for _, item := range items {
		responses = append(responses, repositoryResponse(item))
	}
	return repository.ListRepositories200JSONResponse(api.RepositoryPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// CreateRepository 校验、授权并持久化一个租户仓库。
func (s *Server) CreateRepository(ctx context.Context, request repository.CreateRepositoryRequestObject) (repository.CreateRepositoryResponseObject, error) {
	if s.repositories == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	created, err := s.repositories.Create(ctx, principal, request.TenantSlug, newRepositoryInput(*request.Body))
	if err != nil {
		return nil, err
	}
	body := repositoryResponse(created)
	return repository.CreateRepository201JSONResponse{Body: body, Headers: repository.CreateRepository201ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// GetRepository 返回一个租户仓库（不暴露凭据材料）。
func (s *Server) GetRepository(ctx context.Context, request repository.GetRepositoryRequestObject) (repository.GetRepositoryResponseObject, error) {
	if s.repositories == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.repositories.Get(ctx, principal, request.TenantSlug, serviceUUID(request.RepositoryId))
	if err != nil {
		return nil, err
	}
	body := repositoryResponse(record)
	return repository.GetRepository200JSONResponse{Body: body, Headers: repository.GetRepository200ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// UpdateRepository 在调用者 If-Match ETag 下应用三态补丁。
func (s *Server) UpdateRepository(ctx context.Context, request repository.UpdateRepositoryRequestObject) (repository.UpdateRepositoryResponseObject, error) {
	if s.repositories == nil || request.Body == nil {
		return nil, service.ErrValidation
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := s.repositories.Update(ctx, principal, request.TenantSlug, serviceUUID(request.RepositoryId), request.Params.IfMatch, repositoryPatchInput(*request.Body))
	if err != nil {
		return nil, err
	}
	body := repositoryResponse(updated)
	return repository.UpdateRepository200JSONResponse{Body: body, Headers: repository.UpdateRepository200ResponseHeaders{Etag: new(body.Etag)}}, nil
}

// DeleteRepository 在当前 ETag 下软删除一个仓库。
func (s *Server) DeleteRepository(ctx context.Context, request repository.DeleteRepositoryRequestObject) (repository.DeleteRepositoryResponseObject, error) {
	if s.repositories == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.repositories.Delete(ctx, principal, request.TenantSlug, serviceUUID(request.RepositoryId), request.Params.IfMatch); err != nil {
		return nil, err
	}
	return repository.DeleteRepository204Response{}, nil
}

// newRepositoryInput 将仓库创建请求投影为服务层输入。
func newRepositoryInput(body api.RepositoryCreateRequest) service.NewRepositoryInput {
	return service.NewRepositoryInput{
		URL:           string(body.Url),
		CredentialID:  nullableUUIDValue(body.CredentialId),
		DefaultBranch: body.DefaultBranch,
		BranchPolicy:  branchPolicyInput(body.BranchPolicy),
		FetchConfig:   fetchConfigInput(body.FetchConfig),
		SyncCron:      nullableStringValue(body.SyncCron),
		Note:          nullableStringValue(body.Note),
	}
}

// repositoryPatchInput 将仓库补丁请求投影为服务层输入。
func repositoryPatchInput(body api.RepositoryPatchRequest) service.RepositoryPatchInput {
	return service.RepositoryPatchInput{
		CredentialID:  nullableUUIDPatch(body.CredentialId),
		DefaultBranch: body.DefaultBranch,
		BranchPolicy:  branchPolicyInput(body.BranchPolicy),
		FetchConfig:   fetchConfigInput(body.FetchConfig),
		SyncCron:      nullableStringPatch(body.SyncCron),
		Note:          nullableStringPatch(body.Note),
	}
}

// branchPolicyInput 将可空分支策略请求投影为服务层输入（nil 透传）。
func branchPolicyInput(value *api.BranchPolicy) *service.RepositoryBranchPolicy {
	if value == nil {
		return nil
	}
	// 复制请求切片，避免服务层持有或修改 HTTP DTO 的底层数组。
	branchPatterns := make([]string, len(value.BranchPatterns))
	copy(branchPatterns, value.BranchPatterns)
	tagPatterns := make([]string, len(value.TagPatterns))
	copy(tagPatterns, value.TagPatterns)
	return new(service.RepositoryBranchPolicy{BranchPatterns: branchPatterns, TagPatterns: tagPatterns})
}

// fetchConfigInput 将可空抓取配置请求投影为服务层输入（nil 透传）。
func fetchConfigInput(value *api.FetchConfig) *service.RepositoryFetchConfig {
	if value == nil {
		return nil
	}
	var depth *int
	if value.Depth.IsSpecified() && !value.Depth.IsNull() {
		depth = new(value.Depth.MustGet())
	}
	var proxy *string
	if value.Proxy.IsSpecified() && !value.Proxy.IsNull() {
		proxy = new(value.Proxy.MustGet())
	}
	return new(service.RepositoryFetchConfig{
		Shallow: value.Shallow, Depth: depth, Submodules: value.Submodules, Proxy: proxy,
		PathAllow: append([]string(nil), value.PathAllow...), PathIgnore: append([]string(nil), value.PathIgnore...),
		KnownHostPolicy: string(value.KnownHostPolicy),
	})
}

// nullableUUIDValue 将可空 API UUID 包装为服务层 UUID 指针。
func nullableUUIDValue(value nullable.Nullable[api.Uuid]) *uuid.UUID {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	return new(serviceUUID(value.MustGet()))
}

// nullableUUIDPatch 将可空 API UUID 包装为服务层可空补丁指针（区分「未指定」与「置空」）。
func nullableUUIDPatch(value nullable.Nullable[api.Uuid]) **uuid.UUID {
	if !value.IsSpecified() {
		return nil
	}
	var id *uuid.UUID
	if !value.IsNull() {
		id = new(serviceUUID(value.MustGet()))
	}
	return new(id)
}

// nullableStringValue 将可空字符串包装为指针。
func nullableStringValue(value nullable.Nullable[string]) *string {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	return new(value.MustGet())
}

// nullableStringPatch 将可空字符串包装为服务层可空补丁指针（区分「未指定」与「置空」）。
func nullableStringPatch(value nullable.Nullable[string]) **string {
	if !value.IsSpecified() {
		return nil
	}
	var text *string
	if !value.IsNull() {
		text = new(value.MustGet())
	}
	return new(text)
}

// repositoryResponse 将仓库记录投影为 API 形状。
func repositoryResponse(record service.RepositoryRecord) api.Repository {
	return api.Repository{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKindRepository, record.ID.String(), record.Revision), Url: api.GitRemoteUrl(record.URL),
		CredentialId: nullableUUIDResponse(record.CredentialID), DefaultBranch: record.DefaultBranch,
		BranchPolicy: branchPolicyResponse(record.BranchPolicy), FetchConfig: fetchConfigResponse(record.FetchConfig),
		SyncCron: nullableStringResponse(record.SyncCron), Note: nullableStringResponse(record.Note), Health: repositoryHealthResponse(record.Health),
		Capabilities: api.CapabilityList(record.Capabilities), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// branchPolicyResponse 将分支策略投影为 API 形状。
func branchPolicyResponse(value service.RepositoryBranchPolicy) api.BranchPolicy {
	branches := make([]api.RefGlob, len(value.BranchPatterns))
	for index, pattern := range value.BranchPatterns {
		branches[index] = api.RefGlob(pattern)
	}
	tags := make([]api.RefGlob, len(value.TagPatterns))
	for index, pattern := range value.TagPatterns {
		tags[index] = api.RefGlob(pattern)
	}
	return api.BranchPolicy{BranchPatterns: branches, TagPatterns: tags}
}

// fetchConfigResponse 将抓取配置投影为 API 形状。
func fetchConfigResponse(value service.RepositoryFetchConfig) api.FetchConfig {
	depth := nullable.NewNullNullable[int]()
	if value.Depth != nil {
		depth = nullable.NewNullableWithValue(*value.Depth)
	}
	proxy := nullable.NewNullNullable[string]()
	if value.Proxy != nil {
		proxy = nullable.NewNullableWithValue(*value.Proxy)
	}
	return api.FetchConfig{
		Shallow: value.Shallow, Depth: depth, Submodules: value.Submodules, Proxy: proxy,
		PathAllow: append([]string(nil), value.PathAllow...), PathIgnore: append([]string(nil), value.PathIgnore...),
		KnownHostPolicy: api.FetchConfigKnownHostPolicy(value.KnownHostPolicy),
	}
}

// repositoryHealthResponse 将仓库健康信息投影为 API 形状。
func repositoryHealthResponse(value service.RepositoryHealth) api.RepositoryHealth {
	lastSyncAt := nullable.NewNullNullable[api.Timestamp]()
	if value.LastSyncAt != nil {
		lastSyncAt = nullable.NewNullableWithValue(api.Timestamp(*value.LastSyncAt))
	}
	lastCommit := nullable.NewNullNullable[string]()
	if value.LastCommit != nil {
		lastCommit = nullable.NewNullableWithValue(*value.LastCommit)
	}
	lastError := nullable.NewNullNullable[api.RepositoryError]()
	if value.LastError != nil {
		lastError = nullable.NewNullableWithValue(api.RepositoryError{Class: api.RepositoryErrorClass(value.LastError.Class), Message: value.LastError.Message})
	}
	duration := nullable.NewNullNullable[int]()
	if value.DurationMS != nil {
		duration = nullable.NewNullableWithValue(*value.DurationMS)
	}
	return api.RepositoryHealth{LastSyncAt: lastSyncAt, LastCommit: lastCommit, LastError: lastError, FailStreak: value.FailStreak, DurationMs: duration}
}

// nullableUUIDResponse 将可空 UUID 指针包装为可空 API 值。
func nullableUUIDResponse(value *uuid.UUID) nullable.Nullable[api.Uuid] {
	if value == nil {
		return nullable.NewNullNullable[api.Uuid]()
	}
	return nullable.NewNullableWithValue(api.Uuid(*value))
}

// nullableStringResponse 将可空字符串指针包装为可空 API 值。
func nullableStringResponse(value *string) nullable.Nullable[string] {
	if value == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*value)
}

const (
	// etagKindRepository 是仓库 ETag 的实体类型令牌。
	etagKindRepository = "repository"
)
