import "@typespec/http";
import "@typespec/openapi3";
import "../../common/models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Creates service in repository within the authorized request scope. */
@tag("Service")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:create"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/repositories/{repositoryId}/services")
@post
op createServiceInRepository(
  ...Parameters.TenantSlug,
  ...Parameters.RepositoryId,
  @body body: ServiceCreateRequest,
):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Service;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of services within the authorized request scope. */
@tag("Service")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/services")
@get
op listServices(
  ...Parameters.TenantSlug,
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.Query,
  ...Parameters.Sort,

  /** identifies or filters the group id value in the query. */
  @format("uuid") @query(#{ explode: true }) groupId?: string,

  /** identifies or filters the tag value in the query. */
  @query(#{ explode: true }) tag?: string,

  /** identifies or filters the team value in the query. */
  @query(#{ explode: true }) team?: string,

  /** identifies or filters the lifecycle value in the query. */
  @query(#{ explode: true }) lifecycle?: Lifecycle,

  /** identifies or filters the language value in the query. */
  @query(#{ explode: true }) language?: string,

  /** identifies or filters the kind value in the query. */
  @query(#{ explode: true }) kind?: string,

  /** identifies or filters the starred value in the query. */
  @query(#{ explode: true }) starred?: boolean,

  /** identifies or filters the mine value in the query. */
  @query(#{ explode: true }) mine?: boolean,
):
  | ServicePage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Deletes the selected service within the authorized request scope. */
@tag("Service")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:write"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}")
@delete
op deleteService(...Parameters.TenantSlug, ...Parameters.ServiceSlug, ...Parameters.IfMatch):
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

/** Returns the selected service within the authorized request scope. */
@tag("Service")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-public-if-service-public", true)
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}")
@get
op getService(...Parameters.TenantSlug, ...Parameters.ServiceSlug):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Service;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected service within the authorized request scope. */
@tag("Service")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:write"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}")
@patch
op updateService(
  ...Parameters.TenantSlug,
  ...Parameters.ServiceSlug,
  ...Parameters.IfMatch,
  @body body: ServicePatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Service;
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
    };

/** Performs the star service workflow within the authorized request scope. */
@tag("Service")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}:star")
@post
op starService(...Parameters.TenantSlug, ...Parameters.ServiceSlug):
  | StarState
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of recent services within the authorized request scope. */
@tag("Service")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/services:recent")
@get
op listRecentServices(...Parameters.TenantSlug, ...Parameters.Page, ...Parameters.PageSize):
  | ServicePage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the unstar service workflow within the authorized request scope. */
@tag("Service")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}:unstar")
@post
op unstarService(...Parameters.TenantSlug, ...Parameters.ServiceSlug):
  | StarState
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected service access within the authorized request scope. */
@tag("Service")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:manage_members"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}/access")
@get
op getServiceAccess(...Parameters.TenantSlug, ...Parameters.ServiceSlug):
  | ServiceAccess
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates or replaces the selected service access within the authorized request scope. */
@tag("Service")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:manage_members"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}/access")
@put
op putServiceAccess(
  ...Parameters.TenantSlug,
  ...Parameters.ServiceSlug,
  @body body: ServiceAccessPutRequest,
):
  | ServiceAccess
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

/** Resolves service drift within the authorized request scope. */
@tag("Service")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:write"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}:resolve-drift")
@post
op resolveServiceDrift(
  ...Parameters.TenantSlug,
  ...Parameters.ServiceSlug,
  ...Parameters.IdempotencyKey,
  @body body: DriftResolutionRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Service;
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

/** Returns the selected public service within the authorized request scope. */
@tag("Service")
@extension("x-tenant-scope", "public")
@extension("x-permissions", #[])
@extension("x-milestone", "M1")
@route("/api/v1/public/t/{tenantSlug}/services/{serviceSlug}")
@get
@useAuth({})
op getPublicService(...Parameters.TenantSlug, ...Parameters.ServiceSlug):
  | PublicService
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };
