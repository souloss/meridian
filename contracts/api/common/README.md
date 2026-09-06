# Shared TypeSpec model graph

`models.tsp` is the stable facade for the reusable wire vocabulary. Its
responsibility-focused modules are:

| File | Responsibility |
| --- | --- |
| `foundation.tsp` | Primitive scalars, lifecycle enums, and error vocabulary. |
| `assets.tsp` | Asset, source, binding, and version models. |
| `auth.tsp` | CSRF, login, identity, and user preference models. |
| `collaboration.tsp` | Notifications, comments, subscriptions, audits, and uploads. |
| `diff.tsp` | Diff, search, and rule-set models. |
| `jobs.tsp` | Asynchronous job, log, and event models. |
| `layers.tsp` | Layers, revisions, reviews, and approval models. |
| `platform.tsp` | Platform administration and producer profile models. |
| `repository.tsp` | Repository, health, and discovery models. |
| `service.tsp` | Services, access, configuration, and source models. |
| `tenant.tsp` | Tenants, credentials, teams, tags, and tokens. |
| `views.tsp` | Views, resolution, sharing, and system groups. |
| `parameters.tsp` | Reusable HTTP path, query, and header parameter models. |

The facade is compiled independently so every generated domain document can
reference one stable shared OpenAPI component boundary. Domains and `main.tsp`
must import `models.tsp`, not individual modules, so internal reorganization
does not alter the generation chain.

Keep definitions here only when they are part of the public cross-domain wire
contract. Domain-specific operations remain in `../domains/*/routes.tsp`; do not copy
models into a domain file.

Adding a new module is safe when its symbols remain in the `MeridianApi`
namespace and `models.tsp` imports it. Preserve exported model names and the
single shared component projection.

The generated `build/contracts/api/common/openapi.yaml` is an implementation
artifact for `oapi-codegen` import mapping. It is never edited directly.
