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
var validJobScopes = [...]string{"tenant", "repository", "service", "source", "track", "version", "asset", "diff", "system"}

// Jobs 协调平台任务可见性，并使租户持有的载荷远离管理 API。
type Jobs struct {
	store      JobStore
	identities IdentityStore
	now        func() time.Time
}

// NewJobs 构造租户控制与平台查询用例。
func NewJobs(store JobStore, identities IdentityStore) *Jobs {
	return &Jobs{store: store, identities: identities, now: time.Now}
}

// ListTenant 返回带阶段尝试历史的一页已授权确定性结果。
func (jobs *Jobs) ListTenant(ctx context.Context, actor Principal, tenantSlug string, filter JobFilter, page, pageSize int) ([]JobRecord, int64, error) {
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, scopeJobRead)
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

// GetTenant 返回带持久化阶段尝试历史的一个已授权任务。
func (jobs *Jobs) GetTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID) (JobRecord, error) {
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, scopeJobRead)
	if err != nil {
		return JobRecord{}, err
	}
	record, err := jobs.store.GetTenantJob(ctx, membership.TenantID, id)
	if err != nil {
		return JobRecord{}, err
	}
	return jobs.tenantProjection(record, actor, membership, tenantSlug), nil
}

// CancelTenant 原子取消一个待处理或运行中的领域与 River 任务。
func (jobs *Jobs) CancelTenant(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID) (JobAccepted, error) {
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, scopeJobRun)
	if err != nil {
		return JobAccepted{}, err
	}
	return jobs.store.CancelTenantJob(ctx, membership.TenantID, id, jobs.now().UTC())
}

// RetryTenant 在必需的幂等键下创建一个独立重试代次。
func (jobs *Jobs) RetryTenant(ctx context.Context, actor Principal, tenantSlug string, id, idempotencyKey uuid.UUID) (JobAccepted, error) {
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, scopeJobRun)
	if err != nil {
		return JobAccepted{}, err
	}
	principalType, principalID := rotationPrincipal(actor)
	if idempotencyKey == uuid.Nil() || principalID == uuid.Nil() {
		return JobAccepted{}, ErrValidation
	}
	payload, err := json.Marshal(struct {
		// Operation 将哈希命名空间固定为 OpenAPI 操作标识。
		Operation string `json:"operation"`
		// TenantSlug 将键绑定到租户路由。
		TenantSlug string `json:"tenantSlug"`
		// JobID 将键绑定到确切的源任务。
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

// OpenTenantStream 授权一个重放游标并捕获初始持久化状态。
func (jobs *Jobs) OpenTenantStream(ctx context.Context, actor Principal, tenantSlug string, id uuid.UUID, afterSequence int64) (*JobStream, error) {
	if afterSequence < 0 {
		return nil, ErrValidation
	}
	membership, err := jobs.tenantMembership(ctx, actor, tenantSlug, scopeJobRead)
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

// ListPlatform 向平台管理员返回确定性的脱敏页。
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

// GetPlatform 向平台管理员返回一个脱敏任务。
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
	if strings.ContainsAny(filter.ScopeID, "\x00\r\n") || len(filter.ScopeID) > maxFilterTextRunes {
		return ErrValidation
	}
	if filter.TenantSlug != "" && !slugPattern.MatchString(filter.TenantSlug) {
		return ErrValidation
	}
	return nil
}

// JobStream 持续发出可恢复的持久化日志与持久化状态，直至终止或取消。
type JobStream struct {
	store             JobStore
	tenantID          uuid.UUID
	jobID             uuid.UUID
	initial           JobRecord
	cursor            int64
	pollInterval      time.Duration
	heartbeatInterval time.Duration
}

// Run 阻塞式发出初始快照、有序日志、状态变化与心跳。
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
	record.Capabilities = []string{scopeJobRead}
	if roleAllows(membership.Role, scopeJobRun) && (actor.Kind != PrincipalPAT || patAllowsJobPermission(actor.Scopes, scopeJobRun)) {
		if record.Status == JobStatusPending || record.Status == JobStatusRunning || record.Status == JobStatusFailed || record.Status == JobStatusCancelled {
			record.Capabilities = append(record.Capabilities, scopeJobRun)
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
	if slices.Contains(scopes, scopeWildcard) || slices.Contains(scopes, permission) {
		return true
	}
	return permission == scopeJobRead && slices.Contains(scopes, scopeJobRun)
}

const (
	// streamLogBatch 是任务流单批拉取的日志条数上限。
	streamLogBatch = 100
	// jobProgressComplete 是任务完成的确定性进度百分比。
	jobProgressComplete = 100
	// jobProgressResolve 是 resolve 阶段完成时的进度百分比。
	jobProgressResolve = 5
	// jobProgressDiscover 是 discover 阶段完成时的进度百分比。
	jobProgressDiscover = 20
	// jobProgressExtract 是 extract 阶段完成时的进度百分比。
	jobProgressExtract = 40
	// jobProgressMerge 是 merge 阶段完成时的进度百分比。
	jobProgressMerge = 60
	// jobProgressNormalize 是 normalize 阶段完成时的进度百分比。
	jobProgressNormalize = 80
	// jobProgressIndex 是 index 阶段完成时的进度百分比。
	jobProgressIndex = 95
)

func jobProgress(status string, stage *string) int {
	if status == JobStatusSucceeded || status == JobStatusSucceededWithWarnings {
		return jobProgressComplete
	}
	if status == JobStatusPending || stage == nil {
		return 0
	}
	switch *stage {
	case StageResolve:
		return jobProgressResolve
	case StageDiscover:
		return jobProgressDiscover
	case StageExtract:
		return jobProgressExtract
	case StageMerge:
		return jobProgressMerge
	case StageNormalize:
		return jobProgressNormalize
	case StageIndex:
		return jobProgressIndex
	default:
		return 0
	}
}

func terminalJobStatus(status string) bool {
	return status == JobStatusSucceeded || status == JobStatusSucceededWithWarnings || status == JobStatusFailed || status == JobStatusOutcomeUnknown || status == JobStatusCancelled
}

func jobStateEvent(record JobRecord, cursor int64) JobStateEvent {
	return JobStateEvent{Cursor: cursor, Status: record.Status, Progress: jobProgress(record.Status, record.Stage), At: record.UpdatedAt}
}
