package repository

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
)

// RepositoryStore implements repository persistence with explicit tenant predicates.
type RepositoryStore struct {
	pool    *pgxpool.Pool
	queries *generated.Queries
}

// NewRepositoryStore binds repository persistence to a native pgx pool.
func NewRepositoryStore(pool *pgxpool.Pool) *RepositoryStore {
	return &RepositoryStore{pool: pool, queries: generated.New(pool)}
}

// ResolveCredentialReference resolves a visible tenant or global credential UUID.
func (store *RepositoryStore) ResolveCredentialReference(ctx context.Context, tenantID, userID, credentialID uuid.UUID) (service.CredentialReference, bool, error) {
	row, err := store.queries.ResolveRepositoryCredential(ctx, generated.ResolveRepositoryCredentialParams{TenantID: tenantID, UserID: userID, CredentialID: credentialID})
	if err != nil {
		if err == pgx.ErrNoRows {
			return service.CredentialReference{}, false, nil
		}
		return service.CredentialReference{}, false, normalizeError(err)
	}
	return service.CredentialReference{ID: row.ResolvedID, IsGlobal: row.IsGlobal}, true, nil
}

// CountRepositories returns active tenant count and the frozen repository quota.
func (store *RepositoryStore) CountRepositories(ctx context.Context, tenantID uuid.UUID) (int64, int64, error) {
	row, err := store.queries.CountRepositories(ctx, tenantID)
	if err != nil {
		return 0, 0, normalizeError(err)
	}
	return row.CurrentCount, row.LimitCount, nil
}

// ListRepositories returns one deterministic active repository page.
func (store *RepositoryStore) ListRepositories(ctx context.Context, tenantID uuid.UUID, query string, limit, offset int32) ([]service.RepositoryRecord, int64, error) {
	total, err := store.queries.CountListedRepositories(ctx, generated.CountListedRepositoriesParams{TenantID: tenantID, SearchQuery: query})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListRepositories(ctx, generated.ListRepositoriesParams{TenantID: tenantID, SearchQuery: query, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.RepositoryRecord, 0, len(rows))
	for _, row := range rows {
		item, err := repositoryFromRow(row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

// GetRepository returns one active repository inside the supplied tenant.
func (store *RepositoryStore) GetRepository(ctx context.Context, tenantID, id uuid.UUID) (service.RepositoryRecord, error) {
	row, err := store.queries.GetRepository(ctx, generated.GetRepositoryParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.RepositoryRecord{}, normalizeError(err)
	}
	return repositoryFromRow(row)
}

// CreateRepository inserts one repository and serializes its policy/config JSON atomically.
func (store *RepositoryStore) CreateRepository(ctx context.Context, input service.NewRepository) (service.RepositoryRecord, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.RepositoryRecord{}, fmt.Errorf("begin create repository transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	quota, err := queries.LockRepositoryQuota(ctx, input.TenantID)
	if err != nil {
		return service.RepositoryRecord{}, normalizeError(err)
	}
	count, err := queries.CountRepositories(ctx, input.TenantID)
	if err != nil {
		return service.RepositoryRecord{}, normalizeError(err)
	}
	if count.CurrentCount >= quota {
		return service.RepositoryRecord{}, &service.QuotaExceededError{Resource: "repositories", Current: count.CurrentCount, Limit: quota}
	}
	branchPolicy, err := json.Marshal(input.BranchPolicy)
	if err != nil {
		return service.RepositoryRecord{}, fmt.Errorf("encode repository branch policy: %w", err)
	}
	fetchConfig, err := json.Marshal(input.FetchConfig)
	if err != nil {
		return service.RepositoryRecord{}, fmt.Errorf("encode repository fetch config: %w", err)
	}
	row, err := queries.CreateRepository(ctx, generated.CreateRepositoryParams{
		TenantID: input.TenantID, ID: input.ID, Url: input.URL, CanonicalUrl: input.CanonicalURL,
		CredentialID: input.CredentialID, GlobalCredentialID: input.GlobalCredID, DefaultBranch: input.DefaultBranch,
		BranchPolicy: branchPolicy, FetchConfig: fetchConfig, SyncCron: input.SyncCron, Note: input.Note,
	})
	if err != nil {
		return service.RepositoryRecord{}, normalizeError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return service.RepositoryRecord{}, normalizeError(err)
	}
	return repositoryFromRow(row)
}

// UpdateRepository conditionally changes repository metadata and advances its ETag revision.
func (store *RepositoryStore) UpdateRepository(ctx context.Context, input service.UpdateRepository) (service.RepositoryRecord, error) {
	branchPolicy, err := optionalRepositoryJSON(input.BranchPolicy)
	if err != nil {
		return service.RepositoryRecord{}, err
	}
	fetchConfig, err := optionalRepositoryJSON(input.FetchConfig)
	if err != nil {
		return service.RepositoryRecord{}, err
	}
	var credentialID, globalCredentialID *uuid.UUID
	setCredential, setGlobal := input.CredentialID != nil, input.GlobalCredID != nil
	if setCredential && *input.CredentialID != nil {
		credentialID = new(**input.CredentialID)
	}
	if setGlobal && *input.GlobalCredID != nil {
		globalCredentialID = new(**input.GlobalCredID)
	}
	row, err := store.queries.UpdateRepository(ctx, generated.UpdateRepositoryParams{
		SetCredentialID: setCredential, CredentialID: credentialID,
		SetGlobalCredentialID: setGlobal, GlobalCredentialID: globalCredentialID,
		DefaultBranch: input.DefaultBranch, BranchPolicy: branchPolicy, FetchConfig: fetchConfig,
		SetSyncCron: input.SyncCron != nil, SyncCron: optionalStringPointer(input.SyncCron),
		SetNote: input.Note != nil, Note: optionalStringPointer(input.Note),
		UpdatedAt: timestamp(input.UpdatedAt), TenantID: input.TenantID, ID: input.ID, ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return service.RepositoryRecord{}, service.ErrPrecondition
		}
		return service.RepositoryRecord{}, normalizeError(err)
	}
	return repositoryFromRow(row)
}

// DeleteRepository soft-deletes one active repository under the expected revision.
func (store *RepositoryStore) DeleteRepository(ctx context.Context, tenantID, id uuid.UUID, expectedRevision int64, deletedAt time.Time) error {
	changed, err := store.queries.DeleteRepository(ctx, generated.DeleteRepositoryParams{DeletedAt: timestamp(deletedAt), UpdatedAt: timestamp(deletedAt), TenantID: tenantID, ID: id, ExpectedRevision: expectedRevision})
	if err != nil {
		return normalizeError(err)
	}
	if changed != 1 {
		return service.ErrPrecondition
	}
	return nil
}

func optionalRepositoryJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode repository JSON: %w", err)
	}
	return encoded, nil
}

func optionalStringPointer(value **string) *string {
	if value == nil || *value == nil {
		return nil
	}
	return new(**value)
}

type repositoryHealthWire struct {
	LastSyncAt *time.Time               `json:"lastSyncAt"`
	LastCommit *string                  `json:"lastCommit"`
	LastError  *service.RepositoryError `json:"lastError"`
	FailStreak int                      `json:"failStreak"`
	DurationMS *int                     `json:"durationMs"`
}

func repositoryFromRow(row generated.Repository) (service.RepositoryRecord, error) {
	var branchPolicy service.RepositoryBranchPolicy
	if err := json.Unmarshal(row.BranchPolicy, &branchPolicy); err != nil {
		return service.RepositoryRecord{}, fmt.Errorf("decode repository %s branch policy: %w", row.ID, err)
	}
	var fetchConfig service.RepositoryFetchConfig
	if err := json.Unmarshal(row.FetchConfig, &fetchConfig); err != nil {
		return service.RepositoryRecord{}, fmt.Errorf("decode repository %s fetch config: %w", row.ID, err)
	}
	var health repositoryHealthWire
	if err := json.Unmarshal(row.Health, &health); err != nil {
		return service.RepositoryRecord{}, fmt.Errorf("decode repository %s health: %w", row.ID, err)
	}
	credentialID := row.CredentialID
	if credentialID == nil {
		credentialID = row.GlobalCredentialID
	}
	return service.RepositoryRecord{
		TenantID: row.TenantID, ID: row.ID, URL: row.Url, CanonicalURL: row.CanonicalUrl, CredentialID: credentialID,
		GlobalCredentialID: row.GlobalCredentialID, DefaultBranch: row.DefaultBranch, BranchPolicy: branchPolicy, FetchConfig: fetchConfig,
		SyncCron: row.SyncCron, Note: row.Note,
		Health:   service.RepositoryHealth{LastSyncAt: health.LastSyncAt, LastCommit: health.LastCommit, LastError: health.LastError, FailStreak: health.FailStreak, DurationMS: health.DurationMS},
		Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

var _ service.RepositoryStore = (*RepositoryStore)(nil)
