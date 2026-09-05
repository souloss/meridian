package service

import (
	"context"
	"time"
	"uuid"
)

// CredentialRecord is the persistence projection of a tenant-owned credential.
// Encrypted contains write-only storage material and is never mapped to an API response.
type CredentialRecord struct {
	TenantID    uuid.UUID
	ID          uuid.UUID
	IsGlobal    bool
	Name        string
	Kind        string
	Encrypted   EncryptedCredential
	SharedScope string
	TeamIDs     []uuid.UUID
	CreatedBy   uuid.UUID
	LastUsedAt  *time.Time
	Revision    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// GlobalCredentialRecord is the persistence projection of a platform-owned credential.
// Encrypted contains write-only storage material and is never mapped to an API response.
type GlobalCredentialRecord struct {
	ID         uuid.UUID
	Name       string
	Kind       string
	Encrypted  EncryptedCredential
	CreatedBy  uuid.UUID
	LastUsedAt *time.Time
	Revision   int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CredentialInput contains validated metadata and one write-only secret for creation.
type CredentialInput struct {
	Name        string
	Secret      CredentialSecret
	SharedScope string
	TeamIDs     []uuid.UUID
}

// CredentialPatch contains metadata that may be changed without rotating a secret.
type CredentialPatch struct {
	Name        *string
	SharedScope *string
	TeamIDs     *[]uuid.UUID
}

// CredentialRotation contains one write-only replacement secret and the requested resync behavior.
type CredentialRotation struct {
	Secret             CredentialSecret
	ResyncRepositories bool
}

// CredentialSyncJob is the minimal metadata returned when a credential rotation enqueues repository work.
type CredentialSyncJob struct {
	TenantSlug   string
	RepositoryID uuid.UUID
	JobID        uuid.UUID
	Deduplicated bool
}

// NewCredential is the encrypted row written by the credential use case.
type NewCredential struct {
	TenantID    uuid.UUID
	ID          uuid.UUID
	Name        string
	Kind        string
	Encrypted   EncryptedCredential
	SharedScope string
	TeamIDs     []uuid.UUID
	CreatedBy   uuid.UUID
}

// UpdateCredential contains a conditional metadata update for one tenant credential.
type UpdateCredential struct {
	TenantID         uuid.UUID
	ID               uuid.UUID
	ExpectedRevision int64
	Name             *string
	SharedScope      *string
	TeamIDs          *[]uuid.UUID
	UpdatedAt        time.Time
}

// RotateCredential contains a conditional secret replacement for one tenant credential.
type RotateCredential struct {
	TenantID           uuid.UUID
	ID                 uuid.UUID
	ExpectedRevision   int64
	Encrypted          EncryptedCredential
	ResyncRepositories bool
	UpdatedAt          time.Time
}

// NewGlobalCredential is the encrypted row written by the platform credential use case.
type NewGlobalCredential struct {
	ID        uuid.UUID
	Name      string
	Kind      string
	Encrypted EncryptedCredential
	CreatedBy uuid.UUID
}

// UpdateGlobalCredential contains a conditional metadata update for one global credential.
type UpdateGlobalCredential struct {
	ID               uuid.UUID
	ExpectedRevision int64
	Name             string
	UpdatedAt        time.Time
}

// RotateGlobalCredential contains a conditional secret replacement for one global credential.
type RotateGlobalCredential struct {
	ID                 uuid.UUID
	ExpectedRevision   int64
	Encrypted          EncryptedCredential
	ResyncRepositories bool
	UpdatedAt          time.Time
}

// NewKnownHost contains a fully derived, validated host-key identity.
type NewKnownHost struct {
	TenantID    uuid.UUID
	ID          uuid.UUID
	Host        string
	Port        int32
	KeyType     string
	PublicKey   []byte
	Fingerprint string
	Source      string
	CreatedBy   uuid.UUID
}

// KnownHostRecord is the persistence projection of an approved host key.
type KnownHostRecord struct {
	TenantID    uuid.UUID
	ID          uuid.UUID
	Host        string
	Port        int32
	KeyType     string
	PublicKey   []byte
	Fingerprint string
	Source      string
	CreatedBy   *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CredentialStore is the persistence boundary for credential and known-host use cases.
// Implementations must enforce tenant predicates in every query and keep encrypted secrets out of logs.
type CredentialStore interface {
	CreateCredential(context.Context, NewCredential) (CredentialRecord, error)
	ListCredentials(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]CredentialRecord, int64, error)
	GetCredential(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (CredentialRecord, error)
	UpdateCredential(context.Context, UpdateCredential) (CredentialRecord, error)
	DeleteCredential(context.Context, uuid.UUID, uuid.UUID, int64, bool, time.Time) error
	RotateCredential(context.Context, RotateCredential) (CredentialRecord, []CredentialSyncJob, error)

	ListGlobalCredentials(context.Context, int32, int32) ([]GlobalCredentialRecord, int64, error)
	CreateGlobalCredential(context.Context, NewGlobalCredential) (GlobalCredentialRecord, error)
	GetGlobalCredential(context.Context, uuid.UUID) (GlobalCredentialRecord, error)
	UpdateGlobalCredential(context.Context, UpdateGlobalCredential) (GlobalCredentialRecord, error)
	DeleteGlobalCredential(context.Context, uuid.UUID, int64, bool, time.Time) error
	RotateGlobalCredential(context.Context, RotateGlobalCredential) (GlobalCredentialRecord, []CredentialSyncJob, error)

	ListKnownHosts(context.Context, uuid.UUID, int32, int32) ([]KnownHostRecord, int64, error)
	CreateKnownHost(context.Context, NewKnownHost) (KnownHostRecord, error)
}
