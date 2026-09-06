MERIDIAN_DEV_DATABASE_URL ?= postgres://meridian:meridian@127.0.0.1:54329/meridian?sslmode=disable
MERIDIAN_DEV_TOKEN_PEPPER ?= AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
MERIDIAN_DEV_MASTER_KEY ?= AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
MERIDIAN_DEV_MASTER_KEY_VERSION ?= 1
MERIDIAN_DEV_CREDENTIAL_FINGERPRINT_KEY ?= AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
MILESTONE ?= $(if $(ITEM),$(firstword $(subst -, ,$(ITEM))),M5)

.PHONY: all build generate backend-generate backend-test backend-test-integration backend-run database-up database-down migrate-up migrate-status frontend-install frontend-api frontend-generate frontend-typecheck contracts-generate contracts-generate-then-git-diff-exit-code contracts-validate contracts-lint smoke smoke-all smoke-m0-credentials smoke-runner-test agent-protocol-test agent-preflight quality-gate

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
	MERIDIAN_DATABASE_URL=$(MERIDIAN_DEV_DATABASE_URL) MERIDIAN_TOKEN_PEPPER=$(MERIDIAN_DEV_TOKEN_PEPPER) MERIDIAN_MASTER_KEY=$(MERIDIAN_DEV_MASTER_KEY) MERIDIAN_MASTER_KEY_VERSION=$(MERIDIAN_DEV_MASTER_KEY_VERSION) MERIDIAN_CREDENTIAL_FINGERPRINT_KEY=$(MERIDIAN_DEV_CREDENTIAL_FINGERPRINT_KEY) vfox exec golang@1.27.1 -- go run ./cmd/meridian serve --addr 127.0.0.1:8080 --insecure-cookies

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
	git diff --exit-code -- internal/generated/api internal/generated/repository web/app/api/generated

contracts-validate:
	$(MAKE) agent-protocol-test smoke-runner-test
	vfox exec golang@1.27.1 -- go tool sqlc vet
	vfox exec nodejs@24.20.0 -- pnpm --dir web exec redocly lint --config ../contracts/.redocly.yaml ../contracts/openapi.yaml
	vfox exec nodejs@24.20.0 -- pnpm --dir web check:generated-docs

contracts-lint: contracts-validate

smoke:
	SMK="$(SMK)" sh ./scripts/smoke.sh

smoke-all:
	MILESTONE="$(MILESTONE)" sh ./scripts/smoke.sh --all

# A pass requires each named Smoke test to execute without skips.
smoke-m0-credentials:
	sh ./scripts/smoke.sh --credentials

smoke-runner-test:
	vfox exec nodejs@24.20.0 -- node --test scripts/smoke-runner.test.mjs

agent-protocol-test:
	vfox exec golang@1.27.1 -- go test ./internal/contracttest -run 'TestAgentQueue|TestSmokeCatalog'

agent-preflight:
	ITEM="$(ITEM)" vfox exec golang@1.27.1 -- go test -count=1 -v ./internal/contracttest -run '^TestAgentGateAvailability$$'

quality-gate:
	@case "$(ITEM)" in \
		M0-AGENT-003) $(MAKE) contracts-validate contracts-generate-then-git-diff-exit-code backend-test smoke-all MILESTONE=M0 ;; \
		*) echo "quality-gate is not defined for ITEM=$(ITEM)" >&2; exit 2 ;; \
	esac
