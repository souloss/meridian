package handler

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"strconv"

	"github.com/meridian-labs/meridian/internal/generated/api"
	job "github.com/meridian-labs/meridian/internal/generated/api/job"
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListJobs 返回带尝试历史与能力的一页租户任务。
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

// GetJob 返回一个带持久化尝试历史的租户任务。
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

// StreamJobLogs 返回一个可续传的 SSE 流，终态后自动关闭。
func (s *Server) StreamJobLogs(ctx context.Context, request job.StreamJobLogsRequestObject) (job.StreamJobLogsResponseObject, error) {
	if s.jobs == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	afterSequence := int64(0)
	if request.Params.LastEventId != nil {
		parsed, err := strconv.ParseInt(string(*request.Params.LastEventId), sseLastEventIDBase, sseLastEventIDBits)
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
			CacheControl: sseCacheControl, XAccelBuffering: sseXAccelBuffering,
		},
	}, nil
}

// CancelJob 请求原子取消一个待处理或运行中的租户任务。
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

// RetryJob 为一个失败或取消的租户任务创建独立的重试生成。
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

// ListPlatformJobs 向平台管理员返回分页脱敏任务视图。
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

// GetPlatformJob 向平台管理员返回一个脱敏任务视图。
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

// platformJobFilter 将平台任务过滤请求投影为服务层过滤器。
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

// platformJobResponse 将平台任务记录投影为 API 形状。
func platformJobResponse(value service.PlatformJobRecord) api.PlatformJob {
	return api.PlatformJob{
		Id: api.Uuid(value.ID), TenantSlug: api.Slug(value.TenantSlug), Type: api.JobType(value.Type), Trigger: api.JobTrigger(value.Trigger),
		Status: api.JobStatus(value.Status), Stage: nullableStage(value.Stage), ScopeType: api.JobScopeType(value.ScopeType),
		ScopeId: nullableString(value.ScopeID), CreatedAt: api.Timestamp(value.CreatedAt), StartedAt: nullableTime(value.StartedAt), FinishedAt: nullableTime(value.FinishedAt),
	}
}

// tenantJobFilter 将租户任务过滤请求投影为服务层过滤器。
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

// tenantJobResponse 将租户任务记录投影为 API 形状。
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

// jobAcceptedResponse 将任务受理结果投影为 API 形状。
func jobAcceptedResponse(value service.JobAccepted) api.JobAccepted {
	return api.JobAccepted{JobId: api.Uuid(value.JobID), Deduplicated: value.Deduplicated}
}

// nullableRefType 将可空引用类型指针包装为可空 API 值。
func nullableRefType(value *string) nullable.Nullable[api.RefType] {
	if value == nil {
		return nullable.NewNullNullable[api.RefType]()
	}
	return nullable.NewNullableWithValue(api.RefType(*value))
}

// nullableRefName 将可空引用名指针包装为可空 API 值。
func nullableRefName(value *string) nullable.Nullable[api.RefName] {
	if value == nil {
		return nullable.NewNullNullable[api.RefName]()
	}
	return nullable.NewNullableWithValue(api.RefName(*value))
}

// nullableJobResult 将可空任务结果映射包装为可空 API 值。
func nullableJobResult(value map[string]any) nullable.Nullable[map[string]any] {
	if value == nil {
		return nullable.NewNullNullable[map[string]any]()
	}
	return nullable.NewNullableWithValue(value)
}

// nullableJobError 将可空任务错误包装为可空 API 值。
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

// jobSSEWriter 将任务事件流渲染为 SSE 帧。
type jobSSEWriter struct {
	writer io.Writer
}

// State 写入一个状态变更事件帧。
func (sink *jobSSEWriter) State(event service.JobStateEvent) error {
	return sink.write(sseEventState, event.Cursor, api.JobStateEvent{
		Event: api.State, Id: strconv.FormatInt(event.Cursor, sseCursorBase), At: api.Timestamp(event.At),
		Status: api.JobStatus(event.Status), Progress: event.Progress,
	})
}

// Log 写入一个日志事件帧。
func (sink *jobSSEWriter) Log(event service.JobLogRecord) error {
	return sink.write(sseEventLog, event.Sequence, api.JobLogEvent{
		Event: api.Log, Id: strconv.FormatInt(event.Sequence, sseCursorBase), At: api.Timestamp(event.OccurredAt),
		Message: event.Message, Stage: nullableStage(event.Stage),
	})
}

// Heartbeat 写入一个 SSE 心跳注释行。
func (sink *jobSSEWriter) Heartbeat() error {
	_, err := io.WriteString(sink.writer, sseHeartbeatFrame)
	return err
}

// write 序列化并写出一个 SSE 事件帧。
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

// nullableStage 将可空阶段指针包装为可空 API 值。
func nullableStage(value *string) nullable.Nullable[api.PipelineStage] {
	if value == nil {
		return nullable.NewNullNullable[api.PipelineStage]()
	}
	return nullable.NewNullableWithValue(api.PipelineStage(*value))
}

// nullableString 将可空字符串指针包装为可空 API 值。
func nullableString(value *string) nullable.Nullable[string] {
	if value == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*value)
}

const (
	// sseEventState 是任务状态变更事件的 SSE 事件名。
	sseEventState = "state"
	// sseEventLog 是任务日志事件的 SSE 事件名。
	sseEventLog = "log"
	// sseHeartbeatFrame 是 SSE 心跳注释帧（保持连接活性）。
	sseHeartbeatFrame = ": heartbeat\n\n"
	// sseCacheControl 是 SSE 日志流的 Cache-Control 头值。
	sseCacheControl = "no-cache"
	// sseXAccelBuffering 是 SSE 日志流的 X-Accel-Buffering 头值（关闭 nginx 缓冲）。
	sseXAccelBuffering = "no"
	// sseCursorBase 是 SSE 事件游标/序列号的十进制基数。
	sseCursorBase = 10
	// sseLastEventIDBase 是解析 Last-Event-Id 的十进制基数。
	sseLastEventIDBase = 10
	// sseLastEventIDBits 是解析 Last-Event-Id 的整数位宽。
	sseLastEventIDBits = 64
)
