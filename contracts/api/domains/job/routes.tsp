import "@typespec/http";
import "@typespec/openapi3";
import "../../common/models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Returns the requested page of jobs within the authorized request scope. */
@tag("Job")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["job:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/jobs")
@get
op listJobs(
  ...Parameters.TenantSlug,
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.JobFilters,
):
  | JobPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected job within the authorized request scope. */
@tag("Job")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["job:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/jobs/{jobId}")
@get
op getJob(...Parameters.TenantSlug, ...Parameters.JobId):
  | Job
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Streams ordered events for job logs within the authorized request scope. */
@tag("Job")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["job:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/jobs/{jobId}/logs")
@get
op streamJobLogs(...Parameters.TenantSlug, ...Parameters.JobId, ...Parameters.LastEventId):
  | {
      /** prevents intermediaries from caching a tenant job event stream. */
      @header("Cache-Control") CacheControl: "no-cache";

      /** disables reverse-proxy response buffering for incremental events. */
      @header("X-Accel-Buffering") XAccelBuffering: "no";

      @header contentType: "text/event-stream";
      @body body: JobSseEvent;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the cancel job workflow within the authorized request scope. */
@tag("Job")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["job:run"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/jobs/{jobId}:cancel")
@post
op cancelJob(...Parameters.TenantSlug, ...Parameters.JobId):
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

/** Performs the retry job workflow within the authorized request scope. */
@tag("Job")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["job:run"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/jobs/{jobId}:retry")
@post
op retryJob(...Parameters.TenantSlug, ...Parameters.JobId, ...Parameters.IdempotencyKey):
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
