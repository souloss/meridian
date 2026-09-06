# TypeSpec domain sources

Each domain directory owns one API boundary. Its `routes.tsp` imports
`../../models.tsp`, declares routes under `namespace MeridianApi`, and
keeps operation IDs, tags, authorization decorators and contract extensions
next to the operation they describe. Add a domain `models.tsp` only for models
that are genuinely private to that domain; shared wire models stay under
`../common/`.

| File | Boundary |
| --- | --- |
| `asset/routes.tsp` | Asset kinds, sources, bindings, assets, versions and publishing |
| `auth/routes.tsp` | CSRF, login, logout, identity and preferences |
| `collaboration/routes.tsp` | Notifications, comments, subscriptions, audits and todos |
| `diff/routes.tsp` | Diff uploads, runs, snapshots, exports, links, rules and search |
| `job/routes.tsp` | Jobs, logs, cancellation and retry |
| `layer/routes.tsp` | Layers, revisions, ordering, reviews and approval |
| `platform/routes.tsp` | Platform users, tenants, settings, credentials and jobs |
| `repository/routes.tsp` | Repository CRUD, checks, discovery, sync and imports |
| `service/routes.tsp` | Services, access, sources, views, tags and teams |
| `system/routes.tsp` | Health, readiness, metrics and system groups |
| `tenant/routes.tsp` | Tenant settings, members, credentials, tokens and hosts |
| `view/routes.tsp` | View definitions, overrides, resolution and public access |

The directory name is also the generated Go package name, except `service`,
which keeps the existing `serviceapi` package convention. Paths and operation
IDs must be globally unique. Every public operation and field needs a useful
doc comment because those comments flow into OpenAPI, GoDoc and JSDoc.

Domain OpenAPI files under `build/contracts/api/domains/` are generated
projections. They are consumed by `oapi-codegen` and never edited manually.
