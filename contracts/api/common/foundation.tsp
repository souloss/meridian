/** Shared TypeSpec foundation: primitive scalars and protocol identifiers. */
import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace MeridianApi;

/** is a stable lowercase URL identifier containing letters, digits, and hyphens. */
@pattern("^[a-z0-9][a-z0-9-]{0,63}$")
scalar Slug extends string;

/** Opaque HTTP entity tag including quotes */
@minLength(3)
@maxLength(128)
@pattern("^(W/)?\"[^\"]+\"$")
scalar ETag extends string;

/** UUID v7 */
@format("uuid")
scalar Uuid extends string;

/** RFC 3339 UTC */
scalar Timestamp extends utcDateTime;
