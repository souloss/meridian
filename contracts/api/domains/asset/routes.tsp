import "@typespec/http";
import "@typespec/openapi3";
import "../../common/models.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

@service
namespace MeridianApi;

/** Returns the requested page of asset kinds within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/asset-kinds")
@get
op listAssetKinds(...Parameters.TenantSlug):
  | AssetKindList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Updates the selected asset kind state within the authorized request scope. */
@tag("Asset")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["assetkind:manage"])
@extension("x-milestone", "M0")
@route("/api/v1/t/{tenantSlug}/asset-kinds/{kindId}")
@patch
op updateAssetKindState(
  ...Parameters.TenantSlug,
  ...Parameters.KindId,
  ...Parameters.IfMatch,
  @body body: AssetKindPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: AssetKind;
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

/** Returns the requested page of source specs within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}/sources")
@get
op listSourceSpecs(...Parameters.TenantSlug, ...Parameters.ServiceSlug):
  | SourceSpecList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Creates source spec within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}/sources")
@post
op createSourceSpec(
  ...Parameters.TenantSlug,
  ...Parameters.ServiceSlug,
  @body body: SourceSpecCreateRequest,
):
  | {
      @statusCode statusCode: 201;

      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: SourceSpec;
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

/** Deletes the selected source spec within the authorized request scope. */
@tag("Asset")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/sources/{sourceId}")
@delete
op deleteSourceSpec(...Parameters.TenantSlug, ...Parameters.SourceId, ...Parameters.IfMatch):
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

/** Updates the selected source spec within the authorized request scope. */
@tag("Asset")
@extension("x-idempotency", #{ mode: "optional", ttlHours: 24 })
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/sources/{sourceId}")
@patch
op updateSourceSpec(
  ...Parameters.TenantSlug,
  ...Parameters.SourceId,
  ...Parameters.IfMatch,
  @body body: SourceSpecPatchRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: SourceSpec;
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

/** Returns the requested page of source bindings within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/sources/{sourceId}/bindings")
@get
op listSourceBindings(...Parameters.TenantSlug, ...Parameters.SourceId):
  | SourceBindingList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the produce source workflow within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["job:run"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/sources/{sourceId}:produce")
@post
op produceSource(
  ...Parameters.TenantSlug,
  ...Parameters.SourceId,
  ...Parameters.IdempotencyKey,
  @body body: ProduceSourceRequest,
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

/** Performs the generate missing asset with ai workflow within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/services/{serviceSlug}/assets:ai-generate")
@post
op generateMissingAssetWithAi(
  ...Parameters.TenantSlug,
  ...Parameters.ServiceSlug,
  ...Parameters.IdempotencyKey,
  @body body: ServiceAiGenerateRequest,
):
  | (AcceptedResponse & AiGenerationAccepted)
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

/** Returns the selected asset within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-public-if-service-public", "current-published-summary-only")
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/assets/{assetId}")
@get
op getAsset(
  ...Parameters.TenantSlug,
  ...Parameters.AssetId,
  ...Parameters.RefType,
  ...Parameters.RefName,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: Asset;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of asset versions within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/assets/{assetId}/versions")
@get
op listAssetVersions(
  ...Parameters.TenantSlug,
  ...Parameters.AssetId,
  ...Parameters.RefType,
  ...Parameters.RefName,
  ...Parameters.Page,
  ...Parameters.PageSize,
):
  | AssetVersionPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected asset version within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-public-if-service-public", "current-published-only")
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/asset-versions/{versionId}")
@get
op getAssetVersion(...Parameters.TenantSlug, ...Parameters.VersionId):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: AssetVersion;
    }
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the requested page of asset version items within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:read"])
@extension("x-public-if-service-public", "current-published-only")
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/asset-versions/{versionId}/items")
@get
op listAssetVersionItems(
  ...Parameters.TenantSlug,
  ...Parameters.VersionId,
  ...Parameters.Page,
  ...Parameters.PageSize,
  ...Parameters.Query,
):
  | AssetItemPage
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Returns the selected asset version provenance within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:read"])
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/asset-versions/{versionId}/provenance")
@get
op getAssetVersionProvenance(...Parameters.TenantSlug, ...Parameters.VersionId):
  | ProvenanceList
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };

/** Performs the publish asset version workflow within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:publish"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/asset-versions/{versionId}:publish")
@post
op publishAssetVersion(
  ...Parameters.TenantSlug,
  ...Parameters.VersionId,
  ...Parameters.IfMatch,
  ...Parameters.IdempotencyKey,
  @body body: VersionPublishRequest,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: AssetVersion;
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

/** Performs the deprecate asset version workflow within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:publish"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/asset-versions/{versionId}:deprecate")
@post
op deprecateAssetVersion(
  ...Parameters.TenantSlug,
  ...Parameters.VersionId,
  ...Parameters.IfMatch,
  ...Parameters.IdempotencyKey,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: AssetVersion;
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

/** Performs the retire asset version workflow within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:publish"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M1")
@route("/api/v1/t/{tenantSlug}/asset-versions/{versionId}:retire")
@post
op retireAssetVersion(
  ...Parameters.TenantSlug,
  ...Parameters.VersionId,
  ...Parameters.IfMatch,
  ...Parameters.IdempotencyKey,
):
  | {
      /** Opaque resource revision token. Clients must echo it unchanged in If-Match. */
      @header ETag?: ETag;

      @body body: AssetVersion;
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

/** Performs the generate asset with ai workflow within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/assets/{assetId}:ai-generate")
@post
op generateAssetWithAi(
  ...Parameters.TenantSlug,
  ...Parameters.AssetId,
  ...Parameters.IdempotencyKey,
  @body body: AssetAiGenerateRequest,
):
  | (AcceptedResponse & AiGenerationAccepted)
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

/** Performs the preview merge workflow within the authorized request scope. */
@tag("Asset")
@tag("Layer")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["layer:edit"])
@extension("x-idempotency", #{ mode: "none" })
@extension("x-milestone", "M2")
@route("/api/v1/t/{tenantSlug}/assets:preview-merge")
@post
op previewMerge(...Parameters.TenantSlug, @body body: MergePreviewRequest):
  | MergePreview
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

/** Performs the push asset revision workflow within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "tenant")
@extension("x-permissions", #["asset:push"])
@extension("x-idempotency", #{ mode: "required", ttlHours: 24 })
@extension("x-milestone", "M3")
@route("/api/v1/t/{tenantSlug}/assets:push")
@post
op pushAssetRevision(
  ...Parameters.TenantSlug,
  ...Parameters.IdempotencyKey,
  @body body: AssetPushRequest,
):
  | AssetPushResult
  | (CreatedResponse & AssetPushResult)
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

/** Returns the selected public asset within the authorized request scope. */
@tag("Asset")
@extension("x-tenant-scope", "public")
@extension("x-permissions", #[])
@extension("x-milestone", "M1")
@route("/api/v1/public/t/{tenantSlug}/services/{serviceSlug}/assets/{kindId}/{assetName}")
@get
@useAuth({})
op getPublicAsset(
  ...Parameters.TenantSlug,
  ...Parameters.ServiceSlug,
  ...Parameters.KindId,
  ...Parameters.AssetName,
):
  | PublicAsset
  | {
      @statusCode statusCode: 404;
      @header("X-Request-Id") XRequestId?: string;
      @body body: ErrorResponse;
    };
