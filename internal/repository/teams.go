package repository

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// TeamStore 基于 sqlc 与 pgx 实现 service.TeamStore 团队与成员持久化端口。
type TeamStore struct {
	pool    *pgxpool.Pool
	queries *generated.Queries
}

// NewTeamStore 将团队持久化绑定到原生 pgx 连接池。
func NewTeamStore(pool *pgxpool.Pool) *TeamStore {
	return &TeamStore{pool: pool, queries: generated.New(pool)}
}

// CreateTeam 插入一条团队。
func (store *TeamStore) CreateTeam(ctx context.Context, input service.NewTeam) (service.Team, error) {
	row, err := store.queries.CreateTeam(ctx, generated.CreateTeamParams{
		TenantID: input.TenantID, ID: input.ID, Slug: input.Slug, DisplayName: input.DisplayName,
	})
	if err != nil {
		return service.Team{}, normalizeError(err)
	}
	return teamFromRow(row), nil
}

// GetTeam 按 id 返回一条团队。
func (store *TeamStore) GetTeam(ctx context.Context, tenantID, id uuid.UUID) (service.Team, error) {
	row, err := store.queries.GetTeam(ctx, generated.GetTeamParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.Team{}, normalizeError(err)
	}
	return teamFromRow(row), nil
}

// ListTeams 分页返回租户内团队及总数。
func (store *TeamStore) ListTeams(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]service.Team, int64, error) {
	total, err := store.queries.CountTeams(ctx, tenantID)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListTeams(ctx, generated.ListTeamsParams{TenantID: tenantID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	teams := make([]service.Team, 0, len(rows))
	for _, row := range rows {
		teams = append(teams, teamFromRow(row))
	}
	return teams, total, nil
}

// ListTeamMembers 返回一个团队的成员用户 id。
func (store *TeamStore) ListTeamMembers(ctx context.Context, tenantID, teamID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := store.queries.ListTeamMembers(ctx, generated.ListTeamMembersParams{TenantID: tenantID, TeamID: teamID})
	if err != nil {
		return nil, normalizeError(err)
	}
	return ids, nil
}

// ListTeamMembersForTeams 一次批量返回多个团队的成员，避免逐团队 N+1。
func (store *TeamStore) ListTeamMembersForTeams(ctx context.Context, tenantID uuid.UUID, teamIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if len(teamIDs) == 0 {
		return map[uuid.UUID][]uuid.UUID{}, nil
	}
	rows, err := store.queries.ListTeamMembersForTeams(ctx, generated.ListTeamMembersForTeamsParams{TenantID: tenantID, TeamIds: teamIDs})
	if err != nil {
		return nil, normalizeError(err)
	}
	members := make(map[uuid.UUID][]uuid.UUID, len(teamIDs))
	for _, row := range rows {
		members[row.TeamID] = append(members[row.TeamID], row.UserID)
	}
	return members, nil
}

// ReplaceTeamMembers 原子替换一个团队的成员集合。
func (store *TeamStore) ReplaceTeamMembers(ctx context.Context, tenantID, teamID uuid.UUID, userIDs []uuid.UUID) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return normalizeError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	if _, err := queries.DeleteTeamMembers(ctx, generated.DeleteTeamMembersParams{TenantID: tenantID, TeamID: teamID}); err != nil {
		return normalizeError(err)
	}
	for _, userID := range userIDs {
		if _, err := queries.InsertTeamMember(ctx, generated.InsertTeamMemberParams{TenantID: tenantID, TeamID: teamID, UserID: userID}); err != nil {
			return normalizeError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	return nil
}

// UpdateTeam 在 If-Match 下更新团队展示名并递增 revision。
func (store *TeamStore) UpdateTeam(ctx context.Context, tenantID uuid.UUID, input service.TeamPatchInput) (service.Team, error) {
	row, err := store.queries.UpdateTeam(ctx, generated.UpdateTeamParams{
		TenantID: tenantID, ID: input.ID, DisplayName: *input.DisplayName, ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.Team{}, service.ErrPrecondition
		}
		return service.Team{}, normalizeError(err)
	}
	return teamFromRow(row), nil
}

// DeleteTeam 在 If-Match 下删除一条团队。
func (store *TeamStore) DeleteTeam(ctx context.Context, tenantID, teamID uuid.UUID, expectedRevision int64) error {
	changed, err := store.queries.DeleteTeam(ctx, generated.DeleteTeamParams{
		TenantID: tenantID, ID: teamID, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrPrecondition
	}
	return nil
}

// ListTenantMembers 分页返回租户成员目录。
func (store *TeamStore) ListTenantMembers(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]service.TenantMember, int64, error) {
	total, err := store.queries.CountTenantMembers(ctx, tenantID)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListTenantMembers(ctx, generated.ListTenantMembersParams{TenantID: tenantID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	members := make([]service.TenantMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, tenantMemberFromRow(row))
	}
	return members, total, nil
}

// PutTenantMember 创建或替换一条租户成员关系。
func (store *TeamStore) PutTenantMember(ctx context.Context, tenantID, userID uuid.UUID, role string) (service.TenantMember, error) {
	row, err := store.queries.UpsertTenantMember(ctx, generated.UpsertTenantMemberParams{
		TenantID: tenantID, UserID: userID, Role: role, UpdatedAt: timestamp(time.Now().UTC()),
	})
	if err != nil {
		return service.TenantMember{}, normalizeError(err)
	}
	return service.TenantMember{
		User:     service.User{ID: row.UserID},
		Role:     row.Role,
		JoinedAt: row.CreatedAt.Time,
	}, nil
}

// DeleteTenantMember 移除一条租户成员关系。
func (store *TeamStore) DeleteTenantMember(ctx context.Context, tenantID, userID uuid.UUID) error {
	changed, err := store.queries.DeleteTenantMember(ctx, generated.DeleteTenantMemberParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return normalizeError(err)
	}
	if changed == 0 {
		return service.ErrNotFound
	}
	return nil
}

// SearchTenantUsers 在租户成员目录内按用户名/展示名搜索。
func (store *TeamStore) SearchTenantUsers(ctx context.Context, tenantID uuid.UUID, query string, limit, offset int32) ([]service.TenantMember, int64, error) {
	total, err := store.queries.CountTenantUserSearch(ctx, generated.CountTenantUserSearchParams{TenantID: tenantID, SearchQuery: query})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.SearchTenantUsers(ctx, generated.SearchTenantUsersParams{TenantID: tenantID, SearchQuery: query, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	members := make([]service.TenantMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, searchRowToMember(row))
	}
	return members, total, nil
}

// ListTenantMembersByIDs 校验并返回同租户内的用户 id 子集。
func (store *TeamStore) ListTenantMembersByIDs(ctx context.Context, tenantID uuid.UUID, userIDs []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := store.queries.ListTenantMembersByIDs(ctx, generated.ListTenantMembersByIDsParams{TenantID: tenantID, UserIds: userIDs})
	if err != nil {
		return nil, normalizeError(err)
	}
	return rows, nil
}

func teamFromRow(row generated.Team) service.Team {
	return service.Team{
		TenantID: row.TenantID, ID: row.ID, Slug: row.Slug, DisplayName: row.DisplayName,
		Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func tenantMemberFromRow(row generated.ListTenantMembersRow) service.TenantMember {
	return service.TenantMember{
		User: service.User{
			ID: row.UserID, Username: row.Username, DisplayName: row.DisplayName, Email: row.Email,
			Status: row.Status, IsPlatformAdmin: row.IsPlatformAdmin, Revision: row.UserRevision,
			CreatedAt: row.UserCreatedAt.Time, UpdatedAt: row.UserUpdatedAt.Time,
		},
		Role:     row.Role,
		JoinedAt: row.JoinedAt.Time,
	}
}

func searchRowToMember(row generated.SearchTenantUsersRow) service.TenantMember {
	return service.TenantMember{
		User: service.User{
			ID: row.UserID, Username: row.Username, DisplayName: row.DisplayName, Email: row.Email,
			Status: row.Status, IsPlatformAdmin: row.IsPlatformAdmin, Revision: row.UserRevision,
			CreatedAt: row.UserCreatedAt.Time, UpdatedAt: row.UserUpdatedAt.Time,
		},
		Role:     row.Role,
		JoinedAt: row.JoinedAt.Time,
	}
}

var _ service.TeamStore = (*TeamStore)(nil)
