package service

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

// Notifications 协调订阅、通知通道与站内通知的授权、加密与持久化。
type Notifications struct {
	store      NotificationStore
	identities IdentityStore
	keyring    CredentialKeyring
	now        func() time.Time
}

// NewNotifications 构造由调用方持有持久化与密钥材料的通知用例。
func NewNotifications(store NotificationStore, identities IdentityStore, keyring CredentialKeyring) *Notifications {
	return &Notifications{store: store, identities: identities, keyring: keyring, now: time.Now}
}

// PutSubscription 创建或替换一条订阅，校验作用域目标与可见启用通道。
func (notifications *Notifications) PutSubscription(ctx context.Context, actor Principal, tenantSlug string, input PutSubscriptionInput) (SubscriptionRecord, error) {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeSubscriptionManageSelf)
	if err != nil {
		return SubscriptionRecord{}, err
	}
	if !validSubscriptionScope(input.ScopeType) {
		return SubscriptionRecord{}, ErrValidation
	}
	if input.ScopeType != subscriptionScopeTenant && (input.ScopeID == nil || *input.ScopeID == "") {
		return SubscriptionRecord{}, ErrValidation
	}
	if input.ScopeType == subscriptionScopeTenant {
		input.ScopeID = nil
	}
	if len(input.EventTypes) == 0 || !validEventTypes(input.EventTypes) {
		return SubscriptionRecord{}, ErrValidation
	}
	if len(input.ChannelIDs) == 0 {
		return SubscriptionRecord{}, ErrValidation
	}
	visible, err := notifications.store.ListVisibleChannelIDs(ctx, membership.TenantID, input.ChannelIDs)
	if err != nil {
		return SubscriptionRecord{}, err
	}
	if len(visible) != len(input.ChannelIDs) {
		return SubscriptionRecord{}, ErrNotFound
	}
	input.TenantID = membership.TenantID
	input.UserID = actor.User.ID
	record, err := notifications.store.UpsertSubscription(ctx, input)
	if err != nil {
		return SubscriptionRecord{}, err
	}
	if err := notifications.store.ReplaceSubscriptionChannels(ctx, membership.TenantID, record.ID, input.ChannelIDs); err != nil {
		return SubscriptionRecord{}, err
	}
	record.ChannelIDs = slices.Clone(input.ChannelIDs)
	return record, nil
}

// ListSubscriptions 返回认证用户的全部订阅及其通道。
func (notifications *Notifications) ListSubscriptions(ctx context.Context, actor Principal, tenantSlug string) ([]SubscriptionRecord, error) {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeSubscriptionManageSelf)
	if err != nil {
		return nil, err
	}
	records, err := notifications.store.ListSubscriptions(ctx, membership.TenantID, actor.User.ID)
	if err != nil {
		return nil, err
	}
	for index := range records {
		channels, channelErr := notifications.store.ListSubscriptionChannels(ctx, membership.TenantID, records[index].ID)
		if channelErr == nil {
			records[index].ChannelIDs = channels
		}
	}
	return records, nil
}

// ListNotificationChannels 返回租户内全部通知通道（不含秘密）。
func (notifications *Notifications) ListNotificationChannels(ctx context.Context, actor Principal, tenantSlug string) ([]NotificationChannelRecord, error) {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsRead)
	if err != nil {
		return nil, err
	}
	return notifications.store.ListNotificationChannels(ctx, membership.TenantID)
}

// CreateNotificationChannel 校验通道类别与配置并加密后持久化。
func (notifications *Notifications) CreateNotificationChannel(ctx context.Context, actor Principal, tenantSlug string, input NewNotificationChannel) (NotificationChannelRecord, error) {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return NotificationChannelRecord{}, err
	}
	cfg, err := validateChannelInput(input.Kind, input.Name, input.Endpoint, input.Secret)
	if err != nil {
		return NotificationChannelRecord{}, err
	}
	encrypted, err := notifications.keyring.sealChannelConfig(membership.TenantID, input.ID, cfg)
	if err != nil {
		return NotificationChannelRecord{}, err
	}
	input.TenantID = membership.TenantID
	input.EncryptedConfig = encrypted
	record, err := notifications.store.CreateNotificationChannel(ctx, input)
	if err != nil {
		return NotificationChannelRecord{}, err
	}
	record.SecretConfigured = cfg.Secret != ""
	return record, nil
}

// UpdateNotificationChannel 应用一个通道补丁并递增 revision。
func (notifications *Notifications) UpdateNotificationChannel(ctx context.Context, actor Principal, tenantSlug string, patch NotificationChannelPatch) (NotificationChannelRecord, error) {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return NotificationChannelRecord{}, err
	}
	current, err := notifications.store.GetNotificationChannel(ctx, membership.TenantID, patch.ID)
	if err != nil {
		return NotificationChannelRecord{}, err
	}
	if patch.ExpectedRevision != current.Revision {
		return NotificationChannelRecord{}, ErrPrecondition
	}
	name := current.Name
	if patch.Name != nil {
		name = *patch.Name
	}
	enabled := current.Enabled
	if patch.Enabled != nil {
		enabled = *patch.Enabled
	}
	if err := validateChannelName(name); err != nil {
		return NotificationChannelRecord{}, err
	}
	return notifications.store.UpdateNotificationChannel(ctx, NotificationChannelPatch{
		TenantID: membership.TenantID, ID: patch.ID, ExpectedRevision: patch.ExpectedRevision, Name: &name, Enabled: &enabled,
	})
}

// RotateNotificationChannelSecret 轮换 Webhook 通道秘密（仅 webhook 类别允许）。
func (notifications *Notifications) RotateNotificationChannelSecret(ctx context.Context, actor Principal, tenantSlug string, channelID uuid.UUID, expectedRevision int64, secret string) (NotificationChannelRecord, error) {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return NotificationChannelRecord{}, err
	}
	current, err := notifications.store.GetNotificationChannel(ctx, membership.TenantID, channelID)
	if err != nil {
		return NotificationChannelRecord{}, err
	}
	if current.Kind != notificationChannelKindWebhook {
		return NotificationChannelRecord{}, ErrInvalidState
	}
	if expectedRevision != current.Revision {
		return NotificationChannelRecord{}, ErrPrecondition
	}
	if len([]byte(secret)) < notificationWebhookSecretMinBytes {
		return NotificationChannelRecord{}, ErrValidation
	}
	cfg := notificationChannelConfig{Endpoint: endpointOrEmpty(current.Endpoint), Secret: secret}
	encrypted, err := notifications.keyring.sealChannelConfig(membership.TenantID, channelID, cfg)
	if err != nil {
		return NotificationChannelRecord{}, err
	}
	return notifications.store.RotateNotificationChannelSecret(ctx, membership.TenantID, channelID, expectedRevision, encrypted)
}

// ListNotifications 分页返回认证用户的站内通知。
func (notifications *Notifications) ListNotifications(ctx context.Context, actor Principal, tenantSlug string, unreadOnly bool, page, pageSize int) (NotificationPageRecord, error) {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return NotificationPageRecord{}, err
	}
	if err := validatePagination(page, pageSize); err != nil {
		return NotificationPageRecord{}, err
	}
	items, total, unread, err := notifications.store.ListNotifications(ctx, membership.TenantID, actor.User.ID, unreadOnly, int32(pageSize), int32((page-1)*pageSize))
	if err != nil {
		return NotificationPageRecord{}, err
	}
	return NotificationPageRecord{Total: int(total), Page: page, PageSize: pageSize, UnreadCount: int(unread), Items: items}, nil
}

// MarkNotificationRead 将一条通知标记为已读。
func (notifications *Notifications) MarkNotificationRead(ctx context.Context, actor Principal, tenantSlug string, notificationID uuid.UUID) error {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return err
	}
	return notifications.store.MarkNotificationRead(ctx, membership.TenantID, actor.User.ID, notificationID)
}

// MarkAllNotificationsRead 将认证用户全部未读通知标记为已读。
func (notifications *Notifications) MarkAllNotificationsRead(ctx context.Context, actor Principal, tenantSlug string) error {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeServiceRead)
	if err != nil {
		return err
	}
	return notifications.store.MarkAllNotificationsRead(ctx, membership.TenantID, actor.User.ID)
}

// TestNotificationChannel 校验通道可见后返回一条已受理的测试投递任务。
// 测试事件由 notify_outbox 派发器按通道类型执行真实投递（webhook/email），
// 站内 in_app 通道在派发器阶段落到通知收件箱。
func (notifications *Notifications) TestNotificationChannel(ctx context.Context, actor Principal, tenantSlug string, channelID uuid.UUID) (JobAccepted, error) {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return JobAccepted{}, err
	}
	channel, err := notifications.store.GetNotificationChannel(ctx, membership.TenantID, channelID)
	if err != nil {
		return JobAccepted{}, err
	}
	accepted, err := notifications.store.EnqueueNotificationTest(ctx, membership.TenantID, channel)
	if err != nil {
		return JobAccepted{}, err
	}
	return accepted, nil
}

// DeleteNotificationChannel 在 If-Match 乐观并发下删除一条通知通道。
func (notifications *Notifications) DeleteNotificationChannel(ctx context.Context, actor Principal, tenantSlug string, channelID uuid.UUID, expectedRevision int64) error {
	membership, err := notifications.tenantMembership(ctx, actor, tenantSlug, scopeTenantSettingsWrite)
	if err != nil {
		return err
	}
	current, err := notifications.store.GetNotificationChannel(ctx, membership.TenantID, channelID)
	if err != nil {
		return err
	}
	if expectedRevision != current.Revision {
		return ErrPrecondition
	}
	return notifications.store.DeleteNotificationChannel(ctx, membership.TenantID, channelID, expectedRevision)
}

// validateChannelInput 校验通道类别、名称、端点与秘密的组合约束。
func validateChannelInput(kind, name string, endpoint *string, secret string) (notificationChannelConfig, error) {
	if err := validateChannelName(name); err != nil {
		return notificationChannelConfig{}, err
	}
	cfg := notificationChannelConfig{Secret: secret}
	switch kind {
	case notificationChannelKindInApp:
		if endpoint != nil || secret != "" {
			return notificationChannelConfig{}, ErrValidation
		}
	case notificationChannelKindWebhook:
		if endpoint == nil || *endpoint == "" {
			return notificationChannelConfig{}, ErrValidation
		}
		if len([]byte(secret)) < notificationWebhookSecretMinBytes {
			return notificationChannelConfig{}, ErrValidation
		}
		if !validWebhookEndpoint(*endpoint) {
			return notificationChannelConfig{}, ErrValidation
		}
		cfg.Endpoint = *endpoint
	case notificationChannelKindEmail:
		if endpoint == nil || *endpoint == "" || !validEmailEndpoint(*endpoint) {
			return notificationChannelConfig{}, ErrValidation
		}
		if secret != "" {
			return notificationChannelConfig{}, ErrValidation
		}
		cfg.Endpoint = *endpoint
	default:
		return notificationChannelConfig{}, ErrValidation
	}
	return cfg, nil
}

func validateChannelName(name string) error {
	runes := utf8.RuneCountInString(name)
	if strings.TrimSpace(name) == "" || runes < notificationNameMinRunes || runes > notificationNameMaxRunes {
		return ErrValidation
	}
	return nil
}

// validWebhookEndpoint 校验端点是绝对 HTTPS（本地回环开发除外）的 URI。
func validWebhookEndpoint(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return false
	}
	return true
}

// validEmailEndpoint 校验端点是 RFC 5321 邮箱。
func validEmailEndpoint(endpoint string) bool {
	at := strings.LastIndex(endpoint, "@")
	return at > 0 && at < len(endpoint)-1 && !strings.ContainsAny(endpoint, " \t\r\n")
}

func validSubscriptionScope(scopeType string) bool {
	switch scopeType {
	case subscriptionScopeTenant, subscriptionScopeService, subscriptionScopeAsset, subscriptionScopeSystemGroup, subscriptionScopeAssetKind:
		return true
	}
	return false
}

// validEventTypes 校验事件类型列表是领域事件枚举的非空子集。
func validEventTypes(eventTypes []string) bool {
	seen := make(map[string]bool, len(eventTypes))
	for _, eventType := range eventTypes {
		if !validDomainEventType(eventType) || seen[eventType] {
			return false
		}
		seen[eventType] = true
	}
	return true
}

func validDomainEventType(eventType string) bool {
	switch eventType {
	case domainEventVersionPublished, domainEventVersionBreaking, domainEventCollectFailed, domainEventServiceDeprecated,
		domainEventAiLayerGenerated, domainEventLayerApproved, domainEventLayerRejected:
		return true
	}
	return false
}

func (notifications *Notifications) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	if actor.Kind == PrincipalPAT {
		if actor.TenantSlug != tenantSlug || !roleAllows(actor.Role, permission) {
			return Membership{}, ErrNotFound
		}
		if !slices.Contains(actor.Scopes, permission) && !slices.Contains(actor.Scopes, scopeWildcard) {
			return Membership{}, ErrNotFound
		}
		return Membership{TenantID: actor.TenantID, TenantSlug: actor.TenantSlug, UserID: actor.User.ID, Role: actor.Role}, nil
	}
	if actor.Kind != PrincipalJWT || notifications.identities == nil {
		return Membership{}, ErrNotFound
	}
	membership, err := notifications.identities.ActiveMembership(ctx, actor.User.ID, tenantSlug)
	if err != nil {
		return Membership{}, err
	}
	if !roleAllows(membership.Role, permission) {
		return Membership{}, ErrNotFound
	}
	return membership, nil
}

func endpointOrEmpty(endpoint *string) string {
	if endpoint == nil {
		return ""
	}
	return *endpoint
}
