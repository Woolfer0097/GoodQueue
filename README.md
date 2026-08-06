# GoodQueue

Backend для очереди на покупку дефицитных товаров. Стек: Go, Gin, PostgreSQL, Goose, Go Jet, Swagger и Docker Compose.

По умолчанию API работает с PostgreSQL и реальной бизнес-логикой. Для frontend-разработки можно включить stateless mock API:

- `GOODQUEUE_MOCK_API=false` — PostgreSQL repository, очередь, purchase rights и checkout;
- `GOODQUEUE_MOCK_API=true` — mock-список, mock-очередь и mock-checkout; получение товара по UUID по-прежнему идёт в PostgreSQL.

## Быстрый запуск

Нужны Docker и Docker Compose. Для локального запуска Go без Docker нужен Go 1.25.7+.

```bash
cp .env.example .env
docker compose up --build -d
```

`.env.example` включает mock API. После изменения mock-переменных backend нужно пересоздать:

```bash
docker compose up -d --force-recreate backend
```

Адреса после запуска:

- health check: <http://localhost:8080/healthz>;
- readiness PostgreSQL: <http://localhost:8080/readyz>;
- Swagger UI: <http://localhost:8080/docs/index.html>;
- Swagger JSON: <http://localhost:8080/docs/doc.json>.

PostgreSQL по умолчанию доступен с host-машины только через `127.0.0.1:5432`.

## Текущие HTTP endpoints

| Метод | Путь | Mock API включён | Mock API выключен |
|---|---|---|---|
| `GET` | `/healthz` | `200`, процесс работает | `200` |
| `GET` | `/readyz` | проверка PostgreSQL | проверка PostgreSQL |
| `GET` | `/api/v1/products` | `200`, массив из трёх mock-товаров | `200`, товары из PostgreSQL |
| `GET` | `/api/v1/products/:productID` | реальное чтение PostgreSQL | реальное чтение PostgreSQL |
| `POST` | `/api/v1/products/:productID/queue-entries` | `201`, фиксированный `waiting` | `201`, вход в очередь |
| `GET` | `/api/v1/products/:productID/queue-entry` | `200`, снимок заданного статуса | `200`, текущая запись очереди |
| `DELETE` | `/api/v1/products/:productID/queue-entry` | `200`, фиксированный `cancelled` | `200`, отменённая запись |
| `POST` | `/api/v1/products/:productID/checkout-authorizations` | `200`, фиксированный `purchased` | `200`, погашенное право покупки |

## Mock API для frontend

Mock-режим задаётся при старте backend:

```env
GOODQUEUE_MOCK_API=true
GOODQUEUE_MOCK_QUEUE_STATUS=waiting
```

`GOODQUEUE_MOCK_API` по умолчанию равен `false`. `GOODQUEUE_MOCK_QUEUE_STATUS` по умолчанию равен `waiting`; разрешены `waiting`, `granted`, `purchased`, `cancelled` и `expired`. Некорректное значение останавливает запуск приложения.

При включённом mock-режиме composition root подменяет service-реализации, не меняя router и handlers:

```text
HTTP request
→ Gin router
→ handler
→ mock service
→ immutable mockdata
→ JSON response
```

Исключение — `GET /products/:productID`: mock product service делегирует этот вызов реальному `ProductUseCase`.

### Список товаров

```bash
curl http://127.0.0.1:8080/api/v1/products
```

Ответ — обычный JSON-массив из трёх товаров, без обёртки `items/total`. Каждый товар содержит:

```json
{
  "id": "280f1230-81e3-4e10-aad6-864d8bb12a78",
  "title": "Лимитированная игровая приставка",
  "description": "Игровая приставка ограниченной серии с двумя беспроводными контроллерами.",
  "price": 1999900,
  "image_url": "https://placehold.co/1200x800/png?text=Limited+Console",
  "available": 1,
  "queue_enabled": true,
  "right_ttl_seconds": 120
}
```

`price` передаётся целым числом в копейках.

### Очередь

Все queue endpoints требуют `X-User-ID`. Пример входа в очередь:

```bash
curl -X POST \
  -H 'X-User-ID: user-1' \
  -H 'Content-Type: application/json' \
  -d '{}' \
  http://127.0.0.1:8080/api/v1/products/280f1230-81e3-4e10-aad6-864d8bb12a78/queue-entries
```

```json
{
  "entry_id": 42,
  "product_id": "280f1230-81e3-4e10-aad6-864d8bb12a78",
  "status": "waiting",
  "position": 3,
  "total_waiting": 7,
  "expires_at": null
}
```

`GET /queue-entry` возвращает снимок, заданный `GOODQUEUE_MOCK_QUEUE_STATUS`. `DELETE /queue-entry` всегда возвращает `cancelled`. Поля `position`, `total_waiting` и `expires_at` всегда присутствуют в JSON и могут быть `null`.

### Checkout

```bash
curl -X POST \
  -H 'X-User-ID: user-1' \
  -H 'Content-Type: application/json' \
  -d '{}' \
  http://127.0.0.1:8080/api/v1/products/280f1230-81e3-4e10-aad6-864d8bb12a78/checkout-authorizations
```

Для известного mock-товара checkout всегда возвращает `200`:

```json
{
  "authorized": true,
  "authorization_id": "41cd68a0-5e63-4d6e-a610-b5d3281a4fea",
  "entry_id": 42,
  "product_id": "280f1230-81e3-4e10-aad6-864d8bb12a78",
  "status": "purchased",
  "authorized_at": "2026-08-04T10:16:20Z"
}
```

Mock checkout не проверяет наличие purchase right, не меняет остаток и не обращается к repository.

### Валидация mock-запросов

Для queue и checkout проверки идут в таком порядке:

1. Некорректный UUID товара — `400 INVALID_PRODUCT_ID`.
2. Нет `X-User-ID` — `401 UNAUTHORIZED`; заголовок пустой или невалидный — `401 INVALID_USER_ID`.
3. UUID не входит в mock-каталог — `404 PRODUCT_NOT_FOUND`.

Формат ошибки:

```json
{
  "code": "PRODUCT_NOT_FOUND",
  "message": "Товар не найден",
  "request_id": "7ae799c1-0dfa-4248-b80b-4e60e61f431d"
}
```

### Ограничения mock API

- Mock API не хранит состояние и не пишет в PostgreSQL.
- `POST`, `GET`, `DELETE` очереди и checkout не влияют друг на друга.
- `GOODQUEUE_MOCK_QUEUE_STATUS` задаёт один общий статус для всех пользователей.
- `X-User-ID` валидируется только синтаксически и не является аутентификацией.
- Mock-каталог и PostgreSQL могут содержать разные наборы товаров.
- Backend всё равно требует PostgreSQL при старте, для `/readyz` и реального `GET /products/:productID`.

Полный frontend-контракт находится в [`docx/API_FRONTEND_CONTRACT_DRAFT.md`](docx/API_FRONTEND_CONTRACT_DRAFT.md).

## Реальное получение товара

`GET /api/v1/products/:productID` не подменяется mock API и в обоих режимах работает так:

```text
GET /api/v1/products/:productID
→ ProductHandler.Get
→ ProductUseCase.GetByID
→ ProductRepository.GetByID
→ PostgreSQL products
```

Пример:

```bash
curl http://127.0.0.1:8080/api/v1/products/280f1230-81e3-4e10-aad6-864d8bb12a78
```

Возможные ответы:

- `200` — товар прочитан из PostgreSQL;
- `400 INVALID_PRODUCT_ID` — параметр не является UUID;
- `404 PRODUCT_NOT_FOUND` — товара нет в таблице `products`;
- `500 INTERNAL_ERROR` — ошибка repository или PostgreSQL.

Миграция `00002_add_product_price_kopecks.sql` добавляет в `products` обязательную колонку `price_kopecks INTEGER NOT NULL`. В HTTP DTO она возвращается как `price`.

Миграция `00005_seed_products.sql` добавляет три тестовых товара для реального API и проверки гонок.

## CORS

Встроенный middleware разрешает origin `http://localhost:5173`, методы `GET`, `POST`, `DELETE`, `OPTIONS` и заголовки `Content-Type`, `X-User-ID`, `X-Request-ID`. Разрешённый preflight возвращает `204`, неизвестный origin при preflight — `403`.

## Проверки

```bash
make swagger              # обновить Swagger
make swagger-check        # проверить расхождение Swagger
make jet-generate         # обновить сгенерированный Go Jet
make jet-check            # проверить расхождение Go Jet
make verify               # тесты, race detector и статический анализ
make verify-integration   # изолированные миграции, Jet, Docker и HTTP
make verify-all           # все проверки
```

`verify-integration` запускает отдельный Compose-проект, новую БД и динамические локальные порты, а затем удаляет свои контейнеры и тома. Существующий стек разработчика не затрагивается.

## Конфигурация

Основные переменные перечислены в `.env.example`. `GOODQUEUE_HTTP_READ_HEADER_TIMEOUT` ограничивает чтение HTTP-заголовков, `GOODQUEUE_DATABASE_PING_TIMEOUT` — проверку БД, а `GOODQUEUE_WORKER_INTERVAL` — интервал фоновой обработки очереди.

## Что ещё не реализовано

Аутентификация и платежи ещё не реализованы. Идентификатор пользователя доверенно передаётся в `X-User-ID`, а checkout погашает право на покупку без интеграции с платёжной системой.

## Repository-слой и защита от гонок

Слой `internal/repository/postgres` реализует интерфейсы из `internal/pkg/repository`:

- `Product` — чтение товаров
- `Queue` — очередь (`Join`, `Leave`, `GetByTicketID`, `UpdateStatus`, …)
- `PurchaseRight` — выдача и освобождение прав (`AcquireRight`, `ReleaseRight`, `ListExpiredActiveRights`)

Атомарная резервация выполняется в `AcquireRight` через транзакцию и `SELECT ... FOR UPDATE` по строке товара. При выходе из очереди со статусом `right_issued` метод `Leave` вызывает `ReleaseRight`, чтобы не «зависал» счётчик `reserved`.

### Проверка race condition

Поднять Postgres и применить миграции:

```bash
docker compose up --build -d
```

Скрипт с 10 параллельными попытками на товар `stock=1`:

```bash
go run scripts/race_check.go
```

Ожидаемый результат: `Успешно получено прав: 1`, `reserved = 1`.

Интеграционные тесты repository (требуют запущенный Postgres на `127.0.0.1:5433` или `GOODQUEUE_TEST_DATABASE_URL`):

```bash
go test ./internal/repository/postgres/... -count=1
```

## Линтер

`.golangci.yaml` включает:

- `errorlint` — корректная проверка обёрнутых ошибок (важно для транзакций и `errors.Is`)
- `gosec` — базовые проверки безопасности
- `revive` — стиль и читаемость Go-кода
- `misspell` — опечатки в комментариях и строках

Запуск: `make lint` или `go tool golangci-lint run ./...`
