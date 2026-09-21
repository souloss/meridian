package handler

import (
	"context"
	"strconv"
	"uuid"

	"github.com/meridian-labs/meridian/internal/generated/api"
	collaboration "github.com/meridian-labs/meridian/internal/generated/api/collaboration"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/oapi-codegen/nullable"
)

// etagKindNotificationChannel 是通知通道 ETag 的实体类型令牌。
const etagKindNotificationChannel = "notification-channel"

// ListNotificationChannels 返回租户内全部通知通道（不含秘密）。
func (s *Server) ListNotificationChannels(ctx context.Context, request collaboration.ListNotificationChannelsRequestObject) (collaboration.ListNotificationChannelsResponseObject, error) {
	if s.notifications == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.notifications.ListNotificationChannels(ctx, principal, string(request.TenantSlug))
	if err != nil {
		return nil, err
	}
	items := make([]api.NotificationChannel, 0, len(records))
	for _, record := range records {
		items = append(items, notificationChannelResponse(record))
	}
	return collaboration.ListNotificationChannels200JSONResponse(api.NotificationChannelList{Items: items}), nil
}

// CreateNotificationChannel 校验并加密后创建一条通知通道。
func (s *Server) CreateNotificationChannel(ctx context.Context, request collaboration.CreateNotificationChannelRequestObject) (collaboration.CreateNotificationChannelResponseObject, error) {
	if s.notifications == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	record, err := s.notifications.CreateNotificationChannel(ctx, principal, string(request.TenantSlug), service.NewNotificationChannel{
		ID:       uuid.NewV7(),
		Kind:     string(request.Body.Kind),
		Name:     request.Body.Name,
		Endpoint: optionalString(request.Body.Endpoint),
		Secret:   optionalStringValue(request.Body.Secret),
		Enabled:  request.Body.Enabled,
	})
	if err != nil {
		return nil, err
	}
	body := notificationChannelResponse(record)
	etag := api.ETag(revisionETag(etagKindNotificationChannel, record.ID.String(), record.Revision))
	return collaboration.CreateNotificationChannel201JSONResponse{
		Body:    body,
		Headers: collaboration.CreateNotificationChannel201ResponseHeaders{Etag: &etag},
	}, nil
}

// DeleteNotificationChannel 在 If-Match 乐观并发下删除一条通知通道。
func (s *Server) DeleteNotificationChannel(ctx context.Context, request collaboration.DeleteNotificationChannelRequestObject) (collaboration.DeleteNotificationChannelResponseObject, error) {
	if s.notifications == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	expectedRevision, err := parseChannelETag(request.Params.IfMatch, serviceUUID(request.ChannelId))
	if err != nil {
		return nil, err
	}
	if err := s.notifications.DeleteNotificationChannel(ctx, principal, string(request.TenantSlug), serviceUUID(request.ChannelId), expectedRevision); err != nil {
		return nil, err
	}
	return collaboration.DeleteNotificationChannel204Response{}, nil
}

// UpdateNotificationChannel 应用一个通道补丁并递增 revision。
func (s *Server) UpdateNotificationChannel(ctx context.Context, request collaboration.UpdateNotificationChannelRequestObject) (collaboration.UpdateNotificationChannelResponseObject, error) {
	if s.notifications == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	expectedRevision, err := parseChannelETag(request.Params.IfMatch, serviceUUID(request.ChannelId))
	if err != nil {
		return nil, err
	}
	var name *string
	if request.Body.Name != nil {
		name = request.Body.Name
	}
	record, err := s.notifications.UpdateNotificationChannel(ctx, principal, string(request.TenantSlug), service.NotificationChannelPatch{
		ID: serviceUUID(request.ChannelId), ExpectedRevision: expectedRevision, Name: name, Enabled: request.Body.Enabled,
	})
	if err != nil {
		return nil, err
	}
	body := notificationChannelResponse(record)
	etag := api.ETag(revisionETag(etagKindNotificationChannel, record.ID.String(), record.Revision))
	return collaboration.UpdateNotificationChannel200JSONResponse{
		Body:    body,
		Headers: collaboration.UpdateNotificationChannel200ResponseHeaders{Etag: &etag},
	}, nil
}

// RotateNotificationChannelSecret 轮换 Webhook 通道秘密。
func (s *Server) RotateNotificationChannelSecret(ctx context.Context, request collaboration.RotateNotificationChannelSecretRequestObject) (collaboration.RotateNotificationChannelSecretResponseObject, error) {
	if s.notifications == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	expectedRevision, err := parseChannelETag(request.Params.IfMatch, serviceUUID(request.ChannelId))
	if err != nil {
		return nil, err
	}
	record, err := s.notifications.RotateNotificationChannelSecret(ctx, principal, string(request.TenantSlug), serviceUUID(request.ChannelId), expectedRevision, request.Body.Secret)
	if err != nil {
		return nil, err
	}
	body := notificationChannelResponse(record)
	etag := api.ETag(revisionETag(etagKindNotificationChannel, record.ID.String(), record.Revision))
	return collaboration.RotateNotificationChannelSecret200JSONResponse{
		Body:    body,
		Headers: collaboration.RotateNotificationChannelSecret200ResponseHeaders{Etag: &etag},
	}, nil
}

// TestNotificationChannel 投递一条测试事件并返回已受理任务。
func (s *Server) TestNotificationChannel(ctx context.Context, request collaboration.TestNotificationChannelRequestObject) (collaboration.TestNotificationChannelResponseObject, error) {
	if s.notifications == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	accepted, err := s.notifications.TestNotificationChannel(ctx, principal, string(request.TenantSlug), serviceUUID(request.ChannelId))
	if err != nil {
		return nil, err
	}
	return collaboration.TestNotificationChannel202JSONResponse(jobAcceptedResponse(accepted)), nil
}

// ListNotifications 分页返回认证用户的站内通知。
func (s *Server) ListNotifications(ctx context.Context, request collaboration.ListNotificationsRequestObject) (collaboration.ListNotificationsResponseObject, error) {
	if s.notifications == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	page, pageSize := pagination(request.Params.Page, request.Params.PageSize)
	unreadOnly := request.Params.Unread != nil && *request.Params.Unread
	pageRecord, err := s.notifications.ListNotifications(ctx, principal, string(request.TenantSlug), unreadOnly, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]api.Notification, 0, len(pageRecord.Items))
	for _, record := range pageRecord.Items {
		items = append(items, notificationResponse(record))
	}
	return collaboration.ListNotifications200JSONResponse(api.NotificationPage{
		Total: pageRecord.Total, Page: pageRecord.Page, PageSize: pageRecord.PageSize,
		UnreadCount: pageRecord.UnreadCount, Items: items,
	}), nil
}

// MarkNotificationRead 将一条通知标记为已读。
func (s *Server) MarkNotificationRead(ctx context.Context, request collaboration.MarkNotificationReadRequestObject) (collaboration.MarkNotificationReadResponseObject, error) {
	if s.notifications == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.notifications.MarkNotificationRead(ctx, principal, string(request.TenantSlug), serviceUUID(request.NotificationId)); err != nil {
		return nil, err
	}
	return collaboration.MarkNotificationRead204Response{}, nil
}

// MarkAllNotificationsRead 将认证用户全部未读通知标记为已读。
func (s *Server) MarkAllNotificationsRead(ctx context.Context, request collaboration.MarkAllNotificationsReadRequestObject) (collaboration.MarkAllNotificationsReadResponseObject, error) {
	if s.notifications == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.notifications.MarkAllNotificationsRead(ctx, principal, string(request.TenantSlug)); err != nil {
		return nil, err
	}
	return collaboration.MarkAllNotificationsRead204Response{}, nil
}

// ListSubscriptions 返回认证用户的全部订阅。
func (s *Server) ListSubscriptions(ctx context.Context, request collaboration.ListSubscriptionsRequestObject) (collaboration.ListSubscriptionsResponseObject, error) {
	if s.notifications == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.notifications.ListSubscriptions(ctx, principal, string(request.TenantSlug))
	if err != nil {
		return nil, err
	}
	items := make([]api.Subscription, 0, len(records))
	for _, record := range records {
		items = append(items, subscriptionResponse(record))
	}
	return collaboration.ListSubscriptions200JSONResponse(api.SubscriptionList{Items: items}), nil
}

// PutSubscription 创建或替换一条订阅。
func (s *Server) PutSubscription(ctx context.Context, request collaboration.PutSubscriptionRequestObject) (collaboration.PutSubscriptionResponseObject, error) {
	if s.notifications == nil || request.Body == nil {
		return nil, api.ErrStrictOperationNotImplemented
	}
	principal, err := principalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	scopeType := string(request.Body.ScopeType)
	var scopeID *string
	if request.Body.ScopeId.IsSpecified() && !request.Body.ScopeId.IsNull() {
		value := request.Body.ScopeId.MustGet()
		switch request.Body.ScopeType {
		case api.SubscriptionScopeTypeAssetKind:
			id, kindErr := value.AsKindId()
			if kindErr != nil {
				return nil, service.ErrValidation
			}
			text := string(id)
			scopeID = &text
		default:
			id, uuidErr := value.AsUuid()
			if uuidErr != nil {
				return nil, service.ErrValidation
			}
			text := serviceUUID(id).String()
			scopeID = &text
		}
	}
	eventTypes := make([]string, 0, len(request.Body.EventTypes))
	for _, eventType := range request.Body.EventTypes {
		eventTypes = append(eventTypes, string(eventType))
	}
	channelIDs := make([]uuid.UUID, 0, len(request.Body.ChannelIds))
	for _, channelID := range request.Body.ChannelIds {
		channelIDs = append(channelIDs, serviceUUID(channelID))
	}
	record, err := s.notifications.PutSubscription(ctx, principal, string(request.TenantSlug), service.PutSubscriptionInput{
		ScopeType: scopeType, ScopeID: scopeID, EventTypes: eventTypes, ChannelIDs: channelIDs, Enabled: request.Body.Enabled,
	})
	if err != nil {
		return nil, err
	}
	return collaboration.PutSubscription200JSONResponse(subscriptionResponse(record)), nil
}

// notificationChannelResponse 将通道记录投影为 API 形状（不含秘密）。
func notificationChannelResponse(record service.NotificationChannelRecord) api.NotificationChannel {
	endpoint := nullable.NewNullNullable[string]()
	if record.Endpoint != nil {
		endpoint = nullable.NewNullableWithValue(*record.Endpoint)
	}
	return api.NotificationChannel{
		Id:               api.Uuid(record.ID),
		Etag:             api.ETag(revisionETag(etagKindNotificationChannel, record.ID.String(), record.Revision)),
		Name:             record.Name,
		Kind:             api.NotificationChannelKind(record.Kind),
		Enabled:          record.Enabled,
		Endpoint:         endpoint,
		SecretConfigured: record.SecretConfigured,
		CreatedAt:        api.Timestamp(record.CreatedAt),
		UpdatedAt:        api.Timestamp(record.UpdatedAt),
	}
}

// notificationResponse 将通知记录投影为 API 形状并渲染本地化标题/正文。
func notificationResponse(record service.NotificationRecord) api.Notification {
	readAt := nullable.NewNullNullable[api.Timestamp]()
	if record.ReadAt != nil {
		readAt = nullable.NewNullableWithValue(api.Timestamp(*record.ReadAt))
	}
	resourceURL := nullable.NewNullNullable[string]()
	if record.ResourceURL != nil {
		resourceURL = nullable.NewNullableWithValue(*record.ResourceURL)
	}
	return api.Notification{
		Id:          api.Uuid(record.ID),
		EventType:   api.DomainEventType(record.EventType),
		Title:       record.TitleKey,
		Body:        string(record.BodyArgs),
		ResourceUrl: resourceURL,
		ReadAt:      readAt,
		CreatedAt:   api.Timestamp(record.CreatedAt),
	}
}

// subscriptionResponse 将订阅记录投影为 API 形状。
func subscriptionResponse(record service.SubscriptionRecord) api.Subscription {
	eventTypes := make([]api.DomainEventType, 0, len(record.EventTypes))
	for _, eventType := range record.EventTypes {
		eventTypes = append(eventTypes, api.DomainEventType(eventType))
	}
	channelIDs := make([]api.Uuid, 0, len(record.ChannelIDs))
	for _, channelID := range record.ChannelIDs {
		channelIDs = append(channelIDs, api.Uuid(channelID))
	}
	var scopeID nullable.Nullable[api.Subscription_ScopeId]
	if record.ScopeID != nil {
		scopeID = nullable.NewNullNullable[api.Subscription_ScopeId]()
		union := scopeID.MustGet()
		switch api.SubscriptionScopeType(record.ScopeType) {
		case api.SubscriptionScopeTypeAssetKind:
			_ = union.FromKindId(api.KindId(*record.ScopeID))
		default:
			_ = union.FromUuid(api.Uuid(serviceUUIDValue(*record.ScopeID)))
		}
		scopeID = nullable.NewNullableWithValue(union)
	}
	return api.Subscription{
		Id:         api.Uuid(record.ID),
		EventTypes: eventTypes,
		ScopeType:  api.SubscriptionScopeType(record.ScopeType),
		ScopeId:    scopeID,
		ChannelIds: channelIDs,
		Enabled:    record.Enabled,
		CreatedAt:  api.Timestamp(record.CreatedAt),
		UpdatedAt:  api.Timestamp(record.UpdatedAt),
	}
}

// parseChannelETag 校验通道变更请求携带的 If-Match 令牌并返回 revision。
func parseChannelETag(etag string, channelID uuid.UUID) (int64, error) {
	prefix := `"notification-channel:` + channelID.String() + `:`
	if len(etag) <= len(prefix) || etag[:len(prefix)] != prefix || etag[len(etag)-1] != '"' {
		return 0, service.ErrPrecondition
	}
	revisionText := etag[len(prefix) : len(etag)-1]
	revision, err := strconv.ParseInt(revisionText, 10, 64)
	if err != nil || revision < 1 {
		return 0, service.ErrPrecondition
	}
	return revision, nil
}

func optionalStringValue(value nullable.Nullable[string]) string {
	if !value.IsSpecified() || value.IsNull() {
		return ""
	}
	return value.MustGet()
}

func serviceUUIDValue(text string) uuid.UUID {
	parsed, err := uuid.Parse(text)
	if err != nil {
		return uuid.Nil()
	}
	return parsed
}
