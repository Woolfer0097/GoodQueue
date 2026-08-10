# Frontend и GoodQueue Mock API

Mock API — режим backend для локальной разработки и демонстрации без PostgreSQL, миграций и
фоновых workers. В самом frontend нет отдельного mock-слоя: SPA всегда делает HTTP-запросы
через общий `apiClient` и работает с тем же публичным JSON-контрактом, что и PostgreSQL-режим.

Документ описывает mock только с точки зрения интеграции frontend. Реализация и бизнес-логика
режима принадлежат backend.

## Когда использовать

Mock API удобен, чтобы проверить:

- каталог, карточку и статусы доступности;
- выбор независимых demo-пользователей;
- `waiting`, `invited`, `checkout` и основные terminal states;
- отмену активной попытки;
- публичную demo-оплату с результатом `purchased`;
- альтернативы без подключения базы данных.

Он не заменяет PostgreSQL-режим для проверки конкурентного admission, фоновых TTL-переходов,
workers, outbox, payment inbox и устойчивости данных.

## Локальный запуск

Backend mock запускается из корня репозитория.

PowerShell:

```powershell
$env:GOODQUEUE_MODE = "mock"
go run ./cmd/goodqueue-backend
```

Bash:

```bash
GOODQUEUE_MODE=mock go run ./cmd/goodqueue-backend
```

По умолчанию доступны:

- API: <http://localhost:8080>
- Swagger UI: <http://localhost:8080/docs>
- readiness: <http://localhost:8080/readyz>

Frontend запускается отдельно из `frontend/`. Для Vite укажите в локальном `.env.local`:

```dotenv
VITE_API_URL=http://localhost:8080
```

После `npm run dev` интерфейс по умолчанию доступен на <http://localhost:5173>. Переменная
`GOODQUEUE_MODE` относится к backend и не читается frontend-кодом.

`frontend/.env.example` указывает на `http://localhost:8088` — это единый Caddy origin для
полного Compose-запуска. При прямом `go run` mock backend слушает `8080`, поэтому локальный
override выше обязателен; порт `8088` без запущенного Caddy недоступен.

## Готовые идентификаторы

### Товары

| Alias | ID                                     | Начальное назначение                       |
| ----- | -------------------------------------- | ------------------------------------------ |
| `P1`  | `11111111-1111-1111-1111-111111111111` | один checkout и два пользователя в очереди |
| `P2`  | `22222222-2222-2222-2222-222222222222` | готовые invited и terminal states          |
| `P3`  | `33333333-3333-3333-3333-333333333333` | товар без остатка                          |
| `P4`  | `44444444-4444-4444-4444-444444444444` | очередь отключена                          |

### Пользователи

| Alias | ID                                     |
| ----- | -------------------------------------- |
| `U1`  | `00000000-0000-4000-8000-000000000001` |
| `U2`  | `00000000-0000-4000-8000-000000000002` |
| `U3`  | `00000000-0000-4000-8000-000000000003` |
| `U4`  | `00000000-0000-4000-8000-000000000004` |
| `U5`  | `00000000-0000-4000-8000-000000000005` |

Frontend получает этот список через `GET /api/v1/demo/users`; UUID не захардкожены в SPA.

## Начальные frontend-сценарии

Сценарий определяется парой «товар + `X-User-ID`».

| Товар | Пользователь | Начальный state    | Что открывает frontend    |
| ----- | ------------ | ------------------ | ------------------------- |
| `P1`  | `U1`         | `checkout`         | demo-checkout             |
| `P1`  | `U2`         | `waiting`, место 1 | очередь                   |
| `P1`  | `U3`         | `waiting`, место 2 | очередь                   |
| `P2`  | `U1`         | `invited`          | персональный резерв       |
| `P2`  | `U2`         | `purchased`        | успешный result           |
| `P2`  | `U3`         | `cancelled`        | result отмены             |
| `P2`  | `U4`         | `invite_expired`   | result истёкшего резерва  |
| `P2`  | `U5`         | `checkout_expired` | result истёкшего checkout |

`P3` позволяет проверить disabled CTA «Нет в наличии», а `P4` — «Покупка недоступна».
Прямой join для этих товаров возвращает соответственно `sold_out` и `queue_disabled`;
frontend остаётся на странице товара и показывает бизнес-сообщение.

POST и DELETE меняют общее состояние in-memory. Чтобы вернуть исходные fixtures, нужно
перезапустить backend mock.

## Публичные endpoints, используемые frontend

| Метод    | Endpoint                                                             | Назначение в SPA                      |
| -------- | -------------------------------------------------------------------- | ------------------------------------- |
| `GET`    | `/api/v1/demo/users`                                                 | загрузить demo-аккаунты               |
| `GET`    | `/api/v1/products`                                                   | загрузить каталог                     |
| `GET`    | `/api/v1/products/:productID`                                        | загрузить товар                       |
| `GET`    | `/api/v1/products/:productID/alternatives`                           | загрузить доступные альтернативы      |
| `GET`    | `/api/v1/products/:productID/queue-entry`                            | получить текущую попытку пользователя |
| `POST`   | `/api/v1/products/:productID/queue-entries`                          | начать покупку или войти в очередь    |
| `DELETE` | `/api/v1/products/:productID/queue-entry`                            | отменить активную попытку             |
| `POST`   | `/api/v1/queue-attempts/:attemptID/checkout`                         | перейти из `invited` в `checkout`     |
| `POST`   | `/api/v1/products/:productID/queue-attempts/:attemptID/demo-payment` | завершить безопасную demo-оплату      |

Точный HTTP-контракт следует проверять в Swagger работающего backend. Frontend не использует
internal endpoints.

## Заголовки

`X-User-ID` обязателен для операций с попыткой. Frontend берёт его из выбранного
demo-аккаунта.

`Idempotency-Key` frontend создаёт для:

- `POST .../queue-entries`;
- `POST .../demo-payment`.

Повтор join или demo-payment с тем же ключом безопасно возвращает тот же результат. Старт
checkout и отмена используют только `X-User-ID`.

## Контракты, которые валидирует frontend

### Product

Frontend зависит от следующих полей:

- `id`, `title`, `description`, `category`, `image_url`, `price_cents`;
- `queue_enabled`, `allocatable_stock`, `free_stock`;
- `waiting_count`, `waiting_buffer_capacity`;
- `reserved` принимается схемой, но не показывается покупателю.

Пустой `image_url` допустим: UI использует нейтральную заглушку.

### QueueAttempt

Поддерживаемые states:

`waiting`, `invited`, `checkout`, `purchased`, `invite_expired`, `checkout_expired`,
`payment_failed`, `cancelled`, `sold_out`.

Frontend использует `attempt_id`, `product_id`, timestamps, `state`, `queue_sequence`,
`next_action` и `message_code`. Позиция, число ожидающих и deadlines опциональны и
отображаются только при наличии.

### Alternatives

Ответ имеет форму массива товаров и может дополнительно содержать:

- `recommendation_mode`: `ai_semantic` или `catalog_fallback`;
- `recommendation_score` от 0 до 1;
- `reason_code`: `semantically_similar`, `same_category_available` или `available_now`.

Frontend валидирует эти поля, сохраняет порядок backend и отображает обычные
`ProductCard`. Mock нужен для формы контракта, но не доказывает работу AI-ranking.

## Demo-payment и ограничения

Публичная demo-оплата работает только для владельца активного `checkout`, требует
`Idempotency-Key` и возвращает `purchased`. Кнопка во frontend прямо сообщает, что деньги не
списываются.

Mock не предоставляет frontend-действие для `payment_failed`: публичный demo-endpoint
имитирует только успех. SPA поддерживает отображение `payment_failed`, если такой state
пришёл от полноценного backend-процесса.

Frontend никогда не вызывает `/internal/v1/payment-events` или stock-adjustment endpoints.
Они не являются частью пользовательского контракта.

## Связанные документы

- [Состояния, маршруты и переходы](frontend-flow.md)
- [Реализованный UI/UX](frontend-ui.md)
- [Frontend README](../frontend/README.md)
