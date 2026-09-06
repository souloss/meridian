#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
check=0
if [ "${1:-}" = "--check" ]; then
  check=1
fi

cd "$repo_dir"
if [ "$check" -eq 1 ]; then
  python3 scripts/sync-openapi.py --check
else
  python3 scripts/sync-openapi.py
fi

# Redocly validates the generated canonical projection. It is deliberately not
# the source of the projection: TypeSpec remains the only editable API source.
vfox exec nodejs@24.20.0 -- pnpm --dir web exec redocly lint \
  --config ../contracts/.redocly.yaml ../contracts/openapi.yaml
