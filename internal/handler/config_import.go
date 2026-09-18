package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	repository "github.com/meridian-labs/meridian/internal/generated/api/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// PreviewRepositoryConfigImport 校验并规范化某个 ref 处的仓库配置，
// 返回一份非权威的预览快照。
func (s *Server) PreviewRepositoryConfigImport(ctx context.Context, request repository.PreviewRepositoryConfigImportRequestObject) (repository.PreviewRepositoryConfigImportResponseObject, error) {
	if s.configImport == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	refType, refName := "", ""
	if request.Body.RefType != nil {
		refType = string(*request.Body.RefType)
	}
	if request.Body.Ref != nil {
		refName = string(*request.Body.Ref)
	}
	preview, err := s.configImport.Preview(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.RepositoryId),
		serviceUUID(api.Uuid(request.Params.IdempotencyKey)), refType, refName,
	)
	if err != nil {
		return nil, err
	}
	return repository.PreviewRepositoryConfigImport201JSONResponse(api.ConfigImportPreview{
		PreviewId: api.Uuid(preview.PreviewID), RepositoryId: api.Uuid(preview.RepositoryID),
		Commit: preview.Commit, ConfigDigest: preview.ConfigDigest,
		Services: configPreviewServices(preview.Services), Sources: configPreviewSources(preview.Sources),
		ExpiresAt: preview.ExpiresAt,
	}), nil
}

// ApplyRepositoryConfigImport 落地一份已预览的配置导入。
func (s *Server) ApplyRepositoryConfigImport(ctx context.Context, request repository.ApplyRepositoryConfigImportRequestObject) (repository.ApplyRepositoryConfigImportResponseObject, error) {
	if s.configImport == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	replaceAiBases := false
	if request.Body.ReplaceAiBases != nil {
		replaceAiBases = *request.Body.ReplaceAiBases
	}
	result, err := s.configImport.Apply(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.RepositoryId), serviceUUID(request.PreviewId),
		serviceUUID(api.Uuid(request.Params.IdempotencyKey)), request.Body.ConfigDigest, replaceAiBases,
	)
	if err != nil {
		return nil, err
	}
	created := make([]api.Uuid, 0, len(result.CreatedServices))
	for _, id := range result.CreatedServices {
		created = append(created, api.Uuid(id))
	}
	updated := make([]api.Uuid, 0, len(result.UpdatedServices))
	for _, id := range result.UpdatedServices {
		updated = append(updated, api.Uuid(id))
	}
	specs := make([]api.Uuid, 0, len(result.SourceSpecs))
	for _, id := range result.SourceSpecs {
		specs = append(specs, api.Uuid(id))
	}
	return repository.ApplyRepositoryConfigImport200JSONResponse(api.ConfigImportResult{
		RepositoryId: api.Uuid(result.RepositoryID), Commit: result.Commit, ConfigDigest: result.ConfigDigest,
		CreatedServiceIds: created, UpdatedServiceIds: updated, SourceSpecIds: specs,
	}), nil
}

// configPreviewServices 将服务记录列表投影为 API 形状。
func configPreviewServices(records []service.ServiceRecord) []api.Service {
	services := make([]api.Service, 0, len(records))
	for _, record := range records {
		services = append(services, serviceResponse(record))
	}
	return services
}

// configPreviewSources 将源配置记录列表投影为 API 形状。
func configPreviewSources(records []service.SourceSpecRecord) []api.SourceSpec {
	sources := make([]api.SourceSpec, 0, len(records))
	for _, record := range records {
		sources = append(sources, sourceSpecResponse(record))
	}
	return sources
}
