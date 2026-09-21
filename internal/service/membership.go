package service

import (
	"context"
	"slices"
)

// resolveTenantMembership 解析一次已认证主体的租户成员关系与权限。
// 各用例结构体原本各自实现同名 tenantMembership 方法，逻辑完全一致；
// 收敛为单一包级函数后按需要注入 identities 即可，避免 14 处复制漂移。
func resolveTenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string, identities IdentityStore) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) || (!slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, scopeWildcard)) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil {
		return Membership{}, err
	}
	if !roleAllows(membership.Role, permission) {
		return Membership{}, ErrNotFound
	}
	return membership, nil
}
