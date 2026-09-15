// Package repository adapts generated SQL queries to service persistence ports.
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

// IdentityStore implements the identity service port with sqlc and pgx.
type IdentityStore struct {
	pool    *pgxpool.Pool
	queries *generated.Queries
}

// NewIdentityStore binds identity persistence to a native pgx pool.
func NewIdentityStore(pool *pgxpool.Pool) *IdentityStore {
	return &IdentityStore{pool: pool, queries: generated.New(pool)}
}

// CreateUser atomically inserts an identity and its required default preferences.
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

// UserByUsername returns an identity and its Argon2id verifier for authentication only.
func (store *IdentityStore) UserByUsername(ctx context.Context, username string) (service.User, string, error) {
	row, err := store.queries.GetUserByUsername(ctx, username)
	if err != nil {
		return service.User{}, "", normalizeError(err)
	}
	return userFromRow(row), row.PasswordHash, nil
}

// UserByID returns one identity without exposing its password verifier.
func (store *IdentityStore) UserByID(ctx context.Context, id uuid.UUID) (service.User, error) {
	row, err := store.queries.GetUserByID(ctx, id)
	if err != nil {
		return service.User{}, normalizeError(err)
	}
	return userFromRow(row), nil
}

// ListUsers returns platform identity metadata without password or session columns.
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

// PromotePlatformAdmin grants platform administration to an existing identity.
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

// CreateRefreshToken persists a keyed browser refresh token digest.
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

// RefreshTokenPrincipalByDigest resolves one refresh token to its user and rotation family.
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

// RotateRefreshToken revokes the consumed token and records its replacement in one atomic update.
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

// RevokeRefreshTokenFamily revokes every unrevoked token in one rotation family.
func (store *IdentityStore) RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID, revokedAt time.Time) error {
	_, err := store.queries.RevokeRefreshTokenFamily(ctx, generated.RevokeRefreshTokenFamilyParams{
		RevokedAt: timestamp(revokedAt),
		FamilyID:  familyID,
	})
	return normalizeError(err)
}

// ActiveMemberships returns stable active tenant contexts for one user.
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

// ActiveMembership resolves one user role without revealing disabled or absent tenants.
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

// CreateTenant atomically snapshots platform defaults and inserts one tenant.
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

// TenantBySlug returns tenant metadata in any lifecycle state for platform administration.
func (store *IdentityStore) TenantBySlug(ctx context.Context, slug string) (service.Tenant, error) {
	row, err := store.queries.GetTenantBySlug(ctx, slug)
	if err != nil {
		return service.Tenant{}, normalizeError(err)
	}
	return tenantFromRow(row)
}

// ListTenants returns tenant lifecycle metadata in deterministic order.
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

// UpdateTenant conditionally updates platform-controlled tenant metadata and advances its revision.
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

// PutMembership creates or replaces one tenant role assignment.
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

// CreateToken persists a keyed PAT digest and returns non-secret metadata.
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

// PATPrincipalByDigest resolves one active token, user, membership, and tenant.
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

// TouchToken records successful PAT activity.
func (store *IdentityStore) TouchToken(ctx context.Context, tenantID, tokenID uuid.UUID, usedAt time.Time) error {
	return normalizeError(store.queries.TouchAPIToken(ctx, generated.TouchAPITokenParams{
		UsedAt:   timestamp(usedAt),
		TenantID: tenantID,
		ID:       tokenID,
	}))
}

// ListTokens returns one metadata page and total count for a user's tenant PATs.
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

// RevokeToken idempotently revokes one PAT owned by the current user in one tenant.
func (store *IdentityStore) RevokeToken(ctx context.Context, tenantID, tokenID, userID uuid.UUID, revokedAt time.Time) error {
	_, err := store.queries.RevokeAPIToken(ctx, generated.RevokeAPITokenParams{
		RevokedAt: timestamp(revokedAt),
		TenantID:  tenantID,
		ID:        tokenID,
		UserID:    userID,
	})
	return normalizeError(err)
}

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
	if pgError, ok := errors.AsType[*pgconn.PgError](err); ok && pgError.Code == "23505" {
		return service.ErrDuplicate
	}
	return err
}

var _ service.IdentityStore = (*IdentityStore)(nil)
