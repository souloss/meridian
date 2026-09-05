package repository

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"time"
	"uuid"

	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// ListTenantAudits returns a redacted page constrained by an explicit tenant identifier.
func (store *RepositoryStore) ListTenantAudits(ctx context.Context, tenantID uuid.UUID, filter service.AuditFilter, limit, offset int32) ([]service.AuditRecord, int64, error) {
	values := auditFilterValues(filter)
	total, err := store.queries.CountTenantAuditLogs(ctx, generated.CountTenantAuditLogsParams{
		TenantID: new(tenantID), ActorIDSet: values.actorIDSet, ActorID: values.actorID,
		ActionFilter: filter.Actions, TargetType: filter.ResourceType, TargetID: filter.ResourceID,
		FromSet: values.fromSet, FromTime: timestamp(values.fromTime), ToSet: values.toSet, ToTime: timestamp(values.toTime),
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListTenantAuditLogs(ctx, generated.ListTenantAuditLogsParams{
		TenantID: new(tenantID), ActorIDSet: values.actorIDSet, ActorID: values.actorID,
		ActionFilter: filter.Actions, TargetType: filter.ResourceType, TargetID: filter.ResourceID,
		FromSet: values.fromSet, FromTime: timestamp(values.fromTime), ToSet: values.toSet, ToTime: timestamp(values.toTime),
		PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.AuditRecord, 0, len(rows))
	for _, row := range rows {
		item, err := auditRecord(row.ID, row.TenantSlug, row.ActorID, row.Action, row.TargetType, row.TargetID, row.RequestID, row.Detail, row.CreatedAt.Time)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

// ListPlatformAudits returns a redacted cross-tenant audit page for the control plane.
func (store *RepositoryStore) ListPlatformAudits(ctx context.Context, filter service.AuditFilter, limit, offset int32) ([]service.AuditRecord, int64, error) {
	values := auditFilterValues(filter)
	total, err := store.queries.CountPlatformAuditLogs(ctx, generated.CountPlatformAuditLogsParams{
		ActorIDSet: values.actorIDSet, ActorID: values.actorID, ActionFilter: filter.Actions,
		TargetType: filter.ResourceType, TargetID: filter.ResourceID, FromSet: values.fromSet,
		FromTime: timestamp(values.fromTime), ToSet: values.toSet, ToTime: timestamp(values.toTime), TenantSlug: filter.TenantSlug,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListPlatformAuditLogs(ctx, generated.ListPlatformAuditLogsParams{
		ActorIDSet: values.actorIDSet, ActorID: values.actorID, ActionFilter: filter.Actions,
		TargetType: filter.ResourceType, TargetID: filter.ResourceID, FromSet: values.fromSet,
		FromTime: timestamp(values.fromTime), ToSet: values.toSet, ToTime: timestamp(values.toTime), TenantSlug: filter.TenantSlug,
		PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.AuditRecord, 0, len(rows))
	for _, row := range rows {
		item, err := auditRecord(row.ID, row.TenantSlug, row.ActorID, row.Action, row.TargetType, row.TargetID, row.RequestID, row.Detail, row.CreatedAt.Time)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

type auditValues struct {
	actorIDSet bool
	actorID    uuid.UUID
	fromSet    bool
	fromTime   time.Time
	toSet      bool
	toTime     time.Time
}

func auditFilterValues(filter service.AuditFilter) auditValues {
	values := auditValues{}
	if filter.ActorID != nil {
		values.actorIDSet = true
		values.actorID = *filter.ActorID
	}
	if filter.From != nil {
		values.fromSet = true
		values.fromTime = *filter.From
	}
	if filter.To != nil {
		values.toSet = true
		values.toTime = *filter.To
	}
	return values
}

func auditRecord(id uuid.UUID, tenantSlug string, actorID *uuid.UUID, action, targetType, targetID, requestID string, detail []byte, createdAt time.Time) (service.AuditRecord, error) {
	metadata := make(map[string]any)
	if err := json.Unmarshal(detail, &metadata); err != nil {
		return service.AuditRecord{}, fmt.Errorf("decode redacted audit metadata: %w", err)
	}
	return service.AuditRecord{
		ID: id, TenantSlug: optionalText(tenantSlug), ActorID: actorID, Action: action,
		ResourceType: targetType, ResourceID: optionalText(targetID), RequestID: optionalText(requestID),
		Metadata: metadata, CreatedAt: createdAt,
	}, nil
}

var _ service.AuditStore = (*RepositoryStore)(nil)
