package service

import (
	"context"
	"slices"
	"strings"
)

const maxAuditFilterValues = 50

// Audits 协调租户隔离与平台审计可见性。
type Audits struct {
	store      AuditStore
	identities IdentityStore
}

// NewAudits 构造审计查询用例。
func NewAudits(store AuditStore, identities IdentityStore) *Audits {
	return &Audits{store: store, identities: identities}
}

// ListTenant 在解析租户成员关系后返回脱敏审计页。
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

// ListPlatform 向平台管理员返回脱敏跨租户审计页。
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
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, scopeTenantAuditRead) || (!slices.Contains(actor.Scopes, scopeTenantAuditRead) && !slices.Contains(actor.Scopes, scopeWildcard)) {
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
	if !roleAllows(membership.Role, scopeTenantAuditRead) {
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func validateAuditQuery(filter AuditFilter, page, pageSize int, platform bool) error {
	if err := validatePagination(page, pageSize); err != nil {
		return err
	}
	if len(filter.Actions) > maxAuditFilterValues || invalidAuditFilterText(filter.ResourceType, maxFilterTextRunes) || invalidAuditFilterText(filter.ResourceID, maxFilterTextRunes) {
		return ErrValidation
	}
	for _, action := range filter.Actions {
		if invalidAuditFilterText(action, maxFilterTextRunes) {
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
