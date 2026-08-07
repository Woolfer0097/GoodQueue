GOOSE_DRIVER ?= postgres
DATABASE_URL ?= postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable
JET_OUTPUT ?= internal/repository/postgres/generated

.PHONY: build run test test-race test-e2e test-ac vet lint format format-check swagger swagger-check migrate-up migrate-down migrate-status jet-generate jet-check generate verify verify-integration verify-all load-test compose-up compose-down loadtest-observability-up loadtest-observability-stop loadtest-prometheus-up loadtest-prometheus-stop loadtest-seed loadtest-smoke loadtest-medium loadtest-main loadtest-purchase-smoke loadtest-purchase-medium loadtest-purchase-main loadtest-verify loadtest-clean loadtest loadtest-run loadtest-purchase-run

LOADTEST_ENV_FILE ?= loadtest/.env
LOADTEST_PROFILE ?= smoke

build:
	@set -eu; \
	tmp=$$(mktemp -d); \
	cleanup() { status=$$?; trap - EXIT HUP INT TERM; rm -rf "$$tmp"; exit $$status; }; \
	trap cleanup EXIT HUP INT TERM; \
	go build -o "$$tmp/goodqueue-backend" ./cmd/goodqueue-backend

run:
	go run ./cmd/goodqueue-backend

test:
	go test ./...

test-race:
	go test -race ./...

test-e2e:
	@test -n "$(GOODQUEUE_E2E_BASE_URL)" || (echo "GOODQUEUE_E2E_BASE_URL is required" >&2; exit 1)
	@test -n "$(GOODQUEUE_E2E_DATABASE_URL)" || (echo "GOODQUEUE_E2E_DATABASE_URL is required" >&2; exit 1)
	go test ./internal/e2e -count=1

test-ac:
	@test -n "$(GOODQUEUE_E2E_BASE_URL)" || (echo "GOODQUEUE_E2E_BASE_URL is required" >&2; exit 1)
	@test -n "$(GOODQUEUE_E2E_DATABASE_URL)" || (echo "GOODQUEUE_E2E_DATABASE_URL is required" >&2; exit 1)
	go test ./internal/e2e -run '^TestAC' -count=1

vet:
	go vet ./...

lint:
	go tool golangci-lint run ./...

format:
	go tool golangci-lint fmt

format-check:
	go tool golangci-lint fmt --diff

swagger:
	go tool swag fmt -g cmd/goodqueue-backend/main.go --exclude internal/repository/postgres/generated
	go tool swag init -g cmd/goodqueue-backend/main.go -o internal/app/http/docs --parseInternal --packageName docs

swagger-check:
	@set -eu; \
	tmp=$$(mktemp -d); \
	cleanup() { status=$$?; trap - EXIT HUP INT TERM; rm -rf "$$tmp"; exit $$status; }; \
	trap cleanup EXIT HUP INT TERM; \
	go tool swag init -q -g cmd/goodqueue-backend/main.go -o $$tmp --parseInternal --packageName docs; \
	diff -ru internal/app/http/docs $$tmp

migrate-up:
	go tool goose -dir migrations $(GOOSE_DRIVER) "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir migrations $(GOOSE_DRIVER) "$(DATABASE_URL)" down

migrate-status:
	go tool goose -dir migrations $(GOOSE_DRIVER) "$(DATABASE_URL)" status

jet-generate: migrate-up
	@set -eu; \
	tmp=$$(mktemp -d); \
	cleanup() { status=$$?; trap - EXIT HUP INT TERM; rm -rf "$$tmp"; exit $$status; }; \
	trap cleanup EXIT HUP INT TERM; \
	go tool jet -dsn="$(DATABASE_URL)" -schema=public -path="$$tmp/generated" -ignore-tables=goose_db_version; \
	rm -rf "$(JET_OUTPUT)"; \
	mkdir -p "$$(dirname "$(JET_OUTPUT)")"; \
	mv "$$tmp/generated" "$(JET_OUTPUT)"

jet-check: migrate-up
	@set -eu; \
	tmp=$$(mktemp -d); \
	cleanup() { status=$$?; trap - EXIT HUP INT TERM; rm -rf "$$tmp"; exit $$status; }; \
	trap cleanup EXIT HUP INT TERM; \
	go tool jet -dsn="$(DATABASE_URL)" -schema=public -path="$$tmp/generated" -ignore-tables=goose_db_version; \
	diff -ru "$(JET_OUTPUT)" "$$tmp/generated"

generate: swagger jet-generate

verify: format-check swagger-check test test-race vet lint build

verify-integration:
	sh scripts/verify-integration.sh

verify-all: verify verify-integration

load-test:
	go run scripts/queue_load.go

loadtest-prometheus-up:
	@set -eu; \
	loadtest_env_file="$(LOADTEST_ENV_FILE)"; . ./scripts/loadtest-env-defaults.sh; \
	docker compose -f compose.yaml -f loadtest/compose.loadtest.yaml \
		up -d --wait --wait-timeout 60 prometheus

loadtest-prometheus-stop:
	@set -eu; \
	loadtest_env_file="$(LOADTEST_ENV_FILE)"; . ./scripts/loadtest-env-defaults.sh; \
	docker compose -f compose.yaml -f loadtest/compose.loadtest.yaml stop prometheus

loadtest-observability-up:
	@set -eu; \
	loadtest_env_file="$(LOADTEST_ENV_FILE)"; . ./scripts/loadtest-env-defaults.sh; \
	docker compose -f compose.yaml -f loadtest/compose.loadtest.yaml \
		up -d --wait --wait-timeout 90 prometheus grafana

loadtest-observability-stop:
	@set -eu; \
	loadtest_env_file="$(LOADTEST_ENV_FILE)"; . ./scripts/loadtest-env-defaults.sh; \
	docker compose -f compose.yaml -f loadtest/compose.loadtest.yaml stop grafana prometheus

loadtest-seed:
	@set -eu; \
	requested_run_id="$${LOADTEST_RUN_ID:-}"; \
	loadtest_env_file="$(LOADTEST_ENV_FILE)"; . ./scripts/loadtest-env-defaults.sh; \
	export LOADTEST_PROFILE="$(LOADTEST_PROFILE)"; \
	if test -n "$$requested_run_id"; then export LOADTEST_RUN_ID="$$requested_run_id"; fi; \
	go run ./cmd/loadtest-seed

loadtest-verify:
	@set -eu; \
	requested_run_id="$${LOADTEST_RUN_ID:-}"; \
	loadtest_env_file="$(LOADTEST_ENV_FILE)"; . ./scripts/loadtest-env-defaults.sh; \
	export LOADTEST_PROFILE="$(LOADTEST_PROFILE)"; \
	if test -n "$$requested_run_id"; then export LOADTEST_RUN_ID="$$requested_run_id"; fi; \
	go run ./cmd/loadtest-verify

loadtest-smoke:
	@$(MAKE) --no-print-directory loadtest-run LOADTEST_PROFILE=smoke

loadtest-medium:
	@$(MAKE) --no-print-directory loadtest-run LOADTEST_PROFILE=medium

loadtest-main:
	@$(MAKE) --no-print-directory loadtest-run LOADTEST_PROFILE=main

loadtest-purchase-smoke:
	@$(MAKE) --no-print-directory loadtest-purchase-run LOADTEST_PROFILE=smoke

loadtest-purchase-medium:
	@$(MAKE) --no-print-directory loadtest-purchase-run LOADTEST_PROFILE=medium

loadtest-purchase-main:
	@$(MAKE) --no-print-directory loadtest-purchase-run LOADTEST_PROFILE=main

loadtest: loadtest-smoke

loadtest-run: loadtest-observability-up
	@set -eu; \
	requested_run_id="$${LOADTEST_RUN_ID:-}"; \
	loadtest_env_file="$(LOADTEST_ENV_FILE)"; . ./scripts/loadtest-env-defaults.sh; \
	export LOADTEST_PROFILE="$(LOADTEST_PROFILE)"; \
	if test -n "$$requested_run_id"; then export LOADTEST_RUN_ID="$$requested_run_id"; fi; \
	export LOADTEST_SCENARIO=queue_join_polling; \
	base_url="$${LOADTEST_BASE_URL:-http://localhost:8080}"; \
	database_url="$${LOADTEST_DATABASE_URL:-postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable}"; \
	export LOADTEST_DATABASE_URL="$$database_url"; \
	export K6_PROMETHEUS_RW_SERVER_URL="$${K6_PROMETHEUS_RW_SERVER_URL:-http://localhost:9090/api/v1/write}"; \
	export K6_PROMETHEUS_RW_TREND_STATS="$${K6_PROMETHEUS_RW_TREND_STATS:-avg,min,max,p(90),p(95),p(99)}"; \
	export K6_PROMETHEUS_RW_STALE_MARKERS=false; \
	ready_attempts=0; \
	until curl --fail --silent --show-error "$$base_url/readyz" >/dev/null; do \
		ready_attempts=$$((ready_attempts + 1)); \
		if test $$ready_attempts -ge 60; then echo "Backend did not become ready at $$base_url/readyz" >&2; exit 1; fi; \
		echo "Waiting for $$base_url/readyz ..."; sleep 2; \
	done; \
	go run ./cmd/loadtest-seed; \
	run_id="$${LOADTEST_RUN_ID:-local}"; \
	data_file="$${LOADTEST_DATA_FILE:-loadtest/generated/$$run_id/data.json}"; \
	case "$$data_file" in /*) ;; *) data_file="$$PWD/$$data_file" ;; esac; \
	LOADTEST_DATA_FILE="$$data_file" k6 run -o experimental-prometheus-rw \
		loadtest/k6/queue-join-polling.js; \
	go run ./cmd/loadtest-verify; \
	if test "$${LOADTEST_KEEP_DATA:-true}" = "false"; then go run ./cmd/loadtest-seed --cleanup-only; fi

loadtest-purchase-run: loadtest-observability-up
	@set -eu; \
	requested_run_id="$${LOADTEST_RUN_ID:-}"; \
	loadtest_env_file="$(LOADTEST_ENV_FILE)"; . ./scripts/loadtest-env-defaults.sh; \
	export LOADTEST_PROFILE="$(LOADTEST_PROFILE)"; \
	if test -n "$$requested_run_id"; then export LOADTEST_RUN_ID="$$requested_run_id"; fi; \
	export LOADTEST_SCENARIO=purchase_outcomes; \
	base_url="$${LOADTEST_BASE_URL:-http://localhost:8080}"; \
	database_url="$${LOADTEST_DATABASE_URL:-postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable}"; \
	export LOADTEST_DATABASE_URL="$$database_url"; \
	export K6_PROMETHEUS_RW_SERVER_URL="$${K6_PROMETHEUS_RW_SERVER_URL:-http://localhost:9090/api/v1/write}"; \
	export K6_PROMETHEUS_RW_TREND_STATS="$${K6_PROMETHEUS_RW_TREND_STATS:-avg,min,max,p(90),p(95),p(99)}"; \
	export K6_PROMETHEUS_RW_STALE_MARKERS=false; \
	ready_attempts=0; \
	until curl --fail --silent --show-error "$$base_url/readyz" >/dev/null; do \
		ready_attempts=$$((ready_attempts + 1)); \
		if test $$ready_attempts -ge 60; then echo "Backend did not become ready at $$base_url/readyz" >&2; exit 1; fi; \
		echo "Waiting for $$base_url/readyz ..."; sleep 2; \
	done; \
	go run ./cmd/loadtest-seed; \
	run_id="$${LOADTEST_RUN_ID:-local}"; \
	data_file="$${LOADTEST_DATA_FILE:-loadtest/generated/$$run_id/data.json}"; \
	case "$$data_file" in /*) ;; *) data_file="$$PWD/$$data_file" ;; esac; \
	results_dir="$${LOADTEST_RESULTS_DIR:-loadtest/results}"; \
	events_file="$$results_dir/$$run_id/k6-events.log"; \
	mkdir -p "$$results_dir/$$run_id"; \
	rm -f "$$events_file"; \
	k6_status=0; \
	LOADTEST_DATA_FILE="$$data_file" k6 run -o experimental-prometheus-rw \
		--log-format=raw --log-output="file=$$events_file" \
		loadtest/k6/queue-purchase-outcomes.js || k6_status=$$?; \
	verify_status=0; \
	go run ./cmd/loadtest-verify || verify_status=$$?; \
	if test "$${LOADTEST_KEEP_DATA:-true}" = "false" && test $$k6_status -eq 0 && test $$verify_status -eq 0; then \
		go run ./cmd/loadtest-seed --cleanup-only; \
	fi; \
	test $$k6_status -eq 0 && test $$verify_status -eq 0

loadtest-clean:
	@set -eu; \
	requested_run_id="$${LOADTEST_RUN_ID:-}"; \
	loadtest_env_file="$(LOADTEST_ENV_FILE)"; . ./scripts/loadtest-env-defaults.sh; \
	export LOADTEST_PROFILE="$(LOADTEST_PROFILE)"; \
	if test -n "$$requested_run_id"; then export LOADTEST_RUN_ID="$$requested_run_id"; fi; \
	go run ./cmd/loadtest-seed --cleanup-only; \
	run_id="$${LOADTEST_RUN_ID:-local}"; \
	case "$$run_id" in *[!A-Za-z0-9.-]*|'') echo "Unsafe LOADTEST_RUN_ID" >&2; exit 1;; esac; \
	find "loadtest/results/$$run_id" -mindepth 1 -delete 2>/dev/null || true; \
	rmdir "loadtest/results/$$run_id" 2>/dev/null || true; \
	find "loadtest/generated/$$run_id" -mindepth 1 -delete 2>/dev/null || true; \
	rmdir "loadtest/generated/$$run_id" 2>/dev/null || true

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down
