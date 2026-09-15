import "@typespec/http";
import "@typespec/openapi3";
import "../../models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Returns the requested page of view overrides within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:read"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/view-overrides")
@get
op listViewOverrides(...Parameters.TenantSlug):
  | ViewOverrideList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Deletes the selected view override within the authorized request scope. */
@tag("View")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/view-overrides/{viewId}")
@delete
op deleteViewOverride(...Parameters.TenantSlug, ...Parameters.ViewId, ...Parameters.IfMatch):
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

/** Creates or replaces the selected view override within the authorized request scope. */
@tag("View")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/view-overrides/{viewId}")
@put
op putViewOverride(
  ...Parameters.TenantSlug,
  ...Parameters.ViewId,
  ...Parameters.IfMatch,
  @body body: ViewOverridePutRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: ViewOverride;
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

/** Returns the requested page of views within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/views")
@get
op listViews(...Parameters.TenantSlug, ...Parameters.KindFilter):
  | ViewList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Resolves view within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-idempotency", #{ mode: "none" })
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/views:resolve")
@post
op resolveView(...Parameters.TenantSlug, @body body: ViewResolveRequest):
  | ViewResolution
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

/** Returns the requested page of share links within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/share-links")
@get
op listShareLinks(...Parameters.TenantSlug, ...Parameters.Page, ...Parameters.PageSize):
  | ShareLinkPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates share link within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/share-links")
@post
op createShareLink(...Parameters.TenantSlug, @body body: ShareLinkCreateRequest):
  | (CreatedResponse & ShareLinkCreated)
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

/** Performs the revoke share link workflow within the authorized request scope. */
@tag("View")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/share-links/{shareLinkId}")
@delete
op revokeShareLink(...Parameters.TenantSlug, ...Parameters.ShareLinkId):
  | NoContentResponse
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected shared view within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "public-signed-token")
@extension("x-permissions", #[])
@extension("x-milestone", "M3")
@route("/api/v1/shared/{shareToken}")
@get
@useAuth({})
op getSharedView(...Parameters.ShareToken):
  | SharedView
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of system groups within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/system-groups")
@get
op listSystemGroups(...Parameters.TenantSlug):
  | SystemGroupList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates system group within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["systemgroup:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/system-groups")
@post
op createSystemGroup(...Parameters.TenantSlug, @body body: SystemGroupCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: SystemGroup;
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

/** Deletes the selected system group within the authorized request scope. */
@tag("View")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["systemgroup:manage"])
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/system-groups/{groupId}")
@delete
op deleteSystemGroup(...Parameters.TenantSlug, ...Parameters.GroupId, ...Parameters.IfMatch):
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

/** Returns the selected system group within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/system-groups/{groupId}")
@get
op getSystemGroup(...Parameters.TenantSlug, ...Parameters.GroupId):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: SystemGroup;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected system group within the authorized request scope. */
@tag("View")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["systemgroup:manage"])
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/system-groups/{groupId}")
@patch
op updateSystemGroup(
  ...Parameters.TenantSlug,
  ...Parameters.GroupId,
  ...Parameters.IfMatch,
  @body body: SystemGroupPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: SystemGroup;
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
    }
  | {
      @statusCode statusCode: 422;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates or replaces the selected system group members within the authorized request scope. */
@tag("View")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["systemgroup:manage"])
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/system-groups/{groupId}/members")
@put
op putSystemGroupMembers(
  ...Parameters.TenantSlug,
  ...Parameters.GroupId,
  ...Parameters.IfMatch,
  @body body: SystemGroupMembersReplaceRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: SystemGroup;
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

/** Resolves public view within the authorized request scope. */
@tag("View")
@extension("x-tenant-scope", "public")
@extension("x-permissions", #[])
@extension("x-idempotency", #{ mode: "none" })
@extension("x-milestone", "M1")
@route("/api/v1/public/t/{tenantSlug}/views:resolve")
@post
@useAuth({})
op resolvePublicView(...Parameters.TenantSlug, @body body: PublicViewResolveRequest):
  | ViewResolution
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

/** Previews a layer selection and optional draft layers using the real merge engine without persisting anything. */
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:read"])
@extension("x-idempotency", #{ mode: "none" })
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/assets/{assetId}/views:preview")
@post
op previewAssetView(...Parameters.TenantSlug, ...Parameters.AssetId, @body body: ViewPreviewRequest):
  | ViewPreviewResult
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
