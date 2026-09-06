import "@typespec/http";
import "@typespec/openapi3";
import "../../common/models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Returns the requested page of notification channels within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:read"])
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/notification-channels")
@get
op listNotificationChannels(...Parameters.TenantSlug):
  | NotificationChannelList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates notification channel within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/notification-channels")
@post
op createNotificationChannel(
  ...Parameters.TenantSlug,
  @body body: NotificationChannelCreateRequest,
):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: NotificationChannel;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 422;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Deletes the selected notification channel within the authorized request scope. */
@tag("Collaboration")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/notification-channels/{channelId}")
@delete
op deleteNotificationChannel(
  ...Parameters.TenantSlug,
  ...Parameters.ChannelId,
  ...Parameters.IfMatch,
):
  | NoContentResponse
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 412;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected notification channel within the authorized request scope. */
@tag("Collaboration")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/notification-channels/{channelId}")
@patch
op updateNotificationChannel(
  ...Parameters.TenantSlug,
  ...Parameters.ChannelId,
  ...Parameters.IfMatch,
  @body body: NotificationChannelPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: NotificationChannel;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 412;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Tests notification channel within the authorized request scope. */
@tag("Collaboration")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/notification-channels/{channelId}:test")
@post
op testNotificationChannel(...Parameters.TenantSlug, ...Parameters.ChannelId):
  | (AcceptedResponse & JobAccepted)
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the rotate notification channel secret workflow within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/notification-channels/{channelId}:rotate")
@post
op rotateNotificationChannelSecret(
  ...Parameters.TenantSlug,
  ...Parameters.ChannelId,
  ...Parameters.IfMatch,
  ...Parameters.IdempotencyKey,
  @body body: NotificationChannelRotateRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: NotificationChannel;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 409;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 412;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 422;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of service comments within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}/comments")
@get
op listServiceComments(
  ...Parameters.TenantSlug,
  ...Parameters.ServiceSlug,
  ...Parameters.Page,
  ...Parameters.PageSize,
):
  | CommentPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates service comment within the authorized request scope. */
@tag("Collaboration")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}/comments")
@post
op createServiceComment(
  ...Parameters.TenantSlug,
  ...Parameters.ServiceSlug,
  @body body: CommentCreateRequest,
):
  | (CreatedResponse & Comment)
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of subscriptions within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["subscription:manage_self"])
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/subscriptions")
@get
op listSubscriptions(...Parameters.TenantSlug):
  | SubscriptionList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates or replaces the selected subscription within the authorized request scope. */
@tag("Collaboration")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["subscription:manage_self"])
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/subscriptions")
@put
op putSubscription(...Parameters.TenantSlug, @body body: SubscriptionPutRequest):
  | Subscription
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of notifications within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/notifications")
@get
op listNotifications(
  ...Parameters.TenantSlug,
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.UnreadFilter,
):
  | NotificationPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the mark notification read workflow within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/notifications/{notificationId}:read")
@post
op markNotificationRead(...Parameters.TenantSlug, ...Parameters.NotificationId):
  | NoContentResponse
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the mark all notifications read workflow within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/notifications:read-all")
@post
op markAllNotificationsRead(...Parameters.TenantSlug):
  | NoContentResponse
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of breaking todos within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["todo:read_self"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/breaking-todos")
@get
op listBreakingTodos(
  ...Parameters.TenantSlug,
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.TodoStatusFilter,
):
  | BreakingTodoPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the acknowledge breaking todo workflow within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["todo:read_self"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/breaking-todos/{todoId}:ack")
@post
op acknowledgeBreakingTodo(
  ...Parameters.TenantSlug,
  ...Parameters.TodoId,
  @body body: BreakingTodoAcknowledgeRequest,
):
  | BreakingTodo
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 409;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of audit logs within the authorized request scope. */
@tag("Collaboration")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:audit:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/audit-logs")
@get
op listAuditLogs(
  ...Parameters.TenantSlug,
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.AuditFilters,
):
  | AuditLogPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };
