#!/bin/sh
set -eu

# Generate one strict chi server package per generated domain OpenAPI file. The
# TypeSpec sources under contracts/api are the editable API registry; this
# directory is rebuilt by scripts/sync-openapi.py.
repo_dir=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

found=0
for spec in "$repo_dir"/build/contracts/api/domains/*.yaml; do
  [ -f "$spec" ] || continue
  found=1
  filename=${spec##*/}
  domain=${filename%.yaml}
  package_name=$domain
  if [ "$domain" = service ]; then
    package_name=serviceapi
  fi
  config="$tmp_dir/$domain.yaml"
  printf '%s\n' \
    "package: $package_name" \
    'generate:' \
    '  chi-server: true' \
    '  strict-server: true' \
    'import-mapping:' \
    '  ../common/openapi.yaml: github.com/meridian-labs/meridian/internal/generated/api' \
    "output: internal/generated/api/$domain/server.gen.go" > "$config"
  (
    cd "$repo_dir"
    vfox exec golang@1.27.1 -- go tool oapi-codegen --config "$config" "$spec"
  )

  types_config="$tmp_dir/$domain-types.yaml"
  printf '%s\n' \
    "package: $package_name" \
    'generate:' \
    '  models: true' \
    'output-options:' \
    '  skip-prune: true' \
    '  nullable-type: true' \
    'import-mapping:' \
    '  ../common/openapi.yaml: github.com/meridian-labs/meridian/internal/generated/api' \
    "output: internal/generated/api/$domain/types.gen.go" > "$types_config"
  (
    cd "$repo_dir"
    vfox exec golang@1.27.1 -- go tool oapi-codegen --config "$types_config" "$spec"
  )
done

if [ "$found" -eq 0 ]; then
  echo "no generated TypeSpec domain documents found; run make contracts-sync first" >&2
  exit 1
fi
