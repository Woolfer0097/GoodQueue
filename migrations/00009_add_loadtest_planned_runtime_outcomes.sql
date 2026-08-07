-- +goose Up
ALTER TABLE loadtest.runs
    ADD COLUMN planned_queue_rejected INTEGER NOT NULL DEFAULT 0
        CHECK (planned_queue_rejected >= 0),
    ADD COLUMN planned_sold_out INTEGER NOT NULL DEFAULT 0
        CHECK (planned_sold_out >= 0),
    ADD COLUMN planned_unresolved INTEGER NOT NULL DEFAULT 0
        CHECK (planned_unresolved >= 0);

-- +goose Down
ALTER TABLE loadtest.runs
    DROP COLUMN planned_unresolved,
    DROP COLUMN planned_sold_out,
    DROP COLUMN planned_queue_rejected;
