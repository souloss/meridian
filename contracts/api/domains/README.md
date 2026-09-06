# TypeSpec domain sources

Each `.tsp` file owns one API boundary. It imports `../common/models.tsp`,
declares its routes under `namespace MeridianApi`, and keeps operation IDs,
tags, authorization decorators and contract extensions next to the operation
they describe.

| File | Boundary |
| --- | --- |
| `asset.tsp` | Asset kinds, sources, bindings, assets, versions and publishing |
| `auth.tsp` | CSRF, login, logout, identity and preferences |
| `collaboration.tsp` | Notifications, comments, subscriptions, audits and todos |
| `diff.tsp` | Diff uploads, runs, snapshots, exports, links, rules and search |
| `job.tsp` | Jobs, logs, cancellation and retry |
| `layer.tsp` | Layers, revisions, ordering, reviews and approval |
| `platform.tsp` | Platform users, tenants, settings, credentials and jobs |
| `repository.tsp` | Repository CRUD, checks, discovery, sync and imports |
| `service.tsp` | Services, access, sources, views, tags and teams |
| `system.tsp` | Health, readiness, metrics and system groups |
| `tenant.tsp` | Tenant settings, members, credentials, tokens and hosts |
| `view.tsp` | View definitions, overrides, resolution and public access |

The filename is also the generated Go package name, except `service.tsp`,
which keeps the existing `serviceapi` package convention. Paths and operation
IDs must be globally unique. Every public operation and field needs a useful
doc comment because those comments flow into OpenAPI, GoDoc and JSDoc.

Domain OpenAPI files under `build/contracts/api/domains/` are generated
projections. They are consumed by `oapi-codegen` and never edited manually.
