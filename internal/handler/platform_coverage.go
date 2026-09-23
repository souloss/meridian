package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
	repository "github.com/meridian-labs/meridian/internal/generated/api/repository"
	system "github.com/meridian-labs/meridian/internal/generated/api/system"
	tenant "github.com/meridian-labs/meridian/internal/generated/api/tenant"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListProducerProfiles 返回平台生产者配置分页（仅平台管理员）。
func (s *Server) ListProducerProfiles(ctx context.Context, request platform.ListProducerProfilesRequestObject) (platform.ListProducerProfilesResponseObject, error) {
	if s.producers == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.producers.List(ctx, principal, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.ProducerProfile, 0, len(records))
	for _, record := range records {
		items = append(items, producerProfileResponse(record))
	}
	return platform.ListProducerProfiles200JSONResponse(api.ProducerProfilePage{
		Total: int(total), Page: page, PageSize: pageSize, Items: items,
	}), nil
}

// GetProducerProfile 返回一个平台生产者配置（仅平台管理员）。
func (s *Server) GetProducerProfile(ctx context.Context, request platform.GetProducerProfileRequestObject) (platform.GetProducerProfileResponseObject, error) {
	if s.producers == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.producers.Get(ctx, principal, serviceUUID(request.ProducerProfileId))
	if err != nil {
		return nil, err
	}
	body := producerProfileResponse(record)
	return platform.GetProducerProfile200JSONResponse{
		Body: body, Headers: platform.GetProducerProfile200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// UpdateProducerProfile 在 If-Match 下更新一个平台生产者配置。
func (s *Server) UpdateProducerProfile(ctx context.Context, request platform.UpdateProducerProfileRequestObject) (platform.UpdateProducerProfileResponseObject, error) {
	if s.producers == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.producers.Update(ctx, principal, serviceUUID(request.ProducerProfileId), request.Params.IfMatch, producerProfilePatch(request.Body))
	if err != nil {
		return nil, err
	}
	body := producerProfileResponse(record)
	return platform.UpdateProducerProfile200JSONResponse{
		Body: body, Headers: platform.UpdateProducerProfile200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// DeleteProducerProfile 软删除一个平台生产者配置。
func (s *Server) DeleteProducerProfile(ctx context.Context, request platform.DeleteProducerProfileRequestObject) (platform.DeleteProducerProfileResponseObject, error) {
	if s.producers == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.producers.Delete(ctx, principal, serviceUUID(request.ProducerProfileId)); err != nil {
		return nil, err
	}
	return platform.DeleteProducerProfile204Response{}, nil
}

// DeleteTenant 在 If-Match 与密码确认下禁用租户并入队 tenant.delete 任务。
func (s *Server) DeleteTenant(ctx context.Context, request platform.DeleteTenantRequestObject) (platform.DeleteTenantResponseObject, error) {
	if s.identity == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	accepted, err := s.identity.DeleteTenant(ctx, principal, string(request.TenantSlug), request.Params.IfMatch, string(request.Body.ConfirmationSlug), request.Body.CurrentPassword)
	if err != nil {
		return nil, err
	}
	etag := api.ETag(revisionETag("tenant", string(request.TenantSlug), 1))
	return platform.DeleteTenant202JSONResponse{
		Body:    api.TenantDeletionAccepted{JobId: api.Uuid(accepted.JobID), Deduplicated: accepted.Deduplicated, Etag: etag},
		Headers: platform.DeleteTenant202ResponseHeaders{Etag: &etag},
	}, nil
}

// CreateTenantExport 创建一条租户导出任务记录。
func (s *Server) CreateTenantExport(ctx context.Context, request tenant.CreateTenantExportRequestObject) (tenant.CreateTenantExportResponseObject, error) {
	if s.coverage == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.coverage.CreateTenantExport(ctx, principal, string(request.TenantSlug))
	if err != nil {
		return nil, err
	}
	return tenant.CreateTenantExport202JSONResponse(api.JobAccepted{JobId: api.Uuid(record.ID), Deduplicated: false}), nil
}

// ReceiveGitWebhook 校验签名并为仓库入队一条同步任务。
func (s *Server) ReceiveGitWebhook(ctx context.Context, request repository.ReceiveGitWebhookRequestObject) (repository.ReceiveGitWebhookResponseObject, error) {
	if s.webhooks == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	var before, after *string
	if request.Body.BeforeCommit.IsSpecified() && !request.Body.BeforeCommit.IsNull() {
		value := request.Body.BeforeCommit.MustGet()
		before = &value
	}
	if request.Body.AfterCommit.IsSpecified() && !request.Body.AfterCommit.IsNull() {
		value := request.Body.AfterCommit.MustGet()
		after = &value
	}
	accepted, err := s.webhooks.Receive(ctx, serviceUUID(request.RepositoryId), string(request.Params.XMeridianSignature256), service.GitPushEvent{
		RefType: string(request.Body.RefType), Ref: string(request.Body.Ref), Deleted: request.Body.Deleted,
		BeforeCommit: before, AfterCommit: after,
	})
	if err != nil {
		return nil, err
	}
	return repository.ReceiveGitWebhook202JSONResponse(api.JobAccepted{JobId: api.Uuid(accepted.JobID), Deduplicated: accepted.Deduplicated}), nil
}

// DownloadSignedContent 校验签名内容令牌并流式返回 blob。
func (s *Server) DownloadSignedContent(ctx context.Context, request system.DownloadSignedContentRequestObject) (system.DownloadSignedContentResponseObject, error) {
	if s.diffService == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	digest, err := s.diffService.ResolveContentToken(ctx, string(request.Token))
	if err != nil {
		return nil, err
	}
	file, blob, err := s.diffService.OpenBlob(ctx, digest)
	if err != nil {
		return nil, err
	}
	return system.DownloadSignedContent200ApplicationoctetStreamResponse{Body: file, ContentLength: blob.Size}, nil
}

// producerProfilePatch 将生产者配置补丁请求投影为服务层补丁。
func producerProfilePatch(body *api.ProducerProfilePatchRequest) service.ProducerProfilePatch {
	patch := service.ProducerProfilePatch{
		Name: body.Name, Executable: body.Executable, ReplaySafe: body.ReplaySafe, Enabled: body.Enabled,
		TimeoutSec: body.TimeoutSec, MemoryMiB: body.MemoryMiB, CPUSeconds: body.CpuSeconds, Pids: body.Pids,
	}
	if body.Args != nil {
		args := append([]string(nil), *body.Args...)
		patch.Args = &args
	}
	if body.EnvAllowlist != nil {
		env := append([]string(nil), *body.EnvAllowlist...)
		patch.EnvAllowlist = &env
	}
	if body.SupportedKinds != nil {
		kinds := make([]string, 0, len(*body.SupportedKinds))
		for _, kind := range *body.SupportedKinds {
			kinds = append(kinds, string(kind))
		}
		patch.SupportedKinds = &kinds
	}
	if body.Network != nil {
		network := string(*body.Network)
		patch.Network = &network
	}
	return patch
}

var _ = nullable.NewNullNullable[api.Uuid]()
