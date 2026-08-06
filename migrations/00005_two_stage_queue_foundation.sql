-- +goose Up
-- This is an intentionally destructive development migration for queue and purchase-right test data.
DELETE FROM purchase_rights;
DELETE FROM queue_entries;

DROP TABLE purchase_rights;
DROP TABLE queue_entries;

ALTER TABLE products
    ADD COLUMN next_queue_sequence BIGINT NOT NULL DEFAULT 1;

UPDATE products
SET reserved = 0,
    next_queue_sequence = 1;

ALTER TABLE products
    ADD CONSTRAINT products_next_queue_sequence_positive CHECK (next_queue_sequence >= 1);

ALTER TABLE users ADD COLUMN external_user_id UUID;

UPDATE users
SET external_user_id = CASE id
    WHEN 1 THEN '00000000-0000-4000-8000-000000000001'::UUID
    WHEN 2 THEN '00000000-0000-4000-8000-000000000002'::UUID
    WHEN 3 THEN '00000000-0000-4000-8000-000000000003'::UUID
    WHEN 4 THEN '00000000-0000-4000-8000-000000000004'::UUID
    WHEN 5 THEN '00000000-0000-4000-8000-000000000005'::UUID
    ELSE md5('goodqueue-user:' || id::TEXT)::UUID
END;

ALTER TABLE users
    ALTER COLUMN external_user_id SET NOT NULL,
    ADD CONSTRAINT users_external_user_id_unique UNIQUE (external_user_id);

CREATE TABLE queue_attempts (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    queue_sequence BIGINT NOT NULL CHECK (queue_sequence >= 1),
    external_user_id VARCHAR(255) NOT NULL
        CHECK (external_user_id = btrim(external_user_id) AND length(external_user_id) BETWEEN 1 AND 255),
    idempotency_key VARCHAR(128) NOT NULL
        CHECK (idempotency_key ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    state TEXT NOT NULL CHECK (state IN (
        'waiting', 'invited', 'checkout', 'purchased', 'invite_expired',
        'checkout_expired', 'payment_failed', 'cancelled', 'sold_out'
    )),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    invited_at TIMESTAMPTZ,
    invitation_deadline TIMESTAMPTZ,
    checkout_started_at TIMESTAMPTZ,
    checkout_deadline TIMESTAMPTZ,
    terminal_at TIMESTAMPTZ,
    purchased_at TIMESTAMPTZ,
    accepted_payment_provider VARCHAR(64),
    accepted_payment_reference VARCHAR(200),
    terminal_reason VARCHAR(64),
    terminal_message VARCHAR(500),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    CONSTRAINT queue_attempts_product_sequence_unique UNIQUE (product_id, queue_sequence),
    CONSTRAINT queue_attempts_idempotency_unique UNIQUE (external_user_id, product_id, idempotency_key),
    CONSTRAINT queue_attempts_updated_after_created CHECK (updated_at >= created_at),
    CONSTRAINT queue_attempts_timestamps_after_created CHECK (
        (invited_at IS NULL OR invited_at >= created_at)
        AND (checkout_started_at IS NULL OR checkout_started_at >= created_at)
        AND (terminal_at IS NULL OR terminal_at >= created_at)
        AND (purchased_at IS NULL OR purchased_at >= created_at)
    ),
    CONSTRAINT queue_attempts_invitation_pair CHECK (
        (invited_at IS NULL AND invitation_deadline IS NULL)
        OR (invited_at IS NOT NULL AND invitation_deadline IS NOT NULL AND invitation_deadline > invited_at)
    ),
    CONSTRAINT queue_attempts_checkout_pair CHECK (
        (checkout_started_at IS NULL AND checkout_deadline IS NULL)
        OR (
            checkout_started_at IS NOT NULL
            AND checkout_deadline IS NOT NULL
            AND invited_at IS NOT NULL
            AND invitation_deadline IS NOT NULL
            AND checkout_started_at >= invited_at
            AND checkout_started_at <= invitation_deadline
            AND checkout_deadline > checkout_started_at
        )
    ),
    CONSTRAINT queue_attempts_terminal_after_transitions CHECK (
        terminal_at IS NULL
        OR (
            (invited_at IS NULL OR terminal_at >= invited_at)
            AND (checkout_started_at IS NULL OR terminal_at >= checkout_started_at)
        )
    ),
    CONSTRAINT queue_attempts_terminal_fields CHECK (
        (state IN ('waiting', 'invited', 'checkout') AND terminal_at IS NULL AND terminal_reason IS NULL AND terminal_message IS NULL)
        OR (state IN ('purchased', 'invite_expired', 'checkout_expired', 'payment_failed', 'cancelled', 'sold_out')
            AND terminal_at IS NOT NULL AND terminal_reason IS NOT NULL)
    ),
    CONSTRAINT queue_attempts_state_timestamps CHECK (
        (state = 'waiting' AND invited_at IS NULL AND checkout_started_at IS NULL AND purchased_at IS NULL)
        OR (state = 'invited' AND invited_at IS NOT NULL AND checkout_started_at IS NULL AND purchased_at IS NULL)
        OR (state = 'checkout' AND invited_at IS NOT NULL AND checkout_started_at IS NOT NULL AND purchased_at IS NULL)
        OR (state = 'purchased' AND invited_at IS NOT NULL AND checkout_started_at IS NOT NULL
            AND purchased_at IS NOT NULL AND purchased_at = terminal_at)
        OR (state = 'invite_expired' AND invited_at IS NOT NULL AND checkout_started_at IS NULL AND purchased_at IS NULL
            AND terminal_at >= invitation_deadline)
        OR (state = 'checkout_expired' AND invited_at IS NOT NULL AND checkout_started_at IS NOT NULL
            AND purchased_at IS NULL AND terminal_at >= checkout_deadline)
        OR (state = 'payment_failed' AND invited_at IS NOT NULL AND checkout_started_at IS NOT NULL
            AND purchased_at IS NULL AND terminal_at >= checkout_started_at)
        OR (state = 'sold_out' AND invited_at IS NULL AND checkout_started_at IS NULL AND purchased_at IS NULL)
        OR (state = 'cancelled' AND purchased_at IS NULL)
    ),
    CONSTRAINT queue_attempts_accepted_payment_owner CHECK (
        (state = 'purchased'
            AND accepted_payment_provider IS NOT NULL
            AND accepted_payment_provider = btrim(accepted_payment_provider)
            AND octet_length(accepted_payment_provider) BETWEEN 1 AND 64
            AND accepted_payment_reference IS NOT NULL
            AND accepted_payment_reference = btrim(accepted_payment_reference)
            AND octet_length(accepted_payment_reference) BETWEEN 1 AND 200)
        OR (state <> 'purchased'
            AND accepted_payment_provider IS NULL
            AND accepted_payment_reference IS NULL)
    )
);

CREATE UNIQUE INDEX queue_attempts_one_active_per_user_product_idx
    ON queue_attempts (external_user_id, product_id)
    WHERE state IN ('waiting', 'invited', 'checkout');

CREATE UNIQUE INDEX queue_attempts_one_purchase_per_user_product_idx
    ON queue_attempts (external_user_id, product_id)
    WHERE state = 'purchased';

CREATE UNIQUE INDEX queue_attempts_accepted_payment_reference_unique_idx
    ON queue_attempts (accepted_payment_provider, accepted_payment_reference)
    WHERE state = 'purchased';

CREATE INDEX queue_attempts_waiting_fifo_idx
    ON queue_attempts (product_id, queue_sequence)
    WHERE state = 'waiting';

CREATE INDEX queue_attempts_invitation_expiry_idx
    ON queue_attempts (invitation_deadline, product_id, queue_sequence)
    WHERE state = 'invited';

CREATE INDEX queue_attempts_checkout_expiry_idx
    ON queue_attempts (checkout_deadline, product_id, queue_sequence)
    WHERE state = 'checkout';

CREATE TABLE notification_outbox (
    id UUID PRIMARY KEY,
    attempt_id UUID REFERENCES queue_attempts(id) ON DELETE RESTRICT,
    event_type VARCHAR(100) NOT NULL CHECK (event_type = btrim(event_type) AND length(event_type) BETWEEN 1 AND 100),
    deduplication_key VARCHAR(200) NOT NULL UNIQUE
        CHECK (deduplication_key = btrim(deduplication_key) AND length(deduplication_key) BETWEEN 1 AND 200),
    payload JSONB NOT NULL,
    payload_hash BYTEA NOT NULL CHECK (octet_length(payload_hash) = 32),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'sent', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    lease_until TIMESTAMPTZ,
    lease_token UUID,
    lease_generation BIGINT NOT NULL DEFAULT 0 CHECK (lease_generation >= 0),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    sent_at TIMESTAMPTZ,
    CHECK (updated_at >= created_at),
    CHECK ((lease_until IS NULL) = (lease_token IS NULL)),
    CHECK ((status = 'processing' AND lease_until IS NOT NULL) OR (status <> 'processing' AND lease_until IS NULL)),
    CHECK ((status = 'sent' AND sent_at IS NOT NULL) OR (status <> 'sent' AND sent_at IS NULL))
);

CREATE INDEX notification_outbox_claim_idx
    ON notification_outbox (available_at, created_at, id)
    WHERE status IN ('pending', 'failed');

CREATE INDEX notification_outbox_lease_expiry_idx
    ON notification_outbox (lease_until, id)
    WHERE status = 'processing';

CREATE TABLE payment_inbox (
    id UUID PRIMARY KEY,
    provider VARCHAR(64) NOT NULL CHECK (provider = btrim(provider) AND octet_length(provider) BETWEEN 1 AND 64),
    event_id VARCHAR(200) NOT NULL CHECK (event_id = btrim(event_id) AND octet_length(event_id) BETWEEN 1 AND 200),
    attempt_id UUID NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('succeeded', 'failed')),
    payment_reference VARCHAR(200),
    payload BYTEA NOT NULL,
    payload_hash BYTEA NOT NULL CHECK (octet_length(payload_hash) = 32),
    status TEXT NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'processing', 'retry', 'completed', 'rejected')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    lease_until TIMESTAMPTZ,
    claim_token UUID,
    claim_generation BIGINT NOT NULL DEFAULT 0 CHECK (claim_generation >= 0),
    last_error TEXT,
    response_http_status SMALLINT CHECK (response_http_status BETWEEN 100 AND 599),
    response_body BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT payment_inbox_provider_event_unique UNIQUE (provider, event_id),
    CONSTRAINT payment_inbox_reference_by_outcome CHECK (
        (outcome = 'succeeded' AND payment_reference IS NOT NULL
            AND payment_reference = btrim(payment_reference) AND octet_length(payment_reference) BETWEEN 1 AND 200)
        OR (outcome = 'failed' AND payment_reference IS NULL)
    ),
    CHECK (updated_at >= created_at),
    CHECK ((lease_until IS NULL) = (claim_token IS NULL)),
    CHECK ((status = 'processing' AND lease_until IS NOT NULL) OR (status <> 'processing' AND lease_until IS NULL)),
    CHECK ((response_http_status IS NULL) = (response_body IS NULL)),
    CHECK ((status IN ('completed', 'rejected') AND response_http_status IS NOT NULL AND completed_at IS NOT NULL)
        OR (status NOT IN ('completed', 'rejected') AND completed_at IS NULL))
);

CREATE INDEX payment_inbox_claim_idx
    ON payment_inbox (next_attempt_at, created_at, id)
    WHERE status IN ('received', 'retry');

CREATE INDEX payment_inbox_lease_expiry_idx
    ON payment_inbox (lease_until, id)
    WHERE status = 'processing';

CREATE TABLE inventory_adjustments (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    adjustment_key VARCHAR(128) NOT NULL
        CHECK (adjustment_key ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    delta INTEGER NOT NULL CHECK (delta <> 0),
    payload_hash BYTEA NOT NULL CHECK (octet_length(payload_hash) = 32),
    status TEXT NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'applied', 'rejected')),
    stock_before INTEGER,
    stock_after INTEGER,
    response_http_status SMALLINT CHECK (response_http_status BETWEEN 100 AND 599),
    response_body BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT inventory_adjustments_product_key_unique UNIQUE (product_id, adjustment_key),
    CHECK ((stock_before IS NULL AND stock_after IS NULL)
        OR (stock_before >= 0 AND stock_after >= 0 AND stock_before + delta = stock_after)),
    CHECK ((response_http_status IS NULL) = (response_body IS NULL)),
    CHECK ((status IN ('applied', 'rejected') AND response_http_status IS NOT NULL AND completed_at IS NOT NULL)
        OR (status = 'received' AND completed_at IS NULL))
);

CREATE INDEX inventory_adjustments_created_idx
    ON inventory_adjustments (product_id, created_at, id);

-- +goose Down
DROP TABLE inventory_adjustments;
DROP TABLE payment_inbox;
DROP TABLE notification_outbox;
DROP TABLE queue_attempts;

ALTER TABLE users DROP CONSTRAINT users_external_user_id_unique;
ALTER TABLE users DROP COLUMN external_user_id;

ALTER TABLE products DROP CONSTRAINT products_next_queue_sequence_positive;
ALTER TABLE products
    DROP COLUMN next_queue_sequence;

CREATE TABLE queue_entries (
    ticket_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    external_user_id VARCHAR(255) NOT NULL CHECK (length(btrim(external_user_id)) BETWEEN 1 AND 255),
    idempotency_key UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('waiting', 'right_issued', 'completed', 'cancelled', 'expired')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    right_issued_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    UNIQUE (external_user_id, idempotency_key),
    CHECK (right_issued_at IS NULL OR right_issued_at >= joined_at),
    CHECK (completed_at IS NULL OR completed_at >= joined_at),
    CHECK (cancelled_at IS NULL OR cancelled_at >= joined_at),
    CHECK (expired_at IS NULL OR expired_at >= joined_at),
    CHECK (
        (status = 'waiting' AND right_issued_at IS NULL AND completed_at IS NULL AND cancelled_at IS NULL AND expired_at IS NULL)
        OR (status = 'right_issued' AND right_issued_at IS NOT NULL AND completed_at IS NULL AND cancelled_at IS NULL AND expired_at IS NULL)
        OR (status = 'completed' AND right_issued_at IS NOT NULL AND completed_at IS NOT NULL AND completed_at >= right_issued_at AND cancelled_at IS NULL AND expired_at IS NULL)
        OR (status = 'cancelled' AND right_issued_at IS NULL AND completed_at IS NULL AND cancelled_at IS NOT NULL AND expired_at IS NULL)
        OR (status = 'expired' AND right_issued_at IS NOT NULL AND completed_at IS NULL AND cancelled_at IS NULL AND expired_at IS NOT NULL AND expired_at >= right_issued_at)
    )
);

CREATE UNIQUE INDEX queue_entries_one_open_per_product_user_idx
    ON queue_entries (product_id, external_user_id)
    WHERE status IN ('waiting', 'right_issued');

CREATE INDEX queue_entries_fifo_idx
    ON queue_entries (product_id, ticket_id)
    WHERE status = 'waiting';

CREATE TABLE purchase_rights (
    id UUID PRIMARY KEY,
    queue_ticket_id BIGINT NOT NULL UNIQUE REFERENCES queue_entries(ticket_id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('active', 'expired', 'consumed')),
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    CHECK (expires_at > issued_at),
    CHECK (
        (status IN ('active', 'expired') AND consumed_at IS NULL)
        OR (status = 'consumed' AND consumed_at IS NOT NULL AND consumed_at >= issued_at AND consumed_at <= expires_at)
    )
);

CREATE INDEX purchase_rights_active_expiry_idx
    ON purchase_rights (expires_at, id)
    WHERE status = 'active';

CREATE INDEX idx_purchase_rights_product_status
    ON purchase_rights (product_id, status)
    WHERE status = 'active';

CREATE INDEX idx_purchase_rights_expires_status
    ON purchase_rights (expires_at, status)
    WHERE status = 'active';
