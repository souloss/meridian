import "@typespec/http";
import "@typespec/openapi3";
import "../../common/models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Checks repository connection within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:write"])
@extension("x-idempotency", #{ mode: "none" })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories:check-connection")
@post
op checkRepositoryConnection(
  ...Parameters.TenantSlug,
  @body body: RepositoryConnectionCheckRequest,
):
  | ConnectionTest
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of repositories within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories")
@get
op listRepositories(
  ...Parameters.TenantSlug,
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.Query,
):
  | RepositoryPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates repository within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:write"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories")
@post
op createRepository(...Parameters.TenantSlug, @body body: RepositoryCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Repository;
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

/** Deletes the selected repository within the authorized request scope. */
@tag("Repository")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:write"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}")
@delete
op deleteRepository(...Parameters.TenantSlug, ...Parameters.RepositoryId, ...Parameters.IfMatch):
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

/** Returns the selected repository within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}")
@get
op getRepository(...Parameters.TenantSlug, ...Parameters.RepositoryId):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Repository;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected repository within the authorized request scope. */
@tag("Repository")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:write"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}")
@patch
op updateRepository(
  ...Parameters.TenantSlug,
  ...Parameters.RepositoryId,
  ...Parameters.IfMatch,
  @body body: RepositoryPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Repository;
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

/** Performs the sync repository workflow within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:sync"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}:sync")
@post
op syncRepository(
  ...Parameters.TenantSlug,
  ...Parameters.RepositoryId,
  ...Parameters.IdempotencyKey,
  @body body: RepositorySyncRequest,
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

/** Performs the discover repository workflow within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:sync"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}:discover")
@post
op discoverRepository(
  ...Parameters.TenantSlug,
  ...Parameters.RepositoryId,
  ...Parameters.IdempotencyKey,
  @body body: RepositoryDiscoverRequest,
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

/** Returns the requested page of discovery candidates within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}/candidates")
@get
op listDiscoveryCandidates(
  ...Parameters.TenantSlug,
  ...Parameters.RepositoryId,
  ...Parameters.Page,
  ...Parameters.PageSize,
):
  | CandidatePage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the accept discovery candidates workflow within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:create"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}/candidates:accept")
@post
op acceptDiscoveryCandidates(
  ...Parameters.TenantSlug,
  ...Parameters.RepositoryId,
  ...Parameters.IdempotencyKey,
  @body body: CandidatesAcceptRequest,
):
  | ServiceList
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

/** Performs the dismiss discovery candidate workflow within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:create"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}/candidates/{candidateId}:dismiss")
@post
op dismissDiscoveryCandidate(
  ...Parameters.TenantSlug,
  ...Parameters.RepositoryId,
  ...Parameters.CandidateId,
):
  | DiscoveryCandidate
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the preview repository config import workflow within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:write"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}/config-imports")
@post
op previewRepositoryConfigImport(
  ...Parameters.TenantSlug,
  ...Parameters.RepositoryId,
  ...Parameters.IdempotencyKey,
  @body body: ConfigImportPreviewRequest,
):
  | (CreatedResponse & ConfigImportPreview)
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

/** Performs the apply repository config import workflow within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["repository:write"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}/config-imports/{previewId}:apply")
@post
op applyRepositoryConfigImport(
  ...Parameters.TenantSlug,
  ...Parameters.RepositoryId,
  ...Parameters.PreviewId,
  ...Parameters.IdempotencyKey,
  @body body: ConfigImportApplyRequest,
):
  | ConfigImportResult
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

/** Performs the receive git webhook workflow within the authorized request scope. */
@tag("Repository")
@extension("x-tenant-scope", "signed-webhook")
@extension("x-permissions", #[])
@extension("x-idempotency", #{ mode: "provider-delivery-id", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/webhooks/git/{repositoryId}")
@post
@useAuth({})
op receiveGitWebhook(
  ...Parameters.RepositoryId,
  ...Parameters.WebhookDelivery,
  ...Parameters.WebhookEvent,
  ...Parameters.WebhookSignature,
  @body body: GitPushWebhook,
):
  | (AcceptedResponse & JobAccepted)
  | {
      @statusCode statusCode: 401;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };
