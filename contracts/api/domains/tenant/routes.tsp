import "@typespec/http";
import "@typespec/openapi3";
import "../../models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Returns the selected tenant settings within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/settings")
@get
op getTenantSettings(...Parameters.TenantSlug):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: TenantSettings;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected tenant settings within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/settings")
@patch
op updateTenantSettings(
  ...Parameters.TenantSlug,
  ...Parameters.IfMatch,
  @body body: TenantSettingsPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: TenantSettings;
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

/** Returns the requested page of available producer profiles within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:write"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/producer-profiles")
@get
op listAvailableProducerProfiles(...Parameters.TenantSlug):
  | ProducerProfileOptionList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Searches for tenant users within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/directory/users")
@get
op searchTenantUsers(
  ...Parameters.TenantSlug,
  ...Parameters.Query,
  ...Parameters.Page,
  ...Parameters.PageSize,
):
  | UserPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of tenant members within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/members")
@get
op listTenantMembers(...Parameters.TenantSlug, ...Parameters.Page, ...Parameters.PageSize):
  | MemberPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Deletes the selected tenant member within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/members/{userId}")
@delete
op deleteTenantMember(...Parameters.TenantSlug, ...Parameters.UserId):
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
    };

/** Creates or replaces the selected tenant member within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/members/{userId}")
@put
op putTenantMember(...Parameters.TenantSlug, ...Parameters.UserId, @body body: MemberPutRequest):
  | Member
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

/** Returns the requested page of tokens within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["token:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/tokens")
@get
op listTokens(...Parameters.TenantSlug, ...Parameters.Page, ...Parameters.PageSize):
  | TokenPage
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

/** Creates token within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["token:manage"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/tokens")
@post
op createToken(...Parameters.TenantSlug, @body body: TokenCreateRequest):
  | (CreatedResponse & TokenCreated)
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
      @statusCode statusCode: 422;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the revoke token workflow within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["token:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/tokens/{tokenId}")
@delete
op revokeToken(...Parameters.TenantSlug, ...Parameters.TokenId):
  | NoContentResponse
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

/** Returns the requested page of credentials within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["credential:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/credentials")
@get
op listCredentials(...Parameters.TenantSlug, ...Parameters.Page, ...Parameters.PageSize):
  | CredentialPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates credential within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["credential:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/credentials")
@post
op createCredential(...Parameters.TenantSlug, @body body: CredentialCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Credential;
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

/** Deletes the selected credential within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["credential:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/credentials/{credentialId}")
@delete
op deleteCredential(
  ...Parameters.TenantSlug,
  ...Parameters.CredentialId,
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

/** Updates the selected credential within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["credential:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/credentials/{credentialId}")
@patch
op updateCredential(
  ...Parameters.TenantSlug,
  ...Parameters.CredentialId,
  ...Parameters.IfMatch,
  @body body: CredentialPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Credential;
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

/** Tests credential within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["credential:read"])
@extension("x-idempotency", #{ mode: "none" })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/credentials/{credentialId}:test")
@post
op testCredential(
  ...Parameters.TenantSlug,
  ...Parameters.CredentialId,
  @body body: ConnectionTestRequest,
):
  | ConnectionTest
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the rotate credential workflow within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["credential:manage"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/credentials/{credentialId}:rotate")
@post
op rotateCredential(
  ...Parameters.TenantSlug,
  ...Parameters.CredentialId,
  ...Parameters.IfMatch,
  ...Parameters.IdempotencyKey,
  @body body: CredentialRotateRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: CredentialRotationResult;
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

/** Returns the requested page of known hosts within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["credential:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/known-hosts")
@get
op listKnownHosts(...Parameters.TenantSlug, ...Parameters.Page, ...Parameters.PageSize):
  | KnownHostPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates known host within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["credential:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/known-hosts")
@post
op createKnownHost(...Parameters.TenantSlug, @body body: KnownHostCreateRequest):
  | (CreatedResponse & KnownHost)
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

/** Returns the requested page of teams within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/teams")
@get
op listTeams(...Parameters.TenantSlug, ...Parameters.Page, ...Parameters.PageSize):
  | TeamPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates team within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:write"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/teams")
@post
op createTeam(...Parameters.TenantSlug, @body body: TeamCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Team;
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

/** Deletes the selected team within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:write"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/teams/{teamId}")
@delete
op deleteTeam(...Parameters.TenantSlug, ...Parameters.TeamId, ...Parameters.IfMatch):
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

/** Returns the selected team within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/teams/{teamId}")
@get
op getTeam(...Parameters.TenantSlug, ...Parameters.TeamId):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Team;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected team within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:write"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/teams/{teamId}")
@patch
op updateTeam(
  ...Parameters.TenantSlug,
  ...Parameters.TeamId,
  ...Parameters.IfMatch,
  @body body: TeamPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Team;
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

/** Replaces the selected team members within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:member:write"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/teams/{teamId}/members")
@put
op replaceTeamMembers(
  ...Parameters.TenantSlug,
  ...Parameters.TeamId,
  ...Parameters.IfMatch,
  @body body: TeamMembersReplaceRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Team;
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

/** Returns the requested page of tags within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["service:read"])
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/tags")
@get
op listTags(...Parameters.TenantSlug):
  | TagList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates tag within the authorized request scope. */
@tag("Tenant")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/tags")
@post
op createTag(...Parameters.TenantSlug, @body body: TagCreateRequest):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Tag;
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

/** Deletes the selected tag within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/tags/{tagId}")
@delete
op deleteTag(...Parameters.TenantSlug, ...Parameters.TagId, ...Parameters.IfMatch):
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

/** Updates the selected tag within the authorized request scope. */
@tag("Tenant")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:write"])
@extension("x-milestone", "M4")
@route("/api/v1/t/{tenantSlug}/tags/{tagId}")
@patch
op updateTag(
  ...Parameters.TenantSlug,
  ...Parameters.TagId,
  ...Parameters.IfMatch,
  @body body: TagPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Tag;
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

/** Creates tenant export within the authorized request scope. */
@tag("Tenant")
@tag("Job")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["tenant:settings:read"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M5")
@route("/api/v1/t/{tenantSlug}/exports")
@post
op createTenantExport(...Parameters.TenantSlug, ...Parameters.IdempotencyKey):
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
