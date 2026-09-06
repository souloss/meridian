/** Shared HTTP error vocabulary. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

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
