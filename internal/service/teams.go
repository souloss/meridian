package service

import (
	"context"
	"slices"
	"uuid"
)

// Teams 协调团队、团队成员与租户成员管理用例。
type Teams struct {
	store      TeamStore
	identities IdentityStore
}

// NewTeams 构造 M0 团队与成员用例。
func NewTeams(store TeamStore, identities IdentityStore) *Teams {
	return &Teams{store: store, identities: identities}
}

// CreateTeam 创建一个租户内团队。
func (teams *Teams) CreateTeam(ctx context.Context, actor Principal, tenantSlug string, input NewTeam) (Team, error) {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberWrite)
	if err != nil {
		return Team{}, err
	}
	if !validTeamSlug(input.Slug) || !validTeamDisplayName(input.DisplayName) {
		return Team{}, ErrValidation
	}
	input.TenantID = membership.TenantID
	if input.ID == uuid.Nil() {
		input.ID = uuid.NewV7()
	}
	return teams.store.CreateTeam(ctx, input)
}

// GetTeam 返回一个团队及其成员。
func (teams *Teams) GetTeam(ctx context.Context, actor Principal, tenantSlug string, teamID uuid.UUID) (Team, error) {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberRead)
	if err != nil {
		return Team{}, err
	}
	record, err := teams.store.GetTeam(ctx, membership.TenantID, teamID)
	if err != nil {
		return Team{}, err
	}
	members, err := teams.store.ListTeamMembers(ctx, membership.TenantID, teamID)
	if err != nil {
		return Team{}, err
	}
	record.MemberIDs = members
	return record, nil
}

// ListTeams 分页返回租户内团队及成员。
func (teams *Teams) ListTeams(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]Team, int64, error) {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberRead)
	if err != nil {
		return nil, 0, err
	}
	if page < 1 || pageSize < 1 || pageSize > defaultPageSizeMax {
		return nil, 0, ErrValidation
	}
	records, total, err := teams.store.ListTeams(ctx, membership.TenantID, int32(pageSize), int32((page-1)*pageSize))
	if err != nil {
		return nil, 0, err
	}
	teamIDs := make([]uuid.UUID, 0, len(records))
	for _, record := range records {
		teamIDs = append(teamIDs, record.ID)
	}
	members, err := teams.store.ListTeamMembersForTeams(ctx, membership.TenantID, teamIDs)
	if err != nil {
		return nil, 0, err
	}
	for index := range records {
		records[index].MemberIDs = members[records[index].ID]
	}
	return records, total, nil
}

// UpdateTeam 在 If-Match 下更新团队展示名。
func (teams *Teams) UpdateTeam(ctx context.Context, actor Principal, tenantSlug string, etag string, patch TeamPatchInput) (Team, error) {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberWrite)
	if err != nil {
		return Team{}, err
	}
	if patch.DisplayName == nil {
		return Team{}, ErrValidation
	}
	if !validTeamDisplayName(*patch.DisplayName) {
		return Team{}, ErrValidation
	}
	record, err := teams.store.GetTeam(ctx, membership.TenantID, patch.ID)
	if err != nil {
		return Team{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "team", patch.ID)
	if err != nil {
		return Team{}, ErrPrecondition
	}
	if expectedRevision != record.Revision {
		return Team{}, ErrPrecondition
	}
	patch.ExpectedRevision = expectedRevision
	record, err = teams.store.UpdateTeam(ctx, membership.TenantID, patch)
	if err != nil {
		return Team{}, err
	}
	members, err := teams.store.ListTeamMembers(ctx, membership.TenantID, record.ID)
	if err != nil {
		return Team{}, err
	}
	record.MemberIDs = members
	return record, nil
}

// DeleteTeam 在 If-Match 下删除一个团队（成员关系级联清理）。
func (teams *Teams) DeleteTeam(ctx context.Context, actor Principal, tenantSlug string, teamID uuid.UUID, etag string) error {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberWrite)
	if err != nil {
		return err
	}
	record, err := teams.store.GetTeam(ctx, membership.TenantID, teamID)
	if err != nil {
		return err
	}
	expectedRevision, err := parseRevisionETag(etag, "team", teamID)
	if err != nil {
		return ErrPrecondition
	}
	if expectedRevision != record.Revision {
		return ErrPrecondition
	}
	return teams.store.DeleteTeam(ctx, membership.TenantID, teamID, expectedRevision)
}

// ReplaceTeamMembers 原子替换一个团队的成员集合。
func (teams *Teams) ReplaceTeamMembers(ctx context.Context, actor Principal, tenantSlug string, teamID uuid.UUID, etag string, userIDs []uuid.UUID) (Team, error) {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberWrite)
	if err != nil {
		return Team{}, err
	}
	record, err := teams.store.GetTeam(ctx, membership.TenantID, teamID)
	if err != nil {
		return Team{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "team", teamID)
	if err != nil {
		return Team{}, ErrPrecondition
	}
	if expectedRevision != record.Revision {
		return Team{}, ErrPrecondition
	}
	// 校验成员均属于本租户，缺一即 404。
	validated, err := teams.store.ListTenantMembersByIDs(ctx, membership.TenantID, userIDs)
	if err != nil {
		return Team{}, err
	}
	if len(validated) != len(slices.Compact(slices.Clone(userIDs))) {
		return Team{}, ErrNotFound
	}
	if err := teams.store.ReplaceTeamMembers(ctx, membership.TenantID, teamID, userIDs); err != nil {
		return Team{}, err
	}
	record.MemberIDs = slices.Clone(userIDs)
	return record, nil
}

// ListTenantMembers 分页返回租户成员目录。
func (teams *Teams) ListTenantMembers(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]TenantMember, int64, error) {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberManage)
	if err != nil {
		return nil, 0, err
	}
	if page < 1 || pageSize < 1 || pageSize > defaultPageSizeMax {
		return nil, 0, ErrValidation
	}
	return teams.store.ListTenantMembers(ctx, membership.TenantID, int32(pageSize), int32((page-1)*pageSize))
}

// PutTenantMember 创建或替换一条租户成员关系（租户管理员域，非平台域）。
func (teams *Teams) PutTenantMember(ctx context.Context, actor Principal, tenantSlug string, userID uuid.UUID, role string) (TenantMember, error) {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberManage)
	if err != nil {
		return TenantMember{}, err
	}
	if !validTenantRole(role) {
		return TenantMember{}, ErrValidation
	}
	user, err := teams.identities.UserByID(ctx, userID)
	if err != nil {
		return TenantMember{}, err
	}
	record, err := teams.store.PutTenantMember(ctx, membership.TenantID, userID, role)
	if err != nil {
		return TenantMember{}, err
	}
	record.User = user
	return record, nil
}

// DeleteTenantMember 移除一条租户成员关系。
func (teams *Teams) DeleteTenantMember(ctx context.Context, actor Principal, tenantSlug string, userID uuid.UUID) error {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberManage)
	if err != nil {
		return err
	}
	return teams.store.DeleteTenantMember(ctx, membership.TenantID, userID)
}

// SearchTenantUsers 在租户成员目录内按用户名/展示名搜索。
func (teams *Teams) SearchTenantUsers(ctx context.Context, actor Principal, tenantSlug, query string, page, pageSize int) ([]TenantMember, int64, error) {
	membership, err := teams.tenantMembership(ctx, actor, tenantSlug, scopeTenantMemberManage)
	if err != nil {
		return nil, 0, err
	}
	if page < 1 || pageSize < 1 || pageSize > defaultPageSizeMax {
		return nil, 0, ErrValidation
	}
	return teams.store.SearchTenantUsers(ctx, membership.TenantID, query, int32(pageSize), int32((page-1)*pageSize))
}

func (teams *Teams) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	return resolveTenantMembership(ctx, actor, tenantSlug, permission, teams.identities)
}
