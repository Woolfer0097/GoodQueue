#!/bin/sh

set -eu

# Git Bash must pass container paths such as /app/migrations to Docker unchanged,
# while local Go tools still need normal MSYS-to-Windows path conversion.
docker() {
	MSYS_NO_PATHCONV=1 command docker "$@"
}

project="goodqueue_verify_$$_$(date +%s)"
export COMPOSE_PROJECT_NAME="$project"
export GOODQUEUE_POSTGRES_PORT=0
export GOODQUEUE_HTTP_PORT=0

cleanup() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ -n "${response_body:-}" ]; then
		rm -f "$response_body"
	fi
	docker compose down --volumes --remove-orphans --rmi local >/dev/null 2>&1 || true
	exit "$status"
}

wait_for_backend() {
	attempt=0
	until curl --fail --silent "${backend_url}/readyz" >/dev/null; do
		attempt=$((attempt + 1))
		if [ "$attempt" -ge 60 ]; then
			echo "Backend did not become ready" >&2
			return 1
		fi
		sleep 1
	done
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

query_postgres() {
	docker compose exec -T postgres psql -U goodqueue -d goodqueue -Atc "$1"
}

assert_postgres_rejects() {
	description=$1
	statement=$2
	if docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U goodqueue -d goodqueue -c "$statement" >/dev/null 2>&1; then
		echo "PostgreSQL accepted invalid queue chronology: $description" >&2
		return 1
	fi
}

assert_product_ttls_preserved() {
	result=$(query_postgres "
		SELECT array_agg(right_ttl_seconds ORDER BY id) = ARRAY[137, 421, 86399]
		FROM products
		WHERE id IN (
			'11111111-1111-1111-1111-111111111111',
			'22222222-2222-2222-2222-222222222222',
			'33333333-3333-3333-3333-333333333333'
		);")
	if [ "$result" != "t" ]; then
		echo "Product right_ttl_seconds values were not preserved" >&2
		return 1
	fi
}

capture_preservation_fingerprints() {
	products_before=$(query_postgres "
		SELECT md5(string_agg(concat_ws('|', id::text, title, description, image_url,
			queue_enabled::text, allocatable_stock::text, right_ttl_seconds::text,
			created_at::text, updated_at::text), E'\\n' ORDER BY id))
		FROM products
		WHERE id IN (
			'11111111-1111-1111-1111-111111111111',
			'22222222-2222-2222-2222-222222222222',
			'33333333-3333-3333-3333-333333333333'
		);")
	users_before=$(query_postgres "
		SELECT md5(string_agg(concat_ws('|', id::text, name, created_at::text), E'\\n' ORDER BY id))
		FROM users;")
}

assert_products_and_users_preserved() {
	products_after=$(query_postgres "
		SELECT md5(string_agg(concat_ws('|', id::text, title, description, image_url,
			queue_enabled::text, allocatable_stock::text, right_ttl_seconds::text,
			created_at::text, updated_at::text), E'\\n' ORDER BY id))
		FROM products
		WHERE id IN (
			'11111111-1111-1111-1111-111111111111',
			'22222222-2222-2222-2222-222222222222',
			'33333333-3333-3333-3333-333333333333'
		);")
	users_after=$(query_postgres "
		SELECT md5(string_agg(concat_ws('|', id::text, name, created_at::text), E'\\n' ORDER BY id))
		FROM users;")
	if [ "$products_after" != "$products_before" ] || [ "$users_after" != "$users_before" ]; then
		echo "Migration changed preserved product or user fields" >&2
		return 1
	fi
}

assert_legacy_schema_present() {
	result=$(docker compose exec -T postgres psql -U goodqueue -d goodqueue -Atc \
		"SELECT to_regclass('public.queue_entries') IS NOT NULL AND to_regclass('public.purchase_rights') IS NOT NULL AND to_regclass('public.queue_attempts') IS NULL;")
	if [ "$result" != "t" ]; then
		echo "Legacy queue schema is missing" >&2
		return 1
	fi
}

assert_phase_one_schema() {
	result=$(query_postgres "
		SELECT
			to_regclass('public.queue_entries') IS NULL
			AND to_regclass('public.purchase_rights') IS NULL
			AND to_regclass('public.queue_attempts') IS NOT NULL
			AND to_regclass('public.notification_outbox') IS NOT NULL
			AND to_regclass('public.payment_inbox') IS NOT NULL
			AND to_regclass('public.inventory_adjustments') IS NOT NULL
			AND (SELECT count(*) FROM products) = 12
			AND (SELECT count(*) FROM users) = 5
			AND (SELECT count(*) FROM queue_attempts) = 0
			AND (SELECT bool_and(reserved = 0 AND next_queue_sequence = 1) FROM products)
			AND (SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public'
				AND table_name = 'products' AND column_name = 'right_ttl_seconds') = 1
			AND (SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public'
				AND table_name = 'products' AND column_name IN ('category', 'price_cents')) = 2
			AND to_regclass('public.product_embeddings') IS NOT NULL
			AND EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')
			AND (SELECT count(DISTINCT external_user_id) FROM users) = 5
			AND (SELECT count(*) FROM users WHERE external_user_id IN (
				'00000000-0000-4000-8000-000000000001',
				'00000000-0000-4000-8000-000000000002',
				'00000000-0000-4000-8000-000000000003',
				'00000000-0000-4000-8000-000000000004',
				'00000000-0000-4000-8000-000000000005'
			)) = 5
			AND (SELECT count(*) FROM pg_constraint WHERE conname IN (
				'check_reserved_leq_allocatable',
				'products_next_queue_sequence_positive',
				'queue_attempts_product_sequence_unique',
				'queue_attempts_idempotency_unique',
				'queue_attempts_invitation_pair',
				'queue_attempts_checkout_pair',
				'queue_attempts_terminal_after_transitions',
				'queue_attempts_terminal_fields',
				'queue_attempts_state_timestamps',
				'queue_attempts_accepted_payment_owner',
				'payment_inbox_provider_event_unique',
				'payment_inbox_reference_by_outcome',
				'inventory_adjustments_product_key_unique'
			)) = 13
			AND (SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public'
				AND table_name = 'payment_inbox'
				AND column_name IN ('lease_until', 'claim_token', 'claim_generation', 'attempt_count', 'next_attempt_at')) = 5
			AND (SELECT count(*) FROM pg_constraint
				WHERE contype = 'c' AND pg_get_constraintdef(oid) LIKE '%octet_length(payload_hash) = 32%') = 3
			AND (SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'queue_attempts'
				AND indexname IN (
					'queue_attempts_one_active_per_user_product_idx',
					'queue_attempts_one_purchase_per_user_product_idx',
					'queue_attempts_waiting_fifo_idx',
					'queue_attempts_invitation_expiry_idx',
					'queue_attempts_checkout_expiry_idx'
				)) = 5
			AND (SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'queue_attempts'
				AND indexname = 'queue_attempts_accepted_payment_reference_unique_idx') = 1
			AND (SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'payment_inbox'
				AND indexname = 'payment_inbox_accepted_reference_unique_idx') = 0;")
	if [ "$result" != "t" ]; then
		echo "Phase 1 schema invariants are missing" >&2
		return 1
	fi
}

seed_legacy_queue_data() {
	query_postgres "
		WITH inserted_entry AS (
			INSERT INTO queue_entries (product_id, external_user_id, idempotency_key, status, right_issued_at)
			VALUES ('11111111-1111-1111-1111-111111111111', 'legacy-user', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'right_issued', clock_timestamp())
			RETURNING ticket_id, product_id, right_issued_at
		)
		INSERT INTO purchase_rights (id, queue_ticket_id, product_id, status, issued_at, expires_at)
		SELECT 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', ticket_id, product_id, 'active', right_issued_at, right_issued_at + interval '10 minutes'
		FROM inserted_entry;
		UPDATE products SET reserved = 1 WHERE id = '11111111-1111-1111-1111-111111111111';" >/dev/null
}

seed_distinct_product_ttls() {
	query_postgres "
		UPDATE products
		SET right_ttl_seconds = CASE id
			WHEN '11111111-1111-1111-1111-111111111111'::UUID THEN 137
			WHEN '22222222-2222-2222-2222-222222222222'::UUID THEN 421
			WHEN '33333333-3333-3333-3333-333333333333'::UUID THEN 86399
		END
		WHERE id IN (
			'11111111-1111-1111-1111-111111111111',
			'22222222-2222-2222-2222-222222222222',
			'33333333-3333-3333-3333-333333333333'
		);" >/dev/null
}

assert_queue_attempt_chronology() {
	assert_postgres_rejects "purchase before checkout start" "
		INSERT INTO queue_attempts (
			id, product_id, queue_sequence, external_user_id, idempotency_key, state,
			created_at, updated_at, invited_at, invitation_deadline,
			checkout_started_at, checkout_deadline, terminal_at, purchased_at, terminal_reason,
			accepted_payment_provider, accepted_payment_reference
		) VALUES (
			'10000000-0000-4000-8000-000000000001', '11111111-1111-1111-1111-111111111111', 1,
			'chronology-purchased-invalid', 'chronology-purchased-invalid', 'purchased',
			'2026-01-01 10:00:00+00', '2026-01-01 10:03:00+00',
			'2026-01-01 10:01:00+00', '2026-01-01 10:06:00+00',
			'2026-01-01 10:02:00+00', '2026-01-01 10:07:00+00',
			'2026-01-01 10:01:30+00', '2026-01-01 10:01:30+00', 'purchased', 'test', 'chronology-invalid'
		);"

	assert_postgres_rejects "checkout after invitation deadline" "
		INSERT INTO queue_attempts (
			id, product_id, queue_sequence, external_user_id, idempotency_key, state,
			created_at, updated_at, invited_at, invitation_deadline, checkout_started_at, checkout_deadline
		) VALUES (
			'10000000-0000-4000-8000-000000000002', '11111111-1111-1111-1111-111111111111', 2,
			'chronology-checkout-invalid', 'chronology-checkout-invalid', 'checkout',
			'2026-01-01 10:00:00+00', '2026-01-01 10:04:00+00',
			'2026-01-01 10:01:00+00', '2026-01-01 10:02:00+00',
			'2026-01-01 10:03:00+00', '2026-01-01 10:08:00+00'
		);"

	assert_postgres_rejects "cancellation before invitation" "
		INSERT INTO queue_attempts (
			id, product_id, queue_sequence, external_user_id, idempotency_key, state,
			created_at, updated_at, invited_at, invitation_deadline, terminal_at, terminal_reason
		) VALUES (
			'10000000-0000-4000-8000-000000000003', '11111111-1111-1111-1111-111111111111', 3,
			'chronology-cancel-invite-invalid', 'chronology-cancel-invite-invalid', 'cancelled',
			'2026-01-01 10:00:00+00', '2026-01-01 10:03:00+00',
			'2026-01-01 10:02:00+00', '2026-01-01 10:07:00+00',
			'2026-01-01 10:01:00+00', 'cancelled'
		);"

	assert_postgres_rejects "cancellation before checkout start" "
		INSERT INTO queue_attempts (
			id, product_id, queue_sequence, external_user_id, idempotency_key, state,
			created_at, updated_at, invited_at, invitation_deadline,
			checkout_started_at, checkout_deadline, terminal_at, terminal_reason
		) VALUES (
			'10000000-0000-4000-8000-000000000004', '11111111-1111-1111-1111-111111111111', 4,
			'chronology-cancel-checkout-invalid', 'chronology-cancel-checkout-invalid', 'cancelled',
			'2026-01-01 10:00:00+00', '2026-01-01 10:04:00+00',
			'2026-01-01 10:01:00+00', '2026-01-01 10:06:00+00',
			'2026-01-01 10:03:00+00', '2026-01-01 10:08:00+00',
			'2026-01-01 10:02:00+00', 'cancelled'
		);"

	query_postgres "
		INSERT INTO queue_attempts (
			id, product_id, queue_sequence, external_user_id, idempotency_key, state,
			created_at, updated_at, terminal_at, terminal_reason
		) VALUES (
			'20000000-0000-4000-8000-000000000001', '11111111-1111-1111-1111-111111111111', 10,
			'chronology-waiting-cancel-valid', 'chronology-waiting-cancel-valid', 'cancelled',
			'2026-01-01 10:00:00+00', '2026-01-01 10:01:00+00',
			'2026-01-01 10:01:00+00', 'cancelled'
		);
		INSERT INTO queue_attempts (
			id, product_id, queue_sequence, external_user_id, idempotency_key, state,
			created_at, updated_at, invited_at, invitation_deadline,
			checkout_started_at, checkout_deadline, terminal_at, terminal_reason
		) VALUES (
			'20000000-0000-4000-8000-000000000002', '11111111-1111-1111-1111-111111111111', 11,
			'chronology-checkout-cancel-valid', 'chronology-checkout-cancel-valid', 'cancelled',
			'2026-01-01 10:00:00+00', '2026-01-01 10:04:00+00',
			'2026-01-01 10:01:00+00', '2026-01-01 10:06:00+00',
			'2026-01-01 10:03:00+00', '2026-01-01 10:08:00+00',
			'2026-01-01 10:04:00+00', 'cancelled'
		);
		INSERT INTO queue_attempts (
			id, product_id, queue_sequence, external_user_id, idempotency_key, state,
			created_at, updated_at, invited_at, invitation_deadline,
			checkout_started_at, checkout_deadline, terminal_at, purchased_at, terminal_reason,
			accepted_payment_provider, accepted_payment_reference
		) VALUES (
			'20000000-0000-4000-8000-000000000003', '11111111-1111-1111-1111-111111111111', 12,
			'chronology-purchased-valid', 'chronology-purchased-valid', 'purchased',
			'2026-01-01 10:00:00+00', '2026-01-01 10:05:00+00',
			'2026-01-01 10:01:00+00', '2026-01-01 10:06:00+00',
			'2026-01-01 10:03:00+00', '2026-01-01 10:08:00+00',
			'2026-01-01 10:05:00+00', '2026-01-01 10:05:00+00', 'purchased', 'test', 'chronology-valid'
		);" >/dev/null

	result=$(query_postgres "SELECT count(*) = 3 FROM queue_attempts WHERE id::TEXT LIKE '20000000-%';")
	if [ "$result" != "t" ]; then
		echo "Valid queue chronology was not accepted" >&2
		return 1
	fi
}

run_migration() {
	docker compose run --rm migrate -dir /app/migrations postgres \
		"postgres://goodqueue:goodqueue@postgres:5432/goodqueue?sslmode=disable" "$@"
}

assert_loadtest_schema() {
	result=$(query_postgres "
		SELECT to_regclass('loadtest.runs') IS NOT NULL
		   AND to_regclass('loadtest.request_logs') IS NOT NULL
		   AND (SELECT count(*) = 3 FROM information_schema.columns
		        WHERE table_schema = 'loadtest' AND table_name = 'runs'
		          AND column_name IN ('planned_queue_rejected', 'planned_sold_out', 'planned_unresolved'))
		   AND EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conrelid = 'loadtest.request_logs'::regclass AND contype = 'f'
		   );")
	if [ "$result" != "t" ]; then
		echo "Permanent loadtest reporting schema is incomplete" >&2
		return 1
	fi
}

docker compose build backend
docker compose up -d postgres
wait_for_postgres

run_migration up-to 4
assert_legacy_schema_present
seed_distinct_product_ttls
assert_product_ttls_preserved
seed_legacy_queue_data
capture_preservation_fingerprints
run_migration up
assert_phase_one_schema
assert_loadtest_schema
assert_product_ttls_preserved
assert_products_and_users_preserved
assert_queue_attempt_chronology
run_migration down-to 4
assert_legacy_schema_present
assert_product_ttls_preserved
assert_products_and_users_preserved
result=$(query_postgres "SELECT (SELECT count(*) FROM products) = 3 AND (SELECT count(*) FROM users) = 5 AND (SELECT bool_and(reserved = 0) FROM products);")
[ "$result" = "t" ]
run_migration up
assert_phase_one_schema
assert_loadtest_schema
assert_product_ttls_preserved
assert_products_and_users_preserved
assert_queue_attempt_chronology

postgres_endpoint=$(docker compose port postgres 5432)
postgres_port=${postgres_endpoint##*:}
make jet-check DATABASE_URL="postgres://goodqueue:goodqueue@127.0.0.1:${postgres_port}/goodqueue?sslmode=disable"
GOODQUEUE_TEST_DATABASE_URL="postgres://goodqueue:goodqueue@127.0.0.1:${postgres_port}/goodqueue?sslmode=disable" \
	go test ./internal/repository/postgres -run Integration -count=1
GOODQUEUE_TEST_DATABASE_URL="postgres://goodqueue:goodqueue@127.0.0.1:${postgres_port}/goodqueue?sslmode=disable" \
	go test ./internal/loadtest -run Integration -count=1

query_postgres "TRUNCATE notification_outbox, payment_inbox, inventory_adjustments, queue_attempts; UPDATE products SET reserved=0, next_queue_sequence=1; UPDATE products SET allocatable_stock=1 WHERE id='11111111-1111-1111-1111-111111111111'; UPDATE products SET allocatable_stock=3 WHERE id='22222222-2222-2222-2222-222222222222'; UPDATE products SET allocatable_stock=0 WHERE id='33333333-3333-3333-3333-333333333333';" >/dev/null

export GOODQUEUE_UNSAFE_PAYMENT_CALLBACK=false
docker compose up -d backend
backend_endpoint=$(docker compose port backend 8080)
backend_url="http://${backend_endpoint}"
wait_for_backend

[ "$(curl --fail --silent "${backend_url}/healthz")" = '{"status":"ok"}' ]
[ "$(curl --fail --silent "${backend_url}/readyz")" = '{"status":"ok"}' ]
curl --fail --silent --location "${backend_url}/docs" >/dev/null
curl --fail --silent "${backend_url}/docs/index.html" >/dev/null
curl --fail --silent "${backend_url}/docs/doc.json" >/dev/null

response_body=$(mktemp)
[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' "${backend_url}/api/v1/products")" = "200" ]
[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' "${backend_url}/api/v1/demo/users")" = "200" ]
[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' "${backend_url}/internal/v1/loadtest/request-success-rate")" = "503" ]
grep -q '"code":"metrics_unavailable"' "$response_body"

product_one='11111111-1111-1111-1111-111111111111'
product_two='22222222-2222-2222-2222-222222222222'
product_three='33333333-3333-3333-3333-333333333333'
user_one='00000000-0000-4000-8000-000000000001'
user_two='00000000-0000-4000-8000-000000000002'

[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' \
	-H "X-User-ID: ${user_one}" -H 'Idempotency-Key: runtime-join-1' \
	-X POST "${backend_url}/api/v1/products/${product_one}/queue-entries")" = "201" ]
[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' \
	-H "X-User-ID: ${user_one}" "${backend_url}/api/v1/products/${product_one}/queue-entry")" = "200" ]
attempt_id=$(query_postgres "SELECT id FROM queue_attempts WHERE product_id='${product_one}' AND external_user_id='${user_one}'")
[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' \
	-H "X-User-ID: ${user_one}" -X POST "${backend_url}/api/v1/queue-attempts/${attempt_id}/checkout")" = "200" ]

disabled_callback_before=$(query_postgres "
	SELECT concat_ws('|',
		(SELECT state FROM queue_attempts WHERE id='${attempt_id}'),
		(SELECT reserved FROM products WHERE id='${product_one}'),
		(SELECT allocatable_stock FROM products WHERE id='${product_one}'),
		(SELECT count(*) FROM payment_inbox));")
[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' \
	-H 'Content-Type: application/json' -X POST "${backend_url}/internal/v1/payment-events" \
	-d "{\"provider\":\"demo\",\"event_id\":\"runtime-payment-1\",\"attempt_id\":\"${attempt_id}\",\"outcome\":\"succeeded\",\"payment_reference\":\"runtime-reference-1\"}")" = "404" ]
disabled_callback_after=$(query_postgres "
	SELECT concat_ws('|',
		(SELECT state FROM queue_attempts WHERE id='${attempt_id}'),
		(SELECT reserved FROM products WHERE id='${product_one}'),
		(SELECT allocatable_stock FROM products WHERE id='${product_one}'),
		(SELECT count(*) FROM payment_inbox));")
[ "$disabled_callback_after" = "$disabled_callback_before" ]

export GOODQUEUE_UNSAFE_PAYMENT_CALLBACK=true
docker compose up -d --force-recreate backend
backend_endpoint=$(docker compose port backend 8080)
backend_url="http://${backend_endpoint}"
wait_for_backend
[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' \
	-H 'Content-Type: application/json' -X POST "${backend_url}/internal/v1/payment-events" \
	-d "{\"provider\":\"demo\",\"event_id\":\"runtime-payment-1\",\"attempt_id\":\"${attempt_id}\",\"outcome\":\"succeeded\",\"payment_reference\":\"runtime-reference-1\"}")" = "200" ]

[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' \
	-H "X-User-ID: ${user_two}" -H 'Idempotency-Key: runtime-join-2' \
	-X POST "${backend_url}/api/v1/products/${product_two}/queue-entries")" = "201" ]
[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' \
	-H "X-User-ID: ${user_two}" -X DELETE "${backend_url}/api/v1/products/${product_two}/queue-entry")" = "204" ]

[ "$(curl --silent --output "$response_body" --write-out '%{http_code}' \
	-H 'Content-Type: application/json' -H 'Idempotency-Key: runtime-stock-1' \
	-X POST "${backend_url}/internal/v1/products/${product_three}/stock-adjustments" \
	-d '{"delta":1,"reason":"runtime verification","external_reference":"runtime-stock-reference-1"}')" = "200" ]

[ "$(query_postgres "SELECT (SELECT state='purchased' FROM queue_attempts WHERE id='${attempt_id}') AND (SELECT allocatable_stock=1 FROM products WHERE id='${product_three}')")" = "t" ]
rm -f "$response_body"

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

GOODQUEUE_E2E_BASE_URL="$backend_url" \
	GOODQUEUE_E2E_DATABASE_URL="postgres://goodqueue:goodqueue@127.0.0.1:${postgres_port}/goodqueue?sslmode=disable" \
	make test-e2e
