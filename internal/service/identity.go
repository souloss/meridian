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
	refreshTokenPrefix = "rfs_"
	patTokenPrefix     = "pat_"
	refreshTokenTTL    = 168 * time.Hour
	minimumPasswordLen = 12
	maximumPasswordLen = 1024
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Identity 协调认证、租户成员关系与 PAT 用例。
type Identity struct {
	store    IdentityStore
	password PasswordHasher
	tokens   TokenDigester
	jwt      *JWTIssuer
	now      func() time.Time
}

// NewIdentity 构造由调用方持有持久化与密钥材料的身份用例。
func NewIdentity(store IdentityStore, tokens TokenDigester, jwtIssuer *JWTIssuer) *Identity {
	return &Identity{
		store:    store,
		password: PasswordHasher{},
		tokens:   tokens,
		jwt:      jwtIssuer,
		now:      time.Now,
	}
}

// BootstrapPlatformAdmin 创建平台管理员或将现有用户提升。现有用户的密码与资料在提升期间保持不变。
func (identity *Identity) BootstrapPlatformAdmin(ctx context.Context, input CreateUserInput) (User, error) {
	return BootstrapPlatformAdmin(ctx, identity.store, input, identity.now().UTC())
}

// BootstrapPlatformAdmin 创建或提升平台管理员，无需运行时令牌密钥材料。
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

// CreateUser 在校验平台授权后创建非管理员身份。
func (identity *Identity) CreateUser(ctx context.Context, actor Principal, input CreateUserInput) (User, error) {
	if !isPlatformAdministrator(actor) {
		return User{}, ErrNotFound
	}
	if err := validateUserInput(input); err != nil {
		return User{}, err
	}
	return identity.createUser(ctx, input, false)
}

// ListUsers 返回确定性的、仅平台可见的脱敏身份元数据页。
func (identity *Identity) ListUsers(ctx context.Context, actor Principal, search string, page, pageSize int) ([]User, int64, error) {
	if !isPlatformAdministrator(actor) {
		return nil, 0, ErrNotFound
	}
	if page < 1 || pageSize < 1 || pageSize > defaultPageSizeMax {
		return nil, 0, ErrValidation
	}
	return identity.store.ListUsers(ctx, search, int32(pageSize), int32((page-1)*pageSize))
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

// Login 校验本地密码并铸造浏览器访问令牌与刷新令牌。
func (identity *Identity) Login(ctx context.Context, username, password string) (LoginResult, error) {
	user, passwordHash, err := identity.store.UserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			identity.password.VerifyDummy(password)
			return LoginResult{}, ErrUnauthenticated
		}
		return LoginResult{}, err
	}
	if user.Status != identityStatusActive || !identity.password.Verify(password, passwordHash) {
		return LoginResult{}, ErrUnauthenticated
	}

	accessToken, accessTTL, err := identity.jwt.IssueAccessToken(user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, refreshHash, err := identity.tokens.NewOpaqueToken(refreshTokenPrefix)
	if err != nil {
		return LoginResult{}, err
	}
	now := identity.now().UTC()
	refreshExpiresAt := now.Add(refreshTokenTTL)
	if err := identity.store.CreateRefreshToken(ctx, NewRefreshToken{
		ID:        uuid.NewV7(),
		UserID:    user.ID,
		TokenHash: refreshHash,
		FamilyID:  uuid.NewV7(),
		ExpiresAt: refreshExpiresAt,
	}); err != nil {
		return LoginResult{}, err
	}
	memberships, err := identity.store.ActiveMemberships(ctx, user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		AccessToken:      accessToken,
		ExpiresInSeconds: int(accessTTL / time.Second),
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpiresAt,
		Principal:        Principal{Kind: PrincipalJWT, User: user},
		Memberships:      memberships,
	}, nil
}

// AuthenticateJWT 将短时访问令牌解析为实时用户主体。
func (identity *Identity) AuthenticateJWT(ctx context.Context, plaintext string) (Principal, error) {
	userID, err := identity.jwt.VerifyAccessToken(plaintext)
	if err != nil {
		return Principal{}, err
	}
	user, err := identity.store.UserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Principal{}, ErrUnauthenticated
		}
		return Principal{}, err
	}
	if user.Status != identityStatusActive {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{Kind: PrincipalJWT, User: user}, nil
}

// AuthenticatePAT 解析并触达一个绑定租户的不透明个人访问令牌。
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

// Refresh 轮换刷新令牌并返回新访问令牌与下一个刷新令牌。
//
// 已撤销的刷新令牌表示重放；整个族被撤销，使泄露的令牌无法活过其合法后继。
func (identity *Identity) Refresh(ctx context.Context, plaintext string) (LoginResult, error) {
	if !validOpaqueToken(plaintext, refreshTokenPrefix) {
		return LoginResult{}, ErrUnauthenticated
	}
	now := identity.now().UTC()
	principal, err := identity.store.RefreshTokenPrincipalByDigest(ctx, identity.tokens.Digest(plaintext), now)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return LoginResult{}, ErrUnauthenticated
		}
		return LoginResult{}, err
	}
	if principal.Revoked {
		if err := identity.store.RevokeRefreshTokenFamily(ctx, principal.FamilyID, now); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrUnauthenticated
	}

	accessToken, accessTTL, err := identity.jwt.IssueAccessToken(principal.Principal.User.ID)
	if err != nil {
		return LoginResult{}, err
	}
	nextRefresh, nextHash, err := identity.tokens.NewOpaqueToken(refreshTokenPrefix)
	if err != nil {
		return LoginResult{}, err
	}
	nextID := uuid.NewV7()
	refreshExpiresAt := now.Add(refreshTokenTTL)
	if err := identity.store.CreateRefreshToken(ctx, NewRefreshToken{
		ID:        nextID,
		UserID:    principal.Principal.User.ID,
		TokenHash: nextHash,
		FamilyID:  principal.FamilyID,
		ExpiresAt: refreshExpiresAt,
	}); err != nil {
		return LoginResult{}, err
	}
	if err := identity.store.RotateRefreshToken(ctx, principal.Principal.TokenID, nextID, now); err != nil {
		if errors.Is(err, ErrNotFound) {
			return LoginResult{}, ErrUnauthenticated
		}
		return LoginResult{}, err
	}
	return LoginResult{
		AccessToken:      accessToken,
		ExpiresInSeconds: int(accessTTL / time.Second),
		RefreshToken:     nextRefresh,
		RefreshExpiresAt: refreshExpiresAt,
		Principal:        principal.Principal,
	}, nil
}

// Logout 撤销刷新令牌族；浏览器丢弃内存中的访问令牌。
func (identity *Identity) Logout(ctx context.Context, plaintext string) error {
	if !validOpaqueToken(plaintext, refreshTokenPrefix) {
		return ErrUnauthenticated
	}
	principal, err := identity.store.RefreshTokenPrincipalByDigest(ctx, identity.tokens.Digest(plaintext), identity.now().UTC())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrUnauthenticated
		}
		return err
	}
	if err := identity.store.RevokeRefreshTokenFamily(ctx, principal.FamilyID, identity.now().UTC()); err != nil {
		return err
	}
	return nil
}

// Me 返回浏览器主体的当前用户数据与活跃租户成员关系。
func (identity *Identity) Me(ctx context.Context, principal Principal) (User, []Membership, error) {
	if principal.Kind != PrincipalJWT {
		return User{}, nil, ErrUnauthenticated
	}
	memberships, err := identity.store.ActiveMemberships(ctx, principal.User.ID)
	if err != nil {
		return User{}, nil, err
	}
	return principal.User, memberships, nil
}

// CreateTenant 在校验仅平台授权后创建租户。
func (identity *Identity) CreateTenant(ctx context.Context, actor Principal, input CreateTenantInput) (Tenant, error) {
	if !isPlatformAdministrator(actor) {
		return Tenant{}, ErrNotFound
	}
	if !slugPattern.MatchString(input.Slug) || strings.TrimSpace(input.DisplayName) == "" || utf8.RuneCountInString(input.DisplayName) > maxDisplayNameRunes || !validQuota(input.Quota) {
		return Tenant{}, ErrValidation
	}
	return identity.store.CreateTenant(ctx, NewTenant{
		ID:          uuid.NewV7(),
		Slug:        input.Slug,
		DisplayName: input.DisplayName,
		Quota:       input.Quota,
	})
}

// ListTenants 返回确定性的、仅平台可见的租户生命周期元数据页。
func (identity *Identity) ListTenants(ctx context.Context, actor Principal, page, pageSize int) ([]Tenant, int64, error) {
	if !isPlatformAdministrator(actor) {
		return nil, 0, ErrNotFound
	}
	if page < 1 || pageSize < 1 || pageSize > defaultPageSizeMax {
		return nil, 0, ErrValidation
	}
	return identity.store.ListTenants(ctx, int32(pageSize), int32((page-1)*pageSize))
}

// UpdateTenant 在 If-Match ETag 下应用平台控制的租户元数据。
func (identity *Identity) UpdateTenant(ctx context.Context, actor Principal, slug, etag string, patch TenantPatchInput) (Tenant, error) {
	if !isPlatformAdministrator(actor) {
		return Tenant{}, ErrNotFound
	}
	if patch.DisplayName == nil && patch.Status == nil && patch.Quota == nil {
		return Tenant{}, ErrValidation
	}
	tenant, err := identity.store.TenantBySlug(ctx, slug)
	if err != nil {
		return Tenant{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "tenant", tenant.ID)
	if err != nil {
		return Tenant{}, ErrPrecondition
	}
	if err := validateTenantPatch(patch); err != nil {
		return Tenant{}, err
	}
	return identity.store.UpdateTenant(ctx, UpdateTenant{
		Slug: slug, ExpectedRevision: expectedRevision, DisplayName: patch.DisplayName,
		Status: patch.Status, Quota: patch.Quota, UpdatedAt: identity.now().UTC(),
	})
}

// PutTenantMembership 在校验仅平台授权后创建或替换角色。
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

// CreateToken 为当前浏览器用户创建最小权限 PAT。
func (identity *Identity) CreateToken(ctx context.Context, actor Principal, tenantSlug string, input CreateTokenInput) (CreatedToken, error) {
	membership, err := identity.browserMembership(ctx, actor, tenantSlug)
	if err != nil {
		return CreatedToken{}, err
	}
	scopes, err := allowedTokenScopes(membership.Role, input.Scopes)
	if err != nil || strings.TrimSpace(input.Name) == "" || utf8.RuneCountInString(input.Name) > maxTokenNameRunes {
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

// ListTokens 返回当前浏览器用户在一个活跃租户内的 PAT 元数据。
func (identity *Identity) ListTokens(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]Token, int64, error) {
	if page < 1 || pageSize < 1 || pageSize > defaultPageSizeMax {
		return nil, 0, ErrValidation
	}
	membership, err := identity.browserMembership(ctx, actor, tenantSlug)
	if err != nil {
		return nil, 0, err
	}
	return identity.store.ListTokens(ctx, membership.TenantID, actor.User.ID, int32(pageSize), int32((page-1)*pageSize))
}

// RevokeToken 幂等地撤销当前浏览器用户的某个 PAT。
func (identity *Identity) RevokeToken(ctx context.Context, actor Principal, tenantSlug string, tokenID uuid.UUID) error {
	membership, err := identity.browserMembership(ctx, actor, tenantSlug)
	if err != nil {
		return err
	}
	return identity.store.RevokeToken(ctx, membership.TenantID, tokenID, actor.User.ID, identity.now().UTC())
}

func (identity *Identity) browserMembership(ctx context.Context, actor Principal, tenantSlug string) (Membership, error) {
	if actor.Kind != PrincipalJWT {
		return Membership{}, ErrNotFound
	}
	membership, err := identity.store.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil {
		return Membership{}, err
	}
	if !roleAllows(membership.Role, scopeTokenManage) {
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func isPlatformAdministrator(principal Principal) bool {
	return principal.Kind == PrincipalJWT && principal.User.IsPlatformAdmin
}

func validateUserInput(input CreateUserInput) error {
	usernameLength := utf8.RuneCountInString(input.Username)
	displayNameLength := utf8.RuneCountInString(input.DisplayName)
	if strings.TrimSpace(input.Username) == "" || usernameLength > maxDisplayNameRunes || strings.TrimSpace(input.DisplayName) == "" || displayNameLength > maxDisplayNameRunes {
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

func validateTenantPatch(patch TenantPatchInput) error {
	if patch.DisplayName != nil {
		if strings.TrimSpace(*patch.DisplayName) == "" || utf8.RuneCountInString(*patch.DisplayName) > maxDisplayNameRunes {
			return ErrValidation
		}
	}
	if patch.Status != nil && *patch.Status != identityStatusActive && *patch.Status != identityStatusDisabled {
		return ErrValidation
	}
	if !validQuota(patch.Quota) {
		return ErrValidation
	}
	return nil
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
	return role == tenantRoleAdmin || role == tenantRoleMaintainer || role == tenantRoleViewer
}

func roleAllows(role, permission string) bool {
	if role == tenantRoleAdmin {
		return true
	}
	switch permission {
	case scopeTokenManage:
		return role == tenantRoleMaintainer || role == tenantRoleViewer
	case scopeRepositoryRead, scopeServiceRead, scopeAssetRead, scopeLayerRead:
		return role == tenantRoleMaintainer || role == tenantRoleViewer
	case scopeRepositoryWrite, scopeRepositorySync, scopeServiceWrite, scopeServiceCreate, scopeLayerEdit, scopeLayerApprove, scopeAssetPublish, scopeAssetPush, groupPermissionManage:
		return role == tenantRoleMaintainer
	case scopeJobRead, scopeJobRun:
		return role == tenantRoleMaintainer
	case scopeTodoReadSelf:
		return role == tenantRoleMaintainer || role == tenantRoleViewer
	case scopeCredentialRead, scopeCredentialManage:
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
		allowed := scope == scopeAssetRead
		if role == tenantRoleAdmin || role == tenantRoleMaintainer {
			allowed = allowed || scope == scopeAssetPush || scope == scopeJobRun
		}
		if !allowed {
			return nil, fmt.Errorf("%w: token scope %q exceeds the tenant role", ErrValidation, scope)
		}
	}
	return scopes, nil
}
