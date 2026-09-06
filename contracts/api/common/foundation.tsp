/** Shared TypeSpec foundation: primitive scalars, enums, and cross-domain error vocabulary. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** is a stable lowercase URL identifier containing letters, digits, and hyphens. */
@pattern("^[a-z0-9][a-z0-9-]{0,63}$")
scalar Slug extends string;

/** identifies an asset kind registered in contracts/kinds.yaml. */
@pattern("^[a-z][a-z0-9_]{0,63}$")
scalar KindId extends string;

/** Opaque HTTP entity tag including quotes */
@minLength(3)
@maxLength(128)
@pattern("^(W/)?\"[^\"]+\"$")
scalar ETag extends string;

/** contains an ordered collection of capability values. */
model CapabilityList is string[];

/** represents asset kind data exchanged through the Meridian API. */
model AssetKind {
  /** uniquely identifies this resource. */
  id: KindId;

  /** is the opaque entity tag required for optimistic concurrency control. */
  etag: ETag;

  /** indicates whether enabled applies to this asset kind. */
  enabled: boolean;

  /** specifies the milestone associated with this asset kind. */
  @pattern("^M[0-9]+$") milestone: string;

  /** contains the ordered accepted media types associated with this asset kind. */
  acceptedMediaTypes: string[];

  /** lists actions the authenticated principal may perform on this resource. */
  capabilities: CapabilityList;

  /** contains the ordered default views associated with this asset kind. */
  defaultViews: string[];

  /** specifies the schema version associated with this asset kind. */
  @minValue(1) schemaVersion: integer;
}

/** contains an ordered collection of asset kind values. */
model AssetKindList {
  /** contains the ordered items associated with this asset kind list. */
  items: AssetKind[];
}

/** identifies one supported error code value. */
enum ErrorCode {
  unauthenticated,
  csrf_invalid,
  not_found,
  validation_error,
  asset_path_not_found,
  overlay_invalid,
  input_spec_mismatch,
  branch_not_indexed,
  upload_expired,
  nesting_too_deep,
  producer_profile_unavailable,
  unsupported_kind,
  duplicate,
  version_not_publishable,
  base_layer_exists,
  quota_exceeded,
  last_admin,
  credential_in_use,
  invalid_state,
  idempotency_conflict,
  job_not_cancellable,
  config_import_conflict,
  ai_disabled,
  kind_disabled,
  content_too_large,
  rate_limited,
  precondition_failed,
  internal_error,
}

/** is the stable machine-readable failure envelope returned by every JSON API error. */
@extension(
  "x-details-by-code",
  #{
    validation_error: #{ fields: "FieldError[]", unknownFields: "string[]" },
    asset_path_not_found: #{ candidates: "[{path,reason}]" },
    overlay_invalid: #{ errors: "[{line,column,message}]", coordinates: "1-based" },
    version_not_publishable: #{ blockingRevisions: "[{revisionId,layerId,status}]" },
    quota_exceeded: #{ quota: "string", current: "integer", limit: "integer" },
    rate_limited: #{ retryAfterSec: "integer" },
  }
)
@error
model ErrorResponse {
  /** identifies the stable error category clients may branch on. */
  code: ErrorCode;

  /** is a safe human-readable explanation and never contains a stack trace or secret. */
  message: string;

  /** contains error-code-specific structured diagnostics, or null when no detail is available. */
  details?: Record<unknown> | null;

  /** correlates this result with server logs and audit records. */
  @minLength(1) requestId: string;
}

/** defines validated input for the corresponding Meridian API operation. */
model AssetKindPatchRequest {
  /** indicates whether enabled applies to this asset kind patch request. */
  enabled: boolean;
}

/** UUID v7 */
@format("uuid")
scalar Uuid extends string;

/** represents asset name template data exchanged through the Meridian API. */
@maxLength(128)
@pattern("^(?:[a-z0-9-]|\\{file_stem\\}|\\{parent_dir\\})+$")
scalar AssetNameTemplate extends string;

/** identifies one supported layer role value. */
enum LayerRole {
  base,
  overlay,
}

/** identifies one supported layer origin value. */
enum LayerOrigin {
  repo,
  third_party,
  manual,
  ai_generated,
}

/** identifies one supported source mode value. */
enum SourceMode {
  builtin,
  command,
  push,
  manual,
  ai,
}

/** represents ref glob data exchanged through the Meridian API. */
@minLength(1)
@maxLength(255)
@pattern("^[A-Za-z0-9._/*?-]+$")
@format("git-ref-glob")
scalar RefGlob extends string;

/** identifies one supported config source value. */
enum ConfigSource {
  db_manual,
  repo_file,
  repo_bootstrap,
}

/** represents field source data exchanged through the Meridian API. */
model FieldSource {
  /** specifies the pointer associated with this field source. */
  pointer: string;

  /** contains the config source associated with this field source. */
  source: ConfigSource;
}

/** identifies one supported ref type value. */
enum RefType {
  branch,
  tag,
}

/** is a validated Git branch or tag name without a refs prefix. */
@minLength(1)
@maxLength(255)
@pattern("^[A-Za-z0-9._/-]+$")
@format("git-ref-name")
scalar RefName extends string;

/** identifies one supported job status value. */
enum JobStatus {
  pending,
  running,
  succeeded,
  succeeded_with_warnings,
  failed,
  outcome_unknown,
  cancelled,
}

/** RFC 3339 UTC */
scalar Timestamp extends utcDateTime;
