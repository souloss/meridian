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

// CreateSession persists keyed browser credential digests.
func (store *IdentityStore) CreateSession(ctx context.Context, input service.NewSession) error {
	_, err := store.queries.CreateSession(ctx, generated.CreateSessionParams{
		ID:        input.ID,
		UserID:    input.UserID,
		TokenHash: input.TokenHash,
		CsrfHash:  input.CSRFHash,
		ExpiresAt: timestamp(input.ExpiresAt),
	})
	return normalizeError(err)
}

// SessionPrincipalByDigest resolves one active browser session at the supplied instant.
func (store *IdentityStore) SessionPrincipalByDigest(ctx context.Context, digest []byte, authenticatedAt time.Time) (service.Principal, error) {
	row, err := store.queries.GetSessionPrincipalByTokenHash(ctx, generated.GetSessionPrincipalByTokenHashParams{
		TokenHash:       digest,
		AuthenticatedAt: timestamp(authenticatedAt),
	})
	if err != nil {
		return service.Principal{}, normalizeError(err)
	}
	return service.Principal{
		Kind:      service.PrincipalSession,
		SessionID: row.SessionID,
		CSRFHash:  row.CsrfHash,
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
	}, nil
}

// RotateSessionCSRF replaces the digest used by one active browser session.
func (store *IdentityStore) RotateSessionCSRF(ctx context.Context, id uuid.UUID, digest []byte, updatedAt time.Time) error {
	changed, err := store.queries.RotateSessionCSRFHash(ctx, generated.RotateSessionCSRFHashParams{
		CsrfHash:  digest,
		UpdatedAt: timestamp(updatedAt),
		ID:        id,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed == 0 {
		return service.ErrNotFound
	}
	return nil
}

// TouchSession records successful browser session activity.
func (store *IdentityStore) TouchSession(ctx context.Context, id uuid.UUID, seenAt time.Time) error {
	return normalizeError(store.queries.TouchSession(ctx, generated.TouchSessionParams{
		SeenAt: timestamp(seenAt),
		ID:     id,
	}))
}

// RevokeSession makes one browser session unusable.
func (store *IdentityStore) RevokeSession(ctx context.Context, id uuid.UUID, revokedAt time.Time) error {
	changed, err := store.queries.RevokeSession(ctx, generated.RevokeSessionParams{
		RevokedAt: timestamp(revokedAt),
		ID:        id,
	})
	if err != nil {
		return normalizeError(err)
	}
	if changed == 0 {
		return service.ErrNotFound
	}
	return nil
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
