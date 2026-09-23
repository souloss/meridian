package service

// 本文件集中放置身份/授权域的领域常量：用户与租户状态、租户角色、权限点。
// 值来源：contracts/domain.yaml 的 membershipRole 段与 roleAllows 授权矩阵。

// 用户与租户状态枚举（userStatus / tenantStatus）。
const (
	// identityStatusActive 表示用户/租户处于可用状态。
	identityStatusActive = "active"
	// identityStatusDisabled 表示租户被禁用。
	identityStatusDisabled = "disabled"
)

// 租户成员角色（membershipRole 枚举）。
const (
	// tenantRoleAdmin 是租户管理员角色，拥有全部权限。
	tenantRoleAdmin = "tenant_admin"
	// tenantRoleMaintainer 是维护者角色，拥有读写权限。
	tenantRoleMaintainer = "maintainer"
	// tenantRoleViewer 是只读观察者角色。
	tenantRoleViewer = "viewer"
)

// 授权权限点（authorization scope，PAT scope 与 JWT 角色矩阵共用）。
const (
	// scopeWildcard 是通配全部权限点的 PAT 作用域。
	scopeWildcard = "*"
	// scopeAssetRead 是读取资产的权限点。
	scopeAssetRead = "asset:read"
	// scopeAssetPush 是推送第三方资产修订的权限点。
	scopeAssetPush = "asset:push"
	// scopeAssetPublish 是发布资产版本的权限点。
	scopeAssetPublish = "asset:publish"
	// scopeLayerRead 是读取层的权限点。
	scopeLayerRead = "layer:read"
	// scopeLayerEdit 是编辑层的权限点。
	scopeLayerEdit = "layer:edit"
	// scopeLayerApprove 是审批层候选修订的权限点。
	scopeLayerApprove = "layer:approve"
	// scopeServiceRead 是读取服务的权限点。
	scopeServiceRead = "service:read"
	// scopeServiceWrite 是写服务的权限点。
	scopeServiceWrite = "service:write"
	// scopeServiceCreate 是创建服务的权限点。
	scopeServiceCreate = "service:create"
	// scopeRepositoryRead 是读取仓库的权限点。
	scopeRepositoryRead = "repository:read"
	// scopeRepositoryWrite 是写仓库的权限点。
	scopeRepositoryWrite = "repository:write"
	// scopeRepositorySync 是同步仓库的权限点。
	scopeRepositorySync = "repository:sync"
	// scopeJobRead 是读取任务的权限点。
	scopeJobRead = "job:read"
	// scopeJobRun 是运行/取消/重试任务的权限点。
	scopeJobRun = "job:run"
	// scopeTokenManage 是管理个人访问令牌的权限点。
	scopeTokenManage = "token:manage"
	// scopeCredentialRead 是读取凭据的权限点。
	scopeCredentialRead = "credential:read"
	// scopeCredentialManage 是管理凭据的权限点。
	scopeCredentialManage = "credential:manage"
	// scopeTodoReadSelf 是查看本人破坏性待办的权限点。
	scopeTodoReadSelf = "todo:read_self"
	// scopeTenantAuditRead 是读取租户审计的权限点。
	scopeTenantAuditRead = "tenant:audit:read"
	// scopeTenantMemberRead 是读取团队目录的权限点。
	scopeTenantMemberRead = "tenant:member:read"
	// scopeTenantMemberWrite 是写入团队的权限点。
	scopeTenantMemberWrite = "tenant:member:write"
	// scopeTenantMemberManage 是管理租户成员关系的权限点。
	scopeTenantMemberManage = "tenant:member:manage"
	// scopeTenantSettingsManage 是读取与写入租户设置的权限点。
	scopeTenantSettingsManage = "tenant:settings:manage"
	// scopePlatformUserManage 是平台侧更新用户身份的权限点。
	scopePlatformUserManage = "platform:user:manage"
	// scopePlatformSettingsManage 是平台侧读取与写入平台设置的权限点。
	scopePlatformSettingsManage = "platform:settings:manage"
	// scopeAssetKindManage 是租户级启停资产类别的权限点。
	scopeAssetKindManage = "assetkind:manage"
)
