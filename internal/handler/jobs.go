package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListPlatformJobs returns a paginated redacted job view to platform administrators.
func (s *Server) ListPlatformJobs(ctx context.Context, request api.ListPlatformJobsRequestObject) (api.ListPlatformJobsResponseObject, error) {
	if s.jobs == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.jobs.ListPlatform(ctx, principal, platformJobFilter(request.Params.Filter), page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.PlatformJob, 0, len(items))
	for _, item := range items {
		responses = append(responses, platformJobResponse(item))
	}
	return api.ListPlatformJobs200JSONResponse{PlatformJobPageJSONResponse: api.PlatformJobPageJSONResponse(api.PlatformJobPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	})}, nil
}

// GetPlatformJob returns one redacted job view to platform administrators.
func (s *Server) GetPlatformJob(ctx context.Context, request api.GetPlatformJobRequestObject) (api.GetPlatformJobResponseObject, error) {
	if s.jobs == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.jobs.GetPlatform(ctx, principal, serviceUUID(request.JobId))
	if err != nil {
		return nil, err
	}
	return api.GetPlatformJob200JSONResponse{PlatformJobJSONResponse: api.PlatformJobJSONResponse(platformJobResponse(item))}, nil
}

func platformJobFilter(value *api.PlatformJobFilters) service.PlatformJobFilter {
	if value == nil {
		return service.PlatformJobFilter{}
	}
	filter := service.PlatformJobFilter{}
	if value.ScopeId != nil {
		filter.ScopeID = *value.ScopeId
	}
	if value.ScopeType != nil {
		filter.ScopeType = string(*value.ScopeType)
	}
	if value.TenantSlug != nil {
		filter.TenantSlug = string(*value.TenantSlug)
	}
	if value.Types != nil {
		filter.Types = make([]string, len(*value.Types))
		for index, item := range *value.Types {
			filter.Types[index] = string(item)
		}
	}
	if value.Statuses != nil {
		filter.Statuses = make([]string, len(*value.Statuses))
		for index, item := range *value.Statuses {
			filter.Statuses[index] = string(item)
		}
	}
	return filter
}

func platformJobResponse(value service.PlatformJobRecord) api.PlatformJob {
	return api.PlatformJob{
		Id: api.Uuid(value.ID), TenantSlug: api.Slug(value.TenantSlug), Type: api.JobType(value.Type), Trigger: api.JobTrigger(value.Trigger),
		Status: api.JobStatus(value.Status), Stage: nullableStage(value.Stage), ScopeType: api.JobScopeType(value.ScopeType),
		ScopeId: nullableString(value.ScopeID), CreatedAt: api.Timestamp(value.CreatedAt), StartedAt: nullableTime(value.StartedAt), FinishedAt: nullableTime(value.FinishedAt),
	}
}

func nullableStage(value *string) nullable.Nullable[api.PipelineStage] {
	if value == nil {
		return nullable.NewNullNullable[api.PipelineStage]()
	}
	return nullable.NewNullableWithValue(api.PipelineStage(*value))
}

func nullableString(value *string) nullable.Nullable[string] {
	if value == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*value)
}
