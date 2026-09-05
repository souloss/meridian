package repository

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// CredentialStore implements credential persistence with explicit transaction boundaries.
type CredentialStore struct {
	pool    *pgxpool.Pool
	queries *generated.Queries
}

// NewCredentialStore binds credential persistence to a native pgx pool.
func NewCredentialStore(pool *pgxpool.Pool) *CredentialStore {
	return &CredentialStore{pool: pool, queries: generated.New(pool)}
}

// CreateCredential inserts encrypted metadata and all team shares atomically.
func (store *CredentialStore) CreateCredential(ctx context.Context, input service.NewCredential) (service.CredentialRecord, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.CredentialRecord{}, fmt.Errorf("begin create credential transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	row, err := queries.CreateCredential(ctx, generated.CreateCredentialParams{
		TenantID: input.TenantID, ID: input.ID, Name: input.Name, Kind: input.Kind,
		Ciphertext: input.Encrypted.Ciphertext, Nonce: input.Encrypted.Nonce,
		KeyVersion: input.Encrypted.KeyVersion, Fingerprint: input.Encrypted.Fingerprint,
		SharedScope: input.SharedScope, CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return service.CredentialRecord{}, normalizeError(err)
	}
	if err := replaceCredentialShares(ctx, queries, input.TenantID, input.ID, input.TeamIDs); err != nil {
		return service.CredentialRecord{}, normalizeError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return service.CredentialRecord{}, normalizeError(err)
	}
	return credentialFromRow(row, input.TeamIDs), nil
}

// ListCredentials returns tenant-visible records, including all global credentials.
func (store *CredentialStore) ListCredentials(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]service.CredentialRecord, int64, error) {
	total, err := store.queries.CountTenantCredentials(ctx, generated.CountTenantCredentialsParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListTenantCredentials(ctx, generated.ListTenantCredentialsParams{
		TenantID: tenantID, UserID: userID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.CredentialRecord, 0, len(rows))
	for _, row := range rows {
		teamIDs, err := decodeTeamIDs(row.TeamIds)
		if err != nil {
			return nil, 0, fmt.Errorf("decode credential %s team shares: %w", row.ID, err)
		}
		items = append(items, visibleCredentialFromRow(row, teamIDs))
	}
	return items, int64(total), nil
}

// GetCredential returns one visible tenant-owned credential.
func (store *CredentialStore) GetCredential(ctx context.Context, tenantID, id, userID uuid.UUID) (service.CredentialRecord, error) {
	row, err := store.queries.GetTenantCredential(ctx, generated.GetTenantCredentialParams{TenantID: tenantID, ID: id, UserID: userID})
	if err != nil {
		return service.CredentialRecord{}, normalizeError(err)
	}
	teamIDs, err := decodeTeamIDs(row.TeamIds)
	if err != nil {
		return service.CredentialRecord{}, fmt.Errorf("decode credential %s team shares: %w", row.ID, err)
	}
	return service.CredentialRecord{
		TenantID: row.TenantID, ID: row.ID, Name: row.Name, Kind: row.Kind,
		Encrypted:   service.EncryptedCredential{Ciphertext: row.Ciphertext, Nonce: row.Nonce, KeyVersion: row.KeyVersion, Fingerprint: row.Fingerprint},
		SharedScope: row.SharedScope, TeamIDs: teamIDs, CreatedBy: row.CreatedBy,
		LastUsedAt: timePointer(row.LastUsedAt), Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

// UpdateCredential conditionally updates metadata and replaces team shares in one transaction.
func (store *CredentialStore) UpdateCredential(ctx context.Context, input service.UpdateCredential) (service.CredentialRecord, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.CredentialRecord{}, fmt.Errorf("begin update credential transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	current, err := queries.GetTenantCredentialForMutation(ctx, generated.GetTenantCredentialForMutationParams{TenantID: input.TenantID, ID: input.ID})
	if err != nil {
		return service.CredentialRecord{}, normalizeError(err)
	}
	if current.Revision != input.ExpectedRevision {
		return service.CredentialRecord{}, service.ErrPrecondition
	}
	resultingScope := current.SharedScope
	if input.SharedScope != nil {
		resultingScope = *input.SharedScope
	}
	if resultingScope != "team" && len(valueOrEmpty(input.TeamIDs)) != 0 {
		return service.CredentialRecord{}, service.ErrValidation
	}
	row, err := queries.UpdateCredentialMetadata(ctx, generated.UpdateCredentialMetadataParams{
		Name: input.Name, SharedScope: input.SharedScope, UpdatedAt: timestamp(input.UpdatedAt),
		TenantID: input.TenantID, ID: input.ID, ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.CredentialRecord{}, service.ErrPrecondition
		}
		return service.CredentialRecord{}, normalizeError(err)
	}
	teamIDs := []uuid.UUID(nil)
	if input.SharedScope != nil || input.TeamIDs != nil {
		teamIDs = valueOrEmpty(input.TeamIDs)
		if err := replaceCredentialShares(ctx, queries, input.TenantID, input.ID, teamIDs); err != nil {
			return service.CredentialRecord{}, normalizeError(err)
		}
	} else {
		teamIDs, err = listCredentialShares(ctx, queries, input.TenantID, input.ID)
		if err != nil {
			return service.CredentialRecord{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return service.CredentialRecord{}, normalizeError(err)
	}
	return credentialFromRow(row, teamIDs), nil
}

// DeleteCredential conditionally deletes a tenant credential and optionally unbinds references.
func (store *CredentialStore) DeleteCredential(ctx context.Context, tenantID, id uuid.UUID, expectedRevision int64, force bool, updatedAt time.Time) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin delete credential transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	current, err := queries.GetTenantCredentialForMutation(ctx, generated.GetTenantCredentialForMutationParams{TenantID: tenantID, ID: id})
	if err != nil {
		return normalizeError(err)
	}
	if current.Revision != expectedRevision {
		return service.ErrPrecondition
	}
	count, err := queries.CountCredentialRepositories(ctx, generated.CountCredentialRepositoriesParams{TenantID: tenantID, CredentialID: new(id)})
	if err != nil {
		return normalizeError(err)
	}
	if count > 0 && !force {
		return service.ErrCredentialInUse
	}
	if force {
		if err := queries.UnbindCredentialRepositories(ctx, generated.UnbindCredentialRepositoriesParams{
			UpdatedAt: timestamp(updatedAt), TenantID: tenantID, CredentialID: new(id),
		}); err != nil {
			return normalizeError(err)
		}
	}
	deleted, err := queries.DeleteCredential(ctx, generated.DeleteCredentialParams{TenantID: tenantID, ID: id, ExpectedRevision: expectedRevision})
	if err != nil {
		return normalizeError(err)
	}
	if deleted != 1 {
		return service.ErrPrecondition
	}
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	return nil
}

// RotateCredential atomically updates encrypted secret material and optional repository sync jobs.
func (store *CredentialStore) RotateCredential(ctx context.Context, input service.RotateCredential) (service.CredentialRecord, []service.CredentialSyncJob, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.CredentialRecord{}, nil, fmt.Errorf("begin rotate credential transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	current, err := queries.GetTenantCredentialForMutation(ctx, generated.GetTenantCredentialForMutationParams{TenantID: input.TenantID, ID: input.ID})
	if err != nil {
		return service.CredentialRecord{}, nil, normalizeError(err)
	}
	if current.Revision != input.ExpectedRevision {
		return service.CredentialRecord{}, nil, service.ErrPrecondition
	}
	row, err := queries.RotateCredentialSecret(ctx, generated.RotateCredentialSecretParams{
		Ciphertext: input.Encrypted.Ciphertext, Nonce: input.Encrypted.Nonce,
		KeyVersion: input.Encrypted.KeyVersion, Fingerprint: input.Encrypted.Fingerprint,
		UpdatedAt: timestamp(input.UpdatedAt), TenantID: input.TenantID, ID: input.ID,
		ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.CredentialRecord{}, nil, service.ErrPrecondition
		}
		return service.CredentialRecord{}, nil, normalizeError(err)
	}
	teamIDs, err := listCredentialShares(ctx, queries, input.TenantID, input.ID)
	if err != nil {
		return service.CredentialRecord{}, nil, err
	}
	var jobs []service.CredentialSyncJob
	if input.ResyncRepositories {
		references, err := queries.ListRepositoriesForCredential(ctx, generated.ListRepositoriesForCredentialParams{
			TenantID: input.TenantID, CredentialID: new(input.ID),
		})
		if err != nil {
			return service.CredentialRecord{}, nil, normalizeError(err)
		}
		jobs, err = enqueueCredentialSyncJobs(ctx, queries, credentialSyncReferences(references), input.ID)
		if err != nil {
			return service.CredentialRecord{}, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return service.CredentialRecord{}, nil, normalizeError(err)
	}
	return credentialFromRow(row, teamIDs), jobs, nil
}

// ListGlobalCredentials returns one page of platform-owned credential metadata.
func (store *CredentialStore) ListGlobalCredentials(ctx context.Context, limit, offset int32) ([]service.GlobalCredentialRecord, int64, error) {
	total, err := store.queries.CountGlobalCredentials(ctx)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListGlobalCredentials(ctx, generated.ListGlobalCredentialsParams{PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.GlobalCredentialRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, globalCredentialFromRow(row))
	}
	return items, total, nil
}

// CreateGlobalCredential inserts one platform-owned encrypted credential.
func (store *CredentialStore) CreateGlobalCredential(ctx context.Context, input service.NewGlobalCredential) (service.GlobalCredentialRecord, error) {
	row, err := store.queries.CreateGlobalCredential(ctx, generated.CreateGlobalCredentialParams{
		ID: input.ID, Name: input.Name, Kind: input.Kind, Ciphertext: input.Encrypted.Ciphertext,
		Nonce: input.Encrypted.Nonce, KeyVersion: input.Encrypted.KeyVersion,
		Fingerprint: input.Encrypted.Fingerprint, CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return service.GlobalCredentialRecord{}, normalizeError(err)
	}
	return globalCredentialFromRow(row), nil
}

// GetGlobalCredential returns one platform-owned encrypted credential projection.
func (store *CredentialStore) GetGlobalCredential(ctx context.Context, id uuid.UUID) (service.GlobalCredentialRecord, error) {
	row, err := store.queries.GetGlobalCredential(ctx, id)
	if err != nil {
		return service.GlobalCredentialRecord{}, normalizeError(err)
	}
	return globalCredentialFromRow(row), nil
}

// UpdateGlobalCredential conditionally updates a global credential name.
func (store *CredentialStore) UpdateGlobalCredential(ctx context.Context, input service.UpdateGlobalCredential) (service.GlobalCredentialRecord, error) {
	row, err := store.queries.UpdateGlobalCredentialMetadata(ctx, generated.UpdateGlobalCredentialMetadataParams{
		ID: input.ID, ExpectedRevision: input.ExpectedRevision, Name: input.Name, UpdatedAt: timestamp(input.UpdatedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.GlobalCredentialRecord{}, service.ErrPrecondition
		}
		return service.GlobalCredentialRecord{}, normalizeError(err)
	}
	return globalCredentialFromRow(row), nil
}

// DeleteGlobalCredential conditionally deletes a platform credential and optionally unbinds references.
func (store *CredentialStore) DeleteGlobalCredential(ctx context.Context, id uuid.UUID, expectedRevision int64, force bool, updatedAt time.Time) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin delete global credential transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	current, err := queries.GetGlobalCredential(ctx, id)
	if err != nil {
		return normalizeError(err)
	}
	if current.Revision != expectedRevision {
		return service.ErrPrecondition
	}
	count, err := queries.CountGlobalCredentialRepositories(ctx, new(id))
	if err != nil {
		return normalizeError(err)
	}
	if count > 0 && !force {
		return service.ErrCredentialInUse
	}
	if force {
		if err := queries.UnbindGlobalCredentialRepositories(ctx, generated.UnbindGlobalCredentialRepositoriesParams{
			UpdatedAt: timestamp(updatedAt), CredentialID: new(id),
		}); err != nil {
			return normalizeError(err)
		}
	}
	deleted, err := queries.DeleteGlobalCredential(ctx, generated.DeleteGlobalCredentialParams{ID: id, ExpectedRevision: expectedRevision})
	if err != nil {
		return normalizeError(err)
	}
	if deleted != 1 {
		return service.ErrPrecondition
	}
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	return nil
}

// RotateGlobalCredential atomically updates a platform credential secret and optional sync jobs.
func (store *CredentialStore) RotateGlobalCredential(ctx context.Context, input service.RotateGlobalCredential) (service.GlobalCredentialRecord, []service.CredentialSyncJob, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.GlobalCredentialRecord{}, nil, fmt.Errorf("begin rotate global credential transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	row, err := queries.RotateGlobalCredentialSecret(ctx, generated.RotateGlobalCredentialSecretParams{
		ID: input.ID, ExpectedRevision: input.ExpectedRevision, Ciphertext: input.Encrypted.Ciphertext,
		Nonce: input.Encrypted.Nonce, KeyVersion: input.Encrypted.KeyVersion, Fingerprint: input.Encrypted.Fingerprint,
		UpdatedAt: timestamp(input.UpdatedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return service.GlobalCredentialRecord{}, nil, service.ErrPrecondition
		}
		return service.GlobalCredentialRecord{}, nil, normalizeError(err)
	}
	var jobs []service.CredentialSyncJob
	if input.ResyncRepositories {
		references, err := queries.ListRepositoriesForGlobalCredential(ctx, new(input.ID))
		if err != nil {
			return service.GlobalCredentialRecord{}, nil, normalizeError(err)
		}
		jobs, err = enqueueCredentialSyncJobs(ctx, queries, globalCredentialSyncReferences(references), input.ID)
		if err != nil {
			return service.GlobalCredentialRecord{}, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return service.GlobalCredentialRecord{}, nil, normalizeError(err)
	}
	return globalCredentialFromRow(row), jobs, nil
}

// ListKnownHosts returns approved host identities without private material.
func (store *CredentialStore) ListKnownHosts(ctx context.Context, tenantID uuid.UUID, limit, offset int32) ([]service.KnownHostRecord, int64, error) {
	total, err := store.queries.CountKnownHosts(ctx, tenantID)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListKnownHosts(ctx, generated.ListKnownHostsParams{TenantID: tenantID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.KnownHostRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, knownHostFromRow(row))
	}
	return items, total, nil
}

// CreateKnownHost stores one server-derived host-key identity.
func (store *CredentialStore) CreateKnownHost(ctx context.Context, input service.NewKnownHost) (service.KnownHostRecord, error) {
	row, err := store.queries.CreateKnownHost(ctx, generated.CreateKnownHostParams{
		TenantID: input.TenantID, ID: input.ID, Host: input.Host, Port: input.Port,
		KeyType: input.KeyType, PublicKey: input.PublicKey, Fingerprint: input.Fingerprint,
		Source: input.Source, CreatedBy: new(input.CreatedBy),
	})
	if err != nil {
		return service.KnownHostRecord{}, normalizeError(err)
	}
	return knownHostFromRow(row), nil
}

func replaceCredentialShares(ctx context.Context, queries *generated.Queries, tenantID, credentialID uuid.UUID, teamIDs []uuid.UUID) error {
	if err := queries.ReplaceCredentialTeamShares(ctx, generated.ReplaceCredentialTeamSharesParams{TenantID: tenantID, CredentialID: credentialID}); err != nil {
		return err
	}
	for _, teamID := range teamIDs {
		if err := queries.AddCredentialTeamShare(ctx, generated.AddCredentialTeamShareParams{TenantID: tenantID, CredentialID: credentialID, TeamID: teamID}); err != nil {
			return err
		}
	}
	return nil
}

func listCredentialShares(ctx context.Context, queries *generated.Queries, tenantID, credentialID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := queries.ListCredentialTeamShares(ctx, generated.ListCredentialTeamSharesParams{TenantID: tenantID, CredentialID: credentialID})
	if err != nil {
		return nil, normalizeError(err)
	}
	return rows, nil
}

func decodeTeamIDs(value string) ([]uuid.UUID, error) {
	var teamIDs []uuid.UUID
	if err := json.Unmarshal([]byte(value), &teamIDs); err != nil {
		return nil, err
	}
	return teamIDs, nil
}

func valueOrEmpty(value *[]uuid.UUID) []uuid.UUID {
	if value == nil {
		return nil
	}
	return *value
}

type credentialSyncReference struct {
	tenantID      uuid.UUID
	tenantSlug    string
	repositoryID  uuid.UUID
	defaultBranch string
}

func credentialSyncReferences(references []generated.ListRepositoriesForCredentialRow) []credentialSyncReference {
	converted := make([]credentialSyncReference, len(references))
	for index, reference := range references {
		converted[index] = credentialSyncReference{tenantID: reference.TenantID, tenantSlug: reference.TenantSlug, repositoryID: reference.RepositoryID, defaultBranch: reference.DefaultBranch}
	}
	return converted
}

func globalCredentialSyncReferences(references []generated.ListRepositoriesForGlobalCredentialRow) []credentialSyncReference {
	converted := make([]credentialSyncReference, len(references))
	for index, reference := range references {
		converted[index] = credentialSyncReference{tenantID: reference.TenantID, tenantSlug: reference.TenantSlug, repositoryID: reference.RepositoryID, defaultBranch: reference.DefaultBranch}
	}
	return converted
}

func enqueueCredentialSyncJobs(ctx context.Context, queries *generated.Queries, references []credentialSyncReference, credentialID uuid.UUID) ([]service.CredentialSyncJob, error) {
	jobs := make([]service.CredentialSyncJob, 0, len(references))
	for _, reference := range references {
		job, err := enqueueCredentialSyncJob(ctx, queries, reference.tenantID, reference.tenantSlug, reference.repositoryID, reference.defaultBranch, credentialID)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func enqueueCredentialSyncJob(ctx context.Context, queries *generated.Queries, tenantID uuid.UUID, tenantSlug string, repositoryID uuid.UUID, refName string, credentialID uuid.UUID) (service.CredentialSyncJob, error) {
	dedupeKey := "repository:" + repositoryID.String() + ":branch:" + refName
	jobInput, err := json.Marshal(struct {
		CredentialID uuid.UUID `json:"credentialId"`
		Reason       string    `json:"reason"`
	}{CredentialID: credentialID, Reason: "credential-rotated"})
	if err != nil {
		return service.CredentialSyncJob{}, fmt.Errorf("encode credential sync job input: %w", err)
	}
	for {
		latest, err := queries.LockLatestCredentialSyncJob(ctx, generated.LockLatestCredentialSyncJobParams{TenantID: tenantID, DedupeKey: dedupeKey})
		generation := int64(1)
		if err == nil {
			if latest.Status == "pending" || latest.Status == "running" {
				return service.CredentialSyncJob{TenantSlug: tenantSlug, RepositoryID: repositoryID, JobID: latest.ID, Deduplicated: true}, nil
			}
			generation = latest.ActiveGeneration + 1
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return service.CredentialSyncJob{}, normalizeError(err)
		}

		jobID := uuid.NewV7()
		row, err := queries.CreateCredentialSyncJob(ctx, generated.CreateCredentialSyncJobParams{
			TenantID: tenantID, ID: jobID, RepositoryID: new(repositoryID), RefName: new(refName), JobInput: jobInput,
			DedupeKey: dedupeKey, ActiveGeneration: generation,
		})
		if err == nil {
			return service.CredentialSyncJob{TenantSlug: tenantSlug, RepositoryID: repositoryID, JobID: row.ID, Deduplicated: false}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return service.CredentialSyncJob{}, normalizeError(err)
		}
		// Another transaction won the unique dedupe key between the lock query and
		// insert. Its row is now visible to the next lock iteration.
	}
}

func credentialFromRow(row generated.Credential, teamIDs []uuid.UUID) service.CredentialRecord {
	return service.CredentialRecord{
		TenantID: row.TenantID, ID: row.ID, Name: row.Name, Kind: row.Kind,
		Encrypted:   service.EncryptedCredential{Ciphertext: row.Ciphertext, Nonce: row.Nonce, KeyVersion: row.KeyVersion, Fingerprint: row.Fingerprint},
		SharedScope: row.SharedScope, TeamIDs: teamIDs, CreatedBy: row.CreatedBy,
		LastUsedAt: timePointer(row.LastUsedAt), Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func visibleCredentialFromRow(row generated.ListTenantCredentialsRow, teamIDs []uuid.UUID) service.CredentialRecord {
	return service.CredentialRecord{
		TenantID: row.TenantID, ID: row.ID, IsGlobal: row.IsGlobal, Name: row.Name, Kind: row.Kind,
		Encrypted:   service.EncryptedCredential{Ciphertext: row.Ciphertext, Nonce: row.Nonce, KeyVersion: row.KeyVersion, Fingerprint: row.Fingerprint},
		SharedScope: row.SharedScope, TeamIDs: teamIDs, CreatedBy: row.CreatedBy,
		LastUsedAt: timePointer(row.LastUsedAt), Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func globalCredentialFromRow(row generated.GlobalCredential) service.GlobalCredentialRecord {
	return service.GlobalCredentialRecord{
		ID: row.ID, Name: row.Name, Kind: row.Kind,
		Encrypted: service.EncryptedCredential{Ciphertext: row.Ciphertext, Nonce: row.Nonce, KeyVersion: row.KeyVersion, Fingerprint: row.Fingerprint},
		CreatedBy: row.CreatedBy, LastUsedAt: timePointer(row.LastUsedAt), Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func knownHostFromRow(row generated.KnownHost) service.KnownHostRecord {
	return service.KnownHostRecord{
		TenantID: row.TenantID, ID: row.ID, Host: row.Host, Port: row.Port, KeyType: row.KeyType,
		PublicKey: row.PublicKey, Fingerprint: row.Fingerprint, Source: row.Source,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

var _ service.CredentialStore = (*CredentialStore)(nil)
