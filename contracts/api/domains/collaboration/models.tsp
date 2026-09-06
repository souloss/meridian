/** Shared collaboration, notification, audit, and upload models. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** identifies one supported notification channel kind value. */
enum NotificationChannelKind {
  in_app,
  webhook,
  email,
}

/** represents notification channel data exchanged through the Meridian API. */
model NotificationChannel {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** specifies the name associated with this notification channel. */
  name: string;

  /** contains the notification channel kind associated with this notification channel. */
  kind: NotificationChannelKind;

  /** indicates whether enabled applies to this notification channel. */
  enabled: boolean;

  /** specifies the endpoint associated with this notification channel. */
  endpoint: string | null;

  /** indicates whether secret configured applies to this notification channel. */
  secretConfigured: boolean;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of notification channel values. */
model NotificationChannelList {
  /** contains the ordered items associated with this notification channel list. */
  items: NotificationChannel[];
}

/** defines validated input for the corresponding Meridian API operation. */
model NotificationChannelCreateRequest {
  /** specifies the name associated with this notification channel create request. */
  @minLength(1) @maxLength(64) name: string;

  /** contains the notification channel kind associated with this notification channel create request. */
  kind: NotificationChannelKind;

  /** indicates whether enabled applies to this notification channel create request. */
  enabled: boolean;

  /** specifies the endpoint associated with this notification channel create request. */
  @maxLength(2048) endpoint?: string | null;

  /** is write-only secret material and must never be returned by the API. */

  @minLength(32)
  @maxLength(4096)
  secret?: string | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model NotificationChannelPatchRequest {
  /** specifies the name associated with this notification channel patch request. */
  @minLength(1) @maxLength(64) name?: string;

  /** indicates whether enabled applies to this notification channel patch request. */
  enabled?: boolean;

  /** specifies the endpoint associated with this notification channel patch request. */
  @maxLength(2048) endpoint?: string | null;
}

/** defines validated input for the corresponding Meridian API operation. */
model NotificationChannelRotateRequest {
  /** is write-only secret material and must never be returned by the API. */

  @minLength(32)
  @maxLength(4096)
  secret: string;
}

/** represents comment data exchanged through the Meridian API. */
model Comment {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** identifies the service associated with this resource. */
  serviceId: Uuid;

  /** contains the user associated with this comment. */
  author: User;

  /** specifies the body associated with this comment. */
  body: string;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains one paginated page of comment records. */
model CommentPage {
  ...PageInfo;

  /** contains the ordered items associated with this comment page. */
  items: Comment[];
}

/** defines validated input for the corresponding Meridian API operation. */
model CommentCreateRequest {
  /** specifies the body associated with this comment create request. */
  @minLength(1) @maxLength(2000) body: string;
}

/** represents subscription scope constraints data exchanged through the Meridian API. */
model SubscriptionScopeConstraints {}

/** identifies one supported domain event type value. */
enum DomainEventType {
  `version.published`,
  `version.breaking`,
  `collect.failed`,
  `service.deprecated`,
  `ai_layer.generated`,
  `layer.approved`,
  `layer.rejected`,
}

/** identifies one supported subscription scope type value. */
enum SubscriptionScopeType {
  tenant,
  service,
  asset,
  system_group,
  asset_kind,
}

/** represents subscription data exchanged through the Meridian API. */
model Subscription {
  ...SubscriptionScopeConstraints;

  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the ordered event types associated with this subscription. */
  @minItems(1) eventTypes: DomainEventType[];

  /** contains the subscription scope type associated with this subscription. */
  scopeType: SubscriptionScopeType;

  /** specifies the scope id associated with this subscription. */
  @oneOf scopeId: Uuid | KindId | null;

  /** contains the ordered channel ids associated with this subscription. */
  @minItems(1) channelIds: Uuid[];

  /** indicates whether enabled applies to this subscription. */
  enabled: boolean;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** is the RFC 3339 UTC instant when this resource was last updated. */
  updatedAt: Timestamp;
}

/** contains an ordered collection of subscription values. */
model SubscriptionList {
  /** contains the ordered items associated with this subscription list. */
  items: Subscription[];
}

/** defines validated input for the corresponding Meridian API operation. */
model SubscriptionPutRequest {
  ...SubscriptionScopeConstraints;

  /** contains the ordered event types associated with this subscription put request. */
  @minItems(1) eventTypes: DomainEventType[];

  /** contains the subscription scope type associated with this subscription put request. */
  scopeType: SubscriptionScopeType;

  /** specifies the scope id associated with this subscription put request. */
  @oneOf scopeId: Uuid | KindId | null;

  /** contains the ordered channel ids associated with this subscription put request. */
  @minItems(1) channelIds: Uuid[];

  /** indicates whether enabled applies to this subscription put request. */
  enabled: boolean;
}

/** represents notification data exchanged through the Meridian API. */
model Notification {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the domain event type associated with this notification. */
  eventType: DomainEventType;

  /** specifies the title associated with this notification. */
  title: string;

  /** specifies the body associated with this notification. */
  body: string;

  /** specifies the resource url associated with this notification. */
  @format("uri-reference") resourceUrl: string | null;

  /** specifies the read at associated with this notification. */
  @oneOf readAt: Timestamp | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** contains one paginated page of notification records. */
model NotificationPage {
  ...PageInfo;

  /** contains the ordered items associated with this notification page. */
  items: Notification[];

  /** specifies the unread count associated with this notification page. */
  @minValue(0) unreadCount: integer;
}

/** identifies one supported todo status value. */
enum TodoStatus {
  open,
  acked,
}

/** represents breaking todo data exchanged through the Meridian API. */
model BreakingTodo {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** contains the uuid associated with this breaking todo. */
  assetVersionId: Uuid;

  /** contains the uuid associated with this breaking todo. */
  assigneeId: Uuid;

  /** is the current lifecycle or processing state. */
  status: TodoStatus;

  /** contains the diff counts associated with this breaking todo. */
  summary: DiffCounts;

  /** specifies the comment associated with this breaking todo. */
  comment: string | null;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;

  /** specifies the acknowledged at associated with this breaking todo. */
  @oneOf acknowledgedAt: Timestamp | null;
}

/** contains one paginated page of breaking todo records. */
model BreakingTodoPage {
  ...PageInfo;

  /** contains the ordered items associated with this breaking todo page. */
  items: BreakingTodo[];
}

/** defines validated input for the corresponding Meridian API operation. */
model BreakingTodoAcknowledgeRequest {
  /** specifies the comment associated with this breaking todo acknowledge request. */
  @maxLength(2000) comment?: string | null;
}

/** defines supported filters for selecting audit records. */
model AuditFilter {
  /** contains the uuid associated with this audit filter. */
  actorId?: Uuid;

  /** contains the ordered actions associated with this audit filter. */
  actions?: string[];

  /** specifies the resource type associated with this audit filter. */
  resourceType?: string;

  /** specifies the resource id associated with this audit filter. */
  resourceId?: string;

  /** contains the timestamp associated with this audit filter. */
  from?: Timestamp;

  /** contains the timestamp associated with this audit filter. */
  to?: Timestamp;
}

/** represents audit entry data exchanged through the Meridian API. */
model AuditEntry {
  /** uniquely identifies this resource. */
  id: Uuid;

  /** specifies the tenant slug associated with this audit entry. */
  @oneOf tenantSlug: Slug | null;

  /** specifies the actor id associated with this audit entry. */
  @oneOf actorId: Uuid | null;

  /** specifies the action associated with this audit entry. */
  action: string;

  /** specifies the resource type associated with this audit entry. */
  resourceType: string;

  /** specifies the resource id associated with this audit entry. */
  resourceId: string | null;

  /** correlates this result with server logs and audit records. */
  requestId: string | null;

  /** specifies the metadata associated with this audit entry. */
  metadata: Record<unknown>;

  /** is the RFC 3339 UTC instant when this resource was created. */
  createdAt: Timestamp;
}

/** contains one paginated page of audit log records. */
model AuditLogPage {
  ...PageInfo;

  /** contains the ordered items associated with this audit log page. */
  items: AuditEntry[];
}

/** defines validated input for the corresponding Meridian API operation. */
