package handler

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"strconv"

	"github.com/meridian-labs/meridian/internal/generated/api"
	job "github.com/meridian-labs/meridian/internal/generated/api/job"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"

	// ListJobs returns one tenant-scoped page with attempt history and capabilities.
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
)

func (s *Server) ListJobs(ctx context.Context, request job.ListJobsRequestObject) (job.ListJobsResponseObject, error) {
	if s.jobs == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.jobs.ListTenant(ctx, principal, string(request.TenantSlug), tenantJobFilter(request.Params.Filter), page, pageSize)
	if err != nil {
		return nil, err
	}
	responses := make([]api.Job, len(items))
	for index, item := range items {
		responses[index] = tenantJobResponse(item)
	}
	return job.ListJobs200JSONResponse(api.JobPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// GetJob returns one tenant-scoped job with persisted attempt history.
func (s *Server) GetJob(ctx context.Context, request job.GetJobRequestObject) (job.GetJobResponseObject, error) {
	if s.jobs == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.jobs.GetTenant(ctx, principal, string(request.TenantSlug), serviceUUID(request.JobId))
	if err != nil {
		return nil, err
	}
	return job.GetJob200JSONResponse(tenantJobResponse(item)), nil
}

// StreamJobLogs returns a resumable SSE body that closes after a terminal state.
func (s *Server) StreamJobLogs(ctx context.Context, request job.StreamJobLogsRequestObject) (job.StreamJobLogsResponseObject, error) {
	if s.jobs == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	afterSequence := int64(0)
	if request.Params.LastEventId != nil {
		parsed, err := strconv.ParseInt(string(*request.Params.LastEventId), 10, 64)
		if err != nil || parsed < 0 {
			return nil, service.ErrValidation
		}
		afterSequence = parsed
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := s.jobs.OpenTenantStream(ctx, principal, string(request.TenantSlug), serviceUUID(request.JobId), afterSequence)
	if err != nil {
		return nil, err
	}
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		_ = stream.Run(ctx, &jobSSEWriter{writer: writer})
	}()
	return job.StreamJobLogs200TexteventStreamResponse{
		Body: reader,
		Headers: job.StreamJobLogs200ResponseHeaders{
			CacheControl: "no-cache", XAccelBuffering: "no",
		},
	}, nil
}

// CancelJob requests atomic cancellation of a pending or running tenant job.
func (s *Server) CancelJob(ctx context.Context, request job.CancelJobRequestObject) (job.CancelJobResponseObject, error) {
	if s.jobs == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	accepted, err := s.jobs.CancelTenant(ctx, principal, string(request.TenantSlug), serviceUUID(request.JobId))
	if err != nil {
		return nil, err
	}
	return job.CancelJob202JSONResponse(jobAcceptedResponse(accepted)), nil
}

// RetryJob creates an independent generation for one failed or cancelled tenant job.
func (s *Server) RetryJob(ctx context.Context, request job.RetryJobRequestObject) (job.RetryJobResponseObject, error) {
	if s.jobs == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	accepted, err := s.jobs.RetryTenant(
		ctx, principal, string(request.TenantSlug), serviceUUID(request.JobId), serviceUUID(api.Uuid(request.Params.IdempotencyKey)),
	)
	if err != nil {
		return nil, err
	}
	return job.RetryJob202JSONResponse(jobAcceptedResponse(accepted)), nil
}

// ListPlatformJobs returns a paginated redacted job view to platform administrators.
func (s *Server) ListPlatformJobs(ctx context.Context, request platform.ListPlatformJobsRequestObject) (platform.ListPlatformJobsResponseObject, error) {
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
	return platform.ListPlatformJobs200JSONResponse(api.PlatformJobPage{
		Items: responses, Page: page, PageSize: pageSize, Total: int(total),
	}), nil
}

// GetPlatformJob returns one redacted job view to platform administrators.
func (s *Server) GetPlatformJob(ctx context.Context, request platform.GetPlatformJobRequestObject) (platform.GetPlatformJobResponseObject, error) {
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
	return platform.GetPlatformJob200JSONResponse(platformJobResponse(item)), nil
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

func tenantJobFilter(value *api.JobFilters) service.JobFilter {
	if value == nil {
		return service.JobFilter{}
	}
	filter := service.JobFilter{}
	if value.ScopeId != nil {
		filter.ScopeID = *value.ScopeId
	}
	if value.ScopeType != nil {
		filter.ScopeType = string(*value.ScopeType)
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

func tenantJobResponse(value service.JobRecord) api.Job {
	attempts := make([]api.JobStageAttempt, len(value.Attempts))
	for index, attempt := range value.Attempts {
		attempts[index] = api.JobStageAttempt{
			Stage: api.PipelineStage(attempt.Stage), Attempt: attempt.Attempt, Status: api.JobStatus(attempt.Status),
			StartedAt: nullableTime(attempt.StartedAt), FinishedAt: nullableTime(attempt.FinishedAt), Error: nullableJobError(attempt.Error),
		}
	}
	return api.Job{
		Id: api.Uuid(value.ID), TenantSlug: api.Slug(value.TenantSlug), RetryOfJobId: nullableUUID(value.RetryOfJobID),
		Type: api.JobType(value.Type), Trigger: api.JobTrigger(value.Trigger), Status: api.JobStatus(value.Status),
		Stage: nullableStage(value.Stage), ScopeType: api.JobScopeType(value.ScopeType), ScopeId: nullableString(value.ScopeID),
		RefType: nullableRefType(value.RefType), Ref: nullableRefName(value.Ref), Result: nullableJobResult(value.Result),
		Progress: value.Progress, Dirty: value.Dirty, Attempt: value.Attempt, MaxAttempts: value.MaxAttempts,
		NextAttemptAt: nullableTime(value.NextAttemptAt), Attempts: attempts, Error: nullableJobError(value.Error),
		CreatedAt: api.Timestamp(value.CreatedAt), StartedAt: nullableTime(value.StartedAt), FinishedAt: nullableTime(value.FinishedAt),
		Capabilities: api.CapabilityList(value.Capabilities),
	}
}

func jobAcceptedResponse(value service.JobAccepted) api.JobAccepted {
	return api.JobAccepted{JobId: api.Uuid(value.JobID), Deduplicated: value.Deduplicated}
}

func nullableRefType(value *string) nullable.Nullable[api.RefType] {
	if value == nil {
		return nullable.NewNullNullable[api.RefType]()
	}
	return nullable.NewNullableWithValue(api.RefType(*value))
}

func nullableRefName(value *string) nullable.Nullable[api.RefName] {
	if value == nil {
		return nullable.NewNullNullable[api.RefName]()
	}
	return nullable.NewNullableWithValue(api.RefName(*value))
}

func nullableJobResult(value map[string]any) nullable.Nullable[map[string]any] {
	if value == nil {
		return nullable.NewNullNullable[map[string]any]()
	}
	return nullable.NewNullableWithValue(value)
}

func nullableJobError(value *service.JobError) nullable.Nullable[api.ErrorResponse] {
	if value == nil {
		return nullable.NewNullNullable[api.ErrorResponse]()
	}
	details := nullable.NewNullNullable[map[string]any]()
	if value.Details != nil {
		details = nullable.NewNullableWithValue(value.Details)
	}
	return nullable.NewNullableWithValue(api.ErrorResponse{
		Code: api.ErrorCode(value.Code), Message: value.Message, RequestId: value.RequestID, Details: details,
	})
}

type jobSSEWriter struct {
	writer io.Writer
}

func (sink *jobSSEWriter) State(event service.JobStateEvent) error {
	return sink.write("state", event.Cursor, api.JobStateEvent{
		Event: api.State, Id: strconv.FormatInt(event.Cursor, 10), At: api.Timestamp(event.At),
		Status: api.JobStatus(event.Status), Progress: event.Progress,
	})
}

func (sink *jobSSEWriter) Log(event service.JobLogRecord) error {
	return sink.write("log", event.Sequence, api.JobLogEvent{
		Event: api.Log, Id: strconv.FormatInt(event.Sequence, 10), At: api.Timestamp(event.OccurredAt),
		Message: event.Message, Stage: nullableStage(event.Stage),
	})
}

func (sink *jobSSEWriter) Heartbeat() error {
	_, err := io.WriteString(sink.writer, ": heartbeat\n\n")
	return err
}

func (sink *jobSSEWriter) write(kind string, cursor int64, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	frame := fmt.Appendf(nil, "id: %d\nevent: %s\ndata: ", cursor, kind)
	frame = append(frame, payload...)
	frame = append(frame, '\n', '\n')
	_, err = sink.writer.Write(frame)
	return err
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
