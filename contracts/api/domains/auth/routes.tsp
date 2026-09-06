import "@typespec/http";
import "@typespec/openapi3";
import "../../common/models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Returns a CSRF token bound to the current authenticated browser session. */
@tag("Auth")
@extension("x-tenant-scope", "identity")
@extension("x-permissions", #[])
@extension("x-milestone", "M0")
@route("/api/v1/auth/csrf")
@get
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "meridian_session">)
op getCsrfToken():
  | CsrfToken
  | {
      @statusCode statusCode: 401;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Authenticates local credentials and creates a browser session. */
@tag("Auth")
@extension("x-tenant-scope", "identity")
@extension("x-permissions", #[])
@extension("x-idempotency", #{ mode: "none" })
@extension("x-milestone", "M0")
@route("/api/v1/auth/login")
@post
@useAuth({})
op login(@body body: LoginRequest):
  | {
      @header("Set-Cookie") SetCookie?: string;
      @body body: LoginResult;
    }
  | {
      @statusCode statusCode: 401;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 429;
      @header("X-Request-Id") XRequestId?: string;
      @header("Retry-After") @minValue(1) RetryAfter?: integer;
      @body body: ErrorResponse;
    };

/** Revokes the current browser session and clears its session cookie. */
@tag("Auth")
@extension("x-tenant-scope", "identity")
@extension("x-permissions", #[])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/auth/logout")
@post
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "meridian_session">)
op logout():
  | (NoContentResponse & {
      @header("Set-Cookie") SetCookie?: string;
    })
  | {
      @statusCode statusCode: 401;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 403;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the authenticated principal and tenant memberships. */
@tag("Auth")
@extension("x-tenant-scope", "identity")
@extension("x-permissions", #[])
@extension("x-milestone", "M0")
@route("/api/v1/auth/me")
@get
@useAuth(ApiKeyAuth<ApiKeyLocation.cookie, "meridian_session">)
op getMe():
  | Me
  | {
      @statusCode statusCode: 401;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected my preferences within the authorized request scope. */
@tag("Auth")
@extension("x-tenant-scope", "identity")
@extension("x-permissions", #[])
@extension("x-milestone", "M1")
@route("/api/v1/auth/me/preferences")
@get
op getMyPreferences(): {
  /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
  @header ETag?: ETag;

  @body body: UserPreferences;
};

/** Updates the selected my preferences within the authorized request scope. */
@tag("Auth")
@extension("x-tenant-scope", "identity")
@extension("x-permissions", #[])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/auth/me/preferences")
@patch
op updateMyPreferences(...Parameters.IfMatch, @body body: UserPreferencesPatchRequest):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: UserPreferences;
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
