package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

// Team 是租户内用于授权与凭据共享的团队投影。
type Team struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是团队的标识。
	ID uuid.UUID
	// Slug 是租户内唯一的小写团队标识。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
	// MemberIDs 是团队成员用户 id（按 id 升序）。
	MemberIDs []uuid.UUID
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// TenantMember 是一个租户成员的脱敏用户投影。
type TenantMember struct {
	// User 是成员的用户数据。
	User User
	// Role 是成员角色。
	Role string
	// JoinedAt 是加入时间。
	JoinedAt time.Time
}

// NewTeam 承载一次团队创建请求。
type NewTeam struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是团队的标识。
	ID uuid.UUID
	// Slug 是租户内唯一的小写团队标识。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
}

// TeamPatchInput 承载一次团队 PATCH 的显式字段。
type TeamPatchInput struct {
	// ID 是团队的标识。
	ID uuid.UUID
	// ExpectedRevision 是变更所需的 ETag 版本号。
	ExpectedRevision int64
	// DisplayName 是可选的替换展示名。
	DisplayName *string
}

// TeamStore 是团队、成员关系与租户成员管理的持久化边界。
type TeamStore interface {
	// CreateTeam 承载 TeamStore 的生成 CreateTeam 值。
	CreateTeam(context.Context, NewTeam) (Team, error)
	// GetTeam 承载 TeamStore 的生成 GetTeam 值。
	GetTeam(context.Context, uuid.UUID, uuid.UUID) (Team, error)
	// ListTeams 承载 TeamStore 的生成 ListTeams 值。
	ListTeams(context.Context, uuid.UUID, int32, int32) ([]Team, int64, error)
	// ListTeamMembers 承载 TeamStore 的生成 ListTeamMembers 值。
	ListTeamMembers(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error)
	// ListTeamMembersForTeams 承载 TeamStore 的生成 ListTeamMembersForTeams 值。
	ListTeamMembersForTeams(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	// ReplaceTeamMembers 承载 TeamStore 的生成 ReplaceTeamMembers 值。
	ReplaceTeamMembers(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) error
	// UpdateTeam 承载 TeamStore 的生成 UpdateTeam 值。
	UpdateTeam(context.Context, uuid.UUID, TeamPatchInput) (Team, error)
	// DeleteTeam 承载 TeamStore 的生成 DeleteTeam 值。
	DeleteTeam(context.Context, uuid.UUID, uuid.UUID, int64) error
	// ListTenantMembers 承载 TeamStore 的生成 ListTenantMembers 值。
	ListTenantMembers(context.Context, uuid.UUID, int32, int32) ([]TenantMember, int64, error)
	// PutTenantMember 承载 TeamStore 的生成 PutTenantMember 值。
	PutTenantMember(context.Context, uuid.UUID, uuid.UUID, string) (TenantMember, error)
	// DeleteTenantMember 承载 TeamStore 的生成 DeleteTenantMember 值。
	DeleteTenantMember(context.Context, uuid.UUID, uuid.UUID) error
	// SearchTenantUsers 承载 TeamStore 的生成 SearchTenantUsers 值。
	SearchTenantUsers(context.Context, uuid.UUID, string, int32, int32) ([]TenantMember, int64, error)
	// ListTenantMembersByIDs 校验并返回同租户内的用户 id 子集。
	ListTenantMembersByIDs(context.Context, uuid.UUID, []uuid.UUID) ([]uuid.UUID, error)
}

// validTeamSlug 复用租户级 slug 校验（slugPattern）。
func validTeamSlug(slug string) bool {
	return slugPattern.MatchString(slug)
}

// validTeamDisplayName 校验团队展示名非空且不超长。
func validTeamDisplayName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= maxDisplayNameRunes
}
