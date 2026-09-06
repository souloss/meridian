# TypeSpec API source

`contracts/api/` is the only editable API source. TypeSpec files are compiled
into OpenAPI projections; no OpenAPI YAML under this directory is a source
file.

| Path | Role |
| --- | --- |
| `main.tsp` | Complete service entry point. Imports every domain and defines global API metadata. |
| `models.tsp` | Compatibility facade importing common technical protocols and domain-owned models into the root generation boundary. |
| `domains/<domain>/routes.tsp` | One domain's operations. Each route file imports the shared model graph and owns its routes, operation IDs, tags and extensions. |
| `tspconfig.yaml` | Pinned TypeSpec OpenAPI 3.1 emitter configuration. |
| `package.json`, `pnpm-lock.yaml` | Exact compiler dependency boundary for reproducible contract builds. |

Only `.tsp` files are contract source. Keep `package.json` and
`pnpm-lock.yaml` beside the compiler configuration so contract builds remain
independently reproducible. `node_modules/` is a local, ignored installation
artifact and must never be committed.

The generated graph is:

```text
TypeSpec sources
  |-- main.tsp ----------------------> contracts/openapi.yaml
  |-- models.tsp --------------------> build/contracts/api/common/openapi.yaml
  `-- domains/*/routes.tsp ----------> build/contracts/api/domains/*.yaml
                                            |
                                            `--> oapi-codegen domain Go packages
```

Edit the `.tsp` files, then run `make contracts-sync`. The script compiles all
entries, validates that domain paths and operation IDs exactly cover the main
entry point, and materializes the build projections. Run
`make contracts-generate` to regenerate Go and Orval output. Generated YAML
and Go files must not be edited manually.

The backend generates models once in `internal/generated/api`, then one strict
Chi server package per domain using `oapi-codegen` import mapping. No Go client
is generated. The frontend Orval input is the complete canonical bundle.
