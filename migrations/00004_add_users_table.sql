-- +goose Up
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO users (name) VALUES
    ('Пользователь 1'),
    ('Пользователь 2'),
    ('Пользователь 3'),
    ('Пользователь 4'),
    ('Пользователь 5')
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS users;
