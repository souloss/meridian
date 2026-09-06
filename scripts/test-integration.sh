#!/bin/sh
set -eu

integration_temp=$(mktemp -d)
integration_project=$(basename "$integration_temp" | tr '[:upper:].' '[:lower:]-')
integration_project="meridian-it-$integration_project"
cleanup() {
  docker compose --project-name "$integration_project" --profile integration rm --stop --force postgres-test >/dev/null 2>&1 || true
  docker network rm "${integration_project}_default" >/dev/null 2>&1 || true
  rmdir "$integration_temp"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

docker compose --project-name "$integration_project" --profile integration up --detach --wait --wait-timeout 90 postgres-test
endpoint=$(docker compose --project-name "$integration_project" port postgres-test 5432)
port=${endpoint##*:}
if [ "$#" -eq 0 ]; then
  set -- ./internal/database ./internal/handler
fi
MERIDIAN_TEST_DATABASE_URL="postgres://meridian:meridian@127.0.0.1:${port}/meridian_test?sslmode=disable" \
  vfox exec golang@1.27.1 -- go test -count=1 -timeout 10m -p 1 -tags integration "$@"
