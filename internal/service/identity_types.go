package service

import (
	"context"
	"errors"
	"time"
	"uuid"
)

var (
	// ErrUnauthenticated means no active principal could be established.
	ErrUnauthenticated = errors.New("principal is not authenticated")
	// ErrCSRFInvalid means a browser mutation did not prove session-bound intent.
	ErrCSRFInvalid = errors.New("CSRF token is missing or invalid")
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
)

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

// PrincipalKind identifies whether authentication used a browser session or PAT.
type PrincipalKind string

const (
	// PrincipalSession is a browser session subject to CSRF protection.
	PrincipalSession PrincipalKind = "session"
	// PrincipalPAT is a tenant-bound personal access token.
	PrincipalPAT PrincipalKind = "pat"
)

// Principal is the authenticated identity and credential-specific authorization boundary.
type Principal struct {
	Kind       PrincipalKind
	User       User
	SessionID  uuid.UUID
	TokenID    uuid.UUID
	CSRFHash   []byte
	TenantID   uuid.UUID
	TenantSlug string
	Role       string
	Scopes     []string
}

// LoginResult contains the one-time browser credentials and identity projection.
type LoginResult struct {
	SessionToken string
	CSRFToken    string
	ExpiresAt    time.Time
	Principal    Principal
	Memberships  []Membership
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

// NewSession contains digest-only browser session persistence inputs.
type NewSession struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash []byte
	CSRFHash  []byte
	ExpiresAt time.Time
}

// NewTenant contains tenant values ready for atomic default resolution and insertion.
type NewTenant struct {
	ID          uuid.UUID
	Slug        string
	DisplayName string
	Quota       *Quota
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
	PromotePlatformAdmin(context.Context, uuid.UUID, time.Time) (User, error)
	CreateSession(context.Context, NewSession) error
	SessionPrincipalByDigest(context.Context, []byte, time.Time) (Principal, error)
	RotateSessionCSRF(context.Context, uuid.UUID, []byte, time.Time) error
	TouchSession(context.Context, uuid.UUID, time.Time) error
	RevokeSession(context.Context, uuid.UUID, time.Time) error
	ActiveMemberships(context.Context, uuid.UUID) ([]Membership, error)
	ActiveMembership(context.Context, uuid.UUID, string) (Membership, error)
	CreateTenant(context.Context, NewTenant) (Tenant, error)
	TenantBySlug(context.Context, string) (Tenant, error)
	PutMembership(context.Context, uuid.UUID, uuid.UUID, string, time.Time) (Membership, error)
	CreateToken(context.Context, NewToken) (Token, error)
	PATPrincipalByDigest(context.Context, []byte, time.Time) (Principal, error)
	TouchToken(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	ListTokens(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]Token, int64, error)
	RevokeToken(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
}
