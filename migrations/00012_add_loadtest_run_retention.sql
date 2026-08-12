-- +goose Up
ALTER TABLE loadtest.runs
    ADD COLUMN source VARCHAR(16) NOT NULL DEFAULT 'cli'
        CHECK (source IN ('cli', 'runner_ui')),
    ADD COLUMN keep_data BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX loadtest_runs_disposable_ui_idx
    ON loadtest.runs (completed_at DESC)
    WHERE source = 'runner_ui' AND keep_data = FALSE AND status = 'completed';

-- +goose Down
DROP INDEX loadtest.loadtest_runs_disposable_ui_idx;

ALTER TABLE loadtest.runs
    DROP COLUMN keep_data,
    DROP COLUMN source;
