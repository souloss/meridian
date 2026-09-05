package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

const (
	sessionTokenPrefix = "ses_"
	patTokenPrefix     = "pat_"
	sessionTTL         = 72 * time.Hour
	minimumPasswordLen = 12
	maximumPasswordLen = 1024
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Identity coordinates authentication, tenant membership, and PAT use cases.
type Identity struct {
	store    IdentityStore
	password PasswordHasher
	tokens   TokenDigester
	now      func() time.Time
}

// NewIdentity constructs identity use cases with caller-owned persistence and key material.
func NewIdentity(store IdentityStore, tokens TokenDigester) *Identity {
	return &Identity{
		store:    store,
		password: PasswordHasher{},
		tokens:   tokens,
		now:      time.Now,
	}
}

// BootstrapPlatformAdmin creates a platform administrator or promotes an existing user.
// An existing user's password and profile remain unchanged during promotion.
func (identity *Identity) BootstrapPlatformAdmin(ctx context.Context, input CreateUserInput) (User, error) {
	return BootstrapPlatformAdmin(ctx, identity.store, input, identity.now().UTC())
}

// BootstrapPlatformAdmin creates or promotes a platform administrator without requiring runtime token key material.
func BootstrapPlatformAdmin(ctx context.Context, store IdentityStore, input CreateUserInput, now time.Time) (User, error) {
	if err := validateUserInput(input); err != nil {
		return User{}, err
	}
	user, _, err := store.UserByUsername(ctx, input.Username)
	if err == nil {
		if user.IsPlatformAdmin {
			return user, nil
		}
		return store.PromotePlatformAdmin(ctx, user.ID, now)
	}
	if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}
	passwordHash, err := (PasswordHasher{}).Hash(input.Password)
	if err != nil {
		return User{}, err
	}
	return store.CreateUser(ctx, NewUser{
		ID: uuid.NewV7(), Username: input.Username, PasswordHash: passwordHash,
		DisplayName: input.DisplayName, Email: input.Email, IsPlatformAdmin: true,
	})
}

// CreateUser creates a non-admin identity after checking platform authorization.
func (identity *Identity) CreateUser(ctx context.Context, actor Principal, input CreateUserInput) (User, error) {
	if !isPlatformAdministrator(actor) {
		return User{}, ErrNotFound
	}
	if err := validateUserInput(input); err != nil {
		return User{}, err
	}
	return identity.createUser(ctx, input, false)
}

func (identity *Identity) createUser(ctx context.Context, input CreateUserInput, platformAdmin bool) (User, error) {
	passwordHash, err := identity.password.Hash(input.Password)
	if err != nil {
		return User{}, err
	}
	return identity.store.CreateUser(ctx, NewUser{
		ID:              uuid.NewV7(),
		Username:        input.Username,
		PasswordHash:    passwordHash,
		DisplayName:     input.DisplayName,
		Email:           input.Email,
		IsPlatformAdmin: platformAdmin,
	})
}

// Login authenticates a local password and creates one browser session.
func (identity *Identity) Login(ctx context.Context, username, password string) (LoginResult, error) {
	user, passwordHash, err := identity.store.UserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			identity.password.VerifyDummy(password)
			return LoginResult{}, ErrUnauthenticated
		}
		return LoginResult{}, err
	}
	if user.Status != "active" || !identity.password.Verify(password, passwordHash) {
		return LoginResult{}, ErrUnauthenticated
	}

	sessionToken, sessionHash, err := identity.tokens.NewOpaqueToken(sessionTokenPrefix)
	if err != nil {
		return LoginResult{}, err
	}
	csrfToken, csrfHash, err := identity.tokens.NewOpaqueToken("")
	if err != nil {
		return LoginResult{}, err
	}
	now := identity.now().UTC()
	expiresAt := now.Add(sessionTTL)
	sessionID := uuid.NewV7()
	if err := identity.store.CreateSession(ctx, NewSession{
		ID:        sessionID,
		UserID:    user.ID,
		TokenHash: sessionHash,
		CSRFHash:  csrfHash,
		ExpiresAt: expiresAt,
	}); err != nil {
		return LoginResult{}, err
	}
	memberships, err := identity.store.ActiveMemberships(ctx, user.ID)
	if err != nil {
		cleanupErr := identity.store.RevokeSession(context.WithoutCancel(ctx), sessionID, now)
		return LoginResult{}, errors.Join(err, cleanupErr)
	}
	return LoginResult{
		SessionToken: sessionToken,
		CSRFToken:    csrfToken,
		ExpiresAt:    expiresAt,
		Principal: Principal{
			Kind:      PrincipalSession,
			User:      user,
			SessionID: sessionID,
			CSRFHash:  csrfHash,
		},
		Memberships: memberships,
	}, nil
}

// AuthenticateSession resolves and touches one opaque browser session token.
func (identity *Identity) AuthenticateSession(ctx context.Context, plaintext string) (Principal, error) {
	if !validOpaqueToken(plaintext, sessionTokenPrefix) {
		return Principal{}, ErrUnauthenticated
	}
	now := identity.now().UTC()
	principal, err := identity.store.SessionPrincipalByDigest(ctx, identity.tokens.Digest(plaintext), now)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Principal{}, ErrUnauthenticated
		}
		return Principal{}, err
	}
	if err := identity.store.TouchSession(ctx, principal.SessionID, now); err != nil {
		return Principal{}, err
	}
	return principal, nil
}

// AuthenticatePAT resolves and touches one tenant-bound opaque personal access token.
func (identity *Identity) AuthenticatePAT(ctx context.Context, plaintext string) (Principal, error) {
	if !validOpaqueToken(plaintext, patTokenPrefix) {
		return Principal{}, ErrUnauthenticated
	}
	now := identity.now().UTC()
	principal, err := identity.store.PATPrincipalByDigest(ctx, identity.tokens.Digest(plaintext), now)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Principal{}, ErrUnauthenticated
		}
		return Principal{}, err
	}
	if err := identity.store.TouchToken(ctx, principal.TenantID, principal.TokenID, now); err != nil {
		return Principal{}, err
	}
	return principal, nil
}

// VerifyCSRF validates a browser mutation token against its session digest.
func (identity *Identity) VerifyCSRF(principal Principal, submitted string) error {
	if principal.Kind != PrincipalSession || !identity.tokens.Matches(submitted, principal.CSRFHash) {
		return ErrCSRFInvalid
	}
	return nil
}

// RotateCSRF returns a new plaintext CSRF token after replacing the stored digest.
func (identity *Identity) RotateCSRF(ctx context.Context, principal Principal) (string, error) {
	if principal.Kind != PrincipalSession {
		return "", ErrUnauthenticated
	}
	plaintext, digest, err := identity.tokens.NewOpaqueToken("")
	if err != nil {
		return "", err
	}
	if err := identity.store.RotateSessionCSRF(ctx, principal.SessionID, digest, identity.now().UTC()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrUnauthenticated
		}
		return "", err
	}
	return plaintext, nil
}

// Logout revokes the current browser session.
func (identity *Identity) Logout(ctx context.Context, principal Principal) error {
	if principal.Kind != PrincipalSession {
		return ErrUnauthenticated
	}
	if err := identity.store.RevokeSession(ctx, principal.SessionID, identity.now().UTC()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrUnauthenticated
		}
		return err
	}
	return nil
}

// Me returns current user data and active tenant memberships for browser sessions.
func (identity *Identity) Me(ctx context.Context, principal Principal) (User, []Membership, error) {
	if principal.Kind != PrincipalSession {
		return User{}, nil, ErrUnauthenticated
	}
	memberships, err := identity.store.ActiveMemberships(ctx, principal.User.ID)
	if err != nil {
		return User{}, nil, err
	}
	return principal.User, memberships, nil
}

// CreateTenant creates a tenant after checking platform-only authorization.
func (identity *Identity) CreateTenant(ctx context.Context, actor Principal, input CreateTenantInput) (Tenant, error) {
	if !isPlatformAdministrator(actor) {
		return Tenant{}, ErrNotFound
	}
	if !slugPattern.MatchString(input.Slug) || strings.TrimSpace(input.DisplayName) == "" || utf8.RuneCountInString(input.DisplayName) > 128 || !validQuota(input.Quota) {
		return Tenant{}, ErrValidation
	}
	return identity.store.CreateTenant(ctx, NewTenant{
		ID:          uuid.NewV7(),
		Slug:        input.Slug,
		DisplayName: input.DisplayName,
		Quota:       input.Quota,
	})
}

// PutTenantMembership creates or replaces a role after checking platform-only authorization.
func (identity *Identity) PutTenantMembership(ctx context.Context, actor Principal, tenantSlug string, userID uuid.UUID, role string) (User, Membership, error) {
	if !isPlatformAdministrator(actor) || !validTenantRole(role) {
		return User{}, Membership{}, ErrNotFound
	}
	tenant, err := identity.store.TenantBySlug(ctx, tenantSlug)
	if err != nil {
		return User{}, Membership{}, err
	}
	user, err := identity.store.UserByID(ctx, userID)
	if err != nil {
		return User{}, Membership{}, err
	}
	membership, err := identity.store.PutMembership(ctx, tenant.ID, userID, role, identity.now().UTC())
	if err != nil {
		return User{}, Membership{}, err
	}
	membership.TenantSlug = tenant.Slug
	membership.TenantDisplayName = tenant.DisplayName
	return user, membership, nil
}

// CreateToken creates a least-privilege PAT for the current browser user.
func (identity *Identity) CreateToken(ctx context.Context, actor Principal, tenantSlug string, input CreateTokenInput) (CreatedToken, error) {
	membership, err := identity.browserMembership(ctx, actor, tenantSlug)
	if err != nil {
		return CreatedToken{}, err
	}
	scopes, err := allowedTokenScopes(membership.Role, input.Scopes)
	if err != nil || strings.TrimSpace(input.Name) == "" || utf8.RuneCountInString(input.Name) > 64 {
		return CreatedToken{}, ErrValidation
	}
	if input.ExpiresAt != nil && !input.ExpiresAt.After(identity.now()) {
		return CreatedToken{}, ErrValidation
	}
	plaintext, digest, err := identity.tokens.NewOpaqueToken(patTokenPrefix)
	if err != nil {
		return CreatedToken{}, err
	}
	metadata, err := identity.store.CreateToken(ctx, NewToken{
		TenantID:  membership.TenantID,
		ID:        uuid.NewV7(),
		UserID:    actor.User.ID,
		Name:      input.Name,
		TokenHash: digest,
		Scopes:    scopes,
		ExpiresAt: input.ExpiresAt,
	})
	if err != nil {
		return CreatedToken{}, err
	}
	return CreatedToken{Token: metadata, Plaintext: plaintext}, nil
}

// ListTokens returns the current browser user's PAT metadata inside one active tenant.
func (identity *Identity) ListTokens(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]Token, int64, error) {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return nil, 0, ErrValidation
	}
	membership, err := identity.browserMembership(ctx, actor, tenantSlug)
	if err != nil {
		return nil, 0, err
	}
	return identity.store.ListTokens(ctx, membership.TenantID, actor.User.ID, int32(pageSize), int32((page-1)*pageSize))
}

// RevokeToken idempotently revokes one of the current browser user's PATs.
func (identity *Identity) RevokeToken(ctx context.Context, actor Principal, tenantSlug string, tokenID uuid.UUID) error {
	membership, err := identity.browserMembership(ctx, actor, tenantSlug)
	if err != nil {
		return err
	}
	return identity.store.RevokeToken(ctx, membership.TenantID, tokenID, actor.User.ID, identity.now().UTC())
}

func (identity *Identity) browserMembership(ctx context.Context, actor Principal, tenantSlug string) (Membership, error) {
	if actor.Kind != PrincipalSession {
		return Membership{}, ErrNotFound
	}
	membership, err := identity.store.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil {
		return Membership{}, err
	}
	if !roleAllows(membership.Role, "token:manage") {
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func isPlatformAdministrator(principal Principal) bool {
	return principal.Kind == PrincipalSession && principal.User.IsPlatformAdmin
}

func validateUserInput(input CreateUserInput) error {
	usernameLength := utf8.RuneCountInString(input.Username)
	displayNameLength := utf8.RuneCountInString(input.DisplayName)
	if strings.TrimSpace(input.Username) == "" || usernameLength > 128 || strings.TrimSpace(input.DisplayName) == "" || displayNameLength > 128 {
		return ErrValidation
	}
	if len(input.Password) < minimumPasswordLen || len(input.Password) > maximumPasswordLen {
		return ErrValidation
	}
	return nil
}

func validQuota(quota *Quota) bool {
	return quota == nil || (quota.MaxRepositories >= 0 && quota.MaxServices >= 0 && quota.MaxStorageBytes >= 0 && quota.MaxCollectConcurrency >= 1)
}

func validOpaqueToken(token, prefix string) bool {
	encoded, ok := strings.CutPrefix(token, prefix)
	if !ok {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && len(raw) == opaqueTokenBytes
}

func validTenantRole(role string) bool {
	return role == "tenant_admin" || role == "maintainer" || role == "viewer"
}

func roleAllows(role, permission string) bool {
	if role == "tenant_admin" {
		return true
	}
	switch permission {
	case "token:manage":
		return role == "maintainer" || role == "viewer"
	case "repository:read":
		return role == "maintainer" || role == "viewer"
	case "repository:write", "repository:sync":
		return role == "maintainer"
	case "job:read", "job:run":
		return role == "maintainer"
	case "credential:read", "credential:manage":
		return false
	}
	return false
}

func allowedTokenScopes(role string, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, ErrValidation
	}
	scopes := slices.Clone(requested)
	slices.Sort(scopes)
	scopes = slices.Compact(scopes)
	if len(scopes) != len(requested) {
		return nil, ErrValidation
	}
	for _, scope := range scopes {
		allowed := scope == "asset:read"
		if role == "tenant_admin" || role == "maintainer" {
			allowed = allowed || scope == "asset:push" || scope == "job:run"
		}
		if !allowed {
			return nil, fmt.Errorf("%w: token scope %q exceeds the tenant role", ErrValidation, scope)
		}
	}
	return scopes, nil
}
