package service

import (
	"context"
	"testing"
	"time"
	"uuid"
)

type auditStoreStub struct {
	tenantCalls   int
	platformCalls int
	tenantID      uuid.UUID
	filter        AuditFilter
}

func (stub *auditStoreStub) ListTenantAudits(_ context.Context, tenantID uuid.UUID, filter AuditFilter, _, _ int32) ([]AuditRecord, int64, error) {
	stub.tenantCalls++
	stub.tenantID = tenantID
	stub.filter = filter
	return []AuditRecord{}, 0, nil
}

func (stub *auditStoreStub) ListPlatformAudits(_ context.Context, filter AuditFilter, _, _ int32) ([]AuditRecord, int64, error) {
	stub.platformCalls++
	stub.filter = filter
	return []AuditRecord{}, 0, nil
}

type auditIdentityStub struct {
	IdentityStore
	membership Membership
}

func (stub auditIdentityStub) ActiveMembership(context.Context, uuid.UUID, string) (Membership, error) {
	return stub.membership, nil
}

func TestAuditsEnforceTenantAndPlatformBoundaries(t *testing.T) {
	t.Parallel()
	tenantID := uuid.NewV7()
	store := &auditStoreStub{}
	audits := NewAudits(store, auditIdentityStub{membership: Membership{TenantID: tenantID, TenantSlug: "acme", Role: "tenant_admin"}})
	tenantActor := Principal{Kind: PrincipalJWT, User: User{ID: uuid.NewV7()}}
	if _, _, err := audits.ListTenant(t.Context(), tenantActor, "acme", AuditFilter{}, 1, 20); err != nil {
		t.Fatalf("tenant audit list: %v", err)
	}
	if store.tenantCalls != 1 || store.tenantID != tenantID {
		t.Fatalf("tenant query calls/id = %d/%s, want 1/%s", store.tenantCalls, store.tenantID, tenantID)
	}

	platformActor := Principal{Kind: PrincipalJWT, User: User{IsPlatformAdmin: true}}
	if _, _, err := audits.ListPlatform(t.Context(), platformActor, AuditFilter{TenantSlug: "acme"}, 1, 20); err != nil {
		t.Fatalf("platform audit list: %v", err)
	}
	if store.platformCalls != 1 {
		t.Fatalf("platform query calls = %d, want 1", store.platformCalls)
	}

	for _, actor := range []Principal{
		{Kind: PrincipalPAT, TenantSlug: "acme", TenantID: tenantID, Role: "tenant_admin", Scopes: []string{"asset:read"}},
		{Kind: PrincipalJWT, User: User{IsPlatformAdmin: false}},
	} {
		if _, _, err := audits.ListPlatform(t.Context(), actor, AuditFilter{}, 1, 20); err != ErrNotFound {
			t.Errorf("unauthorized platform actor error = %v, want ErrNotFound", err)
		}
	}
}

func TestAuditsValidateFiltersBeforePersistence(t *testing.T) {
	t.Parallel()
	store := &auditStoreStub{}
	audits := NewAudits(store, nil)
	actor := Principal{Kind: PrincipalJWT, User: User{IsPlatformAdmin: true}}
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)
	valid := AuditFilter{Actions: []string{"job.failed"}, ResourceType: "job", From: &earlier, To: &now, TenantSlug: "acme"}
	if _, _, err := audits.ListPlatform(t.Context(), actor, valid, 2, 10); err != nil {
		t.Fatalf("valid audit filter: %v", err)
	}
	if store.filter.ResourceType != "job" || store.filter.TenantSlug != "acme" {
		t.Fatalf("forwarded filter = %#v", store.filter)
	}
	tooManyActions := make([]string, maxAuditFilterValues+1)
	for _, invalid := range []AuditFilter{
		{Actions: tooManyActions},
		{Actions: []string{"bad\naction"}},
		{ResourceType: "bad\x00type"},
		{From: &now, To: &earlier},
		{TenantSlug: "../acme"},
	} {
		if _, _, err := audits.ListPlatform(t.Context(), actor, invalid, 1, 20); err != ErrValidation {
			t.Errorf("invalid filter %#v error = %v, want ErrValidation", invalid, err)
		}
	}
}
