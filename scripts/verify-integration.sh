#!/bin/sh

set -eu

project="goodqueue_verify_$$_$(date +%s)"
export COMPOSE_PROJECT_NAME="$project"
export GOODQUEUE_POSTGRES_PORT=0
export GOODQUEUE_HTTP_PORT=0
export GOODQUEUE_MOCK_API=false
export GOODQUEUE_MOCK_QUEUE_STATUS=waiting

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ -n "${business_body:-}" ]; then
		rm -f "$business_body"
	fi
	docker compose down --volumes --remove-orphans --rmi local >/dev/null 2>&1 || true
	exit "$status"
}
trap cleanup EXIT HUP INT TERM

wait_for_postgres() {
	attempt=0
	until docker compose exec -T postgres pg_isready -U goodqueue -d goodqueue >/dev/null 2>&1; do
		attempt=$((attempt + 1))
		if [ "$attempt" -ge 60 ]; then
			echo "PostgreSQL did not become ready" >&2
			return 1
		fi
		sleep 1
	done
}

assert_expiry_index_present() {
	result=$(docker compose exec -T postgres psql -U goodqueue -d goodqueue -Atc \
		"SELECT count(*) = 1 AND bool_and(indexdef LIKE '%(expires_at, id)%' AND indexdef LIKE '%status = ''active''::text%') FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'purchase_rights' AND indexname = 'purchase_rights_active_expiry_idx';")
	if [ "$result" != "t" ]; then
		echo "Active purchase-right expiry index is missing or malformed" >&2
		return 1
	fi
}

assert_expiry_index_absent() {
	result=$(docker compose exec -T postgres psql -U goodqueue -d goodqueue -Atc \
		"SELECT count(*) = 0 FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'purchase_rights' AND indexname = 'purchase_rights_active_expiry_idx';")
	if [ "$result" != "t" ]; then
		echo "Active purchase-right expiry index remains after migration down" >&2
		return 1
	fi
}

run_migration() {
	docker compose run --rm migrate -dir /app/migrations postgres \
		"postgres://goodqueue:goodqueue@postgres:5432/goodqueue?sslmode=disable" "$@"
}

docker compose build migrate backend
docker compose up -d postgres
wait_for_postgres

run_migration up
assert_expiry_index_present
run_migration down-to 0
assert_expiry_index_absent
run_migration up
assert_expiry_index_present

postgres_endpoint=$(docker compose port postgres 5432)
postgres_port=${postgres_endpoint##*:}
make jet-check DATABASE_URL="postgres://goodqueue:goodqueue@127.0.0.1:${postgres_port}/goodqueue?sslmode=disable"

docker compose up -d backend
backend_endpoint=$(docker compose port backend 8080)
backend_url="http://${backend_endpoint}"

attempt=0
until curl --fail --silent "${backend_url}/readyz" >/dev/null; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 60 ]; then
		echo "Backend did not become ready" >&2
		exit 1
	fi
	sleep 1
done

[ "$(curl --fail --silent "${backend_url}/healthz")" = '{"status":"ok"}' ]
[ "$(curl --fail --silent "${backend_url}/readyz")" = '{"status":"ok"}' ]
curl --fail --silent --location "${backend_url}/docs" >/dev/null
curl --fail --silent "${backend_url}/docs/index.html" >/dev/null
curl --fail --silent "${backend_url}/docs/doc.json" >/dev/null

business_body=$(mktemp)
business_status=$(curl --silent --output "$business_body" --write-out '%{http_code}' "${backend_url}/api/v1/products")
[ "$business_status" = "200" ]
grep -q '"id":"11111111-1111-1111-1111-111111111111"' "$business_body"
rm -f "$business_body"

backend_container=$(docker compose ps -q backend)
attempt=0
until [ "$(docker inspect --format '{{.State.Health.Status}}' "$backend_container")" = "healthy" ]; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 60 ]; then
		echo "Backend container did not become healthy" >&2
		exit 1
	fi
	sleep 1
done
