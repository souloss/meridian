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

// DiscoveryStore 实现仓库发现、候选接受、服务创建、生产者配置与源配置持久化。
// 它内嵌 RepositoryStore 复用仓库读取路径，并附带 River 客户端用于事务内入队发现任务。
type DiscoveryStore struct {
	*RepositoryStore
	riverClient *river.Client[pgx.Tx]
}

// NewDiscoveryStore 将发现持久化绑定到原生 pgx 连接池。
func NewDiscoveryStore(pool *pgxpool.Pool) *DiscoveryStore {
	return NewDiscoveryStoreWithRiver(pool, nil)
}

// NewDiscoveryStoreWithRiver 将发现持久化绑定到 River 客户端，
// 使新入队的发现任务与其领域行加入同一事务。
func NewDiscoveryStoreWithRiver(pool *pgxpool.Pool, riverClient *river.Client[pgx.Tx]) *DiscoveryStore {
	return &DiscoveryStore{RepositoryStore: NewRepositoryStore(pool), riverClient: riverClient}
}

// BindRiver 在运行时构造完成后挂接进程 River 客户端，使发现任务可事务内入队。
func (store *DiscoveryStore) BindRiver(riverClient *river.Client[pgx.Tx]) {
	store.riverClient = riverClient
}

// EnqueueDiscoveryJob 原子地记录一条 repo.discover 任务及其 River 工作。
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
		generation := jobGenerationInitial
		if err == nil {
			if latest.Status == service.JobStatusPending || latest.Status == service.JobStatusRunning {
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
				} else if changed != rowsAffectedOne {
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
		// 另一事务赢得了去重键；针对新的可见行重试。
	}
}

// UpsertDiscoveryCandidate 写入或刷新一条候选根目录。
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

// ListDiscoveryCandidates 返回按 root_dir 排序的一页确定性候选。
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

// GetDiscoveryCandidate 返回租户边界内的一条候选。
func (store *DiscoveryStore) GetDiscoveryCandidate(ctx context.Context, tenantID, id uuid.UUID) (service.DiscoveryCandidateRecord, error) {
	row, err := store.queries.GetDiscoveryCandidate(ctx, generated.GetDiscoveryCandidateParams{TenantID: tenantID, ID: id})
	if err != nil {
		return service.DiscoveryCandidateRecord{}, normalizeError(err)
	}
	return discoveryCandidateFromRow(row), nil
}

// AcceptDiscoveryCandidate 将一条待处理候选标记为已接受。
func (store *DiscoveryStore) AcceptDiscoveryCandidate(ctx context.Context, tenantID, id uuid.UUID) error {
	changed, err := store.queries.AcceptDiscoveryCandidate(ctx, generated.AcceptDiscoveryCandidateParams{TenantID: tenantID, ID: id})
	if err != nil {
		return normalizeError(err)
	}
	if changed != rowsAffectedOne {
		return service.ErrPrecondition
	}
	return nil
}

// CreateService 将一条已接受候选插入为服务。
func (store *DiscoveryStore) CreateService(ctx context.Context, input service.NewService) (service.ServiceRecord, error) {
	row, err := store.queries.CreateService(ctx, generated.CreateServiceParams{
		TenantID: input.TenantID, ID: input.ID, RepositoryID: input.RepositoryID, Slug: input.Slug, DisplayName: input.DisplayName,
		Description: input.Description, RootDir: input.RootDir, Language: nil, Framework: nil,
		Owners: []string{}, Maintainers: []string{}, Lifecycle: serviceLifecycleDraft, Visibility: input.Visibility,
	})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// GetServiceBySlug 返回租户边界内的一条活跃服务。
func (store *DiscoveryStore) GetServiceBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (service.ServiceRecord, error) {
	row, err := store.queries.GetServiceBySlug(ctx, generated.GetServiceBySlugParams{TenantID: tenantID, Slug: slug})
	if err != nil {
		return service.ServiceRecord{}, normalizeError(err)
	}
	return serviceRecordFromRow(row), nil
}

// CountServices 返回活跃服务数与冻结的服务配额。
func (store *DiscoveryStore) CountServices(ctx context.Context, tenantID uuid.UUID) (int64, int64, error) {
	row, err := store.queries.CountServices(ctx, tenantID)
	if err != nil {
		return 0, 0, normalizeError(err)
	}
	return row.CurrentCount, row.LimitCount, nil
}

// CreateProducerProfile 插入一条平台生产者配置。
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

// ListAvailableProducerProfiles 返回已启用、依赖可用的配置，可按支持的 kind 过滤。
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

// GetProducerProfile 返回一条未删除的平台配置。
func (store *DiscoveryStore) GetProducerProfile(ctx context.Context, id uuid.UUID) (service.ProducerProfile, error) {
	row, err := store.queries.GetProducerProfile(ctx, id)
	if err != nil {
		return service.ProducerProfile{}, normalizeError(err)
	}
	return producerProfileFromRow(row), nil
}

// CreateSourceSpec 插入一条经过校验的源配置。手动模式还会在同一事务内
// 额外创建其唯一全局源绑定与层，并将层 id 作为该源的初始层返回。
func (store *DiscoveryStore) CreateSourceSpec(ctx context.Context, input service.NewSourceSpec) (service.SourceSpecRecord, error) {
	if input.Mode != sourceModeManual || input.TargetAssetID == nil {
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

	// 手动模式：原子创建源配置、其一个全局绑定与一个层，并返回层 ID。
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
		ScopeType: sourceScopeTypeGlobal, ScopeKey: sourceScopeKeyGlobal, ExpansionKey: input.AssetNameTemplate,
		ResolvedPath: nil, SourceSystem: nil, AssetID: *input.TargetAssetID, LayerID: layerID, State: sourceBindingStateActive, LastSeenCommit: nil,
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

// GetAiBaseForService 返回某服务/kind 的 AI 生成 base 源配置。
func (store *DiscoveryStore) GetAiBaseForService(ctx context.Context, tenantID, serviceID uuid.UUID, kind string) (service.SourceSpecRecord, error) {
	row, err := store.queries.GetAiBaseForService(ctx, generated.GetAiBaseForServiceParams{TenantID: tenantID, ServiceID: serviceID, Kind: kind})
	if err != nil {
		return service.SourceSpecRecord{}, normalizeError(err)
	}
	return sourceSpecFromRow(row, 0), nil
}

// ReplaceAiBaseForService 归档 AI base 源与层，然后创建绑定到同一资产的替代仓库
// base 源与层，并保留历史（SMK-033：无双 base，track 代次递增）。
func (store *DiscoveryStore) ReplaceAiBaseForService(ctx context.Context, tenantID, serviceID uuid.UUID, kind string) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin AI base replacement transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)

	// 归档前定位 AI base 层以复用其资产。
	aiLayers, err := queries.ListAiBaseLayersForService(ctx, generated.ListAiBaseLayersForServiceParams{TenantID: tenantID, ServiceID: serviceID, Kind: kind})
	if err != nil {
		return normalizeError(err)
	}
	if len(aiLayers) == 0 {
		return service.ErrNotFound
	}
	assetID := aiLayers[0].AssetID
	archiveCandidates := make([]uuid.UUID, 0, len(aiLayers))
	for _, layer := range aiLayers {
		archiveCandidates = append(archiveCandidates, layer.ID)
	}

	if _, err := queries.ArchiveAiBaseLayersForService(ctx, generated.ArchiveAiBaseLayersForServiceParams{TenantID: tenantID, ServiceID: serviceID, Kind: kind}); err != nil {
		return normalizeError(err)
	}
	if _, err := queries.ArchiveAiBaseForService(ctx, generated.ArchiveAiBaseForServiceParams{TenantID: tenantID, ServiceID: serviceID, Kind: kind}); err != nil {
		return normalizeError(err)
	}

	// 在同一资产上创建仓库 base 层，避免双重 base。
	repoSourceID := uuid.NewV7()
	repoLayerID := uuid.NewV7()
	if _, err := queries.CreateSourceSpec(ctx, generated.CreateSourceSpecParams{
		TenantID: tenantID, ID: repoSourceID, ServiceID: serviceID, Kind: kind,
		AssetNameTemplate: repoBaseSourceAssetNameTemplate, Role: layerRoleBase, Origin: sourceOriginRepo, Mode: sourceModeBuiltin,
		Path: new(repoBaseSourcePath), ProducerProfileID: nil, Ord: layerOrdBase, TimeoutSec: repoBaseSourceTimeoutSec,
		BranchPatterns: []string{branchGlobAll}, Enabled: true, ConfigOrigin: sourceConfigOriginAPI,
	}); err != nil {
		return normalizeError(err)
	}
	if _, err := queries.CreateLayer(ctx, generated.CreateLayerParams{
		TenantID: tenantID, ID: repoLayerID, AssetID: assetID, SourceSpecID: new(repoSourceID),
		Role: layerRoleBase, Origin: sourceOriginRepo, Ord: layerOrdBase, Dialect: nil, Enabled: true,
		BranchPatterns: []string{branchGlobAll}, DisplayName: "",
	}); err != nil {
		return normalizeError(err)
	}
	_ = archiveCandidates
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	return nil
}

// GetSourceSpec 返回一条活跃源配置。
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

// ListSourceBindings 返回某一源配置当前的绑定。
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

// CountSourceBindings 返回某一源配置的绑定数。
func (store *DiscoveryStore) CountSourceBindings(ctx context.Context, tenantID, sourceSpecID uuid.UUID) (int64, error) {
	return store.queries.CountSourceBindings(ctx, generated.CountSourceBindingsParams{TenantID: tenantID, SourceSpecID: sourceSpecID})
}

// ListSourceSpecsForService 返回某一服务的活跃源配置。
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

// CountActiveBindings 返回某一源配置的活跃绑定数。
func (store *DiscoveryStore) CountActiveBindings(ctx context.Context, tenantID, sourceSpecID uuid.UUID) (int64, error) {
	return store.queries.CountActiveBindings(ctx, generated.CountActiveBindingsParams{TenantID: tenantID, SourceSpecID: sourceSpecID})
}

func discoveryDedupeKey(repositoryID uuid.UUID, refType, refName string) string {
	return dedupeKeyPrefixDiscover + repositoryID.String() + ":" + refType + ":" + refName
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

// ListRecentServices 返回一页最近查看过的服务。
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

// UpsertRecentService 记录一次成功的服务详情读取。
func (store *DiscoveryStore) UpsertRecentService(ctx context.Context, tenantID, userID, serviceID uuid.UUID, viewedAt time.Time) error {
	if _, err := store.queries.UpsertRecentService(ctx, generated.UpsertRecentServiceParams{TenantID: tenantID, UserID: userID, ServiceID: serviceID, ViewedAt: timestamp(viewedAt)}); err != nil {
		return normalizeError(err)
	}
	return nil
}

// UpdateSourceSpec 在其修订号下应用一次经过校验的源配置补丁。
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
