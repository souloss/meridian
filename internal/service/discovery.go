package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

const (
	defaultSourceAssetNameTemplate = "{file_stem}"
	defaultSourceTimeoutBuiltin    = 120
	defaultSourceTimeoutCommand    = 120
	defaultSourceTimeoutPush       = 120
	defaultSourceTimeoutManual     = 120
	defaultSourceTimeoutAI         = 600
	maxServiceDisplayNameRunes     = 128
	maxServiceDescriptionRunes     = 2000
	maxRootDirRunes                = 512
	maxSourcePathRunes             = 512
)

// Discovery 协调仓库发现任务入队、候选列表、候选验收与源配置用例。
type Discovery struct {
	store      DiscoveryStore
	identities IdentityStore
	assets     *Assets
	now        func() time.Time
}

// NewDiscovery 构造由调用方持有持久化的发现用例。
func NewDiscovery(store DiscoveryStore, identities IdentityStore) *Discovery {
	return &Discovery{store: store, identities: identities, now: time.Now}
}

// WithAssets 绑定用于丰富服务响应的资产读取用例。
func (discovery *Discovery) WithAssets(assets *Assets) *Discovery {
	discovery.assets = assets
	return discovery
}

// Discover 入队一个幂等仓库发现任务并返回其标识。
func (discovery *Discovery) Discover(ctx context.Context, actor Principal, tenantSlug string, repositoryID, idempotencyKey uuid.UUID, refType, refName string) (JobAccepted, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeRepositorySync)
	if err != nil {
		return JobAccepted{}, err
	}
	if idempotencyKey == uuid.Nil() {
		return JobAccepted{}, ErrValidation
	}
	if refType != refTypeBranch && refType != refTypeTag {
		return JobAccepted{}, ErrValidation
	}
	if refName == "" || !validGitRefName(refName) {
		return JobAccepted{}, ErrValidation
	}
	if _, err := discovery.store.GetRepository(ctx, membership.TenantID, repositoryID); err != nil {
		return JobAccepted{}, err
	}
	return discovery.store.EnqueueDiscoveryJob(ctx, DiscoverJobInput{
		TenantID: membership.TenantID, RepositoryID: repositoryID, RefType: refType, RefName: refName, IdempotencyKey: idempotencyKey,
	})
}

// ListCandidates 返回仓库的一页确定性发现候选。
func (discovery *Discovery) ListCandidates(ctx context.Context, actor Principal, tenantSlug string, repositoryID uuid.UUID, page, pageSize int) ([]DiscoveryCandidateRecord, int64, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeRepositoryRead)
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return discovery.store.ListDiscoveryCandidates(ctx, membership.TenantID, repositoryID, int32(pageSize), int32((page-1)*pageSize))
}

// AcceptCandidates 将待处理候选转为服务并返回已验收服务列表。候选覆盖调整 slug、展示名与
// 可见性；默认可见性按冻结契约为 private。
func (discovery *Discovery) AcceptCandidates(ctx context.Context, actor Principal, tenantSlug string, repositoryID, idempotencyKey uuid.UUID, candidateIDs []uuid.UUID, overrides []CandidateOverride) ([]ServiceRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeServiceCreate)
	if err != nil {
		return nil, err
	}
	if idempotencyKey == uuid.Nil() || len(candidateIDs) == 0 || len(candidateIDs) > maxCandidatesPerAccept {
		return nil, ErrValidation
	}
	if _, err := discovery.store.GetRepository(ctx, membership.TenantID, repositoryID); err != nil {
		return nil, err
	}
	overrideByID := make(map[uuid.UUID]CandidateOverride, len(overrides))
	for _, override := range overrides {
		if _, exists := overrideByID[override.CandidateID]; exists {
			return nil, ErrValidation
		}
		if override.DisplayName != nil && utf8.RuneCountInString(*override.DisplayName) > maxServiceDisplayNameRunes {
			return nil, ErrValidation
		}
		if override.Slug != nil && !slugPattern.MatchString(*override.Slug) {
			return nil, ErrValidation
		}
		if override.Visibility != nil && !validServiceVisibility(*override.Visibility) {
			return nil, ErrValidation
		}
		overrideByID[override.CandidateID] = override
	}

	accepted := make([]ServiceRecord, 0, len(candidateIDs))
	for _, candidateID := range candidateIDs {
		candidate, err := discovery.store.GetDiscoveryCandidate(ctx, membership.TenantID, candidateID)
		if err != nil {
			return nil, err
		}
		if candidate.RepositoryID != repositoryID {
			return nil, ErrNotFound
		}
		if candidate.Status != JobStatusPending {
			return nil, ErrValidation
		}
		override := overrideByID[candidateID]
		slug := serviceSlugFromCandidate(candidate, override)
		displayName := candidate.DisplayName()
		if override.DisplayName != nil {
			displayName = *override.DisplayName
		}
		visibility := serviceVisibilityPrivate
		if override.Visibility != nil {
			visibility = *override.Visibility
		}
		if _, err := discovery.store.GetServiceBySlug(ctx, membership.TenantID, slug); err == nil {
			return nil, ErrDuplicate
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		record, err := discovery.store.CreateService(ctx, NewService{
			TenantID: membership.TenantID, ID: uuid.NewV7(), RepositoryID: repositoryID, Slug: slug,
			DisplayName: displayName, RootDir: candidate.RootDir, Visibility: visibility,
		})
		if err != nil {
			return nil, err
		}
		if err := discovery.store.AcceptDiscoveryCandidate(ctx, membership.TenantID, candidateID); err != nil {
			return nil, err
		}
		accepted = append(accepted, record)
	}
	return accepted, nil
}

// CreateSourceSpec 校验模式字段兼容性并插入一个源配置。
func (discovery *Discovery) CreateSourceSpec(ctx context.Context, actor Principal, tenantSlug, serviceSlug string, input NewSourceSpec) (SourceSpecRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeLayerEdit)
	if err != nil {
		return SourceSpecRecord{}, err
	}
	validated, err := validateNewSourceSpec(input)
	if err != nil {
		return SourceSpecRecord{}, err
	}
	if validated.Mode == sourceModeCommand || validated.Mode == aiMode {
		if validated.ProducerProfileID == nil {
			return SourceSpecRecord{}, ErrValidation
		}
		profile, err := discovery.store.GetProducerProfile(ctx, *validated.ProducerProfileID)
		if err != nil {
			return SourceSpecRecord{}, err
		}
		if profile.DependencyStatus == dependencyStatusUnavailable || !profile.Enabled {
			return SourceSpecRecord{}, &ProducerUnavailableError{Kind: validated.Kind}
		}
		if !slices.Contains(profile.SupportedKinds, validated.Kind) {
			return SourceSpecRecord{}, ErrValidation
		}
	}
	service, err := discovery.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return SourceSpecRecord{}, err
	}
	if service.Lifecycle == serviceLifecycleRetired {
		return SourceSpecRecord{}, ErrInvalidState
	}
	if validated.Mode == sourceModeManual && input.TargetAssetID == nil {
		return SourceSpecRecord{}, ErrValidation
	}
	if validated.Role == layerRoleBase && validated.Origin == layerOriginRepo {
		if err := discovery.enforceRepoBaseReplacement(ctx, membership.TenantID, service.ID, input.Kind, input.ReplaceAiBase); err != nil {
			return SourceSpecRecord{}, err
		}
	}
	record, err := discovery.store.CreateSourceSpec(ctx, NewSourceSpec{
		TenantID: membership.TenantID, ID: uuid.NewV7(), ServiceID: service.ID, Kind: validated.Kind,
		AssetNameTemplate: validated.AssetNameTemplate, Role: validated.Role, Origin: validated.Origin, Mode: validated.Mode,
		Path: validated.Path, ProducerProfileID: validated.ProducerProfileID, Ord: validated.Ord, TimeoutSec: validated.TimeoutSec,
		BranchPatterns: validated.BranchPatterns, Enabled: validated.Enabled, ConfigOrigin: validated.ConfigOrigin,
		TargetAssetID: input.TargetAssetID,
	})
	if err != nil {
		return SourceSpecRecord{}, err
	}
	return record, nil
}

// enforceRepoBaseReplacement 拒绝会替换 AI 生成 base 的隐式仓库 base，并在请求 replaceAiBase
// 时归档 AI base。
func (discovery *Discovery) enforceRepoBaseReplacement(ctx context.Context, tenantID, serviceID uuid.UUID, kind string, replace bool) error {
	if !replace {
		if _, err := discovery.store.GetAiBaseForService(ctx, tenantID, serviceID, kind); err == nil {
			return ErrBaseLayerExists
		} else if !isNotFound(err) {
			return err
		}
		return nil
	}
	return discovery.store.ReplaceAiBaseForService(ctx, tenantID, serviceID, kind)
}

// ListSourceBindings 返回某源配置当前的物化绑定。
func (discovery *Discovery) ListSourceBindings(ctx context.Context, actor Principal, tenantSlug string, sourceID uuid.UUID) ([]SourceBindingRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return nil, err
	}
	if _, err := discovery.store.GetSourceSpec(ctx, membership.TenantID, sourceID); err != nil {
		return nil, err
	}
	return discovery.store.ListSourceBindings(ctx, membership.TenantID, sourceID)
}

// ListSourceSpecs 返回某服务的活跃源配置。
func (discovery *Discovery) ListSourceSpecs(ctx context.Context, actor Principal, tenantSlug, serviceSlug string) ([]SourceSpecRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return nil, err
	}
	service, err := discovery.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return nil, err
	}
	specs, err := discovery.store.ListSourceSpecsForService(ctx, membership.TenantID, service.ID)
	if err != nil {
		return nil, err
	}
	for index := range specs {
		count, err := discovery.store.CountActiveBindings(ctx, membership.TenantID, specs[index].ID)
		if err == nil {
			specs[index].BindingsCount = int(count)
		}
	}
	return specs, nil
}

// Sync 入队一个幂等仓库同步任务。
func (discovery *Discovery) Sync(ctx context.Context, actor Principal, tenantSlug string, repositoryID, idempotencyKey uuid.UUID, input RepositorySyncInput) (JobAccepted, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeRepositorySync)
	if err != nil {
		return JobAccepted{}, err
	}
	if idempotencyKey == uuid.Nil() {
		return JobAccepted{}, ErrValidation
	}
	refType := input.RefType
	if refType == "" {
		refType = refTypeBranch
	}
	if refType != refTypeBranch && refType != refTypeTag {
		return JobAccepted{}, ErrValidation
	}
	refName := input.RefName
	if refName == "" {
		record, err := discovery.store.GetRepository(ctx, membership.TenantID, repositoryID)
		if err != nil {
			return JobAccepted{}, err
		}
		refName = record.DefaultBranch
	}
	if !validGitRefName(refName) {
		return JobAccepted{}, ErrValidation
	}
	if _, err := discovery.store.GetRepository(ctx, membership.TenantID, repositoryID); err != nil {
		return JobAccepted{}, err
	}
	principalType, principalID := rotationPrincipal(actor)
	requestHash, err := RequestDigest("syncRepository", map[string]any{
		"tenantSlug": tenantSlug, "repositoryId": repositoryID.String(),
	}, map[string]any{}, map[string]any{"refType": refType, "ref": refName})
	if err != nil {
		return JobAccepted{}, ErrValidation
	}
	return discovery.store.EnqueueSyncJob(ctx, SyncJobInput{
		TenantID: membership.TenantID, RepositoryID: repositoryID, RefType: refType, RefName: refName,
		IdempotencyKey: idempotencyKey, Force: input.Force, PrincipalType: principalType, PrincipalID: principalID, RequestHash: requestHash,
	})
}

// UpdateSourceSpec 对某源配置应用校验后的补丁。
func (discovery *Discovery) UpdateSourceSpec(ctx context.Context, actor Principal, tenantSlug string, sourceID uuid.UUID, etag string, input SourceSpecPatchInput) (SourceSpecRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeLayerEdit)
	if err != nil {
		return SourceSpecRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "source-spec", sourceID)
	if err != nil {
		return SourceSpecRecord{}, ErrPrecondition
	}
	current, err := discovery.store.GetSourceSpec(ctx, membership.TenantID, sourceID)
	if err != nil {
		return SourceSpecRecord{}, err
	}
	merged := mergeSourceSpecPatch(current, input)
	if err := validateMergedSourceSpec(merged); err != nil {
		return SourceSpecRecord{}, err
	}
	patch := SourceSpecPatch{TenantID: membership.TenantID, ID: sourceID, ExpectedRevision: expectedRevision}
	if input.AssetNameTemplate != nil {
		patch.AssetNameTemplate = input.AssetNameTemplate
	}
	if input.Role != nil {
		patch.Role = input.Role
	}
	if input.Origin != nil {
		patch.Origin = input.Origin
	}
	if input.Mode != nil {
		patch.Mode = input.Mode
	}
	if input.Path != nil {
		patch.Path = input.Path
	}
	if input.ProducerProfileID != nil {
		patch.ProducerProfileID = input.ProducerProfileID
	}
	if input.Ord != nil {
		patch.Ord = input.Ord
	}
	if input.TimeoutSec != nil {
		patch.TimeoutSec = input.TimeoutSec
	}
	if len(input.BranchPatterns) > 0 {
		patch.BranchPatterns = input.BranchPatterns
	}
	if input.Enabled != nil {
		patch.Enabled = input.Enabled
	}
	return discovery.store.UpdateSourceSpec(ctx, patch)
}

// GetService 返回一个服务详情并记录成功读取。
func (discovery *Discovery) GetService(ctx context.Context, actor Principal, tenantSlug, serviceSlug string) (ServiceRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return ServiceRecord{}, err
	}
	record, err := discovery.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return ServiceRecord{}, err
	}
	if err := discovery.store.UpsertRecentService(ctx, membership.TenantID, membership.UserID, record.ID, discovery.now().UTC()); err != nil {
		return ServiceRecord{}, err
	}
	return record, nil
}

// ServiceDetail 携带一个服务及其资产摘要与缺失类别。
type ServiceDetail struct {
	Service        ServiceRecord
	AssetSummaries []AssetSummaryRecord
	MissingKinds   []MissingKindRecord
}

// GetServiceDetail 返回一个经资产摘要与缺失类别丰富的服务详情，并记录成功读取。
func (discovery *Discovery) GetServiceDetail(ctx context.Context, actor Principal, tenantSlug, serviceSlug string) (ServiceDetail, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return ServiceDetail{}, err
	}
	record, err := discovery.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return ServiceDetail{}, err
	}
	if err := discovery.store.UpsertRecentService(ctx, membership.TenantID, membership.UserID, record.ID, discovery.now().UTC()); err != nil {
		return ServiceDetail{}, err
	}
	detail := ServiceDetail{Service: record, AssetSummaries: []AssetSummaryRecord{}, MissingKinds: []MissingKindRecord{}}
	if discovery.assets == nil {
		return detail, nil
	}
	summaries, missing, err := discovery.assets.AssetSummaries(ctx, membership.TenantID, record.ID)
	if err != nil {
		return ServiceDetail{}, err
	}
	detail.AssetSummaries = summaries
	detail.MissingKinds = missing
	return detail, nil
}

// ListRecentServices 返回调用方最近浏览的服务。
func (discovery *Discovery) ListRecentServices(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]ServiceRecord, int64, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return discovery.store.ListRecentServices(ctx, membership.TenantID, membership.UserID, int32(pageSize), int32((page-1)*pageSize))
}

func mergeSourceSpecPatch(current SourceSpecRecord, input SourceSpecPatchInput) SourceSpecRecord {
	if input.AssetNameTemplate != nil {
		current.AssetNameTemplate = *input.AssetNameTemplate
	}
	if input.Role != nil {
		current.Role = *input.Role
	}
	if input.Origin != nil {
		current.Origin = *input.Origin
	}
	if input.Mode != nil {
		current.Mode = *input.Mode
	}
	if input.Path != nil {
		current.Path = input.Path
	}
	if input.ProducerProfileID != nil {
		current.ProducerProfileID = input.ProducerProfileID
	}
	if input.Ord != nil {
		current.Ord = *input.Ord
	}
	if input.TimeoutSec != nil {
		current.TimeoutSec = *input.TimeoutSec
	}
	if len(input.BranchPatterns) > 0 {
		current.BranchPatterns = input.BranchPatterns
	}
	if input.Enabled != nil {
		current.Enabled = *input.Enabled
	}
	return current
}

func validateMergedSourceSpec(spec SourceSpecRecord) error {
	probe := NewSourceSpec{
		Kind: spec.Kind, AssetNameTemplate: spec.AssetNameTemplate, Role: spec.Role, Origin: spec.Origin, Mode: spec.Mode,
		Path: spec.Path, ProducerProfileID: spec.ProducerProfileID, Ord: spec.Ord, TimeoutSec: spec.TimeoutSec,
		BranchPatterns: spec.BranchPatterns, Enabled: spec.Enabled,
	}
	if _, err := validateNewSourceSpec(probe); err != nil {
		return err
	}
	return nil
}

// ProducerUnavailableError 报告所选生产者配置不可用。
// 对外映射：ErrorCodeProducerUnavailable（HTTP 422）。
type ProducerUnavailableError struct {
	Kind string
}

func (err *ProducerUnavailableError) Error() string {
	return "selected producer profile is unavailable for kind " + err.Kind
}

// CandidateOverride 在验收期间调整一个候选。
type CandidateOverride struct {
	CandidateID uuid.UUID
	Slug        *string
	DisplayName *string
	Visibility  *string
}

func (candidate DiscoveryCandidateRecord) DisplayName() string {
	displayName := strings.TrimSuffix(candidate.RootDir, "/")
	if displayName == "" {
		return "service"
	}
	if index := strings.LastIndexByte(displayName, '/'); index >= 0 {
		displayName = displayName[index+1:]
	}
	return displayName
}

func serviceSlugFromCandidate(candidate DiscoveryCandidateRecord, override CandidateOverride) string {
	if override.Slug != nil {
		return *override.Slug
	}
	return slugify(candidate.DisplayName())
}

func slugify(displayName string) string {
	var builder strings.Builder
	lastDash := false
	for _, character := range strings.ToLower(displayName) {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			builder.WriteRune(character)
			lastDash = false
		case character == '-':
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		default:
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.TrimSuffix(builder.String(), "-")
	if result == "" {
		return "service"
	}
	if len(result) > 63 {
		result = strings.TrimSuffix(result[:63], "-")
	}
	return result
}

func validateNewSourceSpec(input NewSourceSpec) (NewSourceSpec, error) {
	if !validKindID(input.Kind) {
		return NewSourceSpec{}, ErrValidation
	}
	if input.Role != layerRoleBase && input.Role != layerRoleOverlay {
		return NewSourceSpec{}, ErrValidation
	}
	switch input.Mode {
	case sourceModeBuiltin:
		if input.Origin != layerOriginRepo || input.Path == nil || input.ProducerProfileID != nil {
			return NewSourceSpec{}, ErrValidation
		}
	case sourceModeCommand:
		if input.Origin != layerOriginRepo || input.Path != nil || input.ProducerProfileID == nil {
			return NewSourceSpec{}, ErrValidation
		}
	case aiMode:
		if input.Origin != aiOrigin || input.Path != nil || input.ProducerProfileID == nil {
			return NewSourceSpec{}, ErrValidation
		}
	case sourceModePush:
		if input.Origin != layerOriginThirdParty || input.Path != nil || input.ProducerProfileID != nil {
			return NewSourceSpec{}, ErrValidation
		}
	case sourceModeManual:
		if input.Origin != layerOriginManual || input.Path != nil || input.ProducerProfileID != nil {
			return NewSourceSpec{}, ErrValidation
		}
	default:
		return NewSourceSpec{}, ErrValidation
	}
	if input.Path != nil && (utf8.RuneCountInString(*input.Path) > maxSourcePathRunes || *input.Path == "") {
		return NewSourceSpec{}, ErrValidation
	}
	if input.AssetNameTemplate == "" {
		input.AssetNameTemplate = defaultSourceAssetNameTemplate
	}
	if input.TimeoutSec == 0 {
		input.TimeoutSec = defaultSourceTimeoutByMode(input.Mode)
	}
	if input.TimeoutSec < sourceTimeoutMinSec || input.TimeoutSec > sourceTimeoutMaxSec {
		return NewSourceSpec{}, ErrValidation
	}
	if len(input.BranchPatterns) == 0 {
		input.BranchPatterns = []string{sourceBranchPatternAll}
	}
	if input.ConfigOrigin == "" {
		input.ConfigOrigin = sourceConfigOriginAPI
	}
	input.BranchPatterns = slices.Clone(input.BranchPatterns)
	return input, nil
}

func defaultSourceTimeoutByMode(mode string) int {
	switch mode {
	case aiMode:
		return defaultSourceTimeoutAI
	case sourceModeBuiltin:
		return defaultSourceTimeoutBuiltin
	case sourceModeCommand:
		return defaultSourceTimeoutCommand
	case sourceModePush:
		return defaultSourceTimeoutPush
	case sourceModeManual:
		return defaultSourceTimeoutManual
	default:
		return 0
	}
}

func validServiceVisibility(visibility string) bool {
	return visibility == serviceVisibilityPrivate || visibility == serviceVisibilityInternal || visibility == serviceVisibilityPublic
}

func (discovery *Discovery) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, scopeWildcard)) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || discovery.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := discovery.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}
