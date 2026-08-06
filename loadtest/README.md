# GoodQueue PostgreSQL load test

Этот контур проверяет реальную очередь GoodQueue только в режиме `GOODQUEUE_MODE=postgres`. Он создаёт изолированные данные, постепенно выполняет массовые Join, повторяет часть запросов с тем же `Idempotency-Key`, опрашивает Current с jitter ±20%, собирает HTTP/custom-метрики и затем проверяет PostgreSQL-инварианты.

Mock API и отдельный тестовый HTTP-контракт не используются. Production handlers, use cases, repositories и business-таблицы не меняются; отчёты хранятся в отдельной схеме `loadtest`.

## Сценарии

Проверяются:

- batch-создание пользователей и товаров;
- уникальные user-product назначения с воспроизводимой случайностью;
- перекошенное распределение: около 60% назначений в hot, 30% в medium и 10% в normal;
- `POST /api/v1/products/{productID}/queue-entries` без body, с `X-User-ID` и `Idempotency-Key`;
- успешные `201 Created`/`200 OK`, а также ожидаемые доменные `409`/`410`;
- повтор Join с тем же пользователем, товаром и ключом, возвращающий тот же `attempt_id`;
- `GET /api/v1/products/{productID}/queue-entry` каждые `LOADTEST_POLL_INTERVAL` до `LOADTEST_POLL_DURATION`;
- состояния `waiting`, `invited`, `checkout` и терминальные состояния без перехода к следующим действиям;
- PostgreSQL-инварианты stock/reserved, active attempt, idempotency, FIFO/sequence, допустимых связей, timestamps и ссылок.

В `purchase_outcomes` для каждой user-product пары воспроизводимо по `LOADTEST_RANDOM_SEED` выбирается:

- `purchase`: успешный `POST /internal/v1/payment-events` и `purchased`;
- `cancel`: `DELETE /api/v1/products/{productID}/queue-entry` и `cancelled`;
- `ttl`: без payment/cancel до реального checkout deadline и `checkout_expired`.

`queue_full`, `sold_out` и не дошедшие до checkout attempts считаются отдельно. Failed/duplicate payment events остаются следующим расширением.

## Важное ограничение queue capacity

В актуальной схеме `products` нет колонки `queue_capacity`. Реальная ёмкость waiting-части рассчитывается backend из `allocatable_stock` и `GOODQUEUE_WAITING_BUFFER_PERCENT`. Поэтому `LOADTEST_QUEUE_CAPACITY` — верхний лимит количества сгенерированных назначений на товар (не более 1000) и метаданные теста. Он не подменяет production-формулу. `queue_full` при достижении реальной ёмкости считается ожидаемым domain response.

## Требования и запуск backend

Нужны Go, PostgreSQL, `curl` и k6. На Ubuntu k6 можно установить через Snap:

```bash
sudo snap install k6
k6 version
```

Для пакетной установки смотрите актуальную инструкцию в официальной документации k6: <https://grafana.com/docs/k6/latest/set-up/install-k6/>.

Запустите PostgreSQL, миграции и backend:

```bash
docker compose up --build -d postgres migrate backend
curl --fail http://localhost:8080/readyz
```

Для `purchase_outcomes` unsafe payment callback включается явно и только в нагрузочном окружении. При вашем способе запуска:

```bash
GOODQUEUE_UNSAFE_PAYMENT_CALLBACK=true \
docker compose --env-file .env.example up -d --build
```

Без override endpoint `/internal/v1/payment-events` не регистрируется. Для короткого smoke можно той же команде передать `GOODQUEUE_CHECKOUT_TTL=5s GOODQUEUE_WORKER_INTERVAL=500ms`.

Backend должен использовать PostgreSQL. Для локального запуска без compose:

```bash
export GOODQUEUE_MODE=postgres
export GOODQUEUE_DATABASE_URL='postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable'
go run ./cmd/goodqueue-backend
```

## Профили

| Профиль | Users | Products | Products/user | Ramp | Polling |
|---|---:|---:|---:|---:|---:|
| smoke | 10 | 5 | 2 | 30s | 1m |
| medium | 100 | 20 | 5 | 1m | 2m |
| main | 1000 | 100 | 10 | 5m | 5m |

Явные env-переменные имеют приоритет над значениями профиля.

```bash
make loadtest-smoke
make loadtest-medium
make loadtest-main    # только явно; это 1000 VU
make loadtest         # безопасный alias smoke

make loadtest-purchase-smoke
make loadtest-purchase-medium
make loadtest-purchase-main
```

Каждая цель ждёт `/readyz`, запускает seed, локальный k6 и verifier. Если `LOADTEST_KEEP_DATA=false`, записи текущего run удаляются только после успешного verifier. Повторный запуск того же run с уже существующими attempts намеренно требует новый `LOADTEST_RUN_ID` или `LOADTEST_CLEANUP_BEFORE_SEED=true`.

Отдельные команды:

```bash
make loadtest-seed
k6 run loadtest/k6/queue-join-polling.js
make loadtest-verify
make loadtest-clean
```

Ручной запуск `purchase_outcomes`:

```bash
export LOADTEST_SCENARIO=purchase_outcomes
export LOADTEST_RUN_ID=purchase-$(date +%Y%m%d-%H%M%S)
make loadtest-seed
mkdir -p "loadtest/results/$LOADTEST_RUN_ID"
k6 run --log-format=raw \
  --log-output="file=loadtest/results/$LOADTEST_RUN_ID/k6-events.log" \
  loadtest/k6/queue-purchase-outcomes.js
make loadtest-verify
```

При прямом `k6 run` его default-путь к данным — `loadtest/generated/data.json` относительно репозитория (в скрипте это `../generated/data.json` относительно каталога JS). Если `LOADTEST_DATA_FILE` задан через env, используйте абсолютный путь.

## Запуск k6 через Docker

Seed выполняется Go-командой на host, затем k6 входит в compose network и обращается к `backend:8080`:

```bash
make loadtest-seed
docker compose \
  -f compose.yaml \
  -f loadtest/compose.loadtest.yaml \
  run --rm k6
make loadtest-verify
```

Для purchase-сценария переопределите command сервиса k6:

```bash
LOADTEST_SCENARIO=purchase_outcomes make loadtest-seed
LOADTEST_SCENARIO=purchase_outcomes docker compose \
  -f compose.yaml -f loadtest/compose.loadtest.yaml \
  run --rm k6 run --log-format=raw \
    --log-output="file=/work/loadtest/results/$LOADTEST_RUN_ID/k6-events.log" \
    loadtest/k6/queue-purchase-outcomes.js
make loadtest-verify
```

Compose запускает k6 с UID:GID `1000:1000`, чтобы он мог читать fixture с безопасными правами и писать результаты в bind mount. Если UID пользователя отличается, передайте `export LOADTEST_DOCKER_USER="$(id -u):$(id -g)"`. Docker URL переопределяется через `LOADTEST_DOCKER_BASE_URL`, а локальный — через `LOADTEST_BASE_URL`.

## Конфигурация

Скопируйте пример при необходимости: `cp loadtest/.env.example loadtest/.env`. Make автоматически читает `loadtest/.env`; для прямых Go/k6-команд экспортируйте переменные самостоятельно.

| Переменная | Default | Назначение |
|---|---|---|
| `LOADTEST_PROFILE` | `smoke` | `smoke`, `medium` или `main` |
| `LOADTEST_SCENARIO` | `queue_join_polling` | `queue_join_polling` или `purchase_outcomes` |
| `LOADTEST_BASE_URL` | `http://localhost:8080` | URL backend для локального k6 |
| `LOADTEST_DATABASE_URL` | локальная GoodQueue PostgreSQL | DSN seed/verifier |
| `LOADTEST_RUN_ID` | `local` | безопасный идентификатор из букв, цифр, `.` и `-` |
| `LOADTEST_RANDOM_SEED` | `42` | воспроизводимость stock, назначений, duplicate и jitter |
| `LOADTEST_USERS` | из профиля | число пользователей/VU |
| `LOADTEST_PRODUCTS` | из профиля | число товаров |
| `LOADTEST_PRODUCTS_PER_USER` | из профиля | разные товары одного пользователя |
| `LOADTEST_RAMP_DURATION` | из профиля | плавный разгон |
| `LOADTEST_POLL_INTERVAL` | `10s` | базовый интервал polling до jitter |
| `LOADTEST_POLL_DURATION` | из профиля | polling каждого VU |
| `LOADTEST_OUTCOME_TIMEOUT` | `7m` | ожидание терминальных purchase-исходов |
| `LOADTEST_QUEUE_CAPACITY` | `1000` | planning cap назначений на товар, 1..1000 |
| `LOADTEST_DUPLICATE_JOIN_PERCENT` | `10` | процент idempotent replay, 0..100 |
| `LOADTEST_MIN_STOCK` | `1` | минимальный `allocatable_stock` |
| `LOADTEST_MAX_STOCK` | `20` | максимальный `allocatable_stock` |
| `LOADTEST_CLEANUP_BEFORE_SEED` | `false` | удалить только текущий run перед seed |
| `LOADTEST_KEEP_DATA` | `true` | сохранить данные после полного Make-запуска |
| `LOADTEST_DATA_FILE` | `loadtest/generated/data.json` (Go) | путь к fixture; для k6 лучше абсолютный |
| `LOADTEST_RESULTS_DIR` | `loadtest/results` | корень результатов |
| `LOADTEST_DOCKER_BASE_URL` | `http://backend:8080` | URL backend внутри compose network |
| `LOADTEST_DOCKER_USER` | `1000:1000` | UID:GID host-пользователя для bind mount |
| `LOADTEST_ENV_FILE` | `loadtest/.env` | env-файл Make |

Обычный backend эти переменные не читает и автоматически тестовые данные не создаёт.

## Метрики и thresholds

Помимо стандартных HTTP-метрик создаются `join_duration`, `current_duration`, `join_success`, `current_success`, `duplicate_join_success`, `unexpected_request_failure_rate`, `unexpected_4xx`, `unexpected_5xx`, `join_errors`, `current_errors`, четыре `state_*` и `polling_requests`. Custom tags ограничены `operation`, `result` и `product_group`; UUID в tags отсутствуют. Для HTTP используется фиксированный `name` endpoint, а не конкретный UUID URL.

Начальные thresholds: 5xx = 0, unexpected failure rate <1%, Join p95 <1000 ms/p99 <2000 ms, Current p95 <500 ms/p99 <1000 ms, dropped iterations = 0. Это локальные стартовые ориентиры, не продуктовые SLO.

## Результаты и verifier

k6 пишет в `loadtest/results/<run-id>/`:

- `summary.json`;
- `summary.txt`;
- `effective-config.json`;
- `k6-events.log` для `purchase_outcomes` — run-scoped источник точных HTTP-исходов verifier.

Verifier добавляет `verifier.json`, печатает каждый check и завершает работу с ненулевым кодом при нарушении. Фактические результаты и `data.json` игнорируются Git.

Миграция `00006` создаёт постоянные `loadtest.runs` и `loadtest.request_logs`. Seed записывает effective config, planned-счётчики и каждую user-product пару. Verifier дополняет attempt/payment IDs, HTTP action/status, final state, actual outcome, техническую ошибку и итоговые счётчики.

Ручной verifier:

```bash
LOADTEST_RUN_ID=local \
LOADTEST_DATABASE_URL='postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable' \
go run ./cmd/loadtest-verify
```

Удалить только данные конкретного run:

```bash
LOADTEST_RUN_ID=local go run ./cmd/loadtest-seed --cleanup-only
# или вместе с локальными generated/results:
LOADTEST_RUN_ID=local make loadtest-clean
```

Команда удаляет business-записи только с точным префиксом `LT-<run-id>-` и reporting-строки только с точным `run_id`. Схема, таблицы и история других прогонов сохраняются.

## Просмотр через DBeaver

Создайте PostgreSQL connection: host `localhost`, port `5432` (или `GOODQUEUE_POSTGRES_PORT`), database/user/password `goodqueue`. Итоги смотрите в `loadtest.runs` и `loadtest.request_logs`; business-состояние — в `public.products`, `public.users`, `public.queue_attempts` с фильтром `title/name LIKE 'LT-local-%'`.

## Ограничения локального результата

Backend, PostgreSQL, k6 и Docker работают на одном компьютере и конкурируют за CPU, RAM, сеть и диск. HDD особенно легко становится главным ограничением PostgreSQL. Поэтому локальный результат полезен для регрессий и проверки конкурентных инвариантов, но не является production benchmark и не прогнозирует пропускную способность отдельной production-инфраструктуры.
