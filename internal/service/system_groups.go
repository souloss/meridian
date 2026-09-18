package service

import (
	"context"
	"strings"
	"unicode/utf8"
	"uuid"
)

// SystemGroups 协调系统分组创建与成员替换。
type SystemGroups struct {
	store      SystemGroupStore
	identities IdentityStore
}

// NewSystemGroups 构造 M4 系统分组用例。
func NewSystemGroups(store SystemGroupStore, identities IdentityStore) *SystemGroups {
	return &SystemGroups{store: store, identities: identities}
}

// CreateSystemGroup 创建一个系统分组及其可选的成员服务。
func (groups *SystemGroups) CreateSystemGroup(ctx context.Context, actor Principal, tenantSlug string, input SystemGroupCreateInput) (SystemGroupRecord, error) {
	membership, err := groups.tenantMembership(ctx, actor, tenantSlug, groupPermissionManage)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	if !slugPattern.MatchString(input.Slug) || strings.TrimSpace(input.DisplayName) == "" || utf8.RuneCountInString(input.DisplayName) > maxDisplayNameRunes {
		return SystemGroupRecord{}, ErrValidation
	}
	var description *string
	if input.Description != nil {
		description = input.Description
	}
	record, err := groups.store.CreateSystemGroup(ctx, NewSystemGroup{
		TenantID: membership.TenantID, ID: uuid.NewV7(), Slug: input.Slug, DisplayName: input.DisplayName, Description: description,
	})
	if err != nil {
		return SystemGroupRecord{}, err
	}
	if len(input.ServiceIDs) > 0 {
		if err := groups.store.ReplaceSystemGroupMembers(ctx, membership.TenantID, record.ID, input.ServiceIDs); err != nil {
			return SystemGroupRecord{}, err
		}
	}
	record.ServiceIDs = input.ServiceIDs
	return record, nil
}

// PutSystemGroupMembers 原子替换一个分组的成员服务。
func (groups *SystemGroups) PutSystemGroupMembers(ctx context.Context, actor Principal, tenantSlug string, groupID uuid.UUID, etag string, serviceIDs []uuid.UUID) (SystemGroupRecord, error) {
	membership, err := groups.tenantMembership(ctx, actor, tenantSlug, groupPermissionManage)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	group, err := groups.store.GetSystemGroup(ctx, membership.TenantID, groupID)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "system-group", groupID)
	if err != nil {
		return SystemGroupRecord{}, ErrPrecondition
	}
	if expectedRevision != group.Revision {
		return SystemGroupRecord{}, ErrPrecondition
	}
	if len(serviceIDs) > 0 {
		if _, err := groups.store.ListServicesByIDs(ctx, membership.TenantID, serviceIDs); err != nil {
			return SystemGroupRecord{}, err
		}
	}
	if err := groups.store.ReplaceSystemGroupMembers(ctx, membership.TenantID, groupID, serviceIDs); err != nil {
		return SystemGroupRecord{}, err
	}
	if err := groups.store.BumpSystemGroupRevision(ctx, membership.TenantID, groupID, expectedRevision); err != nil {
		return SystemGroupRecord{}, err
	}
	group.ServiceIDs = serviceIDs
	group.Revision++
	return group, nil
}

// ListSystemGroups 列出租户内全部分组并解析其成员。
func (groups *SystemGroups) ListSystemGroups(ctx context.Context, actor Principal, tenantSlug string) ([]SystemGroupRecord, error) {
	membership, err := groups.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return nil, err
	}
	records, err := groups.store.ListSystemGroups(ctx, membership.TenantID)
	if err != nil {
		return nil, err
	}
	for index := range records {
		members, memberErr := groups.store.ListSystemGroupMembers(ctx, membership.TenantID, records[index].ID)
		if memberErr == nil {
			records[index].ServiceIDs = members
		}
	}
	return records, nil
}

// SystemGroupCreateInput 承载一次创建系统分组请求。
type SystemGroupCreateInput struct {
	Slug        string
	DisplayName string
	Description *string
	ServiceIDs  []uuid.UUID
}

func (groups *SystemGroups) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!containsString(actor.Scopes, permission) && !containsString(actor.Scopes, scopeWildcard)) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || groups.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := groups.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}
