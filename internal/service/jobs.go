package service

import (
	"context"
	"crypto/sha256"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"time"
	"uuid"
)

var validJobTypes = [...]string{"tenant.delete", "repo.sync", "repo.discover", "asset.produce", "asset.merge", "asset.index", "asset.ai_generate", "asset.reindex", "diff.run", "outbox.dispatch", "workspace.gc", "blob.gc", "retention.cleanup"}
var validJobStatuses = [...]string{"pending", "running", "succeeded", "succeeded_with_warnings", "failed", "outcome_unknown", "cancelled"}
var validJobScopes = [...]string{"tenant", "repository", "service", "source", "track", "version", "asset", "diff", "system"}

// Jobs coordinates platform job visibility and keeps tenant-owned payloads out of the admin API.
type Jobs struct {
	store      JobStore
	identities IdentityStore
	now        func() time.Time
}

// NewJobs constructs the tenant control and platform query use cases.
func NewJobs(store JobStore, identities IdentityStore) *Jobs {
	return &Jobs{store: store, identities: identities, now: time.Now}
}

// ListTenant returns one authorized deterministic page with stage-attempt history.
func (jobs *Jobs) ListTenant(ctx context.Context, actor Principal, tenantSlug string, filter JobFilter, page, pageSize int) ([]JobRecord, int64, error) {
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, "job:read")
	if err != nil {
		return nil, 0, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	if err := validateJobFilter(filter); err != nil {
		return nil, 0, err
	}
	items, total, err := jobs.store.ListTenantJobs(ctx, membership.TenantID, filter, int32(pageSize), int32((page-1)*pageSize))
	if err != nil {
		return nil, 0, err
	}
	for index := range items {
		items[index] = jobs.tenantProjection(items[index], actor, membership, tenantSlug)
	}
	return items, total, nil
}

// GetTenant returns one authorized job with persisted stage-attempt history.
func (jobs *Jobs) GetTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID) (JobRecord, error) {
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, "job:read")
	if err != nil {
		return JobRecord{}, err
	}
	record, err := jobs.store.GetTenantJob(ctx, membership.TenantID, id)
	if err != nil {
		return JobRecord{}, err
	}
	return jobs.tenantProjection(record, actor, membership, tenantSlug), nil
}

// CancelTenant atomically cancels a pending or running domain and River job.
func (jobs *Jobs) CancelTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID) (JobAccepted, error) {
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, "job:run")
	if err != nil {
		return JobAccepted{}, err
	}
	return jobs.store.CancelTenantJob(ctx, membership.TenantID, id, jobs.now().UTC())
}

// RetryTenant creates one independent retry generation under a required idempotency key.
func (jobs *Jobs) RetryTenant(ctx context.Context, actor Principal, tenantSlug string, id, idempotencyKey uuid.UUID) (JobAccepted, error) {
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, "job:run")
	if err != nil {
		return JobAccepted{}, err
	}
	principalType, principalID := rotationPrincipal(actor)
	if idempotencyKey == uuid.Nil() || principalID == uuid.Nil() {
		return JobAccepted{}, ErrValidation
	}
	payload, err := json.Marshal(struct {
		// Operation fixes the hash namespace to the OpenAPI operation identifier.
		Operation string `json:"operation"`
		// TenantSlug binds the key to the tenant route.
		TenantSlug string `json:"tenantSlug"`
		// JobID binds the key to the exact source job.
		JobID uuid.UUID `json:"jobId"`
	}{Operation: "retryJob", TenantSlug: tenantSlug, JobID: id})
	if err != nil {
		return JobAccepted{}, fmt.Errorf("encode retry job identity: %w", err)
	}
	digest := sha256.Sum256(payload)
	return jobs.store.RetryTenantJob(ctx, RetryJobRequest{
		TenantID: membership.TenantID, SourceJobID: id, PrincipalType: principalType,
		PrincipalID: principalID, IdempotencyKey: idempotencyKey, RequestHash: digest[:], RequestedAt: jobs.now().UTC(),
	})
}

// OpenTenantStream authorizes one replay cursor and captures the initial durable state.
func (jobs *Jobs) OpenTenantStream(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID, afterSequence int64) (*JobStream, error) {
	if afterSequence < 0 {
		return nil, ErrValidation
	}
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, "job:read")
	if err != nil {
		return nil, err
	}
	initial, _, err := jobs.store.GetTenantJobStreamState(ctx, membership.TenantID, id)
	if err != nil {
		return nil, err
	}
	initial = jobs.tenantProjection(initial, actor, membership, tenantSlug)
	return &JobStream{
		store: jobs.store, tenantID: membership.TenantID, jobID: id, initial: initial,
		cursor: afterSequence, pollInterval: time.Second, heartbeatInterval: 15 * time.Second,
	}, nil
}

// ListPlatform returns a deterministic redacted page for a platform administrator.
func (jobs *Jobs) ListPlatform(ctx context.Context, actor Principal, filter PlatformJobFilter, page, pageSize int) ([]PlatformJobRecord, int64, error) {
	if !isPlatformAdministrator(actor) {
		return nil, 0, ErrNotFound
	}
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	if err := validatePlatformJobFilter(filter); err != nil {
		return nil, 0, err
	}
	return jobs.store.ListPlatformJobs(ctx, filter, int32(pageSize), int32((page-1)*pageSize))
}

// GetPlatform returns one redacted job for a platform administrator.
func (jobs *Jobs) GetPlatform(ctx context.Context, actor Principal, id uuid.UUID) (PlatformJobRecord, error) {
	if !isPlatformAdministrator(actor) {
		return PlatformJobRecord{}, ErrNotFound
	}
	return jobs.store.GetPlatformJob(ctx, id)
}

func validatePlatformJobFilter(filter PlatformJobFilter) error {
	for _, value := range filter.Types {
		if !slices.Contains(validJobTypes[:], value) {
			return ErrValidation
		}
	}
	for _, value := range filter.Statuses {
		if !slices.Contains(validJobStatuses[:], value) {
			return ErrValidation
		}
	}
	if filter.ScopeType != "" && !slices.Contains(validJobScopes[:], filter.ScopeType) {
		return ErrValidation
	}
	if strings.ContainsAny(filter.ScopeID, "\x00\r\n") || len(filter.ScopeID) > 128 {
		return ErrValidation
	}
	if filter.TenantSlug != "" && !slugPattern.MatchString(filter.TenantSlug) {
		return ErrValidation
	}
	return nil
}

const streamLogBatch = 100

// JobStream emits resumable persisted logs and durable state until termination or cancellation.
type JobStream struct {
	store             JobStore
	tenantID          uuid.UUID
	jobID             uuid.UUID
	initial           JobRecord
	cursor            int64
	pollInterval      time.Duration
	heartbeatInterval time.Duration
}

// Run blocks while emitting the initial snapshot, ordered logs, state changes, and heartbeats.
func (stream *JobStream) Run(ctx context.Context, sink JobEventSink) error {
	cursor := stream.cursor
	current := stream.initial
	if err := sink.State(jobStateEvent(current, cursor)); err != nil {
		return err
	}
	lastStatus, lastProgress := current.Status, current.Progress
	poll := time.NewTicker(stream.pollInterval)
	defer poll.Stop()
	heartbeat := time.NewTicker(stream.heartbeatInterval)
	defer heartbeat.Stop()

	for {
		next, snapshotCursor, err := stream.store.GetTenantJobStreamState(ctx, stream.tenantID, stream.jobID)
		if err != nil {
			return err
		}
		emittedLog := false
		for cursor < snapshotCursor {
			logs, err := stream.store.ListTenantJobLogsAfter(ctx, stream.tenantID, stream.jobID, cursor, streamLogBatch)
			if err != nil {
				return err
			}
			if len(logs) == 0 {
				break
			}
			for _, event := range logs {
				if event.Sequence > snapshotCursor {
					break
				}
				if err := sink.Log(event); err != nil {
					return err
				}
				cursor = event.Sequence
				emittedLog = true
			}
		}
		next.Progress = jobProgress(next.Status, next.Stage)
		stateChanged := next.Status != lastStatus || next.Progress != lastProgress
		if stateChanged || (terminalJobStatus(next.Status) && emittedLog) {
			if err := sink.State(jobStateEvent(next, cursor)); err != nil {
				return err
			}
			lastStatus, lastProgress = next.Status, next.Progress
		}
		if terminalJobStatus(next.Status) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			if err := sink.Heartbeat(); err != nil {
				return err
			}
		case <-poll.C:
		}
	}
}

func (jobs *Jobs) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || !patAllowsJobPermission(actor.Scopes, permission) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || jobs.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := jobs.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil {
		return Membership{}, err
	}
	if !roleAllows(membership.Role, permission) {
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func (jobs *Jobs) tenantProjection(record JobRecord, actor Principal, membership Membership, tenantSlug string) JobRecord {
	record.TenantSlug = tenantSlug
	record.Progress = jobProgress(record.Status, record.Stage)
	record.Capabilities = []string{"job:read"}
	if roleAllows(membership.Role, "job:run") && (actor.Kind != PrincipalPAT || patAllowsJobPermission(actor.Scopes, "job:run")) {
		if record.Status == "pending" || record.Status == "running" || record.Status == "failed" || record.Status == "cancelled" {
			record.Capabilities = append(record.Capabilities, "job:run")
		}
	}
	return record
}

func validateJobFilter(filter JobFilter) error {
	return validatePlatformJobFilter(PlatformJobFilter{
		Types: filter.Types, Statuses: filter.Statuses, ScopeType: filter.ScopeType, ScopeID: filter.ScopeID,
	})
}

func patAllowsJobPermission(scopes []string, permission string) bool {
	if slices.Contains(scopes, "*") || slices.Contains(scopes, permission) {
		return true
	}
	return permission == "job:read" && slices.Contains(scopes, "job:run")
}

func jobProgress(status string, stage *string) int {
	if status == "succeeded" || status == "succeeded_with_warnings" {
		return 100
	}
	if status == "pending" || stage == nil {
		return 0
	}
	switch *stage {
	case "resolve":
		return 5
	case "discover":
		return 20
	case "extract":
		return 40
	case "merge":
		return 60
	case "normalize":
		return 80
	case "index":
		return 95
	default:
		return 0
	}
}

func terminalJobStatus(status string) bool {
	return status == "succeeded" || status == "succeeded_with_warnings" || status == "failed" || status == "outcome_unknown" || status == "cancelled"
}

func jobStateEvent(record JobRecord, cursor int64) JobStateEvent {
	return JobStateEvent{Cursor: cursor, Status: record.Status, Progress: jobProgress(record.Status, record.Stage), At: record.UpdatedAt}
}
