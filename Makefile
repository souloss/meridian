MERIDIAN_DEV_DATABASE_URL ?= postgres://meridian:meridian@127.0.0.1:54329/meridian?sslmode=disable

.PHONY: all build generate backend-generate backend-test backend-test-integration backend-run database-up database-down migrate-up migrate-status frontend-install frontend-api frontend-generate frontend-typecheck contracts-generate contracts-generate-then-git-diff-exit-code contracts-validate contracts-lint

all: build

build: frontend-generate
	mkdir -p bin
	vfox exec golang@1.27.1 -- env CGO_ENABLED=0 go build -tags embed_web -trimpath -o bin/meridian ./cmd/meridian

generate: backend-generate frontend-generate

backend-generate:
	vfox exec golang@1.27.1 -- go generate ./...

backend-test:
	vfox exec golang@1.27.1 -- go test ./...

backend-test-integration:
	./scripts/test-integration.sh

backend-run: database-up
	MERIDIAN_DATABASE_URL=$(MERIDIAN_DEV_DATABASE_URL) vfox exec golang@1.27.1 -- go run ./cmd/meridian serve

database-up:
	docker compose up --detach --wait postgres

database-down:
	docker compose stop postgres

migrate-up: database-up
	MERIDIAN_DATABASE_URL=$(MERIDIAN_DEV_DATABASE_URL) vfox exec golang@1.27.1 -- go run ./cmd/meridian migrate up

migrate-status: database-up
	MERIDIAN_DATABASE_URL=$(MERIDIAN_DEV_DATABASE_URL) vfox exec golang@1.27.1 -- go run ./cmd/meridian migrate status

frontend-install:
	vfox exec nodejs@24.20.0 -- pnpm --dir web install --frozen-lockfile

frontend-api: frontend-install
	vfox exec nodejs@24.20.0 -- pnpm --dir web generate:api

frontend-generate: frontend-api
	vfox exec nodejs@24.20.0 -- pnpm --dir web generate

frontend-typecheck:
	vfox exec nodejs@24.20.0 -- pnpm --dir web typecheck

contracts-generate: backend-generate frontend-api

contracts-generate-then-git-diff-exit-code: contracts-generate
	git diff --exit-code -- internal/generated/api web/app/api/generated

contracts-validate:
	vfox exec nodejs@24.20.0 -- pnpm --dir web exec redocly lint --config ../contracts/.redocly.yaml ../contracts/openapi.yaml
	vfox exec nodejs@24.20.0 -- pnpm --dir web check:generated-docs

contracts-lint: contracts-validate
