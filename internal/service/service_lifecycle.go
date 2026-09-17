package service

import (
	"context"
	"errors"
	"slices"
	"time"
	"unicode/utf8"
	"uuid"
)

const (
	serviceLifecycleDraft      = "draft"
	serviceLifecyclePublished  = "published"
	serviceLifecycleDeprecated = "deprecated"
	serviceLifecycleRetired    = "retired"
	maxServiceVisibilityRunes  = 16
)

// ServiceLifecycle coordinates service metadata updates, public reads, and
// soft deletion under the frozen lifecycle state machine.
type ServiceLifecycle struct {
	store      ServiceLifecycleStore
	assets     *Assets
	identities IdentityStore
	now        func() time.Time
}

// NewServiceLifecycle constructs service lifecycle use cases.
func NewServiceLifecycle(store ServiceLifecycleStore, assets *Assets, identities IdentityStore) *ServiceLifecycle {
	return &ServiceLifecycle{store: store, assets: assets, identities: identities, now: time.Now}
}

// Update applies a validated service patch under the expected revision and
// advances the lifecycle state machine, emitting the service.deprecated event
// on the published-to-deprecated transition.
func (lifecycle *ServiceLifecycle) Update(ctx context.Context, actor Principal, tenantSlug, serviceSlug, etag string, input ServicePatchInput) (ServiceRecord, error) {
	membership, err := lifecycle.tenantMembership(ctx, actor, tenantSlug, "service:write")
	if err != nil {
		return ServiceRecord{}, err
	}
	service, err := lifecycle.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return ServiceRecord{}, err
	}
	expectedRevision, err := parseRevisionETag(etag, "service", service.ID)
	if err != nil {
		return ServiceRecord{}, ErrPrecondition
	}
	current, err := lifecycle.store.GetServiceForUpdate(ctx, membership.TenantID, service.ID)
	if err != nil {
		return ServiceRecord{}, err
	}
	if current.Revision != expectedRevision {
		return ServiceRecord{}, ErrPrecondition
	}
	if err := validateServicePatch(input); err != nil {
		return ServiceRecord{}, err
	}
	nextLifecycle := current.Lifecycle
	if input.Lifecycle != nil && *input.Lifecycle != current.Lifecycle {
		if !validServiceLifecycleTransition(current.Lifecycle, *input.Lifecycle) {
			return ServiceRecord{}, ErrInvalidState
		}
		nextLifecycle = *input.Lifecycle
	}

	patch := ServicePatch{TenantID: membership.TenantID, ID: current.ID, ExpectedRevision: expectedRevision}
	if input.DisplayName != nil {
		patch.DisplayName = input.DisplayName
	}
	if input.Description != nil {
		patch.Description = input.Description
		patch.SetDescription = true
	}
	if input.Visibility != nil {
		visibility := string(*input.Visibility)
		patch.Visibility = &visibility
	}
	if input.Lifecycle != nil {
		lifecycleValue := nextLifecycle
		patch.Lifecycle = &lifecycleValue
	}
	record, err := lifecycle.store.UpdateService(ctx, patch)
	if err != nil {
		return ServiceRecord{}, err
	}
	if input.Lifecycle != nil && current.Lifecycle == serviceLifecyclePublished && nextLifecycle == serviceLifecycleDeprecated {
		if err := lifecycle.appendServiceDeprecatedEvent(ctx, membership.TenantID, record); err != nil {
			return ServiceRecord{}, err
		}
	}
	return record, nil
}

// GetPublic resolves an anonymous public service view gated by visibility and
// lifecycle, returning 404 for private/internal or non-published states.
func (lifecycle *ServiceLifecycle) GetPublic(ctx context.Context, tenantSlug, serviceSlug string) (PublicServiceRecord, error) {
	record, err := lifecycle.store.GetPublicServiceBySlug(ctx, tenantSlug, serviceSlug)
	if err != nil {
		return PublicServiceRecord{}, err
	}
	if record.Visibility != "public" {
		return PublicServiceRecord{}, ErrNotFound
	}
	if record.Lifecycle != serviceLifecyclePublished && record.Lifecycle != serviceLifecycleDeprecated {
		return PublicServiceRecord{}, ErrNotFound
	}
	summaries, _, err := lifecycle.publicAssetSummaries(ctx, record)
	if err != nil {
		return PublicServiceRecord{}, err
	}
	return PublicServiceRecord{
		Slug: record.Slug, DisplayName: record.DisplayName, Description: record.Description,
		Lifecycle: record.Lifecycle, Tags: []string{}, Assets: summaries, UpdatedAt: record.UpdatedAt,
	}, nil
}

// Delete soft-deletes one service under its revision, cascading logical state
// and cancelling pending service-scoped work.
func (lifecycle *ServiceLifecycle) Delete(ctx context.Context, actor Principal, tenantSlug, serviceSlug, etag string) error {
	membership, err := lifecycle.tenantMembership(ctx, actor, tenantSlug, "service:write")
	if err != nil {
		return err
	}
	service, err := lifecycle.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return err
	}
	expectedRevision, err := parseRevisionETag(etag, "service", service.ID)
	if err != nil {
		return ErrPrecondition
	}
	current, err := lifecycle.store.GetServiceForUpdate(ctx, membership.TenantID, service.ID)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return ErrPrecondition
	}
	if _, err := lifecycle.store.DeleteService(ctx, membership.TenantID, current.ID, expectedRevision); err != nil {
		return err
	}
	return nil
}

func (lifecycle *ServiceLifecycle) publicAssetSummaries(ctx context.Context, record ServiceRecord) ([]AssetSummaryRecord, []MissingKindRecord, error) {
	if lifecycle.assets == nil {
		return []AssetSummaryRecord{}, []MissingKindRecord{}, nil
	}
	return lifecycle.assets.AssetSummaries(ctx, record.TenantID, record.ID)
}

func (lifecycle *ServiceLifecycle) appendServiceDeprecatedEvent(ctx context.Context, tenantID uuid.UUID, record ServiceRecord) error {
	// M1 records the service.deprecated event through the notify_outbox; the
	// outbox store is not part of this boundary yet, so this is a no-op seam
	// wired when event delivery lands with M1-AGENT-003/M3.
	return nil
}

func (lifecycle *ServiceLifecycle) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, "*")) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || lifecycle.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := lifecycle.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil || !roleAllows(membership.Role, permission) {
		if err != nil {
			return Membership{}, err
		}
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

// ServicePatchInput carries explicit PATCH fields for one service update.
type ServicePatchInput struct {
	DisplayName *string
	Description *string
	Visibility  *string
	Lifecycle   *string
}

func validateServicePatch(input ServicePatchInput) error {
	if input.DisplayName != nil && (utf8.RuneCountInString(*input.DisplayName) < 1 || utf8.RuneCountInString(*input.DisplayName) > 128) {
		return ErrValidation
	}
	if input.Description != nil && utf8.RuneCountInString(*input.Description) > 2000 {
		return ErrValidation
	}
	if input.Visibility != nil && !validServiceVisibilityValue(*input.Visibility) {
		return ErrValidation
	}
	if input.Lifecycle != nil && !validLifecycleValue(*input.Lifecycle) {
		return ErrValidation
	}
	return nil
}

func validServiceVisibilityValue(value string) bool {
	return value == "private" || value == "internal" || value == "public"
}

func validLifecycleValue(value string) bool {
	switch value {
	case serviceLifecycleDraft, serviceLifecyclePublished, serviceLifecycleDeprecated, serviceLifecycleRetired:
		return true
	}
	return false
}

func validServiceLifecycleTransition(from, to string) bool {
	switch from {
	case serviceLifecycleDraft:
		return to == serviceLifecyclePublished
	case serviceLifecyclePublished:
		return to == serviceLifecycleDeprecated
	case serviceLifecycleDeprecated:
		return to == serviceLifecycleRetired
	case serviceLifecycleRetired:
		return false
	}
	return false
}

var _ = errors.Is
