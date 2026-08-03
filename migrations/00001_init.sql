-- +goose Up
CREATE TABLE products (
    id UUID PRIMARY KEY,
    title TEXT NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
    description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 5000),
    image_url TEXT NOT NULL DEFAULT '' CHECK (length(image_url) <= 2048),
    queue_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    allocatable_stock INTEGER NOT NULL DEFAULT 0 CHECK (allocatable_stock >= 0),
    right_ttl_seconds INTEGER NOT NULL DEFAULT 600 CHECK (right_ttl_seconds BETWEEN 30 AND 86400),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (updated_at >= created_at)
);

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
    CHECK (expires_at > issued_at),
    CHECK (
        (status IN ('active', 'expired') AND consumed_at IS NULL)
        OR (status = 'consumed' AND consumed_at IS NOT NULL AND consumed_at >= issued_at AND consumed_at <= expires_at)
    )
);

CREATE INDEX purchase_rights_active_expiry_idx
    ON purchase_rights (expires_at, id)
    WHERE status = 'active';

-- +goose Down
DROP TABLE purchase_rights;
DROP TABLE queue_entries;
DROP TABLE products;
