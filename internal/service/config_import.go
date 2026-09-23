package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"uuid"
)

const (
	repositoryConfigFile       = ".asset-platform.yaml"
	configImportPreviewTTL     = 24 * time.Hour
	configImportLimitBytes     = 1048576
	configPreviewRefTypeBranch = "branch"
)

// ConfigImportStore 是 M2 gitops 配置导入的持久化边界：预览持久化，以及物化 apply 所需的
// 服务/源/生产者配置读取。
type ConfigImportStore interface {
	// GetRepository 承载 ConfigImportStore 的生成 GetRepository 值。
	GetRepository(context.Context, uuid.UUID, uuid.UUID) (RepositoryRecord, error)
	// CreateConfigImportPreview 承载 ConfigImportStore 的生成 CreateConfigImportPreview 值。
	CreateConfigImportPreview(context.Context, NewConfigImportPreview) (ConfigImportPreviewRecord, error)
	// GetConfigImportPreview 承载 ConfigImportStore 的生成 GetConfigImportPreview 值。
	GetConfigImportPreview(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (ConfigImportPreviewRecord, error)
	// GetConfigImportPreviewByID 承载 ConfigImportStore 的生成 GetConfigImportPreviewByID 值。
	GetConfigImportPreviewByID(context.Context, uuid.UUID, uuid.UUID) (ConfigImportPreviewRecord, error)
	// GetServiceBySlug 承载 ConfigImportStore 的生成 GetServiceBySlug 值。
	GetServiceBySlug(context.Context, uuid.UUID, string) (ServiceRecord, error)
	// CreateService 承载 ConfigImportStore 的生成 CreateService 值。
	CreateService(context.Context, NewService) (ServiceRecord, error)
	// ListServicesByRepository 承载 ConfigImportStore 的生成 ListServicesByRepository 值。
	ListServicesByRepository(context.Context, uuid.UUID, uuid.UUID) ([]ServiceRecord, error)
	// GetProducerProfileByName 承载 ConfigImportStore 的生成 GetProducerProfileByName 值。
	GetProducerProfileByName(context.Context, string) (ProducerProfile, error)
	// CreateSourceSpec 承载 ConfigImportStore 的生成 CreateSourceSpec 值。
	CreateSourceSpec(context.Context, NewSourceSpec) (SourceSpecRecord, error)
	// ListSourceSpecsForService 承载 ConfigImportStore 的生成 ListSourceSpecsForService 值。
	ListSourceSpecsForService(context.Context, uuid.UUID, uuid.UUID) ([]SourceSpecRecord, error)
}

// NewConfigImportPreview 承载一次预览插入。
type NewConfigImportPreview struct {
	// TenantID 承载 NewConfigImportPreview 的生成 TenantID 值。
	TenantID uuid.UUID
	// ID 承载 NewConfigImportPreview 的生成 ID 值。
	ID uuid.UUID
	// RepositoryID 承载 NewConfigImportPreview 的生成 RepositoryID 值。
	RepositoryID uuid.UUID
	// RefType 承载 NewConfigImportPreview 的生成 RefType 值。
	RefType string
	// RefName 承载 NewConfigImportPreview 的生成 RefName 值。
	RefName string
	// Commit 承载 NewConfigImportPreview 的生成 Commit 值。
	Commit string
	// ConfigDigest 承载 NewConfigImportPreview 的生成 ConfigDigest 值。
	ConfigDigest string
	// Preview 承载 NewConfigImportPreview 的生成 Preview 值。
	Preview []byte
	// ExpiresAt 承载 NewConfigImportPreview 的生成 ExpiresAt 值。
	ExpiresAt time.Time
}

// ConfigImportPreviewRecord 是一个已持久化的预览快照。
type ConfigImportPreviewRecord struct {
	// ID 承载 ConfigImportPreviewRecord 的生成 ID 值。
	ID uuid.UUID
	// RepositoryID 承载 ConfigImportPreviewRecord 的生成 RepositoryID 值。
	RepositoryID uuid.UUID
	// RefType 承载 ConfigImportPreviewRecord 的生成 RefType 值。
	RefType string
	// RefName 承载 ConfigImportPreviewRecord 的生成 RefName 值。
	RefName string
	// Commit 承载 ConfigImportPreviewRecord 的生成 Commit 值。
	Commit string
	// ConfigDigest 承载 ConfigImportPreviewRecord 的生成 ConfigDigest 值。
	ConfigDigest string
	// Preview 承载 ConfigImportPreviewRecord 的生成 Preview 值。
	Preview []byte
	// ExpiresAt 承载 ConfigImportPreviewRecord 的生成 ExpiresAt 值。
	ExpiresAt time.Time
}

// ConfigImportPreview 是一个预览的 API 投影。
type ConfigImportPreview struct {
	// PreviewID 承载 ConfigImportPreview 的生成 PreviewID 值。
	PreviewID uuid.UUID
	// RepositoryID 承载 ConfigImportPreview 的生成 RepositoryID 值。
	RepositoryID uuid.UUID
	// Commit 承载 ConfigImportPreview 的生成 Commit 值。
	Commit string
	// ConfigDigest 承载 ConfigImportPreview 的生成 ConfigDigest 值。
	ConfigDigest string
	// Services 承载 ConfigImportPreview 的生成 Services 值。
	Services []ServiceRecord
	// Sources 承载 ConfigImportPreview 的生成 Sources 值。
	Sources []SourceSpecRecord
	// ExpiresAt 承载 ConfigImportPreview 的生成 ExpiresAt 值。
	ExpiresAt time.Time
}

// ConfigImportResult 报告一次 apply 的结局。
type ConfigImportResult struct {
	// RepositoryID 承载 ConfigImportResult 的生成 RepositoryID 值。
	RepositoryID uuid.UUID
	// Commit 承载 ConfigImportResult 的生成 Commit 值。
	Commit string
	// ConfigDigest 承载 ConfigImportResult 的生成 ConfigDigest 值。
	ConfigDigest string
	// CreatedServices 承载 ConfigImportResult 的生成 CreatedServices 值。
	CreatedServices []uuid.UUID
	// UpdatedServices 承载 ConfigImportResult 的生成 UpdatedServices 值。
	UpdatedServices []uuid.UUID
	// SourceSpecs 承载 ConfigImportResult 的生成 SourceSpecs 值。
	SourceSpecs []uuid.UUID
}

// ConfigImport 协调仓库配置导入的 preview/apply 两阶段。数据库始终权威：apply 仅创建或更新
// 预览中显式列出的资源，绝不覆盖手工编辑。
type ConfigImport struct {
	store      ConfigImportStore
	identities IdentityStore
	workspace  string
	gitBinary  string
	now        func() time.Time
}

// NewConfigImport 构造 gitops 配置导入用例。
func NewConfigImport(store ConfigImportStore, identities IdentityStore, workspace string) *ConfigImport {
	return &ConfigImport{store: store, identities: identities, workspace: workspace, gitBinary: "git", now: time.Now}
}

// Preview 校验并规范化某引用处的仓库配置，并持久化一个非权威预览快照。
func (imports *ConfigImport) Preview(ctx context.Context, actor Principal, tenantSlug string, repositoryID, idempotencyKey uuid.UUID, refType, refName string) (ConfigImportPreview, error) {
	membership, err := imports.tenantMembership(ctx, actor, tenantSlug, scopeRepositoryWrite)
	if err != nil {
		return ConfigImportPreview{}, err
	}
	if idempotencyKey == uuid.Nil() {
		return ConfigImportPreview{}, ErrValidation
	}
	record, err := imports.store.GetRepository(ctx, membership.TenantID, repositoryID)
	if err != nil {
		return ConfigImportPreview{}, err
	}
	if refType == "" {
		refType = configPreviewRefTypeBranch
	}
	if refName == "" {
		refName = record.DefaultBranch
	}
	commit, content, err := imports.readConfig(ctx, record, refName)
	if err != nil {
		return ConfigImportPreview{}, err
	}
	digest, config, err := ConfigDigestFromBytes(content)
	if err != nil {
		return ConfigImportPreview{}, err
	}
	services, sources, err := imports.resolvePreview(ctx, membership.TenantID, repositoryID, config)
	if err != nil {
		return ConfigImportPreview{}, err
	}
	previewBytes, err := json.Marshal(config)
	if err != nil {
		return ConfigImportPreview{}, err
	}
	preview := ConfigImportPreview{
		PreviewID: uuid.NewV7(), RepositoryID: repositoryID, Commit: commit, ConfigDigest: digest,
		Services: services, Sources: sources, ExpiresAt: imports.now().UTC().Add(configImportPreviewTTL),
	}
	if _, err := imports.store.CreateConfigImportPreview(ctx, NewConfigImportPreview{
		TenantID: membership.TenantID, ID: preview.PreviewID, RepositoryID: repositoryID,
		RefType: refType, RefName: refName, Commit: commit, ConfigDigest: digest,
		Preview: previewBytes, ExpiresAt: preview.ExpiresAt,
	}); err != nil {
		return ConfigImportPreview{}, err
	}
	return preview, nil
}

// Apply 在预览的提交与配置摘要匹配请求时物化一个先前持久化的预览，仅创建或更新预览中列出的资源。
func (imports *ConfigImport) Apply(ctx context.Context, actor Principal, tenantSlug string, repositoryID, previewID, idempotencyKey uuid.UUID, configDigest string, replaceAiBases bool) (ConfigImportResult, error) {
	membership, err := imports.tenantMembership(ctx, actor, tenantSlug, scopeRepositoryWrite)
	if err != nil {
		return ConfigImportResult{}, err
	}
	if idempotencyKey == uuid.Nil() {
		return ConfigImportResult{}, ErrValidation
	}
	preview, err := imports.store.GetConfigImportPreview(ctx, membership.TenantID, repositoryID, previewID)
	if err != nil {
		return ConfigImportResult{}, err
	}
	if preview.ConfigDigest != configDigest {
		return ConfigImportResult{}, ErrPrecondition
	}
	var config RepositoryConfig
	if err := json.Unmarshal(preview.Preview, &config); err != nil {
		return ConfigImportResult{}, err
	}

	result := ConfigImportResult{
		RepositoryID: repositoryID, Commit: preview.Commit, ConfigDigest: preview.ConfigDigest,
		CreatedServices: []uuid.UUID{}, UpdatedServices: []uuid.UUID{}, SourceSpecs: []uuid.UUID{},
	}

	existing, err := imports.store.ListServicesByRepository(ctx, membership.TenantID, repositoryID)
	if err != nil {
		return ConfigImportResult{}, err
	}
	existingBySlug := make(map[string]ServiceRecord, len(existing))
	existingByRoot := make(map[string]ServiceRecord, len(existing))
	for _, service := range existing {
		existingBySlug[service.Slug] = service
		existingByRoot[service.RootDir] = service
	}

	for _, service := range config.Services {
		var serviceRecord ServiceRecord
		current, exists := existingBySlug[service.Name]
		if !exists {
			// 根目录与不同 slug 冲突是校验错误。
			if other, collision := existingByRoot[service.Root]; collision && other.Slug != service.Name {
				return ConfigImportResult{}, ErrDuplicate
			}
			serviceRecord, err = imports.store.CreateService(ctx, NewService{
				TenantID: membership.TenantID, ID: uuid.NewV7(), RepositoryID: repositoryID,
				Slug: service.Name, DisplayName: displayNameOrDefault(service), RootDir: service.Root,
				Visibility: serviceVisibilityPrivate,
			})
			if err != nil {
				return ConfigImportResult{}, err
			}
			result.CreatedServices = append(result.CreatedServices, serviceRecord.ID)
		} else {
			serviceRecord = current
			// 手工编辑权威：导入绝不覆盖它们。
			result.UpdatedServices = append(result.UpdatedServices, serviceRecord.ID)
		}

		for _, asset := range service.Assets {
			sourceIDs, err := imports.applyAssetSources(ctx, membership.TenantID, serviceRecord, asset, replaceAiBases)
			if err != nil {
				return ConfigImportResult{}, err
			}
			result.SourceSpecs = append(result.SourceSpecs, sourceIDs...)
		}
	}
	return result, nil
}

// resolvePreview 渲染预览将要创建或更新的服务与源配置，而不持久化任何内容。
func (imports *ConfigImport) resolvePreview(ctx context.Context, tenantID, repositoryID uuid.UUID, config RepositoryConfig) ([]ServiceRecord, []SourceSpecRecord, error) {
	services := make([]ServiceRecord, 0, len(config.Services))
	sources := make([]SourceSpecRecord, 0)
	for _, service := range config.Services {
		services = append(services, ServiceRecord{
			TenantID: tenantID, RepositoryID: repositoryID, Slug: service.Name,
			DisplayName: displayNameOrDefault(service), RootDir: service.Root,
			Visibility: serviceVisibilityPrivate, Lifecycle: lifecycleDraft,
		})
		for _, asset := range service.Assets {
			if asset.Base != nil {
				sources = append(sources, configSourceProjection(tenantID, service.Name, asset, *asset.Base, layerRoleBase))
			}
			for _, overlay := range asset.Overlays {
				sources = append(sources, configSourceProjection(tenantID, service.Name, asset, overlay, layerRoleOverlay))
			}
		}
	}
	return services, sources, nil
}

// applyAssetSources 创建某资产声明的源配置，将生产者配置名解析为 id。
func (imports *ConfigImport) applyAssetSources(ctx context.Context, tenantID uuid.UUID, service ServiceRecord, asset ConfigAsset, replaceAiBases bool) ([]uuid.UUID, error) {
	created := make([]uuid.UUID, 0, len(asset.Overlays)+1)
	apply := func(raw ConfigSource, role string) error {
		var producerProfileID *uuid.UUID
		if raw.ProducerProfile != nil {
			profile, err := imports.store.GetProducerProfileByName(ctx, *raw.ProducerProfile)
			if err != nil {
				return err
			}
			producerProfileID = new(profile.ID)
		}
		record, err := imports.store.CreateSourceSpec(ctx, NewSourceSpec{
			TenantID: tenantID, ID: uuid.NewV7(), ServiceID: service.ID, Kind: asset.Kind,
			AssetNameTemplate: assetNameTemplateOrDefault(asset), Role: role, Origin: raw.Origin, Mode: raw.Mode,
			Path: raw.Path, ProducerProfileID: producerProfileID, Ord: raw.Order, TimeoutSec: raw.TimeoutSec,
			BranchPatterns: raw.BranchPatterns, Enabled: raw.Enabled == nil || *raw.Enabled, ConfigOrigin: sourceConfigOriginRepository,
		})
		if err != nil {
			return err
		}
		created = append(created, record.ID)
		return nil
	}
	if asset.Base != nil {
		if err := apply(*asset.Base, layerRoleBase); err != nil {
			return nil, err
		}
	}
	for _, overlay := range asset.Overlays {
		if err := apply(overlay, layerRoleOverlay); err != nil {
			return nil, err
		}
	}
	_ = replaceAiBases
	return created, nil
}

// readConfig 在该引用处克隆仓库，并返回解析出的提交与原始配置文件内容。
func (imports *ConfigImport) readConfig(ctx context.Context, record RepositoryRecord, refName string) (string, []byte, error) {
	if imports.workspace == "" || !filepath.IsAbs(imports.workspace) {
		return "", nil, errors.New("config import workspace root is not an absolute path")
	}
	checkout := filepath.Join(imports.workspace, record.TenantID.String(), record.ID.String(), "config-"+uuid.NewV7().String())
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		return "", nil, fmt.Errorf("create config import workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(checkout) }()
	if _, err := exec.LookPath(imports.gitBinary); err != nil {
		return "", nil, errors.New("git is not available for configuration import")
	}
	command := exec.CommandContext(ctx, imports.gitBinary, "clone", "--quiet", "--branch", refName, record.URL, checkout)
	command.Env = append([]string(nil), os.Environ()...)
	command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0")
	if _, err := command.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("clone repository for config import: %w", err)
	}
	commitOutput, err := exec.CommandContext(ctx, imports.gitBinary, "-C", checkout, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", nil, fmt.Errorf("resolve config import commit: %w", err)
	}
	commit := strings.TrimSpace(string(commitOutput))
	content, err := os.ReadFile(filepath.Join(checkout, repositoryConfigFile))
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", repositoryConfigFile, err)
	}
	if len(content) > configImportLimitBytes {
		return "", nil, ErrValidation
	}
	return commit, content, nil
}

func configSourceProjection(tenantID uuid.UUID, serviceSlug string, asset ConfigAsset, raw ConfigSource, role string) SourceSpecRecord {
	return SourceSpecRecord{
		Kind: asset.Kind, AssetNameTemplate: assetNameTemplateOrDefault(asset), Role: role,
		Origin: raw.Origin, Mode: raw.Mode, Path: raw.Path, Ord: raw.Order, TimeoutSec: raw.TimeoutSec,
		BranchPatterns: raw.BranchPatterns, Enabled: raw.Enabled == nil || *raw.Enabled, ConfigOrigin: "repository",
	}
}

func displayNameOrDefault(service ConfigService) string {
	if service.DisplayName != "" {
		return service.DisplayName
	}
	return service.Name
}

func assetNameTemplateOrDefault(asset ConfigAsset) string {
	if asset.NamingTemplate != "" {
		return asset.NamingTemplate
	}
	return defaultSourceAssetNameTemplate
}

func (imports *ConfigImport) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	return resolveTenantMembership(ctx, actor, tenantSlug, permission, imports.identities)
}

var _ = io.Discard
