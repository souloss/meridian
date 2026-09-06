# Shared TypeSpec model graph

`common/` contains technical protocol definitions only. The root
`contracts/api/models.tsp` facade imports these modules together with
domain-owned models until external cross-domain references are fully adopted.

| File | Responsibility |
| --- | --- |
| `foundation.tsp` | Primitive scalars and protocol identifiers only. |
| `errors.tsp` | Shared HTTP error response and error codes. |
| `pagination.tsp` | Shared pagination protocol. |
| `parameters.tsp` | Reusable HTTP path, query, and header parameter models. |

Business models are owned by `domains/<domain>/models.tsp`; they are imported
by the root facade but are not maintained in `common/`.

The facade is compiled independently so every generated domain document can
reference one stable shared OpenAPI component boundary. Domain routes import
the root facade, not individual model files, so internal reorganization does
not alter the generation chain.

Keep definitions here only when they are transport-wide technical protocols.
Domain-specific models belong in `../domains/<domain>/models.tsp`, and domain
operations belong in `../domains/<domain>/routes.tsp`.

Adding a new technical module is safe when its symbols remain in the
`MeridianApi` namespace and the root facade imports it. Preserve exported model
names and the single shared component projection.

The generated `build/contracts/api/common/openapi.yaml` is an implementation
artifact for `oapi-codegen` import mapping. It is never edited directly.
