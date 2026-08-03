GOOSE_DRIVER ?= postgres
DATABASE_URL ?= postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable
JET_OUTPUT ?= internal/repository/postgres/generated

.PHONY: build run test test-race vet lint format swagger swagger-check migrate-up migrate-down migrate-status jet-generate jet-check generate verify verify-integration verify-all compose-up compose-down

build:
	go build ./cmd/goodqueue-backend

run:
	go run ./cmd/goodqueue-backend

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

lint:
	go tool golangci-lint run ./...

format:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

swagger:
	go tool swag fmt -g cmd/goodqueue-backend/main.go
	go tool swag init -g cmd/goodqueue-backend/main.go -o internal/app/http/docs --parseInternal --packageName docs

swagger-check:
	@set -eu; \
	tmp=$$(mktemp -d); \
	trap 'status=$$?; rm -rf "$$tmp"; exit $$status' EXIT HUP INT TERM; \
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
	trap 'status=$$?; rm -rf "$$tmp"; exit $$status' EXIT HUP INT TERM; \
	go tool jet -dsn="$(DATABASE_URL)" -schema=public -path="$$tmp/generated" -ignore-tables=goose_db_version; \
	rm -rf "$(JET_OUTPUT)"; \
	mkdir -p "$$(dirname "$(JET_OUTPUT)")"; \
	mv "$$tmp/generated" "$(JET_OUTPUT)"

jet-check: migrate-up
	@set -eu; \
	tmp=$$(mktemp -d); \
	trap 'status=$$?; rm -rf "$$tmp"; exit $$status' EXIT HUP INT TERM; \
	go tool jet -dsn="$(DATABASE_URL)" -schema=public -path="$$tmp/generated" -ignore-tables=goose_db_version; \
	diff -ru "$(JET_OUTPUT)" "$$tmp/generated"

generate: swagger jet-generate

verify: swagger-check test test-race vet lint

verify-integration:
	sh scripts/verify-integration.sh

verify-all: verify verify-integration

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down
