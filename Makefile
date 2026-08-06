GOOSE_DRIVER ?= postgres
DATABASE_URL ?= postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable
JET_OUTPUT ?= internal/repository/postgres/generated

.PHONY: build run test test-race test-e2e test-ac vet lint format format-check swagger swagger-check migrate-up migrate-down migrate-status jet-generate jet-check generate verify verify-integration verify-all load-test compose-up compose-down loadtest-seed loadtest-smoke loadtest-medium loadtest-main loadtest-purchase-smoke loadtest-purchase-medium loadtest-purchase-main loadtest-verify loadtest-clean loadtest loadtest-run loadtest-purchase-run

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

loadtest-seed:
	@set -eu; \
	requested_run_id="$${LOADTEST_RUN_ID:-}"; \
	set -a; if test -f "$(LOADTEST_ENV_FILE)"; then . "$(LOADTEST_ENV_FILE)"; fi; set +a; \
	export LOADTEST_PROFILE="$(LOADTEST_PROFILE)"; \
	if test -n "$$requested_run_id"; then export LOADTEST_RUN_ID="$$requested_run_id"; fi; \
	go run ./cmd/loadtest-seed

loadtest-verify:
	@set -eu; \
	requested_run_id="$${LOADTEST_RUN_ID:-}"; \
	set -a; if test -f "$(LOADTEST_ENV_FILE)"; then . "$(LOADTEST_ENV_FILE)"; fi; set +a; \
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

loadtest-run:
	@set -eu; \
	requested_run_id="$${LOADTEST_RUN_ID:-}"; \
	set -a; if test -f "$(LOADTEST_ENV_FILE)"; then . "$(LOADTEST_ENV_FILE)"; fi; set +a; \
	export LOADTEST_PROFILE="$(LOADTEST_PROFILE)"; \
	if test -n "$$requested_run_id"; then export LOADTEST_RUN_ID="$$requested_run_id"; fi; \
	export LOADTEST_SCENARIO=queue_join_polling; \
	base_url="$${LOADTEST_BASE_URL:-http://localhost:8080}"; \
	database_url="$${LOADTEST_DATABASE_URL:-postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable}"; \
	export LOADTEST_DATABASE_URL="$$database_url"; \
	until curl --fail --silent --show-error "$$base_url/readyz" >/dev/null; do \
		echo "Waiting for $$base_url/readyz ..."; sleep 2; \
	done; \
	go run ./cmd/loadtest-seed; \
	data_file="$${LOADTEST_DATA_FILE:-loadtest/generated/data.json}"; \
	case "$$data_file" in /*) ;; *) data_file="$$PWD/$$data_file" ;; esac; \
	LOADTEST_DATA_FILE="$$data_file" k6 run loadtest/k6/queue-join-polling.js; \
	go run ./cmd/loadtest-verify; \
	if test "$${LOADTEST_KEEP_DATA:-true}" = "false"; then go run ./cmd/loadtest-seed --cleanup-only; fi

loadtest-purchase-run:
	@set -eu; \
	requested_run_id="$${LOADTEST_RUN_ID:-}"; \
	set -a; if test -f "$(LOADTEST_ENV_FILE)"; then . "$(LOADTEST_ENV_FILE)"; fi; set +a; \
	export LOADTEST_PROFILE="$(LOADTEST_PROFILE)"; \
	if test -n "$$requested_run_id"; then export LOADTEST_RUN_ID="$$requested_run_id"; fi; \
	export LOADTEST_SCENARIO=purchase_outcomes; \
	base_url="$${LOADTEST_BASE_URL:-http://localhost:8080}"; \
	database_url="$${LOADTEST_DATABASE_URL:-postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable}"; \
	export LOADTEST_DATABASE_URL="$$database_url"; \
	until curl --fail --silent --show-error "$$base_url/readyz" >/dev/null; do \
		echo "Waiting for $$base_url/readyz ..."; sleep 2; \
	done; \
	go run ./cmd/loadtest-seed; \
	data_file="$${LOADTEST_DATA_FILE:-loadtest/generated/data.json}"; \
	case "$$data_file" in /*) ;; *) data_file="$$PWD/$$data_file" ;; esac; \
	run_id="$${LOADTEST_RUN_ID:-local}"; \
	results_dir="$${LOADTEST_RESULTS_DIR:-loadtest/results}"; \
	events_file="$$results_dir/$$run_id/k6-events.log"; \
	mkdir -p "$$results_dir/$$run_id"; \
	rm -f "$$events_file"; \
	k6_status=0; \
	LOADTEST_DATA_FILE="$$data_file" k6 run --log-format=raw --log-output="file=$$events_file" \
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
	set -a; if test -f "$(LOADTEST_ENV_FILE)"; then . "$(LOADTEST_ENV_FILE)"; fi; set +a; \
	export LOADTEST_PROFILE="$(LOADTEST_PROFILE)"; \
	if test -n "$$requested_run_id"; then export LOADTEST_RUN_ID="$$requested_run_id"; fi; \
	go run ./cmd/loadtest-seed --cleanup-only; \
	run_id="$${LOADTEST_RUN_ID:-local}"; \
	case "$$run_id" in *[!A-Za-z0-9.-]*|'') echo "Unsafe LOADTEST_RUN_ID" >&2; exit 1;; esac; \
	find "loadtest/results/$$run_id" -mindepth 1 -delete 2>/dev/null || true; \
	rmdir "loadtest/results/$$run_id" 2>/dev/null || true; \
	find loadtest/generated -maxdepth 1 -type f ! -name .gitkeep -delete

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down
