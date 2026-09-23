package service

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"
)

var (
	// ErrUnauthenticated 表示未能建立有效主体上下文。
	// 对外映射：ErrorCodeUnauthenticated（HTTP 401）。
	ErrUnauthenticated = errors.New("principal is not authenticated")
	// ErrNotFound 有意合并「资源不存在」与「无权访问」两类情况。
	// 对外映射：ErrorCodeNotFound（HTTP 404）。
	ErrNotFound = errors.New("resource is absent or unauthorized")
	// ErrDuplicate 表示契约定义的唯一标识已存在。
	// 对外映射：ErrorCodeDuplicate（HTTP 409）。
	ErrDuplicate = errors.New("resource already exists")
	// ErrValidation 表示用例输入违反冻结的领域规则。
	// 对外映射：ErrorCodeValidation（HTTP 422）。
	ErrValidation = errors.New("input violates a domain rule")
	// ErrPrecondition 表示 If-Match 令牌缺失或不再匹配行版本。
	// 对外映射：ErrorCodePreconditionFailed（HTTP 412）。
	ErrPrecondition = errors.New("resource precondition failed")
	// ErrCredentialInUse 表示删除会使仓库失去显式凭据策略。
	// 对外映射：ErrorCodeCredentialInUse（HTTP 409）。
	ErrCredentialInUse = errors.New("credential is still referenced")
	// ErrIdempotencyConflict 表示一个幂等键被不同请求摘要复用。
	// 对外映射：ErrorCodeIdempotencyConflict（HTTP 409）。
	ErrIdempotencyConflict = errors.New("idempotency key was reused for a different request")
	// ErrQuotaExceeded 表示操作将使租户资源上限超出。
	// 对外映射：ErrorCodeQuotaExceeded（HTTP 409）。
	ErrQuotaExceeded = errors.New("tenant quota would be exceeded")
	// ErrInvalidState 表示资源生命周期禁止所请求的状态迁移。
	// 对外映射：ErrorCodeInvalidState（HTTP 409）。
	ErrInvalidState = errors.New("resource lifecycle forbids the requested transition")
	// ErrBaseLayerExists 表示仓库 base 会在未显式传入 replaceAiBase 标志时
	// 替换一个已存在的 AI 生成 base。
	// 对外映射：ErrorCodeBaseLayerExists（HTTP 409）。
	ErrBaseLayerExists = errors.New("an AI-generated base layer already exists")
)

// QuotaExceededError 保留配额响应所需的安全资源计数。
type QuotaExceededError struct {
	// Resource 标识配额维度，如 repositories。
	Resource string
	// Current 是被拒绝操作前的活跃资源数。
	Current int64
	// Limit 是租户为该资源配置的最大值。
	Limit int64
}

// Error 实现 error 接口，且不包含租户标识或请求秘密。
func (err *QuotaExceededError) Error() string {
	return fmt.Sprintf("tenant quota exceeded for %s", err.Resource)
}

// Unwrap 使 errors.Is 能匹配 ErrQuotaExceeded。
func (err *QuotaExceededError) Unwrap() error { return ErrQuotaExceeded }

// User 是不含密码或会话秘密材料的本地身份。
type User struct {
	// ID 是用户的标识。
	ID uuid.UUID
	// Username 是登录用户名。
	Username string
	// DisplayName 是展示名。
	DisplayName string
	// Email 是邮箱（可为空）。
	Email *string
	// Status 是用户状态。
	Status string
	// IsPlatformAdmin 表示是否为平台管理员。
	IsPlatformAdmin bool
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// Quota 是从平台默认值复制或显式提供的租户资源上限。
type Quota struct {
	// MaxRepositories 是仓库数上限。
	MaxRepositories int `json:"maxRepositories"`
	// MaxServices 是服务数上限。
	MaxServices int `json:"maxServices"`
	// MaxStorageBytes 是存储字节上限。
	MaxStorageBytes int `json:"maxStorageBytes"`
	// MaxCollectConcurrency 是采集并发上限。
	MaxCollectConcurrency int `json:"maxCollectConcurrency"`
}

// Tenant 是 M0 用例返回的租户控制面记录。
type Tenant struct {
	// ID 是租户的标识。
	ID uuid.UUID
	// Slug 是租户的 URL 标识。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
	// Status 是租户状态。
	Status string
	// Quota 是租户配额。
	Quota Quota
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// Membership 授予一个活跃租户中的一个角色。
type Membership struct {
	// TenantID 是租户标识。
	TenantID uuid.UUID
	// TenantSlug 是租户 slug。
	TenantSlug string
	// TenantDisplayName 是租户展示名。
	TenantDisplayName string
	// UserID 是用户标识。
	UserID uuid.UUID
	// Role 是成员角色。
	Role string
	// JoinedAt 是加入时间。
	JoinedAt time.Time
}

// PrincipalKind 标识认证是使用浏览器 JWT 还是 PAT。
type PrincipalKind string

const (
	// PrincipalJWT 是使用短时访问令牌认证的浏览器主体。
	PrincipalJWT PrincipalKind = "jwt"
	// PrincipalPAT 是绑定租户的个人访问令牌主体。
	PrincipalPAT PrincipalKind = "pat"
)

// Principal 是认证后的身份及凭据特定的授权边界。
type Principal struct {
	// Kind 是主体类型（jwt/pat）。
	Kind PrincipalKind
	// User 是底层用户。
	User User
	// TokenID 是 PAT 的令牌标识。
	TokenID uuid.UUID
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// TenantSlug 是所属租户 slug。
	TenantSlug string
	// Role 是租户成员角色。
	Role string
	// Scopes 是 PAT 的授权作用域。
	Scopes []string
}

// LoginResult 包含一次性浏览器凭据与身份投影。
type LoginResult struct {
	// AccessToken 是访问令牌。
	AccessToken string
	// ExpiresInSeconds 是访问令牌剩余有效秒数。
	ExpiresInSeconds int
	// RefreshToken 是刷新令牌。
	RefreshToken string
	// RefreshExpiresAt 是刷新令牌过期时间。
	RefreshExpiresAt time.Time
	// Principal 是认证后的主体。
	Principal Principal
	// Memberships 是主体的活跃成员关系。
	Memberships []Membership
}

// Token 是 PAT 元数据，绝不包含明文承载材料。
type Token struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是令牌的标识。
	ID uuid.UUID
	// UserID 是所属用户的标识。
	UserID uuid.UUID
	// Name 是令牌名。
	Name string
	// Scopes 是授权作用域。
	Scopes []string
	// ExpiresAt 是过期时间（可为空）。
	ExpiresAt *time.Time
	// LastUsedAt 是最近使用时间（可为空）。
	LastUsedAt *time.Time
	// RevokedAt 是撤销时间（可为空）。
	RevokedAt *time.Time
	// CreatedAt 是创建时间。
	CreatedAt time.Time
}

// CreatedToken 在创建时恰好一次包含 PAT 元数据与明文。
type CreatedToken struct {
	// Token 是令牌元数据。
	Token
	// Plaintext 是明文令牌值。
	Plaintext string
}

// NewUser 包含校验后的身份创建输入。
type NewUser struct {
	// ID 是用户的标识。
	ID uuid.UUID
	// Username 是登录用户名。
	Username string
	// PasswordHash 是密码哈希。
	PasswordHash string
	// DisplayName 是展示名。
	DisplayName string
	// Email 是邮箱（可为空）。
	Email *string
	// IsPlatformAdmin 表示是否为平台管理员。
	IsPlatformAdmin bool
}

// NewRefreshToken 包含仅摘要的浏览器刷新令牌持久化输入。
type NewRefreshToken struct {
	// ID 是刷新令牌的标识。
	ID uuid.UUID
	// UserID 是所属用户的标识。
	UserID uuid.UUID
	// TokenHash 是令牌摘要。
	TokenHash []byte
	// FamilyID 是旋转族标识。
	FamilyID uuid.UUID
	// ExpiresAt 是过期时间。
	ExpiresAt time.Time
}

// RefreshTokenPrincipal 将一个刷新令牌解析到其用户与旋转族。
type RefreshTokenPrincipal struct {
	// Principal 是认证后的主体。
	Principal Principal
	// FamilyID 是旋转族标识。
	FamilyID uuid.UUID
	// Revoked 表示令牌是否已撤销。
	Revoked bool
}

// NewTenant 包含准备好做原子默认解析与插入的租户值。
type NewTenant struct {
	// ID 是租户的标识。
	ID uuid.UUID
	// Slug 是租户的 URL 标识。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
	// Quota 是租户配额（可为空，为空走平台默认）。
	Quota *Quota
}

// TenantPatchInput 包含租户上可由平台控制的字段。
// 为 nil 的字段从变更中省略，因此保留当前值。
type TenantPatchInput struct {
	// DisplayName 在提供时替换租户展示名。
	DisplayName *string
	// Status 在提供时替换租户生命周期状态。
	Status *string
	// Quota 在提供时替换完整租户配额。
	Quota *Quota
}

// UpdateTenant 包含一次乐观并发租户变更。
type UpdateTenant struct {
	// Slug 标识被变更的租户。
	Slug string
	// ExpectedRevision 是变更所需的 ETag 版本号。
	ExpectedRevision int64
	// DisplayName 是可选的替换展示名。
	DisplayName *string
	// Status 是可选的替换生命周期状态。
	Status *string
	// Quota 是可选的完整替换配额。
	Quota *Quota
	// UpdatedAt 是 UTC 变更时间戳。
	UpdatedAt time.Time
}

// NewToken 包含仅摘要的 PAT 持久化输入。
type NewToken struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是令牌的标识。
	ID uuid.UUID
	// UserID 是所属用户的标识。
	UserID uuid.UUID
	// Name 是令牌名。
	Name string
	// TokenHash 是令牌摘要。
	TokenHash []byte
	// Scopes 是授权作用域。
	Scopes []string
	// ExpiresAt 是过期时间（可为空）。
	ExpiresAt *time.Time
}

// CreateUserInput 包含平台管理接受的明文身份输入。
type CreateUserInput struct {
	// Username 是登录用户名。
	Username string
	// Password 是明文密码。
	Password string
	// DisplayName 是展示名。
	DisplayName string
	// Email 是邮箱（可为空）。
	Email *string
}

// CreateTenantInput 包含平台授权的租户创建值。
type CreateTenantInput struct {
	// Slug 是租户的 URL 标识。
	Slug string
	// DisplayName 是展示名。
	DisplayName string
	// Quota 是租户配额（可为空）。
	Quota *Quota
}

// CreateTokenInput 包含 PAT 请求的名称、作用域与可选过期时间。
type CreateTokenInput struct {
	// Name 是令牌名。
	Name string
	// Scopes 是授权作用域。
	Scopes []string
	// ExpiresAt 是过期时间（可为空）。
	ExpiresAt *time.Time
}

// UpdateUserProfileInput 承载一次平台身份的非秘密字段补丁。
type UpdateUserProfileInput struct {
	// DisplayName 是可选的替换展示名。
	DisplayName *string
	// Email 是可选的邮箱（可显式置空）。
	Email *string
	// SetEmail 表示是否显式提供 Email（区分省略与置空）。
	SetEmail bool
	// Status 是可选的替换用户状态。
	Status *string
}

// IdentityStore 是身份用例所需的持久化边界。
type IdentityStore interface {
	// CreateUser 承载 IdentityStore 的生成 CreateUser 值。
	CreateUser(context.Context, NewUser) (User, error)
	// UserByUsername 承载 IdentityStore 的生成 UserByUsername 值。
	UserByUsername(context.Context, string) (User, string, error)
	// UserByID 承载 IdentityStore 的生成 UserByID 值。
	UserByID(context.Context, uuid.UUID) (User, error)
	// ListUsers 承载 IdentityStore 的生成 ListUsers 值。
	ListUsers(context.Context, string, int32, int32) ([]User, int64, error)
	// PromotePlatformAdmin 承载 IdentityStore 的生成 PromotePlatformAdmin 值。
	PromotePlatformAdmin(context.Context, uuid.UUID, time.Time) (User, error)
	// CreateRefreshToken 承载 IdentityStore 的生成 CreateRefreshToken 值。
	CreateRefreshToken(context.Context, NewRefreshToken) error
	// RefreshTokenPrincipalByDigest 承载 IdentityStore 的生成 RefreshTokenPrincipalByDigest 值。
	RefreshTokenPrincipalByDigest(context.Context, []byte, time.Time) (RefreshTokenPrincipal, error)
	// RotateRefreshToken 承载 IdentityStore 的生成 RotateRefreshToken 值。
	RotateRefreshToken(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	// RevokeRefreshTokenFamily 承载 IdentityStore 的生成 RevokeRefreshTokenFamily 值。
	RevokeRefreshTokenFamily(context.Context, uuid.UUID, time.Time) error
	// ActiveMemberships 承载 IdentityStore 的生成 ActiveMemberships 值。
	ActiveMemberships(context.Context, uuid.UUID) ([]Membership, error)
	// ActiveMembership 承载 IdentityStore 的生成 ActiveMembership 值。
	ActiveMembership(context.Context, uuid.UUID, string) (Membership, error)
	// CreateTenant 承载 IdentityStore 的生成 CreateTenant 值。
	CreateTenant(context.Context, NewTenant) (Tenant, error)
	// TenantBySlug 承载 IdentityStore 的生成 TenantBySlug 值。
	TenantBySlug(context.Context, string) (Tenant, error)
	// ListTenants 承载 IdentityStore 的生成 ListTenants 值。
	ListTenants(context.Context, int32, int32) ([]Tenant, int64, error)
	// UpdateTenant 承载 IdentityStore 的生成 UpdateTenant 值。
	UpdateTenant(context.Context, UpdateTenant) (Tenant, error)
	// PutMembership 承载 IdentityStore 的生成 PutMembership 值。
	PutMembership(context.Context, uuid.UUID, uuid.UUID, string, time.Time) (Membership, error)
	// CreateToken 承载 IdentityStore 的生成 CreateToken 值。
	CreateToken(context.Context, NewToken) (Token, error)
	// PATPrincipalByDigest 承载 IdentityStore 的生成 PATPrincipalByDigest 值。
	PATPrincipalByDigest(context.Context, []byte, time.Time) (Principal, error)
	// TouchToken 承载 IdentityStore 的生成 TouchToken 值。
	TouchToken(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	// ListTokens 承载 IdentityStore 的生成 ListTokens 值。
	ListTokens(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]Token, int64, error)
	// RevokeToken 承载 IdentityStore 的生成 RevokeToken 值。
	RevokeToken(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
	// UpdateUserProfile 承载 IdentityStore 的生成 UpdateUserProfile 值。
	UpdateUserProfile(context.Context, uuid.UUID, int64, UpdateUserProfileInput) (User, error)
	// UpdateUserPassword 承载 IdentityStore 的生成 UpdateUserPassword 值。
	UpdateUserPassword(context.Context, uuid.UUID, int64, string) (User, error)
	// UserPasswordHash 承载 IdentityStore 的生成 UserPasswordHash 值。
	UserPasswordHash(context.Context, uuid.UUID) (string, error)
	// DeleteTenant 承载 IdentityStore 的生成 DeleteTenant 值。
	DeleteTenant(context.Context, uuid.UUID, int64) (TenantDeletionAccepted, error)
}
