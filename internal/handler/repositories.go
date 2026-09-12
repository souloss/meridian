package handler

import (
	"context"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	repository "github.com/meridian-labs/meridian/internal/generated/api/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListRepositories returns the tenant-scoped repository page and per-item capabilities.
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

// CreateRepository validates, authorizes, and persists one tenant repository.
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

// GetRepository returns one tenant repository without exposing credential material.
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

// UpdateRepository applies a three-state patch under the caller's If-Match ETag.
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

// DeleteRepository soft-deletes one repository under its current ETag.
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

func nullableUUIDValue(value nullable.Nullable[api.Uuid]) *uuid.UUID {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	return new(serviceUUID(value.MustGet()))
}

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

func nullableStringValue(value nullable.Nullable[string]) *string {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	return new(value.MustGet())
}

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

func repositoryResponse(record service.RepositoryRecord) api.Repository {
	return api.Repository{
		Id: api.Uuid(record.ID), Etag: revisionETag("repository", record.ID.String(), record.Revision), Url: api.GitRemoteUrl(record.URL),
		CredentialId: nullableUUIDResponse(record.CredentialID), DefaultBranch: record.DefaultBranch,
		BranchPolicy: branchPolicyResponse(record.BranchPolicy), FetchConfig: fetchConfigResponse(record.FetchConfig),
		SyncCron: nullableStringResponse(record.SyncCron), Note: nullableStringResponse(record.Note), Health: repositoryHealthResponse(record.Health),
		Capabilities: api.CapabilityList(record.Capabilities), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

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

func nullableUUIDResponse(value *uuid.UUID) nullable.Nullable[api.Uuid] {
	if value == nil {
		return nullable.NewNullNullable[api.Uuid]()
	}
	return nullable.NewNullableWithValue(api.Uuid(*value))
}

func nullableStringResponse(value *string) nullable.Nullable[string] {
	if value == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*value)
}
