#!/bin/sh
set -eu

cleanup() {
  docker compose --profile integration rm --stop --force postgres-test >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker compose --profile integration up --detach --wait postgres-test
endpoint=$(docker compose port postgres-test 5432)
port=${endpoint##*:}
MERIDIAN_TEST_DATABASE_URL="postgres://meridian:meridian@127.0.0.1:${port}/meridian_test?sslmode=disable" \
  vfox exec golang@1.27.1 -- go test -p 1 -tags integration ./internal/database ./internal/handler
