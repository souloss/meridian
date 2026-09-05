package service

import (
	"context"
	"testing"
	"uuid"
)

type jobStoreStub struct {
	listCalls int
	filter    PlatformJobFilter
	job       PlatformJobRecord
}

func (stub *jobStoreStub) ListPlatformJobs(_ context.Context, filter PlatformJobFilter, _, _ int32) ([]PlatformJobRecord, int64, error) {
	stub.listCalls++
	stub.filter = filter
	return []PlatformJobRecord{stub.job}, 1, nil
}

func (stub *jobStoreStub) GetPlatformJob(_ context.Context, _ uuid.UUID) (PlatformJobRecord, error) {
	return stub.job, nil
}

func TestJobsRequirePlatformSessionAdministrator(t *testing.T) {
	t.Parallel()
	store := &jobStoreStub{}
	jobs := NewJobs(store)
	principal := Principal{Kind: PrincipalPAT, User: User{IsPlatformAdmin: true}}
	if _, _, err := jobs.ListPlatform(t.Context(), principal, PlatformJobFilter{}, 1, 20); err != ErrNotFound {
		t.Fatalf("ListPlatform PAT error = %v, want ErrNotFound", err)
	}
	principal.Kind = PrincipalSession
	if _, _, err := jobs.ListPlatform(t.Context(), principal, PlatformJobFilter{}, 1, 20); err != nil {
		t.Fatalf("ListPlatform platform session error = %v", err)
	}
	if store.listCalls != 1 {
		t.Fatalf("ListPlatform store calls = %d, want 1", store.listCalls)
	}
}

func TestJobsValidateAndForwardFilters(t *testing.T) {
	t.Parallel()
	store := &jobStoreStub{}
	jobs := NewJobs(store)
	principal := Principal{Kind: PrincipalSession, User: User{IsPlatformAdmin: true}}
	filter := PlatformJobFilter{Types: []string{"repo.sync"}, Statuses: []string{"pending"}, ScopeType: "repository", ScopeID: "018f0b7a-3b54-7c2e-b7ef-3baf9709a2a1", TenantSlug: "acme"}
	if _, _, err := jobs.ListPlatform(t.Context(), principal, filter, 2, 10); err != nil {
		t.Fatalf("ListPlatform filtered error = %v", err)
	}
	if store.filter.ScopeType != filter.ScopeType || store.filter.ScopeID != filter.ScopeID || store.filter.TenantSlug != filter.TenantSlug || len(store.filter.Types) != 1 || store.filter.Types[0] != "repo.sync" {
		t.Fatalf("forwarded filter = %#v", store.filter)
	}
	for _, invalid := range []PlatformJobFilter{
		{Types: []string{"unknown"}},
		{Statuses: []string{"waiting"}},
		{ScopeType: "credential"},
		{TenantSlug: "../acme"},
		{ScopeID: "line\nfeed"},
	} {
		if _, _, err := jobs.ListPlatform(t.Context(), principal, invalid, 1, 20); err != ErrValidation {
			t.Errorf("invalid filter %#v error = %v, want ErrValidation", invalid, err)
		}
	}
}
