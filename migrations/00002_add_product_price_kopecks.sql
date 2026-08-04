-- +goose Up
ALTER TABLE products
    ADD COLUMN price_kopecks INTEGER NOT NULL DEFAULT 0;

ALTER TABLE products
    ALTER COLUMN price_kopecks DROP DEFAULT;

-- +goose Down
ALTER TABLE products
    DROP COLUMN price_kopecks;
