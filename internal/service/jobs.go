package service

import (
	"context"
	"slices"
	"strings"
	"uuid"
)

var validJobTypes = [...]string{"tenant.delete", "repo.sync", "repo.discover", "asset.produce", "asset.merge", "asset.index", "asset.ai_generate", "asset.reindex", "diff.run", "outbox.dispatch", "workspace.gc", "blob.gc", "retention.cleanup"}
var validJobStatuses = [...]string{"pending", "running", "succeeded", "succeeded_with_warnings", "failed", "outcome_unknown", "cancelled"}
var validJobScopes = [...]string{"tenant", "repository", "service", "source", "track", "version", "asset", "diff", "system"}

// Jobs coordinates platform job visibility and keeps tenant-owned payloads out of the admin API.
type Jobs struct {
	store JobStore
}

// NewJobs constructs the platform job query use case.
func NewJobs(store JobStore) *Jobs { return &Jobs{store: store} }

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
