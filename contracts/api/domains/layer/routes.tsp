import "@typespec/http";
import "@typespec/openapi3";
import "../../models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Performs the reorder asset layers workflow within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/assets/{assetId}/layers/order")
@put
op reorderAssetLayers(
  ...Parameters.TenantSlug,
  ...Parameters.AssetId,
  ...Parameters.IfMatch,
  @body body: LayerOrderReplaceRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: LayerList;
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

/** Returns the selected layer within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:read"])
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/layers/{layerId}")
@get
op getLayer(
  ...Parameters.TenantSlug,
  ...Parameters.LayerId,
  ...Parameters.RefType,
  ...Parameters.RefName,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Layer;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected layer within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/layers/{layerId}")
@patch
op updateLayer(
  ...Parameters.TenantSlug,
  ...Parameters.LayerId,
  ...Parameters.IfMatch,
  @body body: LayerPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Layer;
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

/** Returns the requested page of layer revisions within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:read"])
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/layers/{layerId}/revisions")
@get
op listLayerRevisions(
  ...Parameters.TenantSlug,
  ...Parameters.LayerId,
  ...Parameters.RefType,
  ...Parameters.RefName,
  ...Parameters.Page,
  ...Parameters.PageSize,
):
  | LayerRevisionPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates layer revision within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/layers/{layerId}/revisions")
@post
op createLayerRevision(
  ...Parameters.TenantSlug,
  ...Parameters.LayerId,
  ...Parameters.IdempotencyKey,
  @body body: LayerRevisionCreateRequest,
):
  | LayerRevisionSubmission
  | (CreatedResponse & LayerRevisionSubmission)
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
      @statusCode statusCode: 413;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 422;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the rollback layer workflow within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/layers/{layerId}:rollback")
@post
op rollbackLayer(
  ...Parameters.TenantSlug,
  ...Parameters.LayerId,
  ...Parameters.IdempotencyKey,
  @body body: LayerRollbackRequest,
):
  | (AcceptedResponse & JobAccepted)
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

/** Returns the selected layer revision within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:read"])
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/layer-revisions/{revisionId}")
@get
op getLayerRevision(...Parameters.TenantSlug, ...Parameters.RevisionId):
  | LayerRevision
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected review context within the authorized request scope. */
@tag("Layer")
@tag("Diff")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:approve"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/layer-revisions/{revisionId}/review-context")
@get
op getReviewContext(...Parameters.TenantSlug, ...Parameters.RevisionId):
  | ReviewContext
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

/** Performs the approve layer revision workflow within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:approve"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/layer-revisions/{revisionId}:approve")
@post
op approveLayerRevision(
  ...Parameters.TenantSlug,
  ...Parameters.RevisionId,
  ...Parameters.IdempotencyKey,
  @body body: RevisionApproveRequest,
):
  | RevisionReviewResult
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

/** Performs the reject layer revision workflow within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:approve"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/layer-revisions/{revisionId}:reject")
@post
op rejectLayerRevision(
  ...Parameters.TenantSlug,
  ...Parameters.RevisionId,
  ...Parameters.IdempotencyKey,
  @body body: RevisionRejectRequest,
):
  | RevisionReviewResult
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
      @statusCode statusCode: 422;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of reviews within the authorized request scope. */
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:approve"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/reviews")
@get
op listReviews(
  ...Parameters.TenantSlug,
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.ReviewFilters,
):
  | ReviewPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };
