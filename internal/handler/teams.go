package handler

import (
	"context"

	"github.com/meridian-labs/meridian/internal/generated/api"
	tenant "github.com/meridian-labs/meridian/internal/generated/api/tenant"
	"github.com/meridian-labs/meridian/internal/service"
)

// ListTeams 分页返回租户内团队及其成员。
func (s *Server) ListTeams(ctx context.Context, request tenant.ListTeamsRequestObject) (tenant.ListTeamsResponseObject, error) {
	if s.teams == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.teams.ListTeams(ctx, principal, string(request.TenantSlug), page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.Team, 0, len(records))
	for _, record := range records {
		items = append(items, teamResponse(record))
	}
	return tenant.ListTeams200JSONResponse(api.TeamPage{Items: items, Page: page, PageSize: pageSize, Total: int(total)}), nil
}

// CreateTeam 创建一个租户内团队。
func (s *Server) CreateTeam(ctx context.Context, request tenant.CreateTeamRequestObject) (tenant.CreateTeamResponseObject, error) {
	if s.teams == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.teams.CreateTeam(ctx, principal, string(request.TenantSlug), service.NewTeam{
		Slug: string(request.Body.Slug), DisplayName: request.Body.DisplayName,
	})
	if err != nil {
		return nil, err
	}
	body := teamResponse(record)
	return tenant.CreateTeam201JSONResponse{
		Body: body, Headers: tenant.CreateTeam201ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// GetTeam 返回一个团队及其成员。
func (s *Server) GetTeam(ctx context.Context, request tenant.GetTeamRequestObject) (tenant.GetTeamResponseObject, error) {
	if s.teams == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.teams.GetTeam(ctx, principal, string(request.TenantSlug), serviceUUID(request.TeamId))
	if err != nil {
		return nil, err
	}
	body := teamResponse(record)
	return tenant.GetTeam200JSONResponse{
		Body: body, Headers: tenant.GetTeam200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// UpdateTeam 在 If-Match 下更新团队展示名。
func (s *Server) UpdateTeam(ctx context.Context, request tenant.UpdateTeamRequestObject) (tenant.UpdateTeamResponseObject, error) {
	if s.teams == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.teams.UpdateTeam(ctx, principal, string(request.TenantSlug), request.Params.IfMatch, service.TeamPatchInput{
		ID: serviceUUID(request.TeamId), DisplayName: request.Body.DisplayName,
	})
	if err != nil {
		return nil, err
	}
	body := teamResponse(record)
	return tenant.UpdateTeam200JSONResponse{
		Body: body, Headers: tenant.UpdateTeam200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// DeleteTeam 在 If-Match 下删除一个团队。
func (s *Server) DeleteTeam(ctx context.Context, request tenant.DeleteTeamRequestObject) (tenant.DeleteTeamResponseObject, error) {
	if s.teams == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.teams.DeleteTeam(ctx, principal, string(request.TenantSlug), serviceUUID(request.TeamId), request.Params.IfMatch); err != nil {
		return nil, err
	}
	return tenant.DeleteTeam204Response{}, nil
}

// ReplaceTeamMembers 原子替换一个团队的成员集合。
func (s *Server) ReplaceTeamMembers(ctx context.Context, request tenant.ReplaceTeamMembersRequestObject) (tenant.ReplaceTeamMembersResponseObject, error) {
	if s.teams == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.teams.ReplaceTeamMembers(ctx, principal, string(request.TenantSlug), serviceUUID(request.TeamId), request.Params.IfMatch, apiUUIDs(&request.Body.UserIds))
	if err != nil {
		return nil, err
	}
	body := teamResponse(record)
	return tenant.ReplaceTeamMembers200JSONResponse{
		Body: body, Headers: tenant.ReplaceTeamMembers200ResponseHeaders{Etag: new(body.Etag)},
	}, nil
}

// ListTenantMembers 分页返回租户成员目录。
func (s *Server) ListTenantMembers(ctx context.Context, request tenant.ListTenantMembersRequestObject) (tenant.ListTenantMembersResponseObject, error) {
	if s.teams == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.teams.ListTenantMembers(ctx, principal, string(request.TenantSlug), page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.Member, 0, len(records))
	for _, record := range records {
		items = append(items, memberRecordResponse(record))
	}
	return tenant.ListTenantMembers200JSONResponse(api.MemberPage{Items: items, Page: page, PageSize: pageSize, Total: int(total)}), nil
}

// PutTenantMember 创建或替换一条租户成员关系。
func (s *Server) PutTenantMember(ctx context.Context, request tenant.PutTenantMemberRequestObject) (tenant.PutTenantMemberResponseObject, error) {
	if s.teams == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.teams.PutTenantMember(ctx, principal, string(request.TenantSlug), serviceUUID(request.UserId), string(request.Body.Role))
	if err != nil {
		return nil, err
	}
	return tenant.PutTenantMember200JSONResponse(memberRecordResponse(record)), nil
}

// DeleteTenantMember 移除一条租户成员关系。
func (s *Server) DeleteTenantMember(ctx context.Context, request tenant.DeleteTenantMemberRequestObject) (tenant.DeleteTenantMemberResponseObject, error) {
	if s.teams == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.teams.DeleteTenantMember(ctx, principal, string(request.TenantSlug), serviceUUID(request.UserId)); err != nil {
		return nil, err
	}
	return tenant.DeleteTenantMember204Response{}, nil
}

// SearchTenantUsers 在租户成员目录内按用户名/展示名搜索。
func (s *Server) SearchTenantUsers(ctx context.Context, request tenant.SearchTenantUsersRequestObject) (tenant.SearchTenantUsersResponseObject, error) {
	if s.teams == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	query := ""
	if request.Params.Q != nil {
		query = string(*request.Params.Q)
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	records, total, err := s.teams.SearchTenantUsers(ctx, principal, string(request.TenantSlug), query, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.User, 0, len(records))
	for _, record := range records {
		items = append(items, userResponse(record.User))
	}
	return tenant.SearchTenantUsers200JSONResponse(api.UserPage{Items: items, Page: page, PageSize: pageSize, Total: int(total)}), nil
}

// teamResponse 将团队记录投影为 API 形状。
func teamResponse(record service.Team) api.Team {
	memberIDs := make([]api.Uuid, 0, len(record.MemberIDs))
	for _, id := range record.MemberIDs {
		memberIDs = append(memberIDs, api.Uuid(id))
	}
	return api.Team{
		Id: api.Uuid(record.ID), Etag: revisionETag(etagKindTeam, record.ID.String(), record.Revision),
		Slug: api.Slug(record.Slug), DisplayName: record.DisplayName, MemberIds: memberIDs,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

// memberRecordResponse 将租户成员记录投影为 API 形状。
func memberRecordResponse(record service.TenantMember) api.Member {
	return api.Member{User: userResponse(record.User), Role: api.TenantRole(record.Role), JoinedAt: record.JoinedAt}
}

// etagKindTeam 是团队 ETag 的实体类型令牌。
const etagKindTeam = "team"
