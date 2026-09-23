package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"uuid"
)

// AssetKindOption 是资产类别在租户视角的注册投影。
type AssetKindOption struct {
	// ID 是类别标识。
	ID string
	// ContractVersion 是类别契约版本。
	ContractVersion string
	// PluginVersion 是类别插件版本。
	PluginVersion string
	// Enabled 是租户视角的启用状态（覆盖优先，缺省回退平台默认）。
	Enabled bool
	// Revision 是租户覆盖的并发版本号（未覆盖时为 1）。
	Revision int64
}

// AssetKindOverride 是一条租户级资产类别开关覆盖。
type AssetKindOverride struct {
	// TenantID 是所属租户的标识。
	TenantID uuid.UUID
	// KindID 是被覆盖的资产类别标识。
	KindID string
	// Enabled 是租户是否启用该类别。
	Enabled bool
	// Revision 是乐观并发版本号。
	Revision int64
	// CreatedAt 是创建时间。
	CreatedAt time.Time
	// UpdatedAt 是最近更新时间。
	UpdatedAt time.Time
}

// KindStore 是资产类别注册与租户覆盖的持久化边界。
type KindStore interface {
	// GetAssetKind 承载 KindStore 的生成 GetAssetKind 值。
	GetAssetKind(context.Context, string) (AssetKindRecord, error)
	// ListAssetKindOverrides 承载 KindStore 的生成 ListAssetKindOverrides 值。
	ListAssetKindOverrides(context.Context, uuid.UUID) ([]AssetKindOption, error)
	// UpsertAssetKindOverride 承载 KindStore 的生成 UpsertAssetKindOverride 值。
	UpsertAssetKindOverride(context.Context, uuid.UUID, string, bool) (AssetKindOverride, error)
}

// AssetKinds 协调资产类别启停与列示用例。
type AssetKinds struct {
	store      KindStore
	identities IdentityStore
}

// NewAssetKinds 构造 M0 资产类别用例。
func NewAssetKinds(store KindStore, identities IdentityStore) *AssetKinds {
	return &AssetKinds{store: store, identities: identities}
}

// List 返回租户视角的全部资产类别。
func (kinds *AssetKinds) List(ctx context.Context, actor Principal, tenantSlug string) ([]AssetKindOption, error) {
	membership, err := kinds.tenantMembership(ctx, actor, tenantSlug, scopeAssetRead)
	if err != nil {
		return nil, err
	}
	return kinds.store.ListAssetKindOverrides(ctx, membership.TenantID)
}

// UpdateState 在 If-Match 下更新一个租户级资产类别开关。
func (kinds *AssetKinds) UpdateState(ctx context.Context, actor Principal, tenantSlug, kindID, etag string, enabled bool) (AssetKindOption, error) {
	membership, err := kinds.tenantMembership(ctx, actor, tenantSlug, scopeAssetKindManage)
	if err != nil {
		return AssetKindOption{}, err
	}
	if _, err := kinds.store.GetAssetKind(ctx, kindID); err != nil {
		return AssetKindOption{}, err
	}
	current, err := kinds.store.ListAssetKindOverrides(ctx, membership.TenantID)
	if err != nil {
		return AssetKindOption{}, err
	}
	var target *AssetKindOption
	for index := range current {
		if current[index].ID == kindID {
			target = &current[index]
			break
		}
	}
	if target == nil {
		return AssetKindOption{}, ErrNotFound
	}
	expectedRevision, err := parseTextEntityTag(etag, "asset-kind", kindID)
	if err != nil {
		return AssetKindOption{}, ErrPrecondition
	}
	if expectedRevision != target.Revision {
		return AssetKindOption{}, ErrPrecondition
	}
	override, err := kinds.store.UpsertAssetKindOverride(ctx, membership.TenantID, kindID, enabled)
	if err != nil {
		return AssetKindOption{}, err
	}
	return AssetKindOption{
		ID: override.KindID, Enabled: override.Enabled, Revision: override.Revision,
		ContractVersion: target.ContractVersion, PluginVersion: target.PluginVersion,
	}, nil
}

// parseTextEntityTag 解析 id 为任意文本的 ETag 版本号（如 asset-kind 的类别 id）。
func parseTextEntityTag(value, kind, id string) (int64, error) {
	prefix := fmt.Sprintf(`"%s:%s:`, kind, id)
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, `"`) {
		return 0, ErrParseEntityTag
	}
	var revision int64
	if _, err := fmt.Sscan(strings.TrimSuffix(strings.TrimPrefix(value, prefix), `"`), &revision); err != nil || revision < 1 {
		return 0, ErrParseEntityTagRevision
	}
	return revision, nil
}

func (kinds *AssetKinds) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	return resolveTenantMembership(ctx, actor, tenantSlug, permission, kinds.identities)
}
