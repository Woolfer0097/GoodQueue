# GoodQueue PostgreSQL load test

Этот контур проверяет реальную очередь GoodQueue только в режиме `GOODQUEUE_MODE=postgres`. Он создаёт изолированные данные, постепенно выполняет массовые Join, повторяет часть запросов с тем же `Idempotency-Key`, опрашивает Current с jitter ±20%, собирает HTTP/custom-метрики и затем проверяет PostgreSQL-инварианты.

Mock API и отдельный тестовый HTTP-контракт не используются. Production handlers, use cases, repositories и business-таблицы не меняются; отчёты хранятся в отдельной схеме `loadtest`.

## Как проходит полный прогон

Полные профильные цели `make loadtest-{smoke|medium|main}` и `make loadtest-purchase-{smoke|medium|main}` выполняют один и тот же конвейер:

1. Поднимают Prometheus и Grafana из `loadtest/compose.loadtest.yaml`, ждут readiness и автоматически provision-ят datasource и dashboard.
2. Ждут готовности уже запущенного backend по `GET /readyz`.
3. Seed создаёт изолированных пользователей, товары, назначения user-product, строку прогона в `loadtest.runs` и planned-строки в `loadtest.request_logs`.
4. k6 плавно добавляет виртуальных пользователей за `Ramp`. Каждый VU выполняет назначения из fixture, а не бесконечно повторяет один запрос.
5. k6 отправляет стандартные и custom-метрики в Prometheus Remote Write и пишет локальные summary-файлы.
6. Verifier читает фактическое состояние PostgreSQL, проверяет инварианты и исходы, дополняет постоянные отчётные таблицы и завершает команду с ошибкой при несовпадении.

`Ramp` — время постепенного выхода на заданное число VU. `Polling` — период, в течение которого VU повторяет `GET queue-entry`, ожидая изменения состояния очереди. В профиле `purchase_outcomes` вместо фиксированного polling stage используется общий `LOADTEST_OUTCOME_TIMEOUT`, потому что часть попыток должна реально дождаться checkout TTL.

Перед полным прогоном backend и PostgreSQL должны быть запущены отдельно. Пример:

```bash
GOODQUEUE_UNSAFE_PAYMENT_CALLBACK=true \
docker compose --env-file .env.example up -d --build

LOADTEST_RUN_ID=purchase-$(date +%Y%m%d-%H%M%S) \
make loadtest-purchase-smoke
```

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

## Какие HTTP endpoints вызывает k6

| Endpoint | `queue_join_polling` | `purchase_outcomes` | Назначение |
|---|:---:|:---:|---|
| `GET /readyz` | да | да | Проверка готовности backend перед seed и запуском |
| `POST /api/v1/products/{productID}/queue-entries` | да | да | Вход в очередь; часть запросов воспроизводимо повторяется с тем же idempotency key |
| `GET /api/v1/products/{productID}/queue-entry` | да | да | Polling текущего состояния до завершения сценария |
| `POST /api/v1/queue-attempts/{attemptID}/checkout` | нет | да | Переход приглашённой попытки в checkout |
| `DELETE /api/v1/products/{productID}/queue-entry` | нет | только `cancel` | Явный отказ от покупки |
| `POST /internal/v1/payment-events` | нет | только `purchase` | Успешный тестовый callback оплаты |

Вариант `ttl` после старта checkout намеренно ничего не отправляет до deadline, затем polling подтверждает `checkout_expired`. `queue_full` означает, что реальная waiting-ёмкость товара заполнена; `sold_out` — что остаток закончился до получения права на покупку. Оба результата являются допустимыми бизнес-исходами, а не обязательно техническими ошибками HTTP.

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

Prometheus и Grafana запускаются только в loadtest overlay. Обычные `make loadtest-*` поднимают весь observability-контур автоматически. Отдельно:

```bash
make loadtest-observability-up
curl --fail http://localhost:9090/-/ready
curl --fail http://localhost:2002/api/health
# Prometheus: http://localhost:9090
# Grafana: http://localhost:2002 (admin / goodqueue локально)
```

Grafana получает datasource `Prometheus` и dashboard **GoodQueue — нагрузка и конверсия** автоматически из Git. Ручной импорт JSON не нужен. Для среды вне доверенной локальной машины обязательно задайте `LOADTEST_GRAFANA_ADMIN_PASSWORD`; анонимный доступ и самостоятельная регистрация отключены.

Встроенный k6 dashboard по умолчанию выключен, чтобы unattended-прогоны не открывали browser и не конфликтовали за port `5665`. Для разового запуска:

```bash
K6_WEB_DASHBOARD=true K6_WEB_DASHBOARD_OPEN=true make loadtest-smoke
# UI: http://localhost:5665 во время теста
```

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

Явные env-переменные имеют приоритет над `loadtest/.env` и значениями профиля. В `.env.example` профильные counts/durations закомментированы, чтобы `medium` и `main` не превращались в smoke после копирования файла.

## Команды нагрузочного тестирования

| Команда | Сценарий/действие | Отличие |
|---|---|---|
| `make loadtest` | `queue_join_polling` | Безопасный alias для `loadtest-smoke` |
| `make loadtest-smoke` | `queue_join_polling`, smoke | 10 VU; быстрая проверка join и polling |
| `make loadtest-medium` | `queue_join_polling`, medium | 100 VU; средняя локальная нагрузка |
| `make loadtest-main` | `queue_join_polling`, main | 1000 VU; тяжёлый прогон, запускается только явно |
| `make loadtest-purchase-smoke` | `purchase_outcomes`, smoke | Быстрая проверка purchase/cancel/TTL |
| `make loadtest-purchase-medium` | `purchase_outcomes`, medium | Те же исходы при 100 VU |
| `make loadtest-purchase-main` | `purchase_outcomes`, main | Те же исходы при 1000 VU |
| `make loadtest-seed` | Только подготовка | Создаёт fixture и PostgreSQL-записи, k6 не запускает |
| `make loadtest-verify` | Только проверка | Проверяет ранее выполненный `LOADTEST_RUN_ID` |
| `make loadtest-clean` | Очистка одного run | Удаляет DB-строки и локальные файлы только выбранного `LOADTEST_RUN_ID`; Prometheus не очищает |
| `make loadtest-prometheus-up` | Только Prometheus | Запускает/проверяет сервис без k6 |
| `make loadtest-prometheus-stop` | Только Prometheus | Останавливает сервис, сохраняя TSDB volume |
| `make loadtest-observability-up` | Prometheus + Grafana | Запускает хранилище метрик и provisioned dashboard |
| `make loadtest-observability-stop` | Prometheus + Grafana | Останавливает UI и хранилище, сохраняя оба named volume |
| `make load-test` | Старый Go smoke | Отдельный простой конкурентный тест на 20 join; без k6, профилей, Remote Write и `loadtest.*`-отчётов |

`loadtest-run` и `loadtest-purchase-run` — внутренние Make-цели, которые вызываются профильными командами; обычно запускать их напрямую не требуется.

Каждая цель поднимает и проверяет Prometheus с Grafana, ждёт `/readyz`, запускает seed, локальный k6 с Remote Write и verifier. Если `LOADTEST_KEEP_DATA=false`, PostgreSQL-записи текущего run удаляются только после успешного verifier; Prometheus-история сохраняется до retention. Повторный запуск того же run требует новый `LOADTEST_RUN_ID` или `LOADTEST_CLEANUP_BEFORE_SEED=true`.

Отдельные команды:

```bash
make loadtest-seed
k6 run -o experimental-prometheus-rw loadtest/k6/queue-join-polling.js
make loadtest-verify
make loadtest-clean
```

Ручной запуск `purchase_outcomes`:

```bash
export LOADTEST_SCENARIO=purchase_outcomes
export LOADTEST_RUN_ID=purchase-$(date +%Y%m%d-%H%M%S)
make loadtest-seed
mkdir -p "loadtest/results/$LOADTEST_RUN_ID"
k6 run -o experimental-prometheus-rw --log-format=raw \
  --log-output="file=loadtest/results/$LOADTEST_RUN_ID/k6-events.log" \
  loadtest/k6/queue-purchase-outcomes.js
make loadtest-verify
```

При прямом `k6 run` default-путь к данным — `loadtest/generated/<LOADTEST_RUN_ID>/data.json`. Это не даёт параллельным прогонам перезаписывать fixture друг друга. Если `LOADTEST_DATA_FILE` задан через env, используйте абсолютный путь.

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
export LOADTEST_SCENARIO=purchase_outcomes
export LOADTEST_RUN_ID=purchase-$(date +%Y%m%d-%H%M%S)
make loadtest-seed
GOODQUEUE_UNSAFE_PAYMENT_CALLBACK=true docker compose \
  -f compose.yaml -f loadtest/compose.loadtest.yaml \
  run --rm k6 run -o experimental-prometheus-rw --log-format=raw \
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
| `LOADTEST_DATA_FILE` | `loadtest/generated/<run-id>/data.json` | run-scoped путь к fixture; для custom-пути в k6 используйте абсолютный |
| `LOADTEST_RESULTS_DIR` | `loadtest/results` | корень результатов |
| `LOADTEST_DOCKER_BASE_URL` | `http://backend:8080` | URL backend внутри compose network |
| `LOADTEST_DOCKER_USER` | `1000:1000` | UID:GID host-пользователя для bind mount |
| `LOADTEST_ENV_FILE` | `loadtest/.env` | env-файл Make |
| `LOADTEST_PROMETHEUS_PORT` | `9090` | локальный порт Prometheus UI/API |
| `LOADTEST_PROMETHEUS_RETENTION` | `30d` | срок хранения TSDB |
| `LOADTEST_GRAFANA_PORT` | `2002` | локальный порт Grafana |
| `LOADTEST_GRAFANA_ADMIN_USER` | `admin` | локальный администратор Grafana |
| `LOADTEST_GRAFANA_ADMIN_PASSWORD` | `goodqueue` | пароль только для доверенной локальной среды; вне неё обязательна замена |
| `K6_PROMETHEUS_RW_SERVER_URL` | `http://localhost:9090/api/v1/write` | Remote Write URL для host-k6 |
| `K6_PROMETHEUS_RW_TREND_STATS` | `avg,min,max,p(90),p(95),p(99)` | серии для k6 Trend-метрик |
| `GOODQUEUE_LOADTEST_PROMETHEUS_URL` | `http://prometheus:9090` в Compose | Prometheus HTTP API для backend; при локальном backend задайте `http://localhost:9090` |
| `GOODQUEUE_LOADTEST_SUCCESS_RATE_WINDOW` | `30m` | общее окно расчёта HTTP-успешности и конверсии покупки |

Обычный backend не читает `LOADTEST_*`/`K6_*` и автоматически тестовые данные не создаёт. Пара `GOODQUEUE_LOADTEST_*` используется только endpoint сводной Prometheus-статистики.

## Метрики и thresholds

Помимо стандартных HTTP-метрик создаются `join_duration`, `current_duration`, `join_success`, `current_success`, `duplicate_join_success`, `unexpected_request_failure_rate`, `unexpected_4xx`, `unexpected_5xx`, `join_errors`, `current_errors`, четыре `state_*` и `polling_requests`. Custom tags ограничены `operation`, `result` и `product_group`; UUID в tags отсутствуют. Для HTTP используется фиксированный `name` endpoint, а не конкретный UUID URL.

Начальные thresholds: 5xx = 0, unexpected failure rate <1%, Join p95 <1000 ms/p99 <2000 ms, Current p95 <500 ms/p99 <1000 ms, dropped iterations = 0. Это локальные стартовые ориентиры, не продуктовые SLO.

В Prometheus передаются все standard/custom k6-метрики с префиксом `k6_`. Каждая серия имеет labels `testid=<LOADTEST_RUN_ID>`, `profile` и `loadtest_scenario`. Примеры PromQL:

```promql
k6_http_reqs_total{testid="purchase-20260806-220000"}
k6_http_req_failed_rate{testid="purchase-20260806-220000"}
k6_purchased_outcomes_total{testid="purchase-20260806-220000"}
k6_checkout_expired_outcomes_total{testid="purchase-20260806-220000"}
```

Prometheus хранит агрегированные time series. PostgreSQL-таблицы `loadtest.runs` и `loadtest.request_logs` остаются каноническим источником точных итогов, UUID и текстов ошибок.

### Metrics endpoints

| Метод и адрес | Назначение |
|---|---|
| `GET http://localhost:9090/` | Prometheus UI |
| `GET http://localhost:9090/-/ready` | Readiness Prometheus |
| `POST http://localhost:9090/api/v1/write` | Remote Write receiver для k6; это не endpoint чтения человеком |
| `GET http://localhost:9090/api/v1/query` | Prometheus instant query API |
| `GET http://localhost:9090/api/v1/series` | Поиск временных рядов и labels |
| `GET http://localhost:2002/` | Grafana UI с готовым GoodQueue dashboard |
| `GET http://localhost:2002/api/health` | Readiness Grafana |
| `GET http://localhost:8080/internal/v1/loadtest/request-success-rate` | Backend-сводка успешных k6-запросов за настроенное окно |
| `GET http://localhost:8080/internal/v1/adaptive-queue/status` | Решение адаптивного контроллера, текущий/целевой waiting buffer и достаточность выборки |
| `GET http://localhost:8080/internal/v1/loadtest/purchase-success-rate` | Backend-конверсия `purchased / (purchased + cancelled + checkout_expired)` |

Backend не публикует `/metrics`: в этом контуре Prometheus получает только метрики k6. Grafana визуализирует именно нагрузочные и продуктовые метрики теста; postgres-exporter не запускается.

При `GOODQUEUE_ADAPTIVE_QUEUE_ENABLED=true` эти же сохранённые k6-метрики могут использоваться экспериментальным контроллером waiting buffer. Контроллер не доверяет единичному прогону: он требует минимальные размеры HTTP- и checkout-выборки, проверяет техническую успешность HTTP, ограничивает диапазон и скорость изменения. При любой неопределённости сохраняется последнее безопасное значение.

### Dashboard Grafana

Dashboard открывается сразу после входа и содержит:

- общий объём запросов, HTTP success rate, p95 и продуктовую конверсию;
- RPS, avg/p95/p99 задержки и фактическое количество VU;
- `purchased`, `cancelled`, `checkout_expired`, `queue_rejected` и `sold_out`;
- наблюдаемые состояния `waiting`, `invited`, `checkout`, `terminal`;
- успешность join/polling/checkout/cancel/payment;
- неожиданные 4xx/5xx, action errors, outcome mismatches и unresolved outcomes.

Переменные **Прогон** (`testid`) и **Сценарий** позволяют сравнивать сохранённые запуски без изменения PromQL. Dashboard хранится в `loadtest/grafana/dashboards/goodqueue-loadtest.json`, datasource и provider — в `loadtest/grafana/provisioning`. UI не записывает изменения поверх provisioned dashboard: изменения проходят через Git и code review.

### Как посмотреть данные в Prometheus

Откройте <http://localhost:9090>, перейдите на вкладку **Query**, вставьте PromQL и выберите **Table** или **Graph**. Для уже завершённого короткого прогона используйте `last_over_time`: обычный instant selector может перестать показывать завершившиеся серии после lookback-окна Prometheus.

```promql
# Найти сохранённые прогоны и их labels
count by (testid, profile, loadtest_scenario) (
  last_over_time(k6_http_reqs_total[30d])
)

# Итоговое число HTTP-запросов конкретного прогона
sum(last_over_time(k6_http_reqs_total{testid="purchase-20260806-220000"}[30d]))

# Итоговые purchase и TTL-исходы
sum(last_over_time(k6_purchased_outcomes_total{testid="purchase-20260806-220000"}[30d]))
sum(last_over_time(k6_checkout_expired_outcomes_total{testid="purchase-20260806-220000"}[30d]))

# p95 задержки во время активного прогона
k6_http_req_duration_p95{testid="purchase-20260806-220000"}
```

Тот же query API доступен из терминала:

```bash
curl --get 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=sum(last_over_time(k6_http_reqs_total{testid="purchase-20260806-220000"}[30d]))'
```

Observability-контур запускается только через loadtest overlay, поэтому обычный `docker compose up` сам по себе его не создаёт. Полные профильные цели автоматически запускают Prometheus и Grafana, но после теста не останавливают: UI остаются доступны до `make loadtest-observability-stop` или остановки Compose. Повторный запуск подключает сохранённые named volumes. Prometheus хранит метрики по умолчанию `30d`; `make loadtest-clean` их не удаляет. Пока Prometheus остановлен, панели Grafana не получают данные, а backend endpoint сводной успешности вернёт ошибку upstream; после повторного запуска сохранённые ряды снова доступны.

Backend endpoint для сводной успешности:

```bash
curl http://localhost:8080/internal/v1/loadtest/request-success-rate
```

Он работает, когда Prometheus поднят через loadtest Compose overlay, и возвращает `successful_requests`, `total_requests` и `success_percentage`. Успешным считается k6-запрос с label `expected_response="true"`; такие бизнес-ответы, как ожидаемый `queue_full`, не считаются техническим сбоем. Окно задаётся через `GOODQUEUE_LOADTEST_SUCCESS_RATE_WINDOW`, по умолчанию `30m`, максимум `30d`. Расчёт учитывает одноточечные серии коротких k6-прогонов.

Отдельный endpoint конверсии покупки:

```bash
curl http://localhost:8080/internal/v1/loadtest/purchase-success-rate
```

Он суммирует только outcome-счётчики сценария `purchase_outcomes` и рассчитывает `purchased / (purchased + cancelled + checkout_expired) × 100`. `queue_rejected`, `sold_out`, `unresolved` и технические HTTP-запросы в знаменатель не входят. Ответ содержит исходные `purchased`, `cancelled`, `checkout_expired`, их сумму `total_checkout_outcomes` и округлённый до двух знаков `purchase_percentage`. При отсутствии завершённых checkout-исходов процент равен `0`. Используется то же окно `GOODQUEUE_LOADTEST_SUCCESS_RATE_WINDOW`.

## Результаты и verifier

Данные одного прогона распределены по трём типам хранилищ:

| Хранилище | Что сохраняется | Срок жизни |
|---|---|---|
| PostgreSQL `loadtest.runs` | Конфигурация, planned/actual outcome counts, payment counters, статус и результат verifier | Постоянно, до очистки конкретного `run_id` |
| PostgreSQL `loadtest.request_logs` | Одна строка на user-product: planned outcome, UUID, HTTP operation/status, payment event, final state/outcome, timestamps и техническая ошибка | Постоянно, до очистки конкретного `run_id` |
| Prometheus | Агрегированные временные ряды k6: количество/ошибки/latency HTTP и custom outcome counters с labels прогона | До retention или явного удаления TSDB volume |
| `loadtest/generated/<run-id>/` | Входной `data.json` fixture | До `make loadtest-clean` |
| `loadtest/results/<run-id>/` | k6 summary/config/events и `verifier.json` | До `make loadtest-clean` |

PostgreSQL является каноническим источником точных итогов и детальных записей. Prometheus нужен для временных графиков и агрегатов; UUID пользователей/товаров/attempts и тексты ошибок туда намеренно не передаются, чтобы не создавать высокую cardinality.

k6 пишет в `loadtest/results/<run-id>/`:

- `summary.json`;
- `summary.txt`;
- `effective-config.json`;
- `k6-events.log` для `purchase_outcomes` — run-scoped источник точных HTTP-исходов verifier.

Verifier добавляет `verifier.json`, печатает каждый check и завершает работу с ненулевым кодом при нарушении. Фактические результаты и `data.json` игнорируются Git.

Миграции `00006`/`00007` создают и дополняют постоянные `loadtest.runs` и `loadtest.request_logs`. Seed записывает effective config, planned-счётчики и каждую user-product пару. Runtime-исходы `queue_rejected`, `sold_out`, `unresolved` имеют planned-счётчики `0`, поскольку заранее они не назначаются. Verifier дополняет attempt/payment IDs, HTTP action/status, final state, actual outcome, техническую ошибку и итоговые счётчики.

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

`make loadtest-observability-stop` останавливает Grafana и Prometheus, но не удаляет их volumes. `loadtest-clean` также не удаляет Prometheus-метрики; они исчезают после `LOADTEST_PROMETHEUS_RETENTION` или при явном удалении Docker volume.

## Просмотр через DBeaver

Создайте PostgreSQL connection: host `localhost`, port `5432` (или `GOODQUEUE_POSTGRES_PORT`), database/user/password `goodqueue`. Итоги смотрите в `loadtest.runs` и `loadtest.request_logs`; business-состояние — в `public.products`, `public.users`, `public.queue_attempts` с фильтром `title/name LIKE 'LT-local-%'`.

## Ограничения локального результата

Backend, PostgreSQL, k6 и Docker работают на одном компьютере и конкурируют за CPU, RAM, сеть и диск. HDD особенно легко становится главным ограничением PostgreSQL. Поэтому локальный результат полезен для регрессий и проверки конкурентных инвариантов, но не является production benchmark и не прогнозирует пропускную способность отдельной production-инфраструктуры.
