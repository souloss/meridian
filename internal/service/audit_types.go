package service

import (
	"context"
	"time"
	"uuid"
)

// AuditRecord 是 API 暴露的脱敏追加型审计投影。
type AuditRecord struct {
	// ID 唯一标识该审计事实。
	ID uuid.UUID
	// TenantSlug 标识所属租户；平台级事实为 nil。
	TenantSlug *string
	// ActorID 标识用户操作者（存在时）。
	ActorID *uuid.UUID
	// Action 是稳定的被审计操作标识。
	Action string
	// ResourceType 标识受影响的资源类别。
	ResourceType string
	// ResourceID 标识受影响的资源（存在时）。
	ResourceID *string
	// RequestID 在存在时将事实关联到 HTTP 请求。
	RequestID *string
	// Metadata 仅包含脱敏标识、计数与哈希。
	Metadata map[string]any
	// CreatedAt 是事实提交的 UTC 事务时间。
	CreatedAt time.Time
}

// AuditFilter 使用有界、类型化的谓词选择审计元数据。
type AuditFilter struct {
	// ActorID 在非空时将结果限定为一个用户操作者。
	ActorID *uuid.UUID
	// Actions 将结果限定为精确的稳定操作标识。
	Actions []string
	// ResourceType 将结果限定为一个精确的资源类别。
	ResourceType string
	// ResourceID 将结果限定为一个精确的资源标识。
	ResourceID string
	// From 在非空时包含该时刻或之后创建的事实。
	From *time.Time
	// To 在非空时包含该时刻或之前创建的事实。
	To *time.Time
	// TenantSlug 在非空时将平台结果限定为一个租户。
	TenantSlug string
}

// AuditStore 是脱敏审计查询的持久化边界。
type AuditStore interface {
	ListTenantAudits(context.Context, uuid.UUID, AuditFilter, int32, int32) ([]AuditRecord, int64, error)
	ListPlatformAudits(context.Context, AuditFilter, int32, int32) ([]AuditRecord, int64, error)
}
