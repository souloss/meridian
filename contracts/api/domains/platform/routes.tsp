import "@typespec/http";
import "@typespec/openapi3";
import "../../models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Returns the requested page of users within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:user:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/users")
@get
op listUsers(...Parameters.Page, ...Parameters.PageSize, ...Parameters.Query):
  | UserPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates user within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:user:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/admin/users")
@post
op createUser(@body body: UserCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: User;
    }
  | {
      @statusCode statusCode: 401;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 403;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
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
      @statusCode statusCode: 422;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected user within the authorized request scope. */
@tag("Platform")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:user:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/users/{userId}")
@patch
op updateUser(...Parameters.UserId, ...Parameters.IfMatch, @body body: UserPatchRequest):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: User;
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

/** Returns the requested page of tenants within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:tenant:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/tenants")
@get
op listTenants(...Parameters.Page, ...Parameters.PageSize): TenantPage;

/** Creates tenant within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:tenant:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/admin/tenants")
@post
op createTenant(@body body: TenantCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Tenant;
    }
  | {
      @statusCode statusCode: 401;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 403;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
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

/** Deletes the selected tenant within the authorized request scope. */
@tag("Platform")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:tenant:delete"])
@extension("x-milestone", "M5")
@route("/api/v1/admin/tenants/{tenantSlug}")
@delete
op deleteTenant(...Parameters.TenantSlug, ...Parameters.IfMatch, @body body: TenantDeleteRequest):
  | {
      @statusCode statusCode: 202;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: TenantDeletionAccepted;
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

/** Updates the selected tenant within the authorized request scope. */
@tag("Platform")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:tenant:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/tenants/{tenantSlug}")
@patch
op updateTenant(...Parameters.TenantSlug, ...Parameters.IfMatch, @body body: TenantPatchRequest):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Tenant;
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

/** Creates or replaces the selected tenant member as platform admin within the authorized request scope. */
@tag("Platform")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:tenant:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/tenants/{tenantSlug}/members/{userId}")
@put
op putTenantMemberAsPlatformAdmin(
  ...Parameters.TenantSlug,
  ...Parameters.UserId,
  @body body: MemberPutRequest,
):
  | Member
  | {
      @statusCode statusCode: 401;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 403;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected platform settings within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:settings:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/settings")
@get
op getPlatformSettings(): {
  /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
  @header ETag?: ETag;

  @body body: PlatformSettings;
};

/** Updates the selected platform settings within the authorized request scope. */
@tag("Platform")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:settings:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/settings")
@patch
op updatePlatformSettings(...Parameters.IfMatch, @body body: PlatformSettingsPatchRequest):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: PlatformSettings;
    }
  | {
      @statusCode statusCode: 412;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of global credentials within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:credential:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/global-credentials")
@get
op listGlobalCredentials(...Parameters.Page, ...Parameters.PageSize): GlobalCredentialPage;

/** Creates global credential within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:credential:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/admin/global-credentials")
@post
op createGlobalCredential(@body body: GlobalCredentialCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: GlobalCredential;
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

/** Deletes the selected global credential within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:credential:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/admin/global-credentials/{credentialId}")
@delete
op deleteGlobalCredential(...Parameters.CredentialId, ...Parameters.Force, ...Parameters.IfMatch):
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

/** Updates the selected global credential within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:credential:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/admin/global-credentials/{credentialId}")
@patch
op updateGlobalCredential(
  ...Parameters.CredentialId,
  ...Parameters.IfMatch,
  @body body: GlobalCredentialPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: GlobalCredential;
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

/** Tests global credential within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:credential:manage"])
@extension("x-idempotency", #{ mode: "none" })
@extension("x-milestone", "M0")
@route("/api/v1/admin/global-credentials/{credentialId}:test")
@post
op testGlobalCredential(...Parameters.CredentialId, @body body: ConnectionTestRequest):
  | ConnectionTest
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the rotate global credential workflow within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:credential:manage"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/admin/global-credentials/{credentialId}:rotate")
@post
op rotateGlobalCredential(
  ...Parameters.CredentialId,
  ...Parameters.IfMatch,
  ...Parameters.IdempotencyKey,
  @body body: CredentialRotateRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: GlobalCredentialRotationResult;
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

/** Returns the requested page of producer profiles within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:producer:manage"])
@extension("x-milestone", "M1")
@route("/api/v1/admin/producer-profiles")
@get
op listProducerProfiles(...Parameters.Page, ...Parameters.PageSize): ProducerProfilePage;

/** Creates producer profile within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:producer:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/admin/producer-profiles")
@post
op createProducerProfile(@body body: ProducerProfileCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: ProducerProfile;
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

/** Deletes the selected producer profile within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:producer:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/admin/producer-profiles/{producerProfileId}")
@delete
op deleteProducerProfile(
  ...Parameters.ProducerProfileId,
  ...Parameters.Force,
  ...Parameters.IfMatch,
):
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

/** Returns the selected producer profile within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:producer:manage"])
@extension("x-milestone", "M1")
@route("/api/v1/admin/producer-profiles/{producerProfileId}")
@get
op getProducerProfile(...Parameters.ProducerProfileId):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: ProducerProfile;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected producer profile within the authorized request scope. */
@tag("Platform")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:producer:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/admin/producer-profiles/{producerProfileId}")
@patch
op updateProducerProfile(
  ...Parameters.ProducerProfileId,
  ...Parameters.IfMatch,
  @body body: ProducerProfilePatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: ProducerProfile;
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

/** Returns the requested page of platform jobs within the authorized request scope. */
@tag("Platform")
@tag("Job")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:job:read"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/jobs")
@get
op listPlatformJobs(...Parameters.Page, ...Parameters.PageSize, ...Parameters.PlatformJobFilters):
  | PlatformJobPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected platform job within the authorized request scope. */
@tag("Platform")
@tag("Job")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:job:read"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/jobs/{jobId}")
@get
op getPlatformJob(...Parameters.JobId):
  | PlatformJob
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of platform audit logs within the authorized request scope. */
@tag("Platform")
@tag("Collaboration")
@extension("x-tenant-scope", "platform")
@extension("x-permissions", #["platform:audit:read"])
@extension("x-milestone", "M0")
@route("/api/v1/admin/audit-logs")
@get
op listPlatformAuditLogs(
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.PlatformAuditFilters,
):
  | AuditLogPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };
