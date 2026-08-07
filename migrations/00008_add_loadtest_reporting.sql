-- +goose Up
CREATE SCHEMA loadtest;

CREATE TABLE loadtest.runs (
    run_id VARCHAR(40) PRIMARY KEY
        CHECK (run_id ~ '^[A-Za-z0-9][A-Za-z0-9.-]{0,39}$'),
    scenario VARCHAR(32) NOT NULL
        CHECK (scenario IN ('queue_join_polling', 'purchase_outcomes')),
    profile VARCHAR(16) NOT NULL
        CHECK (profile IN ('smoke', 'medium', 'main')),
    random_seed BIGINT NOT NULL,
    effective_config JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'seeded'
        CHECK (status IN ('seeded', 'completed', 'failed')),
    planned_purchase INTEGER NOT NULL DEFAULT 0 CHECK (planned_purchase >= 0),
    planned_cancel INTEGER NOT NULL DEFAULT 0 CHECK (planned_cancel >= 0),
    planned_ttl INTEGER NOT NULL DEFAULT 0 CHECK (planned_ttl >= 0),
    actual_purchased INTEGER NOT NULL DEFAULT 0 CHECK (actual_purchased >= 0),
    actual_cancelled INTEGER NOT NULL DEFAULT 0 CHECK (actual_cancelled >= 0),
    actual_checkout_expired INTEGER NOT NULL DEFAULT 0 CHECK (actual_checkout_expired >= 0),
    actual_queue_rejected INTEGER NOT NULL DEFAULT 0 CHECK (actual_queue_rejected >= 0),
    actual_sold_out INTEGER NOT NULL DEFAULT 0 CHECK (actual_sold_out >= 0),
    actual_unresolved INTEGER NOT NULL DEFAULT 0 CHECK (actual_unresolved >= 0),
    payment_accepted INTEGER NOT NULL DEFAULT 0 CHECK (payment_accepted >= 0),
    payment_rejected INTEGER NOT NULL DEFAULT 0 CHECK (payment_rejected >= 0),
    payment_technical_error INTEGER NOT NULL DEFAULT 0 CHECK (payment_technical_error >= 0),
    verification_passed BOOLEAN,
    seeded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at TIMESTAMPTZ,
    CHECK ((status = 'seeded' AND completed_at IS NULL AND verification_passed IS NULL)
        OR (status IN ('completed', 'failed') AND completed_at IS NOT NULL AND verification_passed IS NOT NULL))
);

CREATE TABLE loadtest.request_logs (
    run_id VARCHAR(40) NOT NULL REFERENCES loadtest.runs(run_id) ON DELETE CASCADE,
    external_user_id UUID NOT NULL,
    product_id UUID NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    planned_outcome VARCHAR(16)
        CHECK (planned_outcome IS NULL OR planned_outcome IN ('purchase', 'cancel', 'ttl')),
    attempt_id UUID,
    operation VARCHAR(128),
    http_status SMALLINT CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    payment_event_id VARCHAR(200),
    payment_inbox_id UUID,
    final_state VARCHAR(32),
    actual_outcome VARCHAR(32)
        CHECK (actual_outcome IS NULL OR actual_outcome IN (
            'purchased', 'cancelled', 'checkout_expired', 'queue_rejected',
            'sold_out', 'unresolved'
        )),
    technical_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (run_id, external_user_id, product_id),
    CHECK (updated_at >= created_at),
    CHECK ((actual_outcome IS NULL AND completed_at IS NULL)
        OR (actual_outcome IS NOT NULL AND completed_at IS NOT NULL))
);

CREATE INDEX loadtest_request_logs_outcome_idx
    ON loadtest.request_logs (run_id, actual_outcome);

CREATE INDEX loadtest_request_logs_attempt_idx
    ON loadtest.request_logs (attempt_id)
    WHERE attempt_id IS NOT NULL;

-- +goose Down
DROP SCHEMA loadtest CASCADE;
