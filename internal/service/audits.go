package service

import (
	"context"
	"slices"
	"strings"
)

const maxAuditFilterValues = 50

// Audits coordinates tenant-isolated and platform audit visibility.
type Audits struct {
	store      AuditStore
	identities IdentityStore
}

// NewAudits constructs the audit query use case.
func NewAudits(store AuditStore, identities IdentityStore) *Audits {
	return &Audits{store: store, identities: identities}
}

// ListTenant returns a redacted audit page after resolving tenant membership.
func (audits *Audits) ListTenant(ctx context.Context, actor Principal, tenantSlug string, filter AuditFilter, page, pageSize int) ([]AuditRecord, int64, error) {
	membership, err := audits.tenantMembership(ctx, actor, tenantSlug)
	if err != nil {
		return nil, 0, err
	}
	if err := validateAuditQuery(filter, page, pageSize, false); err != nil {
		return nil, 0, err
	}
	return audits.store.ListTenantAudits(ctx, membership.TenantID, filter, int32(pageSize), int32((page-1)*pageSize))
}

// ListPlatform returns a redacted cross-tenant audit page to a platform administrator.
func (audits *Audits) ListPlatform(ctx context.Context, actor Principal, filter AuditFilter, page, pageSize int) ([]AuditRecord, int64, error) {
	if !isPlatformAdministrator(actor) {
		return nil, 0, ErrNotFound
	}
	if err := validateAuditQuery(filter, page, pageSize, true); err != nil {
		return nil, 0, err
	}
	return audits.store.ListPlatformAudits(ctx, filter, int32(pageSize), int32((page-1)*pageSize))
}

func (audits *Audits) tenantMembership(ctx context.Context, actor Principal, tenantSlug string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, "tenant:audit:read") || (!slices.Contains(actor.Scopes, "tenant:audit:read") && !slices.Contains(actor.Scopes, "*")) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || audits.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := audits.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil {
		return Membership{}, err
	}
	if !roleAllows(membership.Role, "tenant:audit:read") {
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func validateAuditQuery(filter AuditFilter, page, pageSize int, platform bool) error {
	if err := validatePagination(page, pageSize); err != nil {
		return err
	}
	if len(filter.Actions) > maxAuditFilterValues || invalidAuditFilterText(filter.ResourceType, 128) || invalidAuditFilterText(filter.ResourceID, 128) {
		return ErrValidation
	}
	for _, action := range filter.Actions {
		if invalidAuditFilterText(action, 128) {
			return ErrValidation
		}
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return ErrValidation
	}
	if !platform && filter.TenantSlug != "" {
		return ErrValidation
	}
	if filter.TenantSlug != "" && !slugPattern.MatchString(filter.TenantSlug) {
		return ErrValidation
	}
	return nil
}

func invalidAuditFilterText(value string, maximum int) bool {
	return len(value) > maximum || strings.ContainsAny(value, "\x00\r\n")
}
