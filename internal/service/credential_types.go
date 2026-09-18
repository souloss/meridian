package service

import (
	"context"
	"time"
	"uuid"
)

// CredentialRecord 是租户持有凭据的持久化投影。
// Encrypted 是仅写入的存储材料，绝不映射到 API 响应。
type CredentialRecord struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是凭据的标识。
	ID uuid.UUID
	// IsGlobal 表示是否为平台全局凭据。
	IsGlobal bool
	// Name 是凭据名。
	Name string
	// Kind 是凭据类别（ssh_key/http_token）。
	Kind string
	// Encrypted 是加密后的秘密材料（仅写入）。
	Encrypted EncryptedCredential
	// SharedScope 是共享作用域（private/tenant/team）。
	SharedScope string
	// TeamIDs 是团队共享范围下的团队 ID 列表。
	TeamIDs []uuid.UUID
	// CreatedBy 是创建者标识。
	CreatedBy uuid.UUID
	// LastUsedAt 是最近使用时间（可为空）。
	LastUsedAt *time.Time
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// GlobalCredentialRecord 是平台持有凭据的持久化投影。
// Encrypted 是仅写入的存储材料，绝不映射到 API 响应。
type GlobalCredentialRecord struct {
	// ID 是凭据的标识。
	ID uuid.UUID
	// Name 是凭据名。
	Name string
	// Kind 是凭据类别（ssh_key/http_token）。
	Kind string
	// Encrypted 是加密后的秘密材料（仅写入）。
	Encrypted EncryptedCredential
	// CreatedBy 是创建者标识。
	CreatedBy uuid.UUID
	// LastUsedAt 是最近使用时间（可为空）。
	LastUsedAt *time.Time
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// CredentialInput 承载校验后的元数据与一个仅写入的秘密，用于创建。
type CredentialInput struct {
	// Name 是凭据名。
	Name string
	// Secret 是仅写入的秘密材料。
	Secret CredentialSecret
	// SharedScope 是共享作用域（private/tenant/team）。
	SharedScope string
	// TeamIDs 是团队共享范围下的团队 ID 列表。
	TeamIDs []uuid.UUID
}

// CredentialPatch 承载可在不轮换秘密的情况下修改的元数据。
type CredentialPatch struct {
	// Name 是新的凭据名（可为空）。
	Name *string
	// SharedScope 是新的共享作用域（可为空）。
	SharedScope *string
	// TeamIDs 是新的团队 ID 列表（可为空）。
	TeamIDs *[]uuid.UUID
}

// CredentialRotation 承载一个仅写入的替换秘密与请求的重同步行为。
type CredentialRotation struct {
	// Secret 是仅写入的替换秘密。
	Secret CredentialSecret
	// ResyncRepositories 表示是否重同步引用仓库。
	ResyncRepositories bool
	// IdempotencyKey 标识语义轮换请求；零值对内部调用方禁用重放持久化。
	IdempotencyKey uuid.UUID
	// RequestHash 是 32 字节规范摘要，不含认证信息与幂等键。
	RequestHash []byte
	// PrincipalType 与 PrincipalID 标识认证后的重放边界。
	PrincipalType string
	PrincipalID   uuid.UUID
}

// CredentialSyncJob 是凭据轮换入队仓库工作后返回的最小元数据。
type CredentialSyncJob struct {
	// TenantSlug 是所属租户 slug。
	TenantSlug string `json:"tenantSlug"`
	// RepositoryID 是仓库标识。
	RepositoryID uuid.UUID `json:"repositoryId"`
	// JobID 是任务标识。
	JobID uuid.UUID `json:"jobId"`
	// Deduplicated 表示是否被语义合并去重。
	Deduplicated bool `json:"deduplicated"`
}

// NewCredential 是凭据用例写入的加密行。
type NewCredential struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是凭据的标识。
	ID uuid.UUID
	// Name 是凭据名。
	Name string
	// Kind 是凭据类别（ssh_key/http_token）。
	Kind string
	// Encrypted 是加密后的秘密材料。
	Encrypted EncryptedCredential
	// SharedScope 是共享作用域（private/tenant/team）。
	SharedScope string
	// TeamIDs 是团队 ID 列表。
	TeamIDs []uuid.UUID
	// CreatedBy 是创建者标识。
	CreatedBy uuid.UUID
}

// UpdateCredential 承载一次租户凭据的条件元数据更新。
type UpdateCredential struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是凭据的标识。
	ID uuid.UUID
	// ExpectedRevision 是乐观并发所需版本号。
	ExpectedRevision int64
	// Name 是新的凭据名（可为空）。
	Name *string
	// SharedScope 是新的共享作用域（可为空）。
	SharedScope *string
	// TeamIDs 是新的团队 ID 列表（可为空）。
	TeamIDs *[]uuid.UUID
	// UpdatedAt 是更新时间。
	UpdatedAt time.Time
}

// RotateCredential 承载一次租户凭据的条件秘密替换。
type RotateCredential struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是凭据的标识。
	ID uuid.UUID
	// ExpectedRevision 是乐观并发所需版本号。
	ExpectedRevision int64
	// Encrypted 是加密后的新秘密材料。
	Encrypted EncryptedCredential
	// ResyncRepositories 表示是否重同步引用仓库。
	ResyncRepositories bool
	// UpdatedAt 是更新时间。
	UpdatedAt time.Time
	// IdempotencyKey 是幂等键。
	IdempotencyKey uuid.UUID
	// RequestHash 是请求摘要。
	RequestHash []byte
	// PrincipalType 是重放主体类型。
	PrincipalType string
	// PrincipalID 是重放主体标识。
	PrincipalID uuid.UUID
}

// NewGlobalCredential 是平台凭据用例写入的加密行。
type NewGlobalCredential struct {
	// ID 是凭据的标识。
	ID uuid.UUID
	// Name 是凭据名。
	Name string
	// Kind 是凭据类别（ssh_key/http_token）。
	Kind string
	// Encrypted 是加密后的秘密材料。
	Encrypted EncryptedCredential
	// CreatedBy 是创建者标识。
	CreatedBy uuid.UUID
}

// UpdateGlobalCredential 承载一次全局凭据的条件元数据更新。
type UpdateGlobalCredential struct {
	// ID 是凭据的标识。
	ID uuid.UUID
	// ExpectedRevision 是乐观并发所需版本号。
	ExpectedRevision int64
	// Name 是新的凭据名。
	Name string
	// UpdatedAt 是更新时间。
	UpdatedAt time.Time
}

// RotateGlobalCredential 承载一次全局凭据的条件秘密替换。
type RotateGlobalCredential struct {
	// ID 是凭据的标识。
	ID uuid.UUID
	// ExpectedRevision 是乐观并发所需版本号。
	ExpectedRevision int64
	// Encrypted 是加密后的新秘密材料。
	Encrypted EncryptedCredential
	// ResyncRepositories 表示是否重同步引用仓库。
	ResyncRepositories bool
	// UpdatedAt 是更新时间。
	UpdatedAt time.Time
	// IdempotencyKey 是幂等键。
	IdempotencyKey uuid.UUID
	// RequestHash 是请求摘要。
	RequestHash []byte
	// PrincipalType 是重放主体类型。
	PrincipalType string
	// PrincipalID 是重放主体标识。
	PrincipalID uuid.UUID
}

// NewKnownHost 承载一个完全派生并校验的主机密钥身份。
type NewKnownHost struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是已知主机的标识。
	ID uuid.UUID
	// Host 是规范化主机名。
	Host string
	// Port 是端口。
	Port int32
	// KeyType 是 SSH 公钥算法。
	KeyType string
	// PublicKey 是 RFC 4253 公钥 blob。
	PublicKey []byte
	// Fingerprint 是 OpenSSH SHA-256 指纹。
	Fingerprint string
	// Source 是来源（manual）。
	Source string
	// CreatedBy 是创建者标识。
	CreatedBy uuid.UUID
}

// KnownHostRecord 是已批准主机密钥的持久化投影。
type KnownHostRecord struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// ID 是已知主机的标识。
	ID uuid.UUID
	// Host 是规范化主机名。
	Host string
	// Port 是端口。
	Port int32
	// KeyType 是 SSH 公钥算法。
	KeyType string
	// PublicKey 是 RFC 4253 公钥 blob。
	PublicKey []byte
	// Fingerprint 是 OpenSSH SHA-256 指纹。
	Fingerprint string
	// Source 是来源（manual）。
	Source string
	// CreatedBy 是创建者标识（可为空）。
	CreatedBy *uuid.UUID
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// CredentialStore 是凭据与已知主机用例的持久化边界。
// 实现必须在每个查询中强制租户谓词，并让加密秘密远离日志。
type CredentialStore interface {
	CreateCredential(context.Context, NewCredential) (CredentialRecord, error)
	ListCredentials(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]CredentialRecord, int64, error)
	GetCredential(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (CredentialRecord, error)
	UpdateCredential(context.Context, UpdateCredential) (CredentialRecord, error)
	DeleteCredential(context.Context, uuid.UUID, uuid.UUID, int64, bool, time.Time) error
	LookupCredentialRotation(context.Context, uuid.UUID, string, uuid.UUID, uuid.UUID, []byte) (CredentialRecord, []CredentialSyncJob, bool, error)
	RotateCredential(context.Context, RotateCredential) (CredentialRecord, []CredentialSyncJob, error)

	ListGlobalCredentials(context.Context, int32, int32) ([]GlobalCredentialRecord, int64, error)
	CreateGlobalCredential(context.Context, NewGlobalCredential) (GlobalCredentialRecord, error)
	GetGlobalCredential(context.Context, uuid.UUID) (GlobalCredentialRecord, error)
	UpdateGlobalCredential(context.Context, UpdateGlobalCredential) (GlobalCredentialRecord, error)
	DeleteGlobalCredential(context.Context, uuid.UUID, int64, bool, time.Time) error
	LookupGlobalCredentialRotation(context.Context, string, uuid.UUID, uuid.UUID, []byte) (GlobalCredentialRecord, []CredentialSyncJob, bool, error)
	RotateGlobalCredential(context.Context, RotateGlobalCredential) (GlobalCredentialRecord, []CredentialSyncJob, error)

	ListKnownHosts(context.Context, uuid.UUID, int32, int32) ([]KnownHostRecord, int64, error)
	CreateKnownHost(context.Context, NewKnownHost) (KnownHostRecord, error)
}
