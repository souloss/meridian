MERIDIAN_DEV_DATABASE_URL ?= postgres://meridian:meridian@127.0.0.1:54329/meridian?sslmode=disable
MERIDIAN_DEV_TOKEN_PEPPER ?= AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
MERIDIAN_DEV_JWT_SIGNING_KEY ?= AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
MERIDIAN_DEV_MASTER_KEY ?= AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
MERIDIAN_DEV_MASTER_KEY_VERSION ?= 1
MERIDIAN_DEV_CREDENTIAL_FINGERPRINT_KEY ?= AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
MILESTONE ?= $(if $(ITEM),$(firstword $(subst -, ,$(ITEM))),M5)

.PHONY: all build generate contracts-sync contracts-bundle contracts-check backend-generate backend-test backend-test-integration backend-run database-up database-down migrate-up migrate-status frontend-install frontend-api frontend-generate frontend-typecheck contracts-generate contracts-generate-then-git-diff-exit-code contracts-validate contracts-lint contract-tooling-test smoke smoke-all smoke-m0-credentials smoke-m1-repository smoke-m1-golden-path smoke-m1-service-lifecycle smoke-m1-concurrency smoke-m2-overlay smoke-m2-gitops overlay-property-tests smoke-runner-test agent-protocol-test agent-preflight spike-harness perf-table-cytoscape perf-editor a11y-m0 perf-openapi-pipeline e2e-viewer job-recovery-tests quality-gate

all: build

build: frontend-generate
	mkdir -p bin
	vfox exec golang@1.27.1 -- env CGO_ENABLED=0 go build -tags embed_web -trimpath -o bin/meridian ./cmd/meridian

generate: backend-generate frontend-generate

contracts-sync:
	sh ./scripts/bundle-openapi.sh

contracts-bundle: contracts-sync

contracts-check:
	sh ./scripts/bundle-openapi.sh --check

backend-generate: contracts-sync
	vfox exec golang@1.27.1 -- go generate ./...

backend-test:
	vfox exec golang@1.27.1 -- go test ./...

backend-test-integration:
	./scripts/test-integration.sh

backend-run: database-up
	MERIDIAN_DATABASE_URL=$(MERIDIAN_DEV_DATABASE_URL) MERIDIAN_TOKEN_PEPPER=$(MERIDIAN_DEV_TOKEN_PEPPER) MERIDIAN_JWT_SIGNING_KEY=$(MERIDIAN_DEV_JWT_SIGNING_KEY) MERIDIAN_MASTER_KEY=$(MERIDIAN_DEV_MASTER_KEY) MERIDIAN_MASTER_KEY_VERSION=$(MERIDIAN_DEV_MASTER_KEY_VERSION) MERIDIAN_CREDENTIAL_FINGERPRINT_KEY=$(MERIDIAN_DEV_CREDENTIAL_FINGERPRINT_KEY) vfox exec golang@1.27.1 -- go run ./cmd/meridian serve --addr 127.0.0.1:8080 --insecure-cookies

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

frontend-api: contracts-sync frontend-install
	vfox exec nodejs@24.20.0 -- pnpm --dir web generate:api

frontend-generate: frontend-api
	vfox exec nodejs@24.20.0 -- pnpm --dir web generate

frontend-typecheck:
	vfox exec nodejs@24.20.0 -- pnpm --dir web typecheck

contracts-generate: backend-generate frontend-api

contracts-generate-then-git-diff-exit-code: contracts-generate
	git diff --exit-code -- internal/generated/api internal/generated/repository web/app/api/generated

contracts-validate:
	$(MAKE) agent-protocol-test smoke-runner-test contract-tooling-test
	vfox exec golang@1.27.1 -- go tool sqlc vet
	$(MAKE) contracts-check
	vfox exec nodejs@24.20.0 -- pnpm --dir web exec redocly lint --config ../contracts/.redocly.yaml ../contracts/openapi.yaml
	vfox exec nodejs@24.20.0 -- pnpm --dir web check:generated-docs

contracts-lint: contracts-validate

contract-tooling-test:
	python3 -m unittest scripts/sync_openapi_test.py scripts/frontend_tooling_test.py

smoke:
	SMK="$(SMK)" sh ./scripts/smoke.sh

smoke-all:
	MILESTONE="$(MILESTONE)" sh ./scripts/smoke.sh --all

# A pass requires each named Smoke test to execute without skips.
smoke-m0-credentials:
	sh ./scripts/smoke.sh --credentials

# M1 repository discovery and controlled producer profile selection.
smoke-m1-repository:
	sh ./scripts/smoke.sh --m1-repository

# M1 golden path: sync pipeline, asset materialization, and viewer resolution.
smoke-m1-golden-path:
	sh ./scripts/smoke.sh --m1-golden-path

# M1 service lifecycle: public read gate and soft-delete cascade.
smoke-m1-service-lifecycle:
	sh ./scripts/smoke.sh --m1-service-lifecycle

# M1 concurrency: sync idempotency digest and repository mutex.
smoke-m1-concurrency:
	sh ./scripts/smoke.sh --m1-concurrency

# M2 overlay: platform-v1 overlay determinism, provenance, editor e2e, rollback.
smoke-m2-overlay:
	sh ./scripts/smoke.sh --m2-overlay

# M2 overlay property tests: determinism, provenance, target-miss, invalid rejection.
overlay-property-tests:
	vfox exec golang@1.27.1 -- go test -count=1 ./internal/service -run '^TestOverlay'

# M2 gitops: repository configuration preview/apply and root-dot normalization.
smoke-m2-gitops:
	sh ./scripts/smoke.sh --m2-gitops

# M1 job recovery: worker crash recovery mutex and SSE reconnect.
job-recovery-tests:
	vfox exec golang@1.27.1 -- go test -count=1 -tags integration -run '^TestM1ConcurrencySmoke/SMK-027$$' ./internal/handler

smoke-runner-test:
	vfox exec nodejs@24.20.0 -- node --test scripts/smoke-runner.test.mjs

agent-protocol-test:
	vfox exec golang@1.27.1 -- go test ./internal/contracttest -run 'TestAgentQueue|TestSmokeCatalog'

agent-preflight:
	ITEM="$(ITEM)" vfox exec golang@1.27.1 -- go test -count=1 -v ./internal/contracttest -run '^TestAgentGateAvailability$$'

spike-harness: frontend-install
	vfox exec nodejs@24.20.0 -- pnpm --dir web exec nuxi generate tests/spikes/harness

perf-table-cytoscape: spike-harness
	vfox exec nodejs@24.20.0 -- pnpm --dir web exec node scripts/run-spike.mjs perf-table-cytoscape perf-table-cytoscape.spec.ts desktop

perf-editor: spike-harness
	vfox exec nodejs@24.20.0 -- pnpm --dir web exec node scripts/run-spike.mjs perf-editor perf-editor.spec.ts desktop

a11y-m0: spike-harness
	vfox exec nodejs@24.20.0 -- pnpm --dir web exec node scripts/run-spike.mjs a11y-m0 a11y-m0.spec.ts all

# M1 OpenAPI pipeline: parse + extract 10,000 operations under the perf budget.
perf-openapi-pipeline:
	mkdir -p artifacts/spikes
	SPIKE_REPORT_PATH=artifacts/spikes/perf-openapi-pipeline.json vfox exec golang@1.27.1 -- go run ./cmd/perf-openapi-pipeline

# M1 viewer: resolveView single-document rendering in the spike harness.
e2e-viewer: spike-harness
	vfox exec nodejs@24.20.0 -- pnpm --dir web exec node scripts/run-spike.mjs e2e-viewer e2e-viewer.spec.ts desktop

quality-gate:
	@case "$(ITEM)" in \
		M0-AGENT-003) $(MAKE) contracts-validate contracts-generate-then-git-diff-exit-code backend-test frontend-typecheck frontend-generate smoke-all perf-table-cytoscape perf-editor a11y-m0 MILESTONE=M0 ;; \
		*) echo "quality-gate is not defined for ITEM=$(ITEM)" >&2; exit 2 ;; \
	esac
