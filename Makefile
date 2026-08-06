GOOSE_DRIVER ?= postgres
DATABASE_URL ?= postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable
JET_OUTPUT ?= internal/repository/postgres/generated

.PHONY: build run test test-race test-e2e test-ac vet lint format format-check swagger swagger-check migrate-up migrate-down migrate-status jet-generate jet-check generate verify verify-integration verify-all load-test compose-up compose-down

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

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down
