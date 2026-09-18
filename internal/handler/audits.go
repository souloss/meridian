package handler

import (
	"context"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	collaboration "github.com/meridian-labs/meridian/internal/generated/api/collaboration"
	platform "github.com/meridian-labs/meridian/internal/generated/api/platform"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListAuditLogs 返回租户隔离的一页脱敏审计元数据。
func (s *Server) ListAuditLogs(ctx context.Context, request collaboration.ListAuditLogsRequestObject) (collaboration.ListAuditLogsResponseObject, error) {
	if s.audits == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.audits.ListTenant(ctx, principal, string(request.TenantSlug), tenantAuditFilter(request.Params.Filter), page, pageSize)
	if err != nil {
		return nil, err
	}
	return collaboration.ListAuditLogs200JSONResponse(auditPage(items, total, page, pageSize)), nil
}

// ListPlatformAuditLogs 返回跨租户的一页脱敏审计元数据。
func (s *Server) ListPlatformAuditLogs(ctx context.Context, request platform.ListPlatformAuditLogsRequestObject) (platform.ListPlatformAuditLogsResponseObject, error) {
	if s.audits == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	items, total, err := s.audits.ListPlatform(ctx, principal, platformAuditFilter(request.Params.Filter), page, pageSize)
	if err != nil {
		return nil, err
	}
	return platform.ListPlatformAuditLogs200JSONResponse(auditPage(items, total, page, pageSize)), nil
}

// tenantAuditFilter 将租户审计过滤请求投影为服务层过滤器。
func tenantAuditFilter(value *api.AuditFilters) service.AuditFilter {
	if value == nil {
		return service.AuditFilter{}
	}
	return service.AuditFilter{
		ActorID: serviceUUIDPointer(value.ActorId), Actions: sliceValue(value.Actions),
		ResourceType: stringValue(value.ResourceType), ResourceID: stringValue(value.ResourceId),
		From: timeValue(value.From), To: timeValue(value.To),
	}
}

// platformAuditFilter 将平台审计过滤请求投影为服务层过滤器。
func platformAuditFilter(value *api.PlatformAuditFilters) service.AuditFilter {
	if value == nil {
		return service.AuditFilter{}
	}
	return service.AuditFilter{
		ActorID: serviceUUIDPointer(value.ActorId), Actions: sliceValue(value.Actions),
		ResourceType: stringValue(value.ResourceType), ResourceID: stringValue(value.ResourceId),
		From: timeValue(value.From), To: timeValue(value.To), TenantSlug: slugValue(value.TenantSlug),
	}
}

// auditPage 将审计记录列表投影为 API 分页形状。
func auditPage(items []service.AuditRecord, total int64, page, pageSize int) api.AuditLogPage {
	responses := make([]api.AuditEntry, 0, len(items))
	for _, item := range items {
		responses = append(responses, api.AuditEntry{
			Id: api.Uuid(item.ID), TenantSlug: nullableSlug(item.TenantSlug), ActorId: nullableUUID(item.ActorID),
			Action: item.Action, ResourceType: item.ResourceType, ResourceId: nullableString(item.ResourceID),
			RequestId: nullableString(item.RequestID), Metadata: item.Metadata, CreatedAt: api.Timestamp(item.CreatedAt),
		})
	}
	return api.AuditLogPage{Items: responses, Page: page, PageSize: pageSize, Total: int(total)}
}

// nullableUUID 将可空 UUID 指针包装为可空 API 值。
func nullableUUID(value *uuid.UUID) nullable.Nullable[api.Uuid] {
	if value == nil {
		return nullable.NewNullNullable[api.Uuid]()
	}
	return nullable.NewNullableWithValue(api.Uuid(*value))
}

// nullableSlug 将可空租户 slug 指针包装为可空 API 值。
func nullableSlug(value *string) nullable.Nullable[api.Slug] {
	if value == nil {
		return nullable.NewNullNullable[api.Slug]()
	}
	return nullable.NewNullableWithValue(api.Slug(*value))
}

// serviceUUIDPointer 将可空 API UUID 指针转换为服务层 UUID 指针。
func serviceUUIDPointer(value *api.Uuid) *uuid.UUID {
	if value == nil {
		return nil
	}
	converted := uuid.UUID(*value)
	return new(converted)
}

// sliceValue 将可空字符串切片指针解包为切片（nil 透传）。
func sliceValue(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}

// stringValue 将可空字符串指针解包为字符串（nil 视为空串）。
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// timeValue 将可空时间戳指针转换为时间指针（nil 透传）。
func timeValue(value *api.Timestamp) *time.Time {
	if value == nil {
		return nil
	}
	converted := time.Time(*value)
	return new(converted)
}

// slugValue 将可空 slug 指针解包为字符串（nil 视为空串）。
func slugValue(value *api.Slug) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
