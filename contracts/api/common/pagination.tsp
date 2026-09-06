/** Shared pagination protocol. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** represents page info data exchanged through the Meridian API. */
model PageInfo {
  /** is the number of matching records across all pages. */
  @minValue(0) total: integer;

  /** is the one-based page number. */
  @minValue(1) page: integer;

  /** is the maximum number of records returned on one page. */
  @minValue(1) @maxValue(100) pageSize: integer;
}
