package service

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"
)

type jobStoreStub struct {
	listCalls int
	filter    PlatformJobFilter
	job       PlatformJobRecord
	tenantID  uuid.UUID
	tenantJob JobRecord
	tenantLog []JobLogRecord
	retry     RetryJobRequest
}

func (stub *jobStoreStub) ListPlatformJobs(_ context.Context, filter PlatformJobFilter, _, _ int32) ([]PlatformJobRecord, int64, error) {
	stub.listCalls++
	stub.filter = filter
	return []PlatformJobRecord{stub.job}, 1, nil
}

func (stub *jobStoreStub) GetPlatformJob(_ context.Context, _ uuid.UUID) (PlatformJobRecord, error) {
	return stub.job, nil
}

func (stub *jobStoreStub) ListTenantJobs(_ context.Context, tenantID uuid.UUID, _ JobFilter, _, _ int32) ([]JobRecord, int64, error) {
	stub.tenantID = tenantID
	return []JobRecord{stub.tenantJob}, 1, nil
}

func (stub *jobStoreStub) GetTenantJob(_ context.Context, tenantID, _ uuid.UUID) (JobRecord, error) {
	stub.tenantID = tenantID
	return stub.tenantJob, nil
}

func (stub *jobStoreStub) GetTenantJobStreamState(_ context.Context, tenantID, _ uuid.UUID) (JobRecord, int64, error) {
	stub.tenantID = tenantID
	return stub.tenantJob, int64(len(stub.tenantLog)), nil
}

func (stub *jobStoreStub) ListTenantJobLogsAfter(_ context.Context, _, _ uuid.UUID, after int64, _ int32) ([]JobLogRecord, error) {
	if after >= int64(len(stub.tenantLog)) {
		return []JobLogRecord{}, nil
	}
	return stub.tenantLog[after:], nil
}

func (stub *jobStoreStub) CancelTenantJob(_ context.Context, tenantID, id uuid.UUID, _ time.Time) (JobAccepted, error) {
	stub.tenantID = tenantID
	return JobAccepted{JobID: id}, nil
}

func (stub *jobStoreStub) RetryTenantJob(_ context.Context, request RetryJobRequest) (JobAccepted, error) {
	stub.retry = request
	return JobAccepted{JobID: uuid.NewV7()}, nil
}

type jobIdentityStub struct {
	IdentityStore
	membership Membership
}

func (stub jobIdentityStub) ActiveMembership(context.Context, uuid.UUID, string) (Membership, error) {
	return stub.membership, nil
}

func TestJobsRequirePlatformSessionAdministrator(t *testing.T) {
	t.Parallel()
	store := &jobStoreStub{}
	jobs := NewJobs(store, nil)
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
	jobs := NewJobs(store, nil)
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

func TestJobsTenantAuthorizationAndPATScopeExpansion(t *testing.T) {
	t.Parallel()
	tenantID := uuid.NewV7()
	store := &jobStoreStub{tenantJob: JobRecord{ID: uuid.NewV7(), Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	jobs := NewJobs(store, jobIdentityStub{membership: Membership{TenantID: tenantID, TenantSlug: "acme", Role: "maintainer"}})
	actor := Principal{Kind: PrincipalSession, User: User{ID: uuid.NewV7()}}
	items, total, err := jobs.ListTenant(t.Context(), actor, "acme", JobFilter{}, 1, 20)
	if err != nil {
		t.Fatalf("ListTenant session error = %v", err)
	}
	if total != 1 || store.tenantID != tenantID || len(items) != 1 || items[0].TenantSlug != "acme" {
		t.Fatalf("tenant list = %#v total=%d tenant=%s", items, total, store.tenantID)
	}
	if len(items[0].Capabilities) != 2 || items[0].Capabilities[1] != "job:run" {
		t.Fatalf("maintainer capabilities = %v, want read and run", items[0].Capabilities)
	}

	pat := Principal{Kind: PrincipalPAT, TenantID: tenantID, TenantSlug: "acme", Role: "maintainer", Scopes: []string{"job:run"}}
	if _, err := jobs.GetTenant(t.Context(), pat, "acme", store.tenantJob.ID); err != nil {
		t.Fatalf("job:run PAT implied read error = %v", err)
	}
	pat.Scopes = []string{"asset:read"}
	if _, err := jobs.GetTenant(t.Context(), pat, "acme", store.tenantJob.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unscoped PAT error = %v, want ErrNotFound", err)
	}
}

func TestJobsRetryBindsPrincipalAndSemanticRequest(t *testing.T) {
	t.Parallel()
	tenantID, sourceJobID, sessionID, key := uuid.NewV7(), uuid.NewV7(), uuid.NewV7(), uuid.NewV7()
	store := &jobStoreStub{}
	jobs := NewJobs(store, jobIdentityStub{membership: Membership{TenantID: tenantID, TenantSlug: "acme", Role: "maintainer"}})
	jobs.now = func() time.Time { return time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC) }
	actor := Principal{Kind: PrincipalSession, SessionID: sessionID, User: User{ID: uuid.NewV7()}}
	if _, err := jobs.RetryTenant(t.Context(), actor, "acme", sourceJobID, key); err != nil {
		t.Fatalf("RetryTenant error = %v", err)
	}
	if store.retry.TenantID != tenantID || store.retry.SourceJobID != sourceJobID || store.retry.PrincipalType != "session" || store.retry.PrincipalID != sessionID || store.retry.IdempotencyKey != key || len(store.retry.RequestHash) != 32 {
		t.Fatalf("retry request = %#v", store.retry)
	}
}

type recordingJobSink struct {
	states     []JobStateEvent
	logs       []JobLogRecord
	heartbeats int
}

func (sink *recordingJobSink) State(event JobStateEvent) error {
	sink.states = append(sink.states, event)
	return nil
}

func (sink *recordingJobSink) Log(event JobLogRecord) error {
	sink.logs = append(sink.logs, event)
	return nil
}

func (sink *recordingJobSink) Heartbeat() error {
	sink.heartbeats++
	return nil
}

func TestJobStreamReplaysOrderedLogsAndClosesOnTerminalState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)
	stage := "resolve"
	store := &jobStoreStub{
		tenantJob: JobRecord{ID: uuid.NewV7(), Status: "failed", Stage: &stage, UpdatedAt: now},
		tenantLog: []JobLogRecord{
			{Sequence: 1, Stage: &stage, Message: "started", OccurredAt: now.Add(-time.Second)},
			{Sequence: 2, Stage: &stage, Message: "failed", OccurredAt: now},
		},
	}
	stream := &JobStream{
		store: store, tenantID: uuid.NewV7(), jobID: store.tenantJob.ID, initial: store.tenantJob,
		pollInterval: time.Millisecond, heartbeatInterval: time.Hour,
	}
	sink := &recordingJobSink{}
	if err := stream.Run(t.Context(), sink); err != nil {
		t.Fatalf("stream terminal job: %v", err)
	}
	if len(sink.logs) != 2 || sink.logs[0].Sequence != 1 || sink.logs[1].Sequence != 2 {
		t.Fatalf("stream logs = %#v", sink.logs)
	}
	if len(sink.states) != 2 || sink.states[0].Status != "failed" || sink.states[1].Cursor != 2 || sink.states[1].Progress != 5 {
		t.Fatalf("stream states = %#v", sink.states)
	}
}
