-- +goose Up
INSERT INTO products (id, title, description, image_url, queue_enabled, allocatable_stock, right_ttl_seconds)
VALUES
    ('11111111-1111-1111-1111-111111111111', 'Дефицитный товар (1 шт.)', 'Очень редкая вещь, только один экземпляр', 'https://via.placeholder.com/300', true, 1, 120),
    ('22222222-2222-2222-2222-222222222222', 'Популярный товар (3 шт.)', 'Товар со средним спросом', 'https://via.placeholder.com/300', true, 3, 120),
    ('33333333-3333-3333-3333-333333333333', 'Раскупленный товар', 'Уже всё продано', 'https://via.placeholder.com/300', true, 0, 120)
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM products WHERE id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', '33333333-3333-3333-3333-333333333333');
