// Package repository 将 sqlc 生成的 SQL 查询适配到 service 包的持久化端口（Store 接口）。
package repository

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// IdentityStore 基于 sqlc 与 pgx 实现 service.IdentityStore 身份持久化端口。
type IdentityStore struct {
	pool    *pgxpool.Pool
	queries *generated.Queries
}

// NewIdentityStore 将身份持久化绑定到原生 pgx 连接池。
func NewIdentityStore(pool *pgxpool.Pool) *IdentityStore {
	return &IdentityStore{pool: pool, queries: generated.New(pool)}
}

// CreateUser 原子地插入一个身份及其必需的默认偏好设置。
func (store *IdentityStore) CreateUser(ctx context.Context, input service.NewUser) (service.User, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.User{}, fmt.Errorf("begin create user transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := generated.New(tx)
	row, err := queries.CreateUser(ctx, generated.CreateUserParams{
		ID:              input.ID,
		Username:        input.Username,
		PasswordHash:    input.PasswordHash,
		DisplayName:     input.DisplayName,
		Email:           input.Email,
		IsPlatformAdmin: input.IsPlatformAdmin,
	})
	if err != nil {
		return service.User{}, normalizeError(err)
	}
	if _, err := queries.CreateDefaultUserPreferences(ctx, input.ID); err != nil {
		return service.User{}, normalizeError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return service.User{}, normalizeError(err)
	}
	return userFromRow(row), nil
}

// UserByUsername 返回一个身份及其 Argon2id 校验器，仅供认证使用。
func (store *IdentityStore) UserByUsername(ctx context.Context, username string) (service.User, string, error) {
	row, err := store.queries.GetUserByUsername(ctx, username)
	if err != nil {
		return service.User{}, "", normalizeError(err)
	}
	return userFromRow(row), row.PasswordHash, nil
}

// UserByID 返回一个身份，且不暴露其密码校验器。
func (store *IdentityStore) UserByID(ctx context.Context, id uuid.UUID) (service.User, error) {
	row, err := store.queries.GetUserByID(ctx, id)
	if err != nil {
		return service.User{}, normalizeError(err)
	}
	return userFromRow(row), nil
}

// ListUsers 返回平台身份元数据（不含密码与会话列）。
func (store *IdentityStore) ListUsers(ctx context.Context, search string, limit, offset int32) ([]service.User, int64, error) {
	total, err := store.queries.CountUsers(ctx, search)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListUsers(ctx, generated.ListUsersParams{SearchQuery: search, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	users := make([]service.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromListRow(row))
	}
	return users, total, nil
}

// PromotePlatformAdmin 为已有身份授予平台管理权限。
func (store *IdentityStore) PromotePlatformAdmin(ctx context.Context, id uuid.UUID, updatedAt time.Time) (service.User, error) {
	row, err := store.queries.PromoteUserToPlatformAdmin(ctx, generated.PromoteUserToPlatformAdminParams{
		UpdatedAt: timestamp(updatedAt),
		ID:        id,
	})
	if err != nil {
		return service.User{}, normalizeError(err)
	}
	return userFromRow(row), nil
}

// CreateRefreshToken 持久化一个带键的浏览器刷新令牌摘要。
func (store *IdentityStore) CreateRefreshToken(ctx context.Context, input service.NewRefreshToken) error {
	_, err := store.queries.CreateRefreshToken(ctx, generated.CreateRefreshTokenParams{
		ID:        input.ID,
		UserID:    input.UserID,
		TokenHash: input.TokenHash,
		FamilyID:  input.FamilyID,
		ExpiresAt: timestamp(input.ExpiresAt),
	})
	return normalizeError(err)
}

// RefreshTokenPrincipalByDigest 将一个刷新令牌解析为其用户与轮换家族。
func (store *IdentityStore) RefreshTokenPrincipalByDigest(ctx context.Context, digest []byte, authenticatedAt time.Time) (service.RefreshTokenPrincipal, error) {
	row, err := store.queries.GetRefreshTokenPrincipalByTokenHash(ctx, generated.GetRefreshTokenPrincipalByTokenHashParams{
		TokenHash:       digest,
		AuthenticatedAt: timestamp(authenticatedAt),
	})
	if err != nil {
		return service.RefreshTokenPrincipal{}, normalizeError(err)
	}
	return service.RefreshTokenPrincipal{
		FamilyID: row.FamilyID,
		Revoked:  row.RevokedAt.Valid,
		Principal: service.Principal{
			Kind:    service.PrincipalJWT,
			TokenID: row.TokenID,
			User: service.User{
				ID:              row.UserID,
				Username:        row.Username,
				DisplayName:     row.DisplayName,
				Email:           row.Email,
				Status:          row.UserStatus,
				IsPlatformAdmin: row.IsPlatformAdmin,
				Revision:        row.UserRevision,
				CreatedAt:       row.UserCreatedAt.Time,
				UpdatedAt:       row.UserUpdatedAt.Time,
			},
		},
	}, nil
}

// RotateRefreshToken 在一次原子更新中吊销已消费令牌并记录其替代令牌。
func (store *IdentityStore) RotateRefreshToken(ctx context.Context, id, replacedBy uuid.UUID, revokedAt time.Time) error {
	changed, err := store.queries.RotateRefreshToken(ctx, generated.RotateRefreshTokenParams{
		RevokedAt:  timestamp(revokedAt),
		ReplacedBy: &replacedBy,
		ID:         id,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed == 0 {
		return service.ErrNotFound
	}
	return nil
}

// RevokeRefreshTokenFamily 吊销一个轮换家族中所有尚未吊销的令牌。
func (store *IdentityStore) RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID, revokedAt time.Time) error {
	_, err := store.queries.RevokeRefreshTokenFamily(ctx, generated.RevokeRefreshTokenFamilyParams{
		RevokedAt: timestamp(revokedAt),
		FamilyID:  familyID,
	})
	return normalizeError(err)
}

// ActiveMemberships 返回某一用户的稳定活跃租户上下文列表。
func (store *IdentityStore) ActiveMemberships(ctx context.Context, userID uuid.UUID) ([]service.Membership, error) {
	rows, err := store.queries.ListActiveTenantMemberships(ctx, userID)
	if err != nil {
		return nil, normalizeError(err)
	}
	memberships := make([]service.Membership, 0, len(rows))
	for _, row := range rows {
		memberships = append(memberships, service.Membership{
			TenantID:          row.TenantID,
			TenantSlug:        row.TenantSlug,
			TenantDisplayName: row.TenantDisplayName,
			UserID:            userID,
			Role:              row.Role,
		})
	}
	return memberships, nil
}

// ActiveMembership 解析某一用户的角色，且不暴露已禁用或不存在的租户。
func (store *IdentityStore) ActiveMembership(ctx context.Context, userID uuid.UUID, tenantSlug string) (service.Membership, error) {
	row, err := store.queries.GetActiveTenantMembership(ctx, generated.GetActiveTenantMembershipParams{
		UserID:     userID,
		TenantSlug: tenantSlug,
	})
	if err != nil {
		return service.Membership{}, normalizeError(err)
	}
	return service.Membership{
		TenantID:          row.TenantID,
		TenantSlug:        row.TenantSlug,
		TenantDisplayName: row.TenantDisplayName,
		UserID:            userID,
		Role:              row.Role,
	}, nil
}

// CreateTenant 原子地快照平台默认值并插入一个租户。
func (store *IdentityStore) CreateTenant(ctx context.Context, input service.NewTenant) (service.Tenant, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.Tenant{}, fmt.Errorf("begin create tenant transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)

	settings, err := queries.GetPlatformSettingsForTenantCreate(ctx)
	if err != nil {
		return service.Tenant{}, normalizeError(err)
	}
	var defaults struct {
		DefaultQuota          service.Quota  `json:"defaultQuota"`
		DefaultTenantSettings jsontext.Value `json:"defaultTenantSettings"`
	}
	if err := json.Unmarshal(settings, &defaults); err != nil {
		return service.Tenant{}, fmt.Errorf("decode platform tenant defaults: %w", err)
	}
	quota := defaults.DefaultQuota
	if input.Quota != nil {
		quota = *input.Quota
	}
	quotaJSON, err := json.Marshal(&quota)
	if err != nil {
		return service.Tenant{}, fmt.Errorf("encode tenant quota: %w", err)
	}
	row, err := queries.CreateTenant(ctx, generated.CreateTenantParams{
		ID:          input.ID,
		Slug:        input.Slug,
		DisplayName: input.DisplayName,
		Quota:       quotaJSON,
		Settings:    defaults.DefaultTenantSettings,
	})
	if err != nil {
		return service.Tenant{}, normalizeError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return service.Tenant{}, normalizeError(err)
	}
	return tenantFromRow(row)
}

// TenantBySlug 返回任意生命周期状态下的租户元数据，供平台管理使用。
func (store *IdentityStore) TenantBySlug(ctx context.Context, slug string) (service.Tenant, error) {
	row, err := store.queries.GetTenantBySlug(ctx, slug)
	if err != nil {
		return service.Tenant{}, normalizeError(err)
	}
	return tenantFromRow(row)
}

// ListTenants 以确定性顺序返回租户生命周期元数据。
func (store *IdentityStore) ListTenants(ctx context.Context, limit, offset int32) ([]service.Tenant, int64, error) {
	total, err := store.queries.CountTenants(ctx)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListTenants(ctx, generated.ListTenantsParams{PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	tenants := make([]service.Tenant, 0, len(rows))
	for _, row := range rows {
		tenant, err := tenantFromListRow(row)
		if err != nil {
			return nil, 0, err
		}
		tenants = append(tenants, tenant)
	}
	return tenants, total, nil
}

// UpdateTenant 条件性地更新平台管控的租户元数据并推进其修订号。
func (store *IdentityStore) UpdateTenant(ctx context.Context, input service.UpdateTenant) (service.Tenant, error) {
	quota, err := optionalTenantQuota(input.Quota)
	if err != nil {
		return service.Tenant{}, err
	}
	var displayName, status string
	if input.DisplayName != nil {
		displayName = *input.DisplayName
	}
	if input.Status != nil {
		status = *input.Status
	}
	row, err := store.queries.UpdateTenant(ctx, generated.UpdateTenantParams{
		SetDisplayName:   input.DisplayName != nil,
		DisplayName:      displayName,
		SetStatus:        input.Status != nil,
		Status:           status,
		SetQuota:         input.Quota != nil,
		Quota:            quota,
		UpdatedAt:        timestamp(input.UpdatedAt),
		Slug:             input.Slug,
		ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.Tenant{}, service.ErrPrecondition
		}
		return service.Tenant{}, normalizeError(err)
	}
	return tenantFromRow(row)
}

func optionalTenantQuota(value *service.Quota) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode tenant quota: %w", err)
	}
	return encoded, nil
}

// PutMembership 创建或替换一条租户角色分配。
func (store *IdentityStore) PutMembership(ctx context.Context, tenantID, userID uuid.UUID, role string, updatedAt time.Time) (service.Membership, error) {
	row, err := store.queries.UpsertTenantMember(ctx, generated.UpsertTenantMemberParams{
		TenantID:  tenantID,
		UserID:    userID,
		Role:      role,
		UpdatedAt: timestamp(updatedAt),
	})
	if err != nil {
		return service.Membership{}, normalizeError(err)
	}
	return service.Membership{
		TenantID: row.TenantID,
		UserID:   row.UserID,
		Role:     row.Role,
		JoinedAt: row.CreatedAt.Time,
	}, nil
}

// CreateToken 持久化一个带键的 PAT 摘要并返回非敏感元数据。
func (store *IdentityStore) CreateToken(ctx context.Context, input service.NewToken) (service.Token, error) {
	row, err := store.queries.CreateAPIToken(ctx, generated.CreateAPITokenParams{
		TenantID:  input.TenantID,
		ID:        input.ID,
		UserID:    input.UserID,
		Name:      input.Name,
		TokenHash: input.TokenHash,
		Scopes:    input.Scopes,
		ExpiresAt: optionalTimestamp(input.ExpiresAt),
	})
	if err != nil {
		return service.Token{}, normalizeError(err)
	}
	return tokenFromRow(row), nil
}

// PATPrincipalByDigest 解析一个活跃令牌及其用户、成员关系与租户。
func (store *IdentityStore) PATPrincipalByDigest(ctx context.Context, digest []byte, authenticatedAt time.Time) (service.Principal, error) {
	row, err := store.queries.GetAPITokenPrincipalByTokenHash(ctx, generated.GetAPITokenPrincipalByTokenHashParams{
		TokenHash:       digest,
		AuthenticatedAt: timestamp(authenticatedAt),
	})
	if err != nil {
		return service.Principal{}, normalizeError(err)
	}
	return service.Principal{
		Kind:       service.PrincipalPAT,
		TenantID:   row.TenantID,
		TokenID:    row.TokenID,
		TenantSlug: row.TenantSlug,
		Role:       row.Role,
		Scopes:     row.Scopes,
		User: service.User{
			ID:          row.UserID,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			Email:       row.Email,
			Status:      row.UserStatus,
			Revision:    row.UserRevision,
			CreatedAt:   row.UserCreatedAt.Time,
			UpdatedAt:   row.UserUpdatedAt.Time,
		},
	}, nil
}

// TouchToken 记录一次成功的 PAT 活跃。
func (store *IdentityStore) TouchToken(ctx context.Context, tenantID, tokenID uuid.UUID, usedAt time.Time) error {
	return normalizeError(store.queries.TouchAPIToken(ctx, generated.TouchAPITokenParams{
		UsedAt:   timestamp(usedAt),
		TenantID: tenantID,
		ID:       tokenID,
	}))
}

// ListTokens 返回某用户租户内 PAT 的一页元数据及总数。
func (store *IdentityStore) ListTokens(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]service.Token, int64, error) {
	total, err := store.queries.CountAPITokensByUser(ctx, generated.CountAPITokensByUserParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListAPITokensByUser(ctx, generated.ListAPITokensByUserParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	tokens := make([]service.Token, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, tokenFromRow(row))
	}
	return tokens, total, nil
}

// RevokeToken 幂等地吊销当前用户在某一租户内持有的一个 PAT。
func (store *IdentityStore) RevokeToken(ctx context.Context, tenantID, tokenID, userID uuid.UUID, revokedAt time.Time) error {
	_, err := store.queries.RevokeAPIToken(ctx, generated.RevokeAPITokenParams{
		RevokedAt: timestamp(revokedAt),
		TenantID:  tenantID,
		ID:        tokenID,
		UserID:    userID,
	})
	return normalizeError(err)
}

// UpdateUserProfile 在 If-Match 下更新一个平台身份的非秘密字段。
func (store *IdentityStore) UpdateUserProfile(ctx context.Context, userID uuid.UUID, expectedRevision int64, input service.UpdateUserProfileInput) (service.User, error) {
	var displayName, status string
	if input.DisplayName != nil {
		displayName = *input.DisplayName
	}
	if input.Status != nil {
		status = *input.Status
	}
	row, err := store.queries.UpdateUserProfile(ctx, generated.UpdateUserProfileParams{
		SetDisplayName:   input.DisplayName != nil,
		DisplayName:      displayName,
		SetEmail:         input.SetEmail,
		Email:            input.Email,
		SetStatus:        input.Status != nil,
		Status:           status,
		ID:               userID,
		ExpectedRevision: expectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.User{}, service.ErrPrecondition
		}
		return service.User{}, normalizeError(err)
	}
	return userFromRow(row), nil
}

// UpdateUserPassword 在 If-Match 下轮换一个平台身份的密码。
func (store *IdentityStore) UpdateUserPassword(ctx context.Context, userID uuid.UUID, expectedRevision int64, passwordHash string) (service.User, error) {
	row, err := store.queries.UpdateUserPassword(ctx, generated.UpdateUserPasswordParams{
		PasswordHash: passwordHash, ID: userID, ExpectedRevision: expectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.User{}, service.ErrPrecondition
		}
		return service.User{}, normalizeError(err)
	}
	return userFromRow(row), nil
}

// UserPasswordHash 返回一个平台身份当前的密码哈希，供密码轮换。
func (store *IdentityStore) UserPasswordHash(ctx context.Context, userID uuid.UUID) (string, error) {
	hash, err := store.queries.GetUserPasswordHash(ctx, userID)
	if err != nil {
		return "", normalizeError(err)
	}
	return hash, nil
}

// DeleteTenant 在 If-Match 下禁用租户并记录一条 tenant.delete 任务。
func (store *IdentityStore) DeleteTenant(ctx context.Context, tenantID uuid.UUID, expectedRevision int64) (service.TenantDeletionAccepted, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return service.TenantDeletionAccepted{}, normalizeError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	tenant, err := queries.DisableTenant(ctx, generated.DisableTenantParams{TenantID: tenantID, ExpectedRevision: expectedRevision})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.TenantDeletionAccepted{}, service.ErrPrecondition
		}
		return service.TenantDeletionAccepted{}, normalizeError(err)
	}
	_ = tenant
	jobID := uuid.NewV7()
	job, err := queries.CreateTenantDeleteJob(ctx, generated.CreateTenantDeleteJobParams{
		TenantID: tenantID, ID: jobID, DedupeKey: dedupeKeyTenantDelete + tenantID.String(),
	})
	if err != nil {
		return service.TenantDeletionAccepted{}, normalizeError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return service.TenantDeletionAccepted{}, normalizeError(err)
	}
	return service.TenantDeletionAccepted{JobID: job.ID, Deduplicated: false}, nil
}

const dedupeKeyTenantDelete = "tenant-delete:"

func userFromRow(row generated.User) service.User {
	return service.User{
		ID:              row.ID,
		Username:        row.Username,
		DisplayName:     row.DisplayName,
		Email:           row.Email,
		Status:          row.Status,
		IsPlatformAdmin: row.IsPlatformAdmin,
		Revision:        row.Revision,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func userFromListRow(row generated.ListUsersRow) service.User {
	return service.User{
		ID: row.ID, Username: row.Username, DisplayName: row.DisplayName, Email: row.Email,
		Status: row.Status, IsPlatformAdmin: row.IsPlatformAdmin, Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func tenantFromRow(row generated.Tenant) (service.Tenant, error) {
	var quota service.Quota
	if err := json.Unmarshal(row.Quota, &quota); err != nil {
		return service.Tenant{}, fmt.Errorf("decode tenant %s quota: %w", row.ID, err)
	}
	return service.Tenant{
		ID:          row.ID,
		Slug:        row.Slug,
		DisplayName: row.DisplayName,
		Status:      row.Status,
		Quota:       quota,
		Revision:    row.Revision,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}

func tenantFromListRow(row generated.ListTenantsRow) (service.Tenant, error) {
	var quota service.Quota
	if err := json.Unmarshal(row.Quota, &quota); err != nil {
		return service.Tenant{}, fmt.Errorf("decode tenant %s quota: %w", row.ID, err)
	}
	return service.Tenant{
		ID: row.ID, Slug: row.Slug, DisplayName: row.DisplayName, Status: row.Status,
		Quota: quota, Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func tokenFromRow(row generated.ApiToken) service.Token {
	return service.Token{
		TenantID:   row.TenantID,
		ID:         row.ID,
		UserID:     row.UserID,
		Name:       row.Name,
		Scopes:     row.Scopes,
		ExpiresAt:  timePointer(row.ExpiresAt),
		LastUsedAt: timePointer(row.LastUsedAt),
		RevokedAt:  timePointer(row.RevokedAt),
		CreatedAt:  row.CreatedAt.Time,
	}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func optionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return new(value.Time)
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return service.ErrNotFound
	}
	if pgError, ok := errors.AsType[*pgconn.PgError](err); ok && pgError.Code == pgUniqueViolationCode {
		return service.ErrDuplicate
	}
	return err
}

var _ service.IdentityStore = (*IdentityStore)(nil)
