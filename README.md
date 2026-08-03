# GoodQueue

Каркас серверной части очереди на покупку дефицитных товаров. Стек: Go, Gin, PostgreSQL, Goose и Go Jet. Бизнес-маршруты пока возвращают точный ответ `501` и не обращаются к базе данных.

## Запуск

Требуются Go 1.25.7+ и Docker Compose.

```bash
cp .env.example .env
docker compose up --build -d
```

- проверка процесса: <http://localhost:8080/healthz>
- проверка PostgreSQL: <http://localhost:8080/readyz>
- веб-интерфейс Swagger: <http://localhost:8080/docs>
- JSON-схема Swagger: <http://localhost:8080/docs/doc.json>

PostgreSQL по умолчанию доступен только через `127.0.0.1:5432`.

## Маршруты

| Метод | Путь | Назначение |
|---|---|---|
| `GET` | `/healthz` | Проверка процесса |
| `GET` | `/readyz` | Проверка соединения с PostgreSQL |
| `GET` | `/api/v1/products` | Список товаров |
| `GET` | `/api/v1/products/:productID` | Товар |
| `POST` | `/api/v1/products/:productID/queue-entries` | Вход в очередь |
| `GET` | `/api/v1/products/:productID/queue-entry` | Текущая позиция |
| `DELETE` | `/api/v1/products/:productID/queue-entry` | Выход из очереди |
| `POST` | `/api/v1/products/:productID/checkout-authorizations` | Разрешение покупки |

Swagger 2.0 централизованно создаётся из аннотаций. Файлы в `internal/app/http/docs` хранятся в репозитории.

## Проверка

```bash
make swagger              # обновить Swagger
make swagger-check        # проверить расхождение Swagger
make jet-generate         # безопасно обновить код Go Jet
make jet-check            # проверить расхождение Go Jet
make verify               # контракт, тесты, гонки данных и статический анализ
make verify-integration   # изолированные миграции, Jet, Docker и HTTP
make verify-all           # все проверки
```

`verify-integration` использует отдельный проект Compose, новую БД, динамические локальные порты и всегда удаляет свои контейнеры и тома. Существующий стек разработчика не затрагивается.

## Конфигурация

Основные переменные перечислены в `.env.example`. `GOODQUEUE_HTTP_READ_HEADER_TIMEOUT` отдельно ограничивает чтение заголовков HTTP, а `GOODQUEUE_DATABASE_PING_TIMEOUT` — проверку БД.

## Ограничения каркаса

Аутентификация, платежи и бизнес-транзакции ещё не реализованы. Согласованность состояний между очередью и правом покупки намеренно отложена до будущей атомарной транзакции. Её нужно проверять настоящими интеграционными тестами с PostgreSQL, включая блокировки, истечение прав и повторные запросы.
