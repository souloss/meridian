package service

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"
)

var (
	// ErrUnauthenticated means no active principal could be established.
	ErrUnauthenticated = errors.New("principal is not authenticated")
	// ErrNotFound intentionally combines absent and unauthorized resources.
	ErrNotFound = errors.New("resource is absent or unauthorized")
	// ErrDuplicate means a contract-defined unique identity already exists.
	ErrDuplicate = errors.New("resource already exists")
	// ErrValidation means use-case input violates a frozen domain rule.
	ErrValidation = errors.New("input violates a domain rule")
	// ErrPrecondition means an If-Match token is missing or no longer matches the row revision.
	ErrPrecondition = errors.New("resource precondition failed")
	// ErrCredentialInUse means deletion would leave a repository without an explicit credential policy.
	ErrCredentialInUse = errors.New("credential is still referenced")
	// ErrIdempotencyConflict means one idempotency key was reused with a different request digest.
	ErrIdempotencyConflict = errors.New("idempotency key was reused for a different request")
	// ErrQuotaExceeded means a tenant resource ceiling would be exceeded by the operation.
	ErrQuotaExceeded = errors.New("tenant quota would be exceeded")
	// ErrInvalidState means the resource lifecycle forbids the requested transition.
	ErrInvalidState = errors.New("resource lifecycle forbids the requested transition")
	// ErrBaseLayerExists means a repo base would replace an AI-generated base
	// without the explicit replaceAiBase flag.
	ErrBaseLayerExists = errors.New("an AI-generated base layer already exists")
)

// QuotaExceededError preserves the safe resource counters needed by a quota response.
type QuotaExceededError struct {
	// Resource identifies the quota dimension, such as repositories.
	Resource string
	// Current is the active resource count before the rejected operation.
	Current int64
	// Limit is the tenant's configured maximum for the resource.
	Limit int64
}

// Error implements error without including tenant identifiers or request secrets.
func (err *QuotaExceededError) Error() string {
	return fmt.Sprintf("tenant quota exceeded for %s", err.Resource)
}

// Unwrap allows errors.Is to match ErrQuotaExceeded.
func (err *QuotaExceededError) Unwrap() error { return ErrQuotaExceeded }

// User is a local identity without password or session secret material.
type User struct {
	ID              uuid.UUID
	Username        string
	DisplayName     string
	Email           *string
	Status          string
	IsPlatformAdmin bool
	Revision        int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Quota is the tenant resource ceiling copied from platform defaults or supplied explicitly.
type Quota struct {
	MaxRepositories       int `json:"maxRepositories"`
	MaxServices           int `json:"maxServices"`
	MaxStorageBytes       int `json:"maxStorageBytes"`
	MaxCollectConcurrency int `json:"maxCollectConcurrency"`
}

// Tenant is the tenant control-plane record returned by M0 use cases.
type Tenant struct {
	ID          uuid.UUID
	Slug        string
	DisplayName string
	Status      string
	Quota       Quota
	Revision    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Membership grants one role in one active tenant.
type Membership struct {
	TenantID          uuid.UUID
	TenantSlug        string
	TenantDisplayName string
	UserID            uuid.UUID
	Role              string
	JoinedAt          time.Time
}

// PrincipalKind identifies whether authentication used a browser JWT or PAT.
type PrincipalKind string

const (
	// PrincipalJWT is a browser subject authenticated with a short-lived access token.
	PrincipalJWT PrincipalKind = "jwt"
	// PrincipalPAT is a tenant-bound personal access token.
	PrincipalPAT PrincipalKind = "pat"
)

// Principal is the authenticated identity and credential-specific authorization boundary.
type Principal struct {
	Kind       PrincipalKind
	User       User
	TokenID    uuid.UUID
	TenantID   uuid.UUID
	TenantSlug string
	Role       string
	Scopes     []string
}

// LoginResult contains the one-time browser credentials and identity projection.
type LoginResult struct {
	AccessToken      string
	ExpiresInSeconds int
	RefreshToken     string
	RefreshExpiresAt time.Time
	Principal        Principal
	Memberships      []Membership
}

// Token is PAT metadata that never includes plaintext bearer material.
type Token struct {
	TenantID   uuid.UUID
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	Scopes     []string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// CreatedToken contains PAT metadata and plaintext exactly once at creation.
type CreatedToken struct {
	Token
	Plaintext string
}

// NewUser contains validated identity creation inputs.
type NewUser struct {
	ID              uuid.UUID
	Username        string
	PasswordHash    string
	DisplayName     string
	Email           *string
	IsPlatformAdmin bool
}

// NewRefreshToken contains digest-only browser refresh token persistence inputs.
type NewRefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash []byte
	FamilyID  uuid.UUID
	ExpiresAt time.Time
}

// RefreshTokenPrincipal resolves one refresh token to its user and rotation family.
type RefreshTokenPrincipal struct {
	Principal Principal
	FamilyID  uuid.UUID
	Revoked   bool
}

// NewTenant contains tenant values ready for atomic default resolution and insertion.
type NewTenant struct {
	ID          uuid.UUID
	Slug        string
	DisplayName string
	Quota       *Quota
}

// TenantPatchInput contains the platform-controlled fields that may be changed on a tenant.
// A nil field is omitted from the mutation and therefore retains its current value.
type TenantPatchInput struct {
	// DisplayName replaces the tenant display name when supplied.
	DisplayName *string
	// Status replaces the tenant lifecycle status when supplied.
	Status *string
	// Quota replaces the complete tenant quota when supplied.
	Quota *Quota
}

// UpdateTenant contains one optimistic-concurrency tenant mutation.
type UpdateTenant struct {
	// Slug identifies the tenant being changed.
	Slug string
	// ExpectedRevision is the ETag revision required by the mutation.
	ExpectedRevision int64
	// DisplayName is an optional replacement display name.
	DisplayName *string
	// Status is an optional replacement lifecycle status.
	Status *string
	// Quota is an optional complete replacement quota.
	Quota *Quota
	// UpdatedAt is the UTC mutation timestamp.
	UpdatedAt time.Time
}

// NewToken contains digest-only PAT persistence inputs.
type NewToken struct {
	TenantID  uuid.UUID
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	TokenHash []byte
	Scopes    []string
	ExpiresAt *time.Time
}

// CreateUserInput contains plaintext identity input accepted by platform administration.
type CreateUserInput struct {
	Username    string
	Password    string
	DisplayName string
	Email       *string
}

// CreateTenantInput contains platform-authorized tenant creation values.
type CreateTenantInput struct {
	Slug        string
	DisplayName string
	Quota       *Quota
}

// CreateTokenInput contains the requested name, scopes, and optional expiry for a PAT.
type CreateTokenInput struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

// IdentityStore is the persistence boundary required by identity use cases.
type IdentityStore interface {
	CreateUser(context.Context, NewUser) (User, error)
	UserByUsername(context.Context, string) (User, string, error)
	UserByID(context.Context, uuid.UUID) (User, error)
	ListUsers(context.Context, string, int32, int32) ([]User, int64, error)
	PromotePlatformAdmin(context.Context, uuid.UUID, time.Time) (User, error)
	CreateRefreshToken(context.Context, NewRefreshToken) error
	RefreshTokenPrincipalByDigest(context.Context, []byte, time.Time) (RefreshTokenPrincipal, error)
	RotateRefreshToken(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	RevokeRefreshTokenFamily(context.Context, uuid.UUID, time.Time) error
	ActiveMemberships(context.Context, uuid.UUID) ([]Membership, error)
	ActiveMembership(context.Context, uuid.UUID, string) (Membership, error)
	CreateTenant(context.Context, NewTenant) (Tenant, error)
	TenantBySlug(context.Context, string) (Tenant, error)
	ListTenants(context.Context, int32, int32) ([]Tenant, int64, error)
	UpdateTenant(context.Context, UpdateTenant) (Tenant, error)
	PutMembership(context.Context, uuid.UUID, uuid.UUID, string, time.Time) (Membership, error)
	CreateToken(context.Context, NewToken) (Token, error)
	PATPrincipalByDigest(context.Context, []byte, time.Time) (Principal, error)
	TouchToken(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	ListTokens(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]Token, int64, error)
	RevokeToken(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
}
