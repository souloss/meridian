# Shared TypeSpec model graph

`models.tsp` owns the reusable wire vocabulary: scalars, enums, request and
response models, errors, multipart inputs, event payloads and parameter model
definitions. It is compiled independently so every generated domain document
can reference one stable shared OpenAPI component boundary.

Keep definitions here only when they are part of the public cross-domain wire
contract. Domain-specific operations remain in `../domains/*.tsp`; do not copy
models into a domain file. When the graph becomes large enough to warrant it,
split this file into imported TypeSpec modules such as `errors.tsp` and
`security.tsp`, while preserving the same exported model names and one shared
component projection.

The generated `build/contracts/api/common/openapi.yaml` is an implementation
artifact for `oapi-codegen` import mapping. It is never edited directly.
