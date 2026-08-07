# GoodQueue Mock API

Mock API предназначен для локальной разработки frontend без PostgreSQL, миграций и фоновых workers. Он использует существующие frontend-маршруты и JSON-контракты GoodQueue, но хранит состояние только в памяти процесса.

## Запуск

Из корня проекта:

```bash
GOODQUEUE_MODE=mock go run ./cmd/goodqueue-backend
```

После запуска:

- API: <http://localhost:8080>
- Swagger UI: <http://localhost:8080/docs>
- readiness: <http://localhost:8080/readyz>

`GOODQUEUE_DATABASE_URL` в mock-режиме не требуется. После перезапуска приложения состояние сбрасывается к исходным fixtures.

Для примеров ниже удобно объявить переменные:

```bash
BASE=http://localhost:8080

P1=11111111-1111-1111-1111-111111111111
P2=22222222-2222-2222-2222-222222222222
P3=33333333-3333-3333-3333-333333333333
P4=44444444-4444-4444-4444-444444444444

U1=00000000-0000-4000-8000-000000000001
U2=00000000-0000-4000-8000-000000000002
U3=00000000-0000-4000-8000-000000000003
U4=00000000-0000-4000-8000-000000000004
U5=00000000-0000-4000-8000-000000000005
```

## Готовые сценарии

Сценарий определяется комбинацией товара и `X-User-ID`.

| Товар | Пользователь | Начальный state | Position | `next_action` | `message_code` |
|---|---|---|---:|---|---|
| `P1` | `U1` | `checkout` | — | `complete_payment` | `checkout_started` |
| `P1` | `U2` | `waiting` | 1 | `wait` | `queue_waiting` |
| `P1` | `U3` | `waiting` | 2 | `wait` | `queue_waiting` |
| `P2` | `U1` | `invited` | — | `start_checkout` | `checkout_available` |
| `P2` | `U2` | `purchased` | — | `none` | `purchased` |
| `P2` | `U3` | `cancelled` | — | `join_queue` | `cancelled` |
| `P2` | `U4` | `invite_expired` | — | `join_queue` | `invitation_expired` |
| `P2` | `U5` | `checkout_expired` | — | `join_queue` | `checkout_expired` |

Специальные товары:

- `P3` имеет нулевой stock: join возвращает `410 sold_out`.
- `P4` имеет выключенную очередь: join возвращает `409 queue_disabled`.

## Infrastructure

### `GET /healthz`

```bash
curl -s "$BASE/healthz" | jq
```

```json
{
  "status": "ok"
}
```

### `GET /readyz`

```bash
curl -s "$BASE/readyz" | jq
```

Mock не проверяет PostgreSQL и возвращает:

```json
{
  "status": "ok"
}
```

## Products

### `GET /api/v1/products`

```bash
curl -s "$BASE/api/v1/products" | jq
```

Начальные показатели:

| ID | `queue_enabled` | `allocatable_stock` | `reserved` | `free_stock` | `waiting_count` |
|---|---:|---:|---:|---:|---:|
| `P1` | true | 1 | 1 | 0 | 2 |
| `P2` | true | 3 | 1 | 2 | 0 |
| `P3` | true | 0 | 0 | 0 | 0 |
| `P4` | false | 10 | 0 | 10 | 0 |

### `GET /api/v1/products/:productID`

```bash
curl -s "$BASE/api/v1/products/$P1" | jq
```

Возвращает один товар в том же формате. Неизвестный product UUID возвращает `404 not_found`.

### `GET /api/v1/products/:productID/alternatives`

```bash
curl -s "$BASE/api/v1/products/$P1/alternatives" | jq
```

В alternatives попадают другие товары с включённой очередью и положительным `free_stock`. Для `P1` в начальном состоянии вернётся `P2`.

## Demo users

### `GET /api/v1/demo/users`

```bash
curl -s "$BASE/api/v1/demo/users" | jq
```

Возвращает пользователей `U1`–`U5` с именами `Пользователь 1`–`Пользователь 5`.

## Queue

Для всех queue-запросов обязателен канонический lowercase UUID в заголовке `X-User-ID`.

### `GET /api/v1/products/:productID/queue-entry`

Waiting, позиция 1:

```bash
curl -s \
  -H "X-User-ID: $U2" \
  "$BASE/api/v1/products/$P1/queue-entry" | jq
```

Основные поля ответа:

```json
{
  "state": "waiting",
  "queue_sequence": 2,
  "position_ahead": 0,
  "position": 1,
  "total_waiting": 2,
  "next_action": "wait",
  "message_code": "queue_waiting"
}
```

Invited:

```bash
curl -s \
  -H "X-User-ID: $U1" \
  "$BASE/api/v1/products/$P2/queue-entry" | jq
```

Для `invited` ответ содержит будущий `expires_at`, `next_action=start_checkout` и `message_code=checkout_available`.

Checkout:

```bash
curl -s \
  -H "X-User-ID: $U1" \
  "$BASE/api/v1/products/$P1/queue-entry" | jq
```

Для `checkout` ответ содержит будущий `expires_at`, `next_action=complete_payment` и `message_code=checkout_started`.

Если у пользователя нет попыток для товара, endpoint возвращает `404 not_found`.

### `POST /api/v1/products/:productID/queue-entries`

Кроме `X-User-ID`, требуется заголовок `Idempotency-Key` длиной до 128 символов.

Добавление нового пользователя в заполненный `P1`:

```bash
NEW_USER=aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa

curl -i -X POST \
  -H "X-User-ID: $NEW_USER" \
  -H "Idempotency-Key: mock-join-1" \
  "$BASE/api/v1/products/$P1/queue-entries"
```

Первый запрос возвращает `201 Created`:

```json
{
  "state": "waiting",
  "position_ahead": 2,
  "position": 3,
  "total_waiting": 3,
  "next_action": "wait",
  "message_code": "queue_waiting"
}
```

Повтор с теми же товаром, пользователем и `Idempotency-Key` возвращает тот же `attempt_id` с `200 OK`.

Если у товара есть свободный stock, новая попытка сразу создаётся в `checkout`. Это можно проверить на `P2` с новым UUID пользователя.

Ошибки специальных сценариев:

```bash
# 410 sold_out
curl -i -X POST \
  -H "X-User-ID: $NEW_USER" \
  -H "Idempotency-Key: sold-out-test" \
  "$BASE/api/v1/products/$P3/queue-entries"

# 409 queue_disabled
curl -i -X POST \
  -H "X-User-ID: $NEW_USER" \
  -H "Idempotency-Key: disabled-test" \
  "$BASE/api/v1/products/$P4/queue-entries"
```

### `DELETE /api/v1/products/:productID/queue-entry`

```bash
curl -i -X DELETE \
  -H "X-User-ID: $NEW_USER" \
  "$BASE/api/v1/products/$P1/queue-entry"
```

Успешная отмена возвращает `204 No Content`. Последующий GET вернёт `state=cancelled`, `next_action=join_queue` и `message_code=cancelled`.

Отмена `purchased` возвращает `409 already_purchased`.

## Checkout

### `POST /api/v1/queue-attempts/:attemptID/checkout`

Сначала получаем `attempt_id` приглашённого пользователя:

```bash
ATTEMPT_ID=$(
  curl -s \
    -H "X-User-ID: $U1" \
    "$BASE/api/v1/products/$P2/queue-entry" |
  jq -r '.attempt_id'
)
```

Запускаем checkout:

```bash
curl -i -X POST \
  -H "X-User-ID: $U1" \
  "$BASE/api/v1/queue-attempts/$ATTEMPT_ID/checkout"
```

Endpoint переводит `invited` в `checkout` и возвращает `200 OK`:

```json
{
  "state": "checkout",
  "deadline_at": "будущая дата",
  "next_action": "complete_payment",
  "message_code": "checkout_started"
}
```

Повторный POST для `checkout` возвращает тот же результат с `200 OK`. Чужой `X-User-ID` возвращает `404 not_found`, terminal attempt — `409 invalid_transition` или `410 expired` в зависимости от state.

Непосредственный checkout endpoint сохраняет существующий контракт с `deadline_at`. Поля `expires_at` и `total_waiting` доступны при последующем `GET queue-entry`.

## Internal endpoints

В mock-режиме internal endpoints не регистрируются:

- `POST /internal/v1/products/:productID/stock-adjustments`
- `POST /internal/v1/payment-events`

Они возвращают `404`. В PostgreSQL-режиме их поведение не изменено.

## Важные ограничения

- POST и DELETE изменяют состояние для всех последующих запросов.
- Данные не сохраняются между перезапусками.
- Фоновые TTL-переходы, payment, stock adjustment, reconciliation и outbox не моделируются.
- Для повторения исходного сценария достаточно перезапустить backend.
- Mock API предназначен для разработки UI, а не для проверки полной бизнес-логики очереди.

