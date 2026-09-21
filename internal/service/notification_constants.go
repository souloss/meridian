package service

// 本文件集中放置 M5 通知与订阅域的领域常量。
// 值来源：contracts/openapi.yaml 的 NotificationChannelKind / SubscriptionScopeType
// 与 contracts/domain.yaml 的 notificationChannels / events.subscriptions 段。
// DB 列值常量见 migration 注释；技术维度常量不在此列。

// 通知通道类别（NotificationChannelKind 枚举）。
const (
	// notificationChannelKindInApp 是站内通知通道。
	notificationChannelKindInApp = "in_app"
	// NotificationChannelKindWebhook 是外发 Webhook 通道（导出供 repository 复用列值）。
	NotificationChannelKindWebhook = "webhook"
	// notificationChannelKindEmail 是邮件通道（部署管理 SMTP）。
	notificationChannelKindEmail = "email"
)

// 订阅作用域类别（SubscriptionScopeType 枚举）。
const (
	// subscriptionScopeTenant 是租户级订阅作用域。
	subscriptionScopeTenant = "tenant"
	// subscriptionScopeService 是服务级订阅作用域。
	subscriptionScopeService = "service"
	// subscriptionScopeAsset 是资产级订阅作用域。
	subscriptionScopeAsset = "asset"
	// subscriptionScopeSystemGroup 是系统分组级订阅作用域。
	subscriptionScopeSystemGroup = "system_group"
	// subscriptionScopeAssetKind 是资产类别级订阅作用域。
	subscriptionScopeAssetKind = "asset_kind"
)

// 通道配置校验边界（值来源 domain.yaml kindRules）。
const (
	// notificationNameMinRunes 是通道展示名的最小长度。
	notificationNameMinRunes = 1
	// notificationNameMaxRunes 是通道展示名的最大长度。
	notificationNameMaxRunes = 64
	// notificationWebhookSecretMinBytes 是 Webhook 通道秘密的最小字节长度。
	notificationWebhookSecretMinBytes = 32
)

// 领域事件类型枚举（contracts/events.yaml channels 注册的七类事件）。
const (
	// domainEventVersionPublished 是版本发布事件（version.published）。
	domainEventVersionPublished = "version.published"
	// domainEventVersionBreaking 是版本破坏性变更事件（version.breaking）。
	domainEventVersionBreaking = "version.breaking"
	// domainEventCollectFailed 是采集失败事件（collect.failed）。
	domainEventCollectFailed = "collect.failed"
	// DomainEventCollectFailed 是采集失败事件的导出别名（供 repository 复用列值）。
	DomainEventCollectFailed = domainEventCollectFailed
	// domainEventServiceDeprecated 是服务弃用事件（service.deprecated）。
	domainEventServiceDeprecated = "service.deprecated"
	// domainEventAiLayerGenerated 是 AI 层生成事件（ai_layer.generated）。
	domainEventAiLayerGenerated = "ai_layer.generated"
	// domainEventLayerApproved 是层审批通过事件（layer.approved）。
	domainEventLayerApproved = "layer.approved"
	// domainEventLayerRejected 是层审批拒绝事件（layer.rejected）。
	domainEventLayerRejected = "layer.rejected"
)

// 通知与订阅域授权权限点（值来源 domain.yaml permissionCatalog）。
const (
	// scopeSubscriptionManageSelf 是管理本人订阅的权限点。
	scopeSubscriptionManageSelf = "subscription:manage_self"
	// scopeTenantSettingsRead 是读取租户通知通道设置的权限点。
	scopeTenantSettingsRead = "tenant:settings:read"
	// scopeTenantSettingsWrite 是写租户通知通道设置的权限点。
	scopeTenantSettingsWrite = "tenant:settings:write"
)
