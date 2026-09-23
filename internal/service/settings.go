package service

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"
	"uuid"
)

// PlatformSettingsValue 是平台默认配置 settings jsonb 的运行时快照。
type PlatformSettingsValue struct {
	// Settings 是 settings jsonb 的原始 JSON（保留未知字段，不丢失）。
	Settings jsontext.Value
	// Revision 是乐观并发版本号。
	Revision int64
}

// TenantSettingsValue 是租户 settings jsonb 的运行时快照。
type TenantSettingsValue struct {
	// Settings 是 tenants.settings jsonb 的原始 JSON。
	Settings jsontext.Value
	// Revision 是租户行 revision（与 tenants.revision 同源）。
	Revision int64
}

// SettingsStore 是平台与租户设置的持久化边界。
type SettingsStore interface {
	// GetPlatformSettings 承载 SettingsStore 的生成 GetPlatformSettings 值。
	GetPlatformSettings(context.Context) (PlatformSettingsValue, error)
	// UpdatePlatformSettings 承载 SettingsStore 的生成 UpdatePlatformSettings 值。
	UpdatePlatformSettings(context.Context, int64, jsontext.Value) (PlatformSettingsValue, error)
	// GetTenantSettings 承载 SettingsStore 的生成 GetTenantSettings 值。
	GetTenantSettings(context.Context, uuid.UUID) (TenantSettingsValue, error)
	// UpdateTenantSettings 承载 SettingsStore 的生成 UpdateTenantSettings 值。
	UpdateTenantSettings(context.Context, uuid.UUID, int64, jsontext.Value) (TenantSettingsValue, error)
}

// Settings 协调平台默认配置与租户运行设置用例。
type Settings struct {
	store      SettingsStore
	identities IdentityStore
}

// NewSettings 构造 M0 设置用例。
func NewSettings(store SettingsStore, identities IdentityStore) *Settings {
	return &Settings{store: store, identities: identities}
}

// GetPlatformSettings 返回平台默认配置快照（仅平台管理员）。
func (settings *Settings) GetPlatformSettings(ctx context.Context, actor Principal) (PlatformSettingsValue, error) {
	if !isPlatformAdministrator(actor) {
		return PlatformSettingsValue{}, ErrNotFound
	}
	return settings.store.GetPlatformSettings(ctx)
}

// UpdatePlatformSettings 在 If-Match 下对平台默认配置应用顶层浅合并补丁。
func (settings *Settings) UpdatePlatformSettings(ctx context.Context, actor Principal, etag string, patch jsontext.Value) (PlatformSettingsValue, error) {
	if !isPlatformAdministrator(actor) {
		return PlatformSettingsValue{}, ErrNotFound
	}
	current, err := settings.store.GetPlatformSettings(ctx)
	if err != nil {
		return PlatformSettingsValue{}, err
	}
	expectedRevision, err := parseSettingsETag(etag, "platform-settings")
	if err != nil {
		return PlatformSettingsValue{}, ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return PlatformSettingsValue{}, ErrPrecondition
	}
	merged, err := mergeSettingsPatch(current.Settings, patch)
	if err != nil {
		return PlatformSettingsValue{}, err
	}
	return settings.store.UpdatePlatformSettings(ctx, expectedRevision, merged)
}

// GetTenantSettings 返回租户运行设置快照。
func (settings *Settings) GetTenantSettings(ctx context.Context, actor Principal, tenantSlug string) (TenantSettingsValue, error) {
	membership, err := settings.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsManage)
	if err != nil {
		return TenantSettingsValue{}, err
	}
	return settings.store.GetTenantSettings(ctx, membership.TenantID)
}

// UpdateTenantSettings 在 If-Match 下对租户设置应用顶层浅合并补丁。
func (settings *Settings) UpdateTenantSettings(ctx context.Context, actor Principal, tenantSlug, etag string, patch jsontext.Value) (TenantSettingsValue, error) {
	membership, err := settings.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsManage)
	if err != nil {
		return TenantSettingsValue{}, err
	}
	current, err := settings.store.GetTenantSettings(ctx, membership.TenantID)
	if err != nil {
		return TenantSettingsValue{}, err
	}
	expectedRevision, err := parseSettingsETag(etag, "tenant-settings")
	if err != nil {
		return TenantSettingsValue{}, ErrPrecondition
	}
	if expectedRevision != current.Revision {
		return TenantSettingsValue{}, ErrPrecondition
	}
	merged, err := mergeSettingsPatch(current.Settings, patch)
	if err != nil {
		return TenantSettingsValue{}, err
	}
	return settings.store.UpdateTenantSettings(ctx, membership.TenantID, expectedRevision, merged)
}

// mergeSettingsPatch 将 patch 的顶层字段浅合并进当前 settings 对象。
// patch 仅包含调用方提供的字段（序列化时 omitempty 已省略 nil），嵌套对象按整值替换。
func mergeSettingsPatch(current, patch jsontext.Value) (jsontext.Value, error) {
	var currentMap map[string]jsontext.Value
	if err := json.Unmarshal(current, &currentMap); err != nil {
		return nil, fmt.Errorf("decode current settings: %w", err)
	}
	var patchMap map[string]jsontext.Value
	if err := json.Unmarshal(patch, &patchMap); err != nil {
		return nil, fmt.Errorf("decode settings patch: %w", err)
	}
	for key, value := range patchMap {
		currentMap[key] = value
	}
	return json.Marshal(currentMap)
}

// parseSettingsETag 解析单例/文本 id 实体的 ETag 版本号。
// 与 parseRevisionETag 不同，设置实体的 ETag id 是文本（如 default）或无需按 uuid 校验，
// 只需校验 kind 前缀、`id:revision` 结构与正数版本号。
func parseSettingsETag(value, kind string) (int64, error) {
	prefix := fmt.Sprintf(`"%s:`, kind)
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, `"`) {
		return 0, ErrParseEntityTag
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(value, prefix), `"`)
	separator := strings.LastIndex(rest, ":")
	if separator < 0 || separator == len(rest)-1 {
		return 0, ErrParseEntityTag
	}
	var revision int64
	if _, err := fmt.Sscan(rest[separator+1:], &revision); err != nil || revision < 1 {
		return 0, ErrParseEntityTagRevision
	}
	return revision, nil
}

func (settings *Settings) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	return resolveTenantMembership(ctx, actor, tenantSlug, permission, settings.identities)
}
