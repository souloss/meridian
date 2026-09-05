package handler

import (
	"context"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// ListAuditLogs returns one tenant-isolated page of redacted audit metadata.
func (s *Server) ListAuditLogs(ctx context.Context, request api.ListAuditLogsRequestObject) (api.ListAuditLogsResponseObject, error) {
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
	return api.ListAuditLogs200JSONResponse{AuditLogPageJSONResponse: api.AuditLogPageJSONResponse(auditPage(items, total, page, pageSize))}, nil
}

// ListPlatformAuditLogs returns one cross-tenant page of redacted audit metadata.
func (s *Server) ListPlatformAuditLogs(ctx context.Context, request api.ListPlatformAuditLogsRequestObject) (api.ListPlatformAuditLogsResponseObject, error) {
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
	return api.ListPlatformAuditLogs200JSONResponse{AuditLogPageJSONResponse: api.AuditLogPageJSONResponse(auditPage(items, total, page, pageSize))}, nil
}

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

func nullableUUID(value *uuid.UUID) nullable.Nullable[api.Uuid] {
	if value == nil {
		return nullable.NewNullNullable[api.Uuid]()
	}
	return nullable.NewNullableWithValue(api.Uuid(*value))
}

func nullableSlug(value *string) nullable.Nullable[api.Slug] {
	if value == nil {
		return nullable.NewNullNullable[api.Slug]()
	}
	return nullable.NewNullableWithValue(api.Slug(*value))
}

func serviceUUIDPointer(value *api.Uuid) *uuid.UUID {
	if value == nil {
		return nil
	}
	converted := uuid.UUID(*value)
	return new(converted)
}

func sliceValue(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func timeValue(value *api.Timestamp) *time.Time {
	if value == nil {
		return nil
	}
	converted := time.Time(*value)
	return new(converted)
}

func slugValue(value *api.Slug) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
