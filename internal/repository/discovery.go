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
	"github.com/meridian-labs/meridian/internal/task"
	"github.com/riverqueue/river"
)

// DiscoveryStore implements repository discovery, candidate acceptance, service
// creation, producer configuration, and source configuration persistence. It
// embeds RepositoryStore to reuse the repository read path and adds a River
// client for transactional discovery job enqueueing.
type DiscoveryStore struct {
	*RepositoryStore
	riverClient *river.Client[pgx.Tx]
}

// NewDiscoveryStore binds discovery persistence to a native pgx pool.
func NewDiscoveryStore(pool *pgxpool.Pool) *DiscoveryStore {
	return NewDiscoveryStoreWithRiver(pool, nil)
}

// NewDiscoveryStoreWithRiver binds discovery persistence to a River client so
// newly enqueued discovery jobs join the same transaction as their domain row.
func NewDiscoveryStoreWithRiver(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *DiscoveryStore {
	return &DiscoveryStore{RepositoryStore: NewRepositoryStore(pool), riverClient: riverClient}
}

// BindRiver attaches the process River client after the runtime is constructed,
// so discovery jobs can be enqueued transactionally.
func (store *DiscoveryStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
}

// EnqueueDiscoveryJob records one repo.discover job and its River work atomically.
func (store *DiscoveryStore) EnqueueDiscoveryJob(ctx context.Context, input service.DiscoverJobInput) (service.JobAccepted, error) {
	dedupeKey := discoveryDedupeKey(input.RepositoryID, input.RefType, input.RefName)
	jobInput, err := json.Marshal(struct {
		PathPrefixes []string `json:"pathPrefixes"`
	}{PathPrefixes: []string{}})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("encode discovery job input: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.JobAccepted{}, fmt.Errorf("begin discovery job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)

	for {
		latest, err := queries.LockLatestDiscoveryJob(ctx, generated.LockLatestDiscoveryJobParams{TenantID: input.TenantID, DedupeKey: dedupeKey})
		generation := int64(1)
		if err == nil {
			if latest.Status == "pending" || latest.Status == "running" {
				return service.JobAccepted{JobID: latest.ID, Deduplicated: true}, nil
			}
			generation = latest.ActiveGeneration + 1
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return service.JobAccepted{}, normalizeError(err)
		}

		jobID := uuid.NewV7()
		refType := input.RefType
		refName := input.RefName
		row, err := queries.CreateDiscoveryJob(ctx, generated.CreateDiscoveryJobParams{
			TenantID: input.TenantID, ID: jobID, RepositoryID: new(input.RepositoryID),
			RefType: new(refType), RefName: new(refName), JobInput: jobInput,
			DedupeKey: dedupeKey, ActiveGeneration: generation,
		})
		if err == nil {
			if store.riverClient != nil {
				inserted, err := store.riverClient.InsertTx(ctx, tx, task.DiscoverArgs{
					TenantID: input.TenantID, JobID: row.ID, RepositoryID: input.RepositoryID,
					RefType: refType, RefName: refName,
				}, &river.InsertOpts{MaxAttempts: int(row.MaxAttempts)})
				if err != nil {
					return service.JobAccepted{}, fmt.Errorf("insert River discovery job: %w", err)
				}
				if changed, err := queries.AttachRiverJobID(ctx, generated.AttachRiverJobIDParams{
					TenantID: input.TenantID, ID: row.ID, RiverJobID: new(inserted.Job.ID), UpdatedAt: timestamp(time.Now().UTC()),
				}); err != nil {
					return service.JobAccepted{}, normalizeError(err)
				} else if changed != 1 {
					return service.JobAccepted{}, fmt.Errorf("attach River job %d to domain job %s: %w", inserted.Job.ID, row.ID, service.ErrPrecondition)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				return service.JobAccepted{}, normalizeError(err)
			}
			return service.JobAccepted{JobID: row.ID, Deduplicated: false}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return service.JobAccepted{}, normalizeError(err)
		}
		// Another transaction won the dedupe key; retry against the new visible row.
	}
}

// UpsertDiscoveryCandidate writes or refreshes one candidate root directory.
func (store *DiscoveryStore) UpsertDiscoveryCandidate(ctx context.Context, input service.NewDiscoveryCandidate) (service.DiscoveryCandidateRecord, error) {
	detected, err := json.Marshal(input.Detected)
	if err != nil {
		return service.DiscoveryCandidateRecord{}, fmt.Errorf("encode detected candidate metadata: %w", err)
	}
	row, err := store.queries.UpsertDiscoveryCandidate(ctx, generated.UpsertDiscoveryCandidateParams{
		TenantID: input.TenantID, ID: input.ID, RepositoryID: input.RepositoryID, CommitSha: input.CommitSHA, RootDir: input.RootDir, Detected: detected,
	})
	if err != nil {
		return service.DiscoveryCandidateRecord{}, normalizeError(err)
	}
	return discoveryCandidateFromRow(row), nil
}

// ListDiscoveryCandidates returns one deterministic candidate page ordered by root_dir.
func (store *DiscoveryStore) ListDiscoveryCandidates(ctx context.Context, tenantID, repositoryID uuid.UUID, limit, offset int32) ([]service.DiscoveryCandidateRecord, int64, error) {
	rows, err := store.queries.ListDiscoveryCandidates(ctx, generated.ListDiscoveryCandidatesParams{
		TenantID: tenantID, RepositoryID: repositoryID, PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.DiscoveryCandidateRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, discoveryCandidateFromRow(row))
	}
	total, err := store.queries.CountDiscoveryCandidates(ctx, generated.CountDiscoveryCandidatesParams{TenantID: tenantID, RepositoryID: repositoryID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	return items, total, nil
}

// GetDiscoveryCandidate returns one candidate within the tenant boundary.
func (store *DiscoveryStore) GetDiscoveryCandidate(ctx context.Context, tenantID, id uuid.UUID) (service.DiscoveryCandidateRecord, error) {
	row, err := store.queries.GetDiscoveryCandidate(ctx, generated.GetDiscoveryCandidateParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.DiscoveryCandidateRecord{}, normalizeError(err)
	}
	return discoveryCandidateFromRow(row), nil
}

// AcceptDiscoveryCandidate marks one pending candidate as accepted.
func (store *DiscoveryStore) AcceptDiscoveryCandidate(ctx context.Context, tenantID, id uuid.UUID) error {
	changed, err := store.queries.AcceptDiscoveryCandidate(ctx, generated.AcceptDiscoveryCandidateParams{TenantID: tenantID, ID: id})
	if err != nil {
		return normalizeError(err)
	}
	if changed != 1 {
		return service.ErrPrecondition
	}
	return nil
}

// CreateService inserts one accepted candidate as a service.
func (store *DiscoveryStore) CreateService(ctx context.Context, input service.NewService) (service.ServiceRecord, error) {
	row, err := store.queries.CreateService(ctx, generated.CreateServiceParams{
		TenantID: input.TenantID, ID: input.ID, RepositoryID: input.RepositoryID, Slug: input.Slug, DisplayName: input.DisplayName,
		Description: input.Description, RootDir: input.RootDir, Language: nil, Framework: nil,
		Owners: []string{}, Maintainers: []string{}, Lifecycle: "draft", Visibility: input.Visibility,
	})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// GetServiceBySlug returns one active service within the tenant boundary.
func (store *DiscoveryStore) GetServiceBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceBySlug(ctx, generated.GetServiceBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// CountServices returns the active service count and frozen service quota.
func (store *DiscoveryStore) CountServices(ctx context.Context, tenantID uuid.UUID) (int64, int64, error) {
	row, err := store.queries.CountServices(ctx, tenantID)
	if err != nil {
		return 0, 0, normalizeError(err)
	}
	return row.CurrentCount, row.LimitCount, nil
}

// CreateProducerProfile inserts one platform producer configuration.
func (store *DiscoveryStore) CreateProducerProfile(ctx context.Context, input service.NewProducerProfile) (service.ProducerProfile, error) {
	args := input.Args
	if args == nil {
		args = []string{}
	}
	envAllowlist := input.EnvAllowlist
	if envAllowlist == nil {
		envAllowlist = []string{}
	}
	supportedKinds := input.SupportedKinds
	if supportedKinds == nil {
		supportedKinds = []string{}
	}
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		return service.ProducerProfile{}, fmt.Errorf("encode producer args: %w", err)
	}
	row, err := store.queries.CreateProducerProfile(ctx, generated.CreateProducerProfileParams{
		ID: input.ID, Name: input.Name, Kind: input.Kind, Executable: input.Executable,
		Args: encodedArgs, EnvAllowlist: envAllowlist, SupportedKinds: supportedKinds,
		ReplaySafe: input.ReplaySafe, Network: input.Network, TimeoutSec: int32(input.TimeoutSec),
		MemoryMib: int32(input.MemoryMiB), CpuSeconds: int32(input.CPUSeconds), Pids: int32(input.Pids),
		Enabled: input.Enabled, DependencyStatus: input.DependencyStatus, UnavailableReason: input.UnavailableReason,
	})
	if err != nil {
		return service.ProducerProfile{}, normalizeError(err)
	}
	return producerProfileFromRow(row), nil
}

// ListAvailableProducerProfiles returns enabled, dependency-available profiles,
// optionally filtered to a supported kind.
func (store *DiscoveryStore) ListAvailableProducerProfiles(ctx context.Context, kind string) ([]service.ProducerProfile, error) {
	rows, err := store.queries.ListAvailableProducerProfiles(ctx, kind)
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.ProducerProfile, 0, len(rows))
	for _, row := range rows {
		items = append(items, producerProfileFromRow(row))
	}
	return items, nil
}

// GetProducerProfile returns one non-deleted platform profile.
func (store *DiscoveryStore) GetProducerProfile(ctx context.Context, id uuid.UUID) (service.ProducerProfile, error) {
	row, err := store.queries.GetProducerProfile(ctx, id)
	if err != nil {
		return service.ProducerProfile{}, normalizeError(err)
	}
	return producerProfileFromRow(row), nil
}

// CreateSourceSpec inserts one validated source configuration. Manual mode
// additionally creates its one global source binding and layer in the same
// transaction and returns the layer id as the source's initial layer.
func (store *DiscoveryStore) CreateSourceSpec(ctx context.Context, input service.NewSourceSpec) (service.SourceSpecRecord, error) {
	if input.Mode != "manual" || input.TargetAssetID == nil {
		row, err := store.queries.CreateSourceSpec(ctx, generated.CreateSourceSpecParams{
			TenantID: input.TenantID, ID: input.ID, ServiceID: input.ServiceID, Kind: input.Kind, AssetNameTemplate: input.AssetNameTemplate,
			Role: input.Role, Origin: input.Origin, Mode: input.Mode, Path: input.Path,
			ProducerProfileID: input.ProducerProfileID, Ord: int32(input.Ord), TimeoutSec: int32(input.TimeoutSec),
			BranchPatterns: input.BranchPatterns, Enabled: input.Enabled, ConfigOrigin: input.ConfigOrigin,
		})
		if err != nil {
			return service.SourceSpecRecord{}, normalizeError(err)
		}
		return sourceSpecFromRow(row, 0), nil
	}

	// Manual mode: create the source spec, its one global binding, and its one
	// layer atomically, and return the layer id.
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return service.SourceSpecRecord{}, fmt.Errorf("begin manual source spec transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)

	spec, err := queries.CreateSourceSpec(ctx, generated.CreateSourceSpecParams{
		TenantID: input.TenantID, ID: input.ID, ServiceID: input.ServiceID, Kind: input.Kind, AssetNameTemplate: input.AssetNameTemplate,
		Role: input.Role, Origin: input.Origin, Mode: input.Mode, Path: input.Path,
		ProducerProfileID: input.ProducerProfileID, Ord: int32(input.Ord), TimeoutSec: int32(input.TimeoutSec),
		BranchPatterns: input.BranchPatterns, Enabled: input.Enabled, ConfigOrigin: input.ConfigOrigin,
	})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	layerID := uuid.NewV7()
	if _, err := queries.GetAssetForSourceSpec(ctx, generated.GetAssetForSourceSpecParams{
		TenantID: input.TenantID, AssetID: *input.TargetAssetID, ServiceID: input.ServiceID, Kind: input.Kind,
	}); err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	if _, err := queries.CreateLayer(ctx, generated.CreateLayerParams{
		TenantID: input.TenantID, ID: layerID, AssetID: *input.TargetAssetID, SourceSpecID: new(input.ID),
		Role: input.Role, Origin: input.Origin, Ord: int32(input.Ord), Dialect: nil, Enabled: input.Enabled,
		BranchPatterns: input.BranchPatterns, DisplayName: input.AssetNameTemplate,
	}); err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	if _, err := queries.UpsertSourceBinding(ctx, generated.UpsertSourceBindingParams{
		TenantID: input.TenantID, ID: uuid.NewV7(), SourceSpecID: input.ID,
		ScopeType: "global", ScopeKey: "*", ExpansionKey: input.AssetNameTemplate,
		ResolvedPath: nil, SourceSystem: nil, AssetID: *input.TargetAssetID, LayerID: layerID, State: "active", LastSeenCommit: nil,
	}); err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	record := sourceSpecFromRow(spec, 1)
	record.InitialLayerID = new(layerID)
	return record, nil
}

// GetSourceSpec returns one active source configuration.
func (store *DiscoveryStore) GetSourceSpec(ctx context.Context, tenantID, id uuid.UUID) (service.SourceSpecRecord, error) {
	row, err := store.queries.GetSourceSpec(ctx, generated.GetSourceSpecParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	count, err := store.queries.CountSourceBindings(ctx, generated.CountSourceBindingsParams{TenantID: tenantID, SourceSpecID: id})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	return sourceSpecFromRow(row, int(count)), nil
}

// ListSourceBindings returns the current bindings for one source spec.
func (store *DiscoveryStore) ListSourceBindings(ctx context.Context, tenantID, sourceSpecID uuid.UUID) ([]service.SourceBindingRecord, error) {
	rows, err := store.queries.ListSourceBindings(ctx, generated.ListSourceBindingsParams{TenantID: tenantID, SourceSpecID: sourceSpecID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.SourceBindingRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.SourceBindingRecord{
			ID: row.ID, SourceSpecID: row.SourceSpecID, ScopeType: row.ScopeType, ScopeKey: row.ScopeKey,
			ExpansionKey: row.ExpansionKey, ResolvedPath: row.ResolvedPath, SourceSystem: row.SourceSystem,
			AssetID: row.AssetID, LayerID: row.LayerID, State: row.State, LastSeenCommit: row.LastSeenCommit,
		})
	}
	return items, nil
}

// CountSourceBindings returns the binding count for one source spec.
func (store *DiscoveryStore) CountSourceBindings(ctx context.Context, tenantID, sourceSpecID uuid.UUID) (int64, error) {
	return store.queries.CountSourceBindings(ctx, generated.CountSourceBindingsParams{TenantID: tenantID, SourceSpecID: sourceSpecID})
}

// ListSourceSpecsForService returns active source specs for one service.
func (store *DiscoveryStore) ListSourceSpecsForService(ctx context.Context, tenantID, serviceID uuid.UUID) ([]service.SourceSpecRecord, error) {
	rows, err := store.queries.ListSourceSpecsForService(ctx, generated.ListSourceSpecsForServiceParams{TenantID: tenantID, ServiceID: serviceID})
	if err != nil {
		return nil, normalizeError(err)
	}
	items := make([]service.SourceSpecRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.SourceSpecRecord{
			ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, AssetNameTemplate: row.AssetNameTemplate,
			Role: row.Role, Origin: row.Origin, Mode: row.Mode, Path: row.Path, ProducerProfileID: row.ProducerProfileID,
			Ord: int(row.Ord), TimeoutSec: int(row.TimeoutSec), BranchPatterns: append([]string(nil), row.BranchPatterns...),
			Enabled: row.Enabled, ConfigOrigin: row.ConfigOrigin, Revision: row.Revision,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return items, nil
}

// CountActiveBindings returns the active binding count for one source spec.
func (store *DiscoveryStore) CountActiveBindings(ctx context.Context, tenantID, sourceSpecID uuid.UUID) (int64, error) {
	return store.queries.CountActiveBindings(ctx, generated.CountActiveBindingsParams{TenantID: tenantID, SourceSpecID: sourceSpecID})
}

func discoveryDedupeKey(repositoryID uuid.UUID, refType, refName string) string {
	return "discover:" + repositoryID.String() + ":" + refType + ":" + refName
}

func discoveryCandidateFromRow(row generated.DiscoveryCandidate) service.DiscoveryCandidateRecord {
	var detected map[string]any
	if len(row.Detected) > 0 {
		_ = json.Unmarshal(row.Detected, &detected)
	}
	if detected == nil {
		detected = map[string]any{}
	}
	return service.DiscoveryCandidateRecord{
		ID: row.ID, RepositoryID: row.RepositoryID, CommitSHA: row.CommitSha, RootDir: row.RootDir,
		Detected: detected, Status: row.Status, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func serviceRecordFromRow(row generated.Service) service.ServiceRecord {
	return service.ServiceRecord{
		TenantID: row.TenantID, ID: row.ID, RepositoryID: row.RepositoryID, Slug: row.Slug, DisplayName: row.DisplayName,
		Description: row.Description, RootDir: row.RootDir, Language: row.Language, Framework: row.Framework,
		Visibility: row.Visibility, Lifecycle: row.Lifecycle, Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func producerProfileFromRow(row generated.ProducerProfile) service.ProducerProfile {
	var args []string
	if len(row.Args) > 0 {
		_ = json.Unmarshal(row.Args, &args)
	}
	return service.ProducerProfile{
		ID: row.ID, Name: row.Name, Kind: row.Kind, Executable: row.Executable, Args: args,
		EnvAllowlist: append([]string(nil), row.EnvAllowlist...), SupportedKinds: append([]string(nil), row.SupportedKinds...),
		ReplaySafe: row.ReplaySafe, Network: row.Network, TimeoutSec: int(row.TimeoutSec),
		MemoryMiB: int(row.MemoryMib), CPUSeconds: int(row.CpuSeconds), Pids: int(row.Pids),
		Enabled: row.Enabled, DependencyStatus: row.DependencyStatus, UnavailableReason: row.UnavailableReason,
		Revision: row.Revision, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func sourceSpecFromRow(row generated.SourceSpec, bindingsCount int) service.SourceSpecRecord {
	return service.SourceSpecRecord{
		ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, AssetNameTemplate: row.AssetNameTemplate,
		Role: row.Role, Origin: row.Origin, Mode: row.Mode, Path: row.Path, ProducerProfileID: row.ProducerProfileID,
		Ord: int(row.Ord), TimeoutSec: int(row.TimeoutSec), BranchPatterns: append([]string(nil), row.BranchPatterns...),
		Enabled: row.Enabled, ConfigOrigin: row.ConfigOrigin, BindingsCount: bindingsCount, Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

// ListRecentServices returns one page of recently viewed services.
func (store *DiscoveryStore) ListRecentServices(ctx context.Context, tenantID, userID uuid.UUID, limit, offset int32) ([]service.ServiceRecord, int64, error) {
	total, err := store.queries.CountRecentServices(ctx, generated.CountRecentServicesParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	rows, err := store.queries.ListRecentServices(ctx, generated.ListRecentServicesParams{TenantID: tenantID, UserID: userID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	items := make([]service.ServiceRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.ServiceRecord{
			TenantID: row.TenantID, ID: row.ID, RepositoryID: row.RepositoryID, Slug: row.Slug, DisplayName: row.DisplayName,
			Description: row.Description, RootDir: row.RootDir, Language: row.Language, Framework: row.Framework,
			Visibility: row.Visibility, Lifecycle: row.Lifecycle, Revision: row.Revision,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		})
	}
	return items, total, nil
}

// UpsertRecentService records one successful service detail read.
func (store *DiscoveryStore) UpsertRecentService(ctx context.Context, tenantID, userID, serviceID uuid.UUID, viewedAt time.Time) error {
	if _, err := store.queries.UpsertRecentService(ctx, generated.UpsertRecentServiceParams{TenantID: tenantID, UserID: userID, ServiceID: serviceID, ViewedAt: timestamp(viewedAt)}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// UpdateSourceSpec applies a validated source spec patch under its revision.
func (store *DiscoveryStore) UpdateSourceSpec(ctx context.Context, input service.SourceSpecPatch) (service.SourceSpecRecord, error) {
	row, err := store.queries.UpdateSourceSpec(ctx, generated.UpdateSourceSpecParams{
		TenantID: input.TenantID, ID: input.ID, ExpectedRevision: input.ExpectedRevision,
		AssetNameTemplate: input.AssetNameTemplate, Role: input.Role, Origin: input.Origin, Mode: input.Mode,
		Path: input.Path, ProducerProfileID: input.ProducerProfileID,
		Ord: int32Pointer(input.Ord), TimeoutSec: int32Pointer(input.TimeoutSec),
		BranchPatterns: input.BranchPatterns, Enabled: input.Enabled,
	})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	return service.SourceSpecRecord{
		ID: row.ID, ServiceID: row.ServiceID, Kind: row.Kind, AssetNameTemplate: row.AssetNameTemplate,
		Role: row.Role, Origin: row.Origin, Mode: row.Mode, Path: row.Path, ProducerProfileID: row.ProducerProfileID,
		Ord: int(row.Ord), TimeoutSec: int(row.TimeoutSec), BranchPatterns: append([]string(nil), row.BranchPatterns...),
		Enabled: row.Enabled, ConfigOrigin: row.ConfigOrigin, Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

var _ service.DiscoveryStore = (*DiscoveryStore)(nil)
