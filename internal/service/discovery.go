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

// Discovery coordinates repository discovery job enqueueing, candidate listing,
// candidate acceptance, and source configuration use cases.
type Discovery struct {
	store      DiscoveryStore
	identities IdentityStore
	assets     *Assets
	now        func() time.Time
}

// NewDiscovery constructs discovery use cases with caller-owned persistence.
func NewDiscovery(store DiscoveryStore, identities IdentityStore) *Discovery {
	return &Discovery{store: store, identities: identities, now: time.Now}
}

// WithAssets binds the asset read use cases used to enrich service responses.
func (discovery *Discovery) WithAssets(assets *Assets) *Discovery {
	discovery.assets = assets
	return discovery
}

// Discover enqueues one idempotent repository discovery job and returns its identity.
func (discovery *Discovery) Discover(ctx context.Context, actor Principal, tenantSlug string, repositoryID, idempotencyKey uuid.UUID, refType, refName string) (JobAccepted, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "repository:sync")
	if err != nil {
		return JobAccepted{}, err
	}
	if idempotencyKey == uuid.Nil() {
		return JobAccepted{}, ErrValidation
	}
	if refType != "branch" && refType != "tag" {
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

// ListCandidates returns one deterministic page of discovery candidates for a repository.
func (discovery *Discovery) ListCandidates(ctx context.Context, actor Principal, tenantSlug string, repositoryID uuid.UUID, page, pageSize int) ([]DiscoveryCandidateRecord, int64, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "repository:read")
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return discovery.store.ListDiscoveryCandidates(ctx, membership.TenantID, repositoryID, int32(pageSize), int32((page-1)*pageSize))
}

// AcceptCandidates converts pending candidates into services and returns the
// accepted service list. Candidate overrides adjust slug, display name, and
// visibility; the default visibility is private per the frozen contract.
func (discovery *Discovery) AcceptCandidates(ctx context.Context, actor Principal, tenantSlug string, repositoryID, idempotencyKey uuid.UUID, candidateIDs []uuid.UUID, overrides []CandidateOverride) ([]ServiceRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "service:create")
	if err != nil {
		return nil, err
	}
	if idempotencyKey == uuid.Nil() || len(candidateIDs) == 0 || len(candidateIDs) > 100 {
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
		if candidate.Status != "pending" {
			return nil, ErrValidation
		}
		override := overrideByID[candidateID]
		slug := serviceSlugFromCandidate(candidate, override)
		displayName := candidate.DisplayName()
		if override.DisplayName != nil {
			displayName = *override.DisplayName
		}
		visibility := "private"
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

// CreateSourceSpec validates mode field compatibility and inserts one source configuration.
func (discovery *Discovery) CreateSourceSpec(ctx context.Context, actor Principal, tenantSlug, serviceSlug string, input NewSourceSpec) (SourceSpecRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "layer:edit")
	if err != nil {
		return SourceSpecRecord{}, err
	}
	validated, err := validateNewSourceSpec(input)
	if err != nil {
		return SourceSpecRecord{}, err
	}
	if validated.Mode == "command" || validated.Mode == "ai" {
		if validated.ProducerProfileID == nil {
			return SourceSpecRecord{}, ErrValidation
		}
		profile, err := discovery.store.GetProducerProfile(ctx, *validated.ProducerProfileID)
		if err != nil {
			return SourceSpecRecord{}, err
		}
		if profile.DependencyStatus == "unavailable" || !profile.Enabled {
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
	record, err := discovery.store.CreateSourceSpec(ctx, NewSourceSpec{
		TenantID: membership.TenantID, ID: uuid.NewV7(), ServiceID: service.ID, Kind: validated.Kind,
		AssetNameTemplate: validated.AssetNameTemplate, Role: validated.Role, Origin: validated.Origin, Mode: validated.Mode,
		Path: validated.Path, ProducerProfileID: validated.ProducerProfileID, Ord: validated.Ord, TimeoutSec: validated.TimeoutSec,
		BranchPatterns: validated.BranchPatterns, Enabled: validated.Enabled, ConfigOrigin: validated.ConfigOrigin,
	})
	if err != nil {
		return SourceSpecRecord{}, err
	}
	return record, nil
}

// ListSourceBindings returns the current materialized bindings for one source spec.
func (discovery *Discovery) ListSourceBindings(ctx context.Context, actor Principal, tenantSlug string, sourceID uuid.UUID) ([]SourceBindingRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "asset:read")
	if err != nil {
		return nil, err
	}
	if _, err := discovery.store.GetSourceSpec(ctx, membership.TenantID, sourceID); err != nil {
		return nil, err
	}
	return discovery.store.ListSourceBindings(ctx, membership.TenantID, sourceID)
}

// ListSourceSpecs returns the active source specs for one service.
func (discovery *Discovery) ListSourceSpecs(ctx context.Context, actor Principal, tenantSlug, serviceSlug string) ([]SourceSpecRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "asset:read")
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

// Sync enqueues one idempotent repository synchronization job.
func (discovery *Discovery) Sync(ctx context.Context, actor Principal, tenantSlug string, repositoryID, idempotencyKey uuid.UUID, input RepositorySyncInput) (JobAccepted, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "repository:sync")
	if err != nil {
		return JobAccepted{}, err
	}
	if idempotencyKey == uuid.Nil() {
		return JobAccepted{}, ErrValidation
	}
	refType := input.RefType
	if refType == "" {
		refType = "branch"
	}
	if refType != "branch" && refType != "tag" {
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
	return discovery.store.EnqueueSyncJob(ctx, SyncJobInput{
		TenantID: membership.TenantID, RepositoryID: repositoryID, RefType: refType, RefName: refName,
		IdempotencyKey: idempotencyKey, Force: input.Force,
	})
}

// UpdateSourceSpec applies a validated patch to one source spec.
func (discovery *Discovery) UpdateSourceSpec(ctx context.Context, actor Principal, tenantSlug string, sourceID uuid.UUID, etag string, input SourceSpecPatchInput) (SourceSpecRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "layer:edit")
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

// GetService returns one service detail and records a successful read.
func (discovery *Discovery) GetService(ctx context.Context, actor Principal, tenantSlug, serviceSlug string) (ServiceRecord, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "service:read")
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

// ServiceDetail carries one service together with its asset summaries and missing kinds.
type ServiceDetail struct {
	Service        ServiceRecord
	AssetSummaries []AssetSummaryRecord
	MissingKinds   []MissingKindRecord
}

// GetServiceDetail returns one service detail enriched with its asset summaries
// and missing kinds, and records a successful read.
func (discovery *Discovery) GetServiceDetail(ctx context.Context, actor Principal, tenantSlug, serviceSlug string) (ServiceDetail, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "service:read")
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

// ListRecentServices returns the caller's recently viewed services.
func (discovery *Discovery) ListRecentServices(ctx context.Context, actor Principal, tenantSlug string, page, pageSize int) ([]ServiceRecord, int64, error) {
	membership, err := discovery.tenantMembership(ctx, actor, tenantSlug, "service:read")
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

// ProducerUnavailableError reports that the selected producer profile is not usable.
type ProducerUnavailableError struct {
	Kind string
}

func (err *ProducerUnavailableError) Error() string {
	return "selected producer profile is unavailable for kind " + err.Kind
}

// CandidateOverride adjusts one candidate during acceptance.
type CandidateOverride struct {
	CandidateID  uuid.UUID
	Slug         *string
	DisplayName  *string
	Visibility   *string
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
	if input.Role != "base" && input.Role != "overlay" {
		return NewSourceSpec{}, ErrValidation
	}
	switch input.Mode {
	case "builtin":
		if input.Origin != "repo" || input.Path == nil || input.ProducerProfileID != nil {
			return NewSourceSpec{}, ErrValidation
		}
	case "command":
		if input.Origin != "repo" || input.Path != nil || input.ProducerProfileID == nil {
			return NewSourceSpec{}, ErrValidation
		}
	case "ai":
		if input.Origin != "ai_generated" || input.Path != nil || input.ProducerProfileID == nil {
			return NewSourceSpec{}, ErrValidation
		}
	case "push":
		if input.Origin != "third_party" || input.Path != nil || input.ProducerProfileID != nil {
			return NewSourceSpec{}, ErrValidation
		}
	case "manual":
		if input.Origin != "manual" || input.Path != nil || input.ProducerProfileID != nil {
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
	if input.TimeoutSec < 10 || input.TimeoutSec > 3600 {
		return NewSourceSpec{}, ErrValidation
	}
	if len(input.BranchPatterns) == 0 {
		input.BranchPatterns = []string{"**"}
	}
	if input.ConfigOrigin == "" {
		input.ConfigOrigin = "api"
	}
	input.BranchPatterns = slices.Clone(input.BranchPatterns)
	return input, nil
}

func defaultSourceTimeoutByMode(mode string) int {
	switch mode {
	case "ai":
		return defaultSourceTimeoutAI
	case "builtin":
		return defaultSourceTimeoutBuiltin
	case "command":
		return defaultSourceTimeoutCommand
	case "push":
		return defaultSourceTimeoutPush
	case "manual":
		return defaultSourceTimeoutManual
	default:
		return 0
	}
}

func validServiceVisibility(visibility string) bool {
	return visibility == "private" || visibility == "internal" || visibility == "public"
}

func (discovery *Discovery) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, "*")) {
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
