import "@typespec/http";
import "@typespec/openapi3";
import "../../common/models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Creates diff upload within the authorized request scope. */
@tag("Diff")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-idempotency", #{ mode: "none" })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/uploads")
@post
op createDiffUpload(
  ...Parameters.TenantSlug,
  @header contentType: "multipart/form-data",
  @multipartBody body: UploadRequest,
):
  | (CreatedResponse & Upload)
  | {
      @statusCode statusCode: 404;
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

/** Runs diff within the authorized request scope. */
@tag("Diff")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff")
@post
op runDiff(...Parameters.TenantSlug, @body body: DiffCreateRequest):
  | DiffResult
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

/** Returns the requested page of diff snapshots within the authorized request scope. */
@tag("Diff")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff-snapshots")
@get
op listDiffSnapshots(...Parameters.TenantSlug, ...Parameters.Page, ...Parameters.PageSize):
  | DiffSnapshotPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Deletes the selected diff snapshot within the authorized request scope. */
@tag("Diff")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff-snapshots/{snapshotId}")
@delete
op deleteDiffSnapshot(...Parameters.TenantSlug, ...Parameters.SnapshotId):
  | NoContentResponse
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected diff snapshot within the authorized request scope. */
@tag("Diff")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff-snapshots/{snapshotId}")
@get
op getDiffSnapshot(...Parameters.TenantSlug, ...Parameters.SnapshotId):
  | DiffSnapshot
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Exports diff snapshot within the authorized request scope. */
@tag("Diff")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff-snapshots/{snapshotId}/export")
@get
op exportDiffSnapshot(
  ...Parameters.TenantSlug,
  ...Parameters.SnapshotId,
  ...Parameters.ExportFormat,
):
  | ArtifactLink
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates diff snapshot share link within the authorized request scope. */
@tag("Diff")
@tag("View")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff-snapshots/{snapshotId}/share-links")
@post
op createDiffSnapshotShareLink(
  ...Parameters.TenantSlug,
  ...Parameters.SnapshotId,
  @body body: DiffSnapshotShareRequest,
):
  | (CreatedResponse & ShareLinkCreated)
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of diff rule sets within the authorized request scope. */
@tag("Diff")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff-rule-sets")
@get
op listDiffRuleSets(...Parameters.TenantSlug):
  | DiffRuleSetList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates diff rule set within the authorized request scope. */
@tag("Diff")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff-rule-sets")
@post
op createDiffRuleSet(...Parameters.TenantSlug, @body body: DiffRuleSetCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: DiffRuleSet;
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
    };

/** Deletes the selected diff rule set within the authorized request scope. */
@tag("Diff")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff-rule-sets/{ruleSetId}")
@delete
op deleteDiffRuleSet(...Parameters.TenantSlug, ...Parameters.RuleSetId, ...Parameters.IfMatch):
  | NoContentResponse
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
    };

/** Updates the selected diff rule set within the authorized request scope. */
@tag("Diff")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/diff-rule-sets/{ruleSetId}")
@patch
op updateDiffRuleSet(
  ...Parameters.TenantSlug,
  ...Parameters.RuleSetId,
  ...Parameters.IfMatch,
  @body body: DiffRuleSetPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: DiffRuleSet;
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

/** Performs the search workflow within the authorized request scope. */
@tag("Diff")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/search")
@get
op search(
  ...Parameters.TenantSlug,
  ...Parameters.RequiredQuery,
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.SearchFacets,
):
  | SearchResult
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
