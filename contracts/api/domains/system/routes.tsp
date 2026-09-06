import "@typespec/http";
import "@typespec/openapi3";
import "../../common/models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Reports whether the Meridian process is alive. */
@tag("System")
@extension("x-tenant-scope", "public")
@extension("x-permissions", #[])
@extension("x-milestone", "M0")
@route("/healthz")
@get
@useAuth({})
op healthz(): Health;

/** Reports whether Meridian dependencies are ready to serve traffic. */
@tag("System")
@extension("x-tenant-scope", "public")
@extension("x-permissions", #[])
@extension("x-milestone", "M0")
@route("/readyz")
@get
@useAuth({})
op readyz():
  | Health
  | {
      @statusCode statusCode: 503;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns deployment metrics in Prometheus text exposition format. */
@tag("System")
@extension("x-tenant-scope", "deployment-private")
@extension("x-permissions", #[])
@extension("x-milestone", "M5")
@route("/metrics")
@get
@useAuth({})
op metrics(): {
  @header contentType: "text/plain";
  @body body: string;
};

/** Returns the exact OpenAPI contract embedded in this server build. */
@tag("System")
@extension("x-tenant-scope", "public")
@extension("x-permissions", #[])
@extension("x-milestone", "M0")
@route("/api/v1/openapi.yaml")
@get
@useAuth({})
op getOpenApiContract(): {
  @header contentType: "application/yaml";
  @body body: string;
};

/** Returns the server build, API, and contract versions. */
@tag("System")
@extension("x-tenant-scope", "public")
@extension("x-permissions", #[])
@extension("x-milestone", "M0")
@route("/api/v1/version")
@get
@useAuth({})
op getVersion(): VersionInfo;

/** Downloads signed content within the authorized request scope. */
@tag("System")
@extension("x-tenant-scope", "public-signed-token")
@extension("x-permissions", #[])
@extension("x-milestone", "M1")
@route("/api/v1/content/{token}")
@get
@useAuth({})
op downloadSignedContent(...Parameters.ContentToken):
  | {
      @header contentType: "application/octet-stream";
      @body body: bytes;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };
