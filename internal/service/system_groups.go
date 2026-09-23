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
	groupIDs := make([]uuid.UUID, 0, len(records))
	for _, record := range records {
		groupIDs = append(groupIDs, record.ID)
	}
	members, err := groups.store.ListSystemGroupMembersForGroups(ctx, membership.TenantID, groupIDs)
	if err != nil {
		return nil, err
	}
	for index := range records {
		records[index].ServiceIDs = members[records[index].ID]
	}
	return records, nil
}

// SystemGroupCreateInput 承载一次创建系统分组请求。
type SystemGroupCreateInput struct {
	// Slug 承载 SystemGroupCreateInput 的生成 Slug 值。
	Slug string
	// DisplayName 承载 SystemGroupCreateInput 的生成 DisplayName 值。
	DisplayName string
	// Description 承载 SystemGroupCreateInput 的生成 Description 值。
	Description *string
	// ServiceIDs 承载 SystemGroupCreateInput 的生成 ServiceIDs 值。
	ServiceIDs []uuid.UUID
}

// SystemGroupPatchInput 承载一次系统分组 PATCH 的显式字段。
type SystemGroupPatchInput struct {
	// DisplayName 是替换展示名（可为空）。
	DisplayName *string
	// Description 是替换描述（可为空）。
	Description *string
	// SetDescription 表示是否显式提供 Description（区分省略与置空）。
	SetDescription bool
}

// GetSystemGroup 返回一个系统分组及其成员服务。
func (groups *SystemGroups) GetSystemGroup(ctx context.Context, actor Principal, tenantSlug string, groupID uuid.UUID) (SystemGroupRecord, error) {
	membership, err := groups.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	record, err := groups.store.GetSystemGroup(ctx, membership.TenantID, groupID)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	members, err := groups.store.ListSystemGroupMembers(ctx, membership.TenantID, groupID)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	record.ServiceIDs = members
	return record, nil
}

// UpdateSystemGroup 在 If-Match 下更新一个系统分组的展示名与描述。
func (groups *SystemGroups) UpdateSystemGroup(ctx context.Context, actor Principal, tenantSlug string, groupID uuid.UUID, etag string, patch SystemGroupPatchInput) (SystemGroupRecord, error) {
	membership, err := groups.tenantMembership(ctx, actor, tenantSlug, groupPermissionManage)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	if patch.DisplayName == nil && !patch.SetDescription {
		return SystemGroupRecord{}, ErrValidation
	}
	if patch.DisplayName != nil && (strings.TrimSpace(*patch.DisplayName) == "" || utf8.RuneCountInString(*patch.DisplayName) > maxDisplayNameRunes) {
		return SystemGroupRecord{}, ErrValidation
	}
	current, err := groups.store.GetSystemGroup(ctx, membership.TenantID, groupID)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "system-group", groupID)
	if err != nil {
		return SystemGroupRecord{}, ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return SystemGroupRecord{}, ErrPrecondition
	}
	record, err := groups.store.UpdateSystemGroup(ctx, membership.TenantID, groupID, expectedRevision, patch)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	members, err := groups.store.ListSystemGroupMembers(ctx, membership.TenantID, groupID)
	if err != nil {
		return SystemGroupRecord{}, err
	}
	record.ServiceIDs = members
	return record, nil
}

// DeleteSystemGroup 在 If-Match 下删除一个系统分组。
func (groups *SystemGroups) DeleteSystemGroup(ctx context.Context, actor Principal, tenantSlug string, groupID uuid.UUID, etag string) error {
	membership, err := groups.tenantMembership(ctx, actor, tenantSlug, groupPermissionManage)
	if err != nil {
		return err
	}
	current, err := groups.store.GetSystemGroup(ctx, membership.TenantID, groupID)
	if err != nil {
		return err
	}
	expectedRevision, err := parseRevisionETag(etag, "system-group", groupID)
	if err != nil {
		return ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return ErrPrecondition
	}
	return groups.store.DeleteSystemGroup(ctx, membership.TenantID, groupID, expectedRevision)
}

func (groups *SystemGroups) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	return resolveTenantMembership(ctx, actor, tenantSlug, permission, groups.identities)
}
