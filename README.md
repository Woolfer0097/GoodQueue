# GoodQueue

GoodQueue — full-stack MVP пользовательской очереди для покупки дефицитных товаров. Пользователь выбирает товар, занимает место в FIFO-очереди, получает персональное ограниченное по времени право на checkout и видит понятный результат. Если товар закончился, сервис предлагает доступные похожие лоты.

Главная гарантия решения: при остатке в одну единицу право на покупку не смогут одновременно получить два пользователя. Распределение выполняется транзакционно в PostgreSQL, а интерфейс только отображает подтверждённое сервером состояние.

## Быстрый запуск

Нужен Docker Desktop с Docker Compose. Локальный frontend работает на `2000`, backend — на `2001`, PostgreSQL — на `5432`.

PowerShell:

```powershell
$env:GOODQUEUE_POSTGRES_PASSWORD = "goodqueue-local"
$env:VITE_API_URL = "http://localhost:2001"
$env:GOODQUEUE_CORS_ALLOWED_ORIGINS = "http://localhost:2000,http://127.0.0.1:2000,http://localhost:5173,http://127.0.0.1:5173"
docker compose up --build -d
```

Bash:

```bash
GOODQUEUE_POSTGRES_PASSWORD=goodqueue-local \
VITE_API_URL=http://localhost:2001 \
GOODQUEUE_CORS_ALLOWED_ORIGINS=http://localhost:2000,http://127.0.0.1:2000,http://localhost:5173,http://127.0.0.1:5173 \
docker compose up --build -d
```

Compose дожидается готовности PostgreSQL, применяет миграции и только после этого запускает backend и frontend. Проверить состояние можно командой `docker compose ps`.

| Компонент | Адрес |
|---|---|
| Пользовательский интерфейс | <http://localhost:2000> |
| Backend API | <http://localhost:2001> |
| Swagger UI | <http://localhost:2001/docs> |
| Логи контейнеров — Dozzle | <http://localhost:9999> |

Остановить проект:

```bash
docker compose down
```

Если порт PostgreSQL `5432` занят, перед запуском задайте другой внешний порт, например `$env:GOODQUEUE_POSTGRES_PORT = "15432"` в PowerShell или `GOODQUEUE_POSTGRES_PORT=15432` в Bash. Внутри Compose база по-прежнему доступна как `postgres:5432`.

## Цель и границы MVP

### Проблема

Когда спрос на редкий товар превышает остаток, несколько покупателей одновременно доходят до оплаты и только в конце узнают, что товар уже недоступен. Пользователь теряет время и доверие, а площадка получает лишнюю нагрузку и риск некорректных заказов.

### Идея решения

GoodQueue дополняет существующий сценарий Авито серверной FIFO-очередью перед checkout. Сервис атомарно резервирует доступную единицу, выдаёт одному пользователю персональное право с deadline, освобождает забытый резерв и автоматически приглашает следующего. Регистрацию, настоящую авторизацию и платёжную форму GoodQueue не заменяет.

### Реализованный MVP

MVP отвечает на четыре продуктовых вопроса:

- кто следующим получает возможность купить товар;
- сколько времени право на покупку остаётся активным;
- почему право нельзя передать, повторно использовать или получить в обход очереди;
- что показать пользователю, если ожидание, приглашение или оплата завершились неуспешно.

В `main` реализован полный демонстрационный контур: каталог и карточка товара, выбор тестового пользователя, постановка в очередь, экран ожидания с позицией, приглашение, checkout, безопасная демонстрационная оплата, финальные состояния и рекомендации. Регистрация, настоящая авторизация, платёжная форма и реальное списание денег считаются внешними системами Авито; их границы показаны фиксированными пользователями и явно обозначенной demo-оплатой.

### Ценность для Авито

Очередь переносит отказ с последнего шага покупки на прозрачный управляемый процесс: товар не продаётся дважды, пользователь всегда понимает своё состояние и следующий шаг, а неиспользованное право быстро возвращается в оборот. Рекомендации удерживают неудовлетворённый спрос внутри площадки, а метрики конверсии и адаптивный waiting buffer позволяют экспериментально балансировать вероятность покупки и нагрузку.

## Технологии

| Область | Технологии |
|---|---|
| Backend | Go 1.25.7+, Gin, pgx, Zap, Oops, Swaggo |
| Данные | PostgreSQL 18, pgvector, транзакции и row-level locking |
| Миграции и генерация | Goose, Go Jet |
| Frontend | React 19, TypeScript 6, Vite 8, React Router 8 |
| UI и данные | Mantine 9, TanStack Query 5, Zod, Tabler Icons |
| Frontend-архитектура | Feature-Sliced Design, Steiger |
| Тестирование | Go testing, race detector, Jest, React Testing Library, PostgreSQL integration, HTTP E2E/acceptance, k6 |
| Качество | golangci-lint, go vet, ESLint, Prettier, TypeScript typecheck |
| Наблюдаемость | Prometheus, Grafana, Dozzle, структурированные Zap-логи |
| Инфраструктура | Docker Compose, multi-stage Docker images, Nginx, Caddy, GitHub Actions |
| Опциональный AI | OpenAI embeddings API; детерминированный fallback без внешней зависимости |

## Архитектура и структура проекта

```mermaid
flowchart LR
    UI[React frontend] -->|HTTP + X-User-ID| API[Gin handlers]
    API --> UC[Use cases]
    UC --> REPO[PostgreSQL repositories]
    REPO --> DB[(PostgreSQL + pgvector)]
    WORKER[Reconciliation и outbox workers] --> REPO
    K6[k6] --> API
    K6 --> PROM[Prometheus]
    PROM --> GRAFANA[Grafana dashboard]
    PROM --> ADAPT[Adaptive queue controller]
    ADAPT --> REPO
    UC -. embeddings при включении .-> OPENAI[OpenAI API]
```

Backend разделён на HTTP-адаптеры, use case-слой и PostgreSQL repositories. Бизнес-инварианты проверяются в приложении и повторяются ограничениями базы. Frontend построен по Feature-Sliced Design: маршрутизация и провайдеры находятся в `app`, бизнес-сущности — в `entities`, действия пользователя — в `features`, страницы — в `pages`, переиспользуемые блоки — в `widgets`.

```text
.
├── cmd/                         # backend и служебные load-test команды
├── internal/
│   ├── adaptivequeue/           # контроллер динамической ёмкости очереди
│   ├── app/                     # конфигурация, HTTP, запуск и graceful shutdown
│   ├── pkg/domain/              # доменные модели, состояния и ошибки
│   ├── usecase/                 # сценарии каталога, очереди, checkout и оплаты
│   ├── repository/postgres/     # транзакции, SQL и реализация хранилищ
│   ├── worker/                  # reconciliation и notification outbox
│   ├── recommendation/openai/   # клиент embeddings
│   ├── e2e/                     # E2E и acceptance criteria
│   ├── loadtest/                # подготовка и проверка нагрузочных прогонов
│   └── mockapi/                 # автономные frontend-сценарии без PostgreSQL
├── frontend/
│   └── src/{app,entities,features,pages,shared,widgets}
├── migrations/                  # Goose-миграции и demo seed
├── loadtest/                    # k6, Prometheus и provisioned Grafana dashboard
├── scripts/                     # интеграционные и конкурентные проверки
├── docs/                        # frontend flow, UI/UX и Mock API
├── compose.yaml                 # основной локальный стек
├── Dockerfile                   # backend/migration image
└── Makefile                     # единые команды разработки и проверки
```

Дополнительная документация:

- [Frontend README](frontend/README.md) — архитектура, команды и восстановление маршрутов;
- [пользовательский frontend-flow](docs/frontend-flow.md);
- [UI/UX-правила](docs/frontend-ui.md);
- [Mock API](docs/mock-api.md);
- [нагрузочное тестирование](loadtest/README.md).

### Mock API для frontend

Backend можно запустить без PostgreSQL с готовыми сценариями очереди:

```bash
GOODQUEUE_MODE=mock go run ./cmd/goodqueue-backend
```

Mock использует те же frontend-маршруты и JSON-контракты, хранит изменения только в памяти и восстанавливает fixtures после перезапуска. Сценарии выбираются комбинацией UUID товара и одного из пользователей из `GET /api/v1/demo/users`: товары `11111111-1111-1111-1111-111111111111` и `22222222-2222-2222-2222-222222222222` показывают активные и terminal-статусы, `33333333-3333-3333-3333-333333333333` — sold out, `44444444-4444-4444-4444-444444444444` — выключенную очередь. Internal stock/payment маршруты в mock-режиме не регистрируются.

По умолчанию используется `GOODQUEUE_MODE=postgres`, и `GOODQUEUE_DATABASE_URL` остаётся обязательной. Полное описание fixtures, endpoint-ов и curl-сценариев находится в [документации Mock API](docs/mock-api.md).

## Пользовательский путь

1. Пользователь открывает лимитированный товар и нажимает «Купить».
2. Backend создаёт персональную попытку. При наличии свободного резерва пользователь сразу переходит в `checkout`; иначе — в `waiting`.
3. Клиент опрашивает состояние попытки. Когда место освобождается, самый ранний пользователь получает `invited` и ограниченное время на начало checkout.
4. Пользователь подтверждает переход к покупке. Сервис ещё раз проверяет владельца, товар, состояние и deadline, после чего переводит попытку в `checkout`.
5. В MVP пользователь нажимает «Оплатить и завершить покупку». Безопасный demo-endpoint проверяет владельца и активный checkout, после чего успешный результат завершает попытку как `purchased` и атомарно списывает товар. Отказ, отмена или expiry освобождают резерв и позволяют продвинуть следующего пользователя.
6. Если товар закончился, пользователь получает `sold_out` и может запросить доступные альтернативные лоты.

Такой путь уменьшает число поздних отказов: право выдаётся только под существующий резерв, неоплаченная попытка не удерживает товар бесконечно, а каждое состояние сообщает клиенту однозначный следующий шаг.

## Как устроена очередь

Запрос проходит через `handler → usecase → transactional PostgreSQL repository`. Операции одного товара сериализуются блокировкой строки товара и атомарно меняют попытку, остаток, резерв, payment inbox и notification outbox. Два фоновых worker-а:

- reconciliation переводит просроченные попытки, продвигает очередь и обрабатывает исчерпание остатка;
- outbox забирает уведомления с lease/fencing, повторяет неудачные публикации и сейчас передаёт их демонстрационному logging publisher-у.

### Почему PostgreSQL

PostgreSQL выбран не только как постоянное хранилище, но и как источник истины для конкурентного распределения товара:

- транзакции и `SELECT ... FOR UPDATE` сериализуют изменения одного товара без отдельного распределённого lock-сервиса; блокируются активные попытки и только нужные операции terminal-записи, а не вся история товара;
- ошибка инварианта одного товара исключает его до конца текущего reconciliation-цикла, поэтому TTL и очередь остальных товаров продолжают обрабатываться;
- ограничения, уникальные индексы и внешние ключи защищают инварианты даже при ошибке прикладного кода;
- `clock_timestamp()` даёт единое серверное время для invitation и checkout deadlines;
- payment inbox и notification outbox атомарно записываются вместе с изменением очереди;
- зрелые средства диагностики позволяют исследовать блокировки, contention и медленные запросы.

Для MVP это проще и надёжнее отдельной связки основной БД, Redis-lock и брокера. Горизонтальное масштабирование backend остаётся возможным: экземпляры координируются через транзакции PostgreSQL, а не через память процесса.

### Состояния

| Состояние | Значение |
|---|---|
| `waiting` | пользователь ждёт свободный резерв |
| `invited` | место выделено; есть 10 минут на старт checkout |
| `checkout` | оплата начата; есть 5 минут на результат |
| `purchased` | платёж принят, товар куплен |
| `invite_expired` | приглашение просрочено |
| `checkout_expired` | checkout просрочен |
| `payment_failed` | провайдер сообщил об отказе |
| `cancelled` | попытка отменена пользователем |
| `sold_out` | остаток стал нулевым до продвижения из `waiting` |

Рекомендуемое отображение состояний в клиенте:

| Состояние | Сообщение пользователю | Следующий шаг |
|---|---|---|
| `waiting` | «Вы в очереди. Мы сообщим, когда товар будет зарезервирован» | продолжать опрос состояния или отменить ожидание |
| `invited` | «Товар временно зарезервирован за вами» | показать deadline и кнопку перехода к покупке |
| `checkout` | «Товар сохранён за вами» | завершить прозрачную demo-оплату или отказаться |
| `purchased` | «Покупка подтверждена» | перейти к заказу |
| `invite_expired` | «Время на переход к покупке истекло» | при наличии товара встать в очередь заново |
| `checkout_expired` | «Время оформления истекло» | проверить альтернативы или начать новую попытку |
| `payment_failed` | «Оплата не прошла, резерв освобождён» | повторить путь с новым ключом или выбрать альтернативу |
| `cancelled` | «Вы вышли из очереди» | вернуться к товару |
| `sold_out` | «Товар закончился» | показать альтернативные лоты |

Активный клик может сразу создать попытку в `checkout`, если после продвижения всех более старых `waiting` ещё осталось место. Пользователь, уже стоящий в очереди, сначала получает `invited`, а затем сам вызывает старт checkout.

Все временные границы вычисляются по `clock_timestamp()` PostgreSQL. На точном равенстве deadline попытка уже просрочена. Повтор join или checkout возвращает существующую попытку и не продлевает срок.

### Остаток и резерв

- `allocatable_stock` — доступное для распределения количество товара;
- `reserved` — число попыток в `invited` и `checkout`;
- успешная покупка уменьшает и `allocatable_stock`, и `reserved` на один;
- отмена, expiry или неуспешный платёж освобождает только `reserved`;
- при нулевом остатке все `waiting` переходят в `sold_out`.

Ёмкость ожидания считается как `ceil(allocatable_stock × bufferPercent / 100)`. `GOODQUEUE_WAITING_BUFFER_PERCENT` по умолчанию равен `100`, допустимый диапазон — `0..500`. Например, при остатке `3` система допускает `3` резерва и ещё `3` записи `waiting`. Положительное уменьшение остатка не удаляет уже принятых `waiting`: новые входы блокируются, пока их число не станет меньше нового лимита. Уменьшить остаток ниже `reserved` нельзя.

### Адаптивная ёмкость ожидания

Опциональный контроллер может менять глобальный `bufferPercent` по результатам нагрузочных прогонов. Он выключен по умолчанию (`GOODQUEUE_ADAPTIVE_QUEUE_ENABLED=false`), поэтому базовое поведение остаётся полностью детерминированным.

При включении контроллер раз в `GOODQUEUE_ADAPTIVE_QUEUE_INTERVAL` читает из Prometheus:

- техническую успешность HTTP-запросов k6;
- число `purchased`, `cancelled` и `checkout_expired` исходов.

HTTP-успешность используется как предохранитель качества данных. Решение принимается только при достижении минимального количества HTTP-запросов и checkout-исходов и при успешности HTTP не ниже настроенного порога. Целевой запас ожидающих рассчитывается из ожидаемого числа неуспешных приглашений на одну покупку: `ceil(100 × (completed - purchased) / purchased)`. При нуле покупок используется настроенный максимум.

Результат ограничивается диапазоном `GOODQUEUE_ADAPTIVE_QUEUE_MIN_BUFFER_PERCENT..GOODQUEUE_ADAPTIVE_QUEUE_MAX_BUFFER_PERCENT`, а за один цикл процент меняется не более чем на `GOODQUEUE_ADAPTIVE_QUEUE_MAX_STEP_PERCENT`. При недоступном Prometheus, недостаточной выборке или деградации HTTP сохраняется последнее безопасное значение; начальным fallback всегда служит `GOODQUEUE_WAITING_BUFFER_PERCENT`. Снижение лимита не удаляет уже принятых пользователей.

Текущее решение и использованную выборку показывает `GET /internal/v1/adaptive-queue/status`. Адаптация глобальная, а не per-product: текущие k6-метрики не содержат достаточной устойчивой выборки конверсии для каждого товара. Endpoint-ы возвращают проценты успешности, а не статистические latency-процентили.

В текущем MVP вычисленное значение хранится в памяти процесса и после перезапуска начинается с `GOODQUEUE_WAITING_BUFFER_PERCENT`. Для нескольких backend-реплик адаптивный режим следует включать только после переноса состояния контроллера в общее хранилище; статический режим не имеет этого ограничения.

### FIFO и идемпотентность

- FIFO строгий внутри одного товара и определяется неизменяемым `queue_sequence`;
- join не принимает body: пользователь задаётся заголовком `X-User-ID`, ключ — заголовком `Idempotency-Key`;
- ключ join имеет scope `(user, product, key)`: тот же ключ возвращает ту же попытку, включая terminal state;
- новый ключ после terminal state создаёт новую попытку в хвосте; одновременно разрешена только одна активная попытка пользователя на товар;
- после `purchased` новый ключ может создать следующую попытку, если товар и очередь доступны; одновременно активной остаётся только одна попытка пользователя на товар;
- ключ содержит 1–128 ASCII-букв, цифр или символов `. _ : -` и начинается с буквы или цифры.

### Право на покупку и защита от обхода

Отдельный передаваемый токен права в MVP не выдаётся. Правом является серверная попытка в состоянии `invited` или `checkout`, связанная с конкретными `external_user_id`, `product_id` и `attempt_id`.

- старт checkout принимает `attemptID`, но обязательно сверяет владельца с `X-User-ID`;
- попытку другого пользователя использовать нельзя, даже если известен её UUID;
- приглашение относится только к одному товару и не даёт доступа к checkout другого товара;
- начать checkout можно только из допустимого активного состояния и до PostgreSQL deadline;
- повтор запроса идемпотентен и не продлевает срок права;
- уникальные ограничения запрещают несколько активных попыток одного пользователя на товар и повторное принятие одной платёжной ссылки; завершённые покупки сохраняются как независимая история;
- публичного маршрута, создающего покупку в обход queue attempt, у сервиса нет;
- блокировка строки товара и проверка `reserved <= allocatable_stock` не позволяют конкурентным запросам выделить больше прав, чем доступно единиц.

В production `X-User-ID` должен поступать только от доверенного API gateway после штатной аутентификации Авито. Клиентский заголовок используется исключительно для автономной демонстрации MVP.

## API

Публичные маршруты:

| Метод | Путь | Назначение |
|---|---|---|
| `GET` | `/healthz` | процесс работает |
| `GET` | `/readyz` | PostgreSQL доступен |
| `GET` | `/docs` | переход в Swagger UI |
| `GET` | `/docs/doc.json` | Swagger JSON |
| `GET` | `/api/v1/products` | товары, остатки и заполнение очереди |
| `GET` | `/api/v1/products/:productID` | один товар |
| `GET` | `/api/v1/products/:productID/alternatives` | до четырёх доступных рекомендаций с режимом, score и причиной |
| `POST` | `/api/v1/products/:productID/queue-entries` | войти в очередь; body отсутствует |
| `GET` | `/api/v1/products/:productID/queue-entry` | активная или последняя попытка пользователя |
| `DELETE` | `/api/v1/products/:productID/queue-entry` | отменить активную попытку |
| `POST` | `/api/v1/queue-attempts/:attemptID/checkout` | начать или повторить checkout |
| `POST` | `/api/v1/products/:productID/queue-attempts/:attemptID/demo-payment` | безопасно и идемпотентно имитировать успешную оплату владельцем checkout |
| `GET` | `/api/v1/demo/users` | пять демонстрационных пользователей |

Внутренние маршруты:

| Метод | Путь | Назначение |
|---|---|---|
| `POST` | `/internal/v1/products/:productID/stock-adjustments` | идемпотентно изменить остаток; регистрируется только при явном включении |
| `POST` | `/internal/v1/payment-events` | демонстрационный callback оплаты; регистрируется только при явном включении |
| `GET` | `/internal/v1/loadtest/request-success-rate` | техническая успешность HTTP-запросов k6 за настроенное окно |
| `GET` | `/internal/v1/loadtest/purchase-success-rate` | доля `purchased` среди завершённых checkout-исходов k6 |
| `GET` | `/internal/v1/adaptive-queue/status` | текущий процент waiting buffer, целевое значение, причина решения и качество выборки |

### Пример

После запуска Compose доступны 12 товаров из нескольких категорий и пять пользователей с UUID от `...0001` до `...0005`.

```bash
BASE=http://localhost:2001
USER_ID=00000000-0000-4000-8000-000000000001
USER_ID_2=00000000-0000-4000-8000-000000000002
PRODUCT_ID=22222222-2222-2222-2222-222222222222

# Состояние сервиса, Swagger JSON, товары и демо-пользователи
curl "$BASE/healthz"
curl "$BASE/readyz"
curl "$BASE/docs/doc.json"
curl "$BASE/api/v1/products"
curl "$BASE/api/v1/products/$PRODUCT_ID"
curl "$BASE/api/v1/products/$PRODUCT_ID/alternatives"
curl "$BASE/api/v1/demo/users"

# Join без request body. Сохраните attempt_id из ответа.
curl -X POST \
  -H "X-User-ID: $USER_ID" \
  -H 'Idempotency-Key: demo-join-1' \
  "$BASE/api/v1/products/$PRODUCT_ID/queue-entries"

curl -H "X-User-ID: $USER_ID" \
  "$BASE/api/v1/products/$PRODUCT_ID/queue-entry"

ATTEMPT_ID=<attempt_id>
curl -X POST -H "X-User-ID: $USER_ID" \
  "$BASE/api/v1/queue-attempts/$ATTEMPT_ID/checkout"

# Безопасная успешная demo-оплата. Повтор с тем же ключом не спишет товар второй раз.
curl -X POST \
  -H "X-User-ID: $USER_ID" \
  -H 'Idempotency-Key: demo-payment-1' \
  "$BASE/api/v1/products/$PRODUCT_ID/queue-attempts/$ATTEMPT_ID/demo-payment"

# Оба небезопасных demo-маршрута нужно явно включить в .env и перезапустить backend:
# GOODQUEUE_UNSAFE_PAYMENT_CALLBACK=true
# GOODQUEUE_UNSAFE_STOCK_ADJUSTMENT=true
# Для failed передайте пустую payment_reference.
curl -X POST -H 'Content-Type: application/json' \
  -d "{\"provider\":\"demo\",\"event_id\":\"demo-payment-1\",\"attempt_id\":\"$ATTEMPT_ID\",\"outcome\":\"succeeded\",\"payment_reference\":\"demo-reference-1\"}" \
  "$BASE/internal/v1/payment-events"

# Идемпотентная корректировка остатка.
curl -X POST \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: demo-stock-1' \
  -d '{"delta":1,"reason":"demo replenishment","external_reference":"demo-restock-1"}' \
  "$BASE/internal/v1/products/$PRODUCT_ID/stock-adjustments"

# Отдельная попытка второго демо-пользователя и её отмена.
curl -X POST \
  -H "X-User-ID: $USER_ID_2" \
  -H 'Idempotency-Key: demo-join-cancel-1' \
  "$BASE/api/v1/products/$PRODUCT_ID/queue-entries"
curl -X DELETE -H "X-User-ID: $USER_ID_2" \
  "$BASE/api/v1/products/$PRODUCT_ID/queue-entry"
```

Для stock adjustment scope ключа — `(product, Idempotency-Key)`. Повтор с тем же нормализованным payload возвращает сохранённый ответ; другой payload с тем же ключом даёт конфликт. Сохраняются и успешные, и отклонённые результаты.

## Рекомендации похожих товаров

`GET /api/v1/products/:productID/alternatives` продолжает покупательский сценарий после `sold_out`, `checkout_expired` или отказа от ожидания. Ответ сохраняет поля обычной карточки товара и добавляет:

- `recommendation_mode`: `ai_semantic` или `catalog_fallback`;
- `recommendation_score`: гибридная оценка от `0` до `1`;
- `reason_code`: стабильный код для объяснения рекомендации в интерфейсе.

Frontend использует один переиспользуемый блок рекомендаций на странице товара, во время ожидания и в восстановимых terminal-состояниях. Переход к альтернативному товару не отменяет действующую очередь исходного товара: пользователь может изучать другие предложения и вернуться к своей попытке.

При включённом AI текст карточки — название, описание, категория и цена — пакетно преобразуется моделью embeddings в вектор из 1536 измерений. Вектор и SHA-256 содержимого сохраняются в `product_embeddings`; повторный запрос к провайдеру выполняется только для новой или изменившейся карточки. PostgreSQL advisory lease на модель не допускает одинаковых refresh-запросов к OpenAI одновременно с нескольких экземпляров backend; проигравший запрос использует готовый semantic cache или fallback. pgvector ранжирует доступных кандидатов по cosine similarity, категории и свободному остатку. Исходный товар, выключенная очередь и товары без свободного остатка всегда исключаются.

AI не является зависимостью основного сценария. По умолчанию он выключен. При ошибке или timeout внешнего API endpoint использует уже сохранённые векторы, а если их нет — детерминированный `catalog_fallback`: сначала возвращаются только доступные товары той же категории, ранжированные по близости цены и свободному остатку. К межкатегорийным доступным предложениям fallback переходит лишь тогда, когда в исходной категории не осталось вариантов; frontend честно подписывает такой блок как «Другие доступные товары». Без ключа этот режим работает сразу. Поэтому отказ AI не влияет на очередь, checkout и возможность продолжить пользовательский путь.

Для локального включения добавьте ключ только в `.env`, не в Git:

```dotenv
GOODQUEUE_RECOMMENDATIONS_AI_ENABLED=true
GOODQUEUE_OPENAI_API_KEY=<secret>
```

По умолчанию используется `text-embedding-3-small` через `/v1/embeddings`. Провайдер изолирован интерфейсом `EmbeddingProvider`; модель, base URL и timeout задаются конфигурацией без изменения бизнес-логики. Распределение дефицитного остатка AI не контролирует — оно по-прежнему выполняется только транзакционной очередью PostgreSQL.

### Изображения товаров

Демонстрационный каталог содержит 12 локальных WebP-изображений в `frontend/public/product-images`. Backend возвращает относительные пути `/product-images/...`, а Nginx раздаёт файлы с того же origin, что и frontend. Поэтому каталог, checkout и рекомендации не зависят от внешних placeholder-сервисов, блокировок сети или доступности стороннего CDN. Встроенный нейтральный SVG остаётся аварийным fallback на случай отсутствующего или повреждённого файла.

## Продуктовые решения

- **Ограниченный waiting buffer.** Очередь не растёт бесконечно относительно остатка. Это снижает бесполезное ожидание и нагрузку, но оставляет запас пользователей на случай отказов и expiry.
- **Два временных этапа.** Короткое приглашение отделено от checkout. Пользователь сначала подтверждает готовность купить, а затем получает отдельное время на оформление.
- **Однозначные terminal states.** Клиент всегда может объяснить результат и предложить действие вместо неопределённого «что-то пошло не так».
- **Объяснимые похожие лоты.** После `sold_out` пользователь не попадает в тупик: API возвращает до четырёх доступных семантически похожих товаров, а `reason_code` позволяет интерфейсу объяснить предложение. Детерминированный fallback сохраняет сценарий при отказе AI.
- **Идемпотентные пользовательские и внутренние команды.** Повторы из-за нестабильной сети не создают дубликаты попыток, списаний или платёжных событий.
- **Истечение права без ручного вмешательства.** Reconciliation worker освобождает забытые резервы и автоматически продвигает очередь.

## Payment callback и outbox

Публичный demo-маршрут `/api/v1/products/:productID/queue-attempts/:attemptID/demo-payment` принимает `X-User-ID` и `Idempotency-Key`, сверяет пользователя, товар, attempt и активное состояние `checkout`, а затем передаёт детерминированное событие в тот же payment inbox, который используется интеграцией провайдера. Повтор запроса безопасен: уже завершённая покупка возвращается без второго списания.

`/internal/v1/payment-events` и `/internal/v1/products/:productID/stock-adjustments` — небезопасные MVP-маршруты без production-аутентификации. По умолчанию `GOODQUEUE_UNSAFE_PAYMENT_CALLBACK=false` и `GOODQUEUE_UNSAFE_STOCK_ADJUSTMENT=false`, поэтому оба маршрута отсутствуют. Включайте их только явно для локальной демонстрации и не публикуйте в сеть.

Payment inbox обеспечивает scope `(provider, event_id)`: завершённый повтор с тем же каноническим payload возвращает точно сохранённые HTTP status и body, а изменённый payload получает конфликт. Любой успешный платёж вне состояния `checkout`, включая преждевременный callback для `waiting` или `invited`, не меняет право и создаёт событие `payment.compensation_required`; в реальной интеграции его обработчик должен запустить возврат или ручную сверку.

Продвижение `waiting → invited` и запись `queue.invited` выполняются в одной транзакции. Outbox worker отбрасывает устаревшие приглашения, повторяет временные ошибки с backoff и защищает завершение lease token/generation. Текущий publisher только пишет событие в Zap-лог; внешнего брокера, почты или push-провайдера нет.

## Запуск

Для основного сценария нужен только Docker Compose. Go `1.25.7+`, Node.js `24.19.x`, npm `11.17.x`, Make и k6 требуются лишь при запуске компонентов и проверок вне контейнеров.

Используйте команды из раздела «Быстрый запуск»: они явно связывают production-сборку frontend с локальным backend и разрешают правильный CORS origin. Секреты при необходимости задаются через локальный `.env`, который не должен попадать в Git.

Compose выполняет миграции перед стартом backend, по умолчанию отключает небезопасные internal mutation-маршруты и публикует сервисы только на loopback-интерфейсе. Порты можно изменить через `GOODQUEUE_POSTGRES_PORT`, `GOODQUEUE_HTTP_PORT`, `GOODQUEUE_FRONTEND_PORT` и `GOODQUEUE_DOZZLE_PORT`.

Swagger UI: <http://localhost:2001/docs>.

Логи контейнеров доступны в Dozzle: <http://localhost:9999>. UI показывает только контейнеры текущего Compose-проекта, включая backend, PostgreSQL, migration, Prometheus и Grafana из loadtest overlay. Управление контейнерами и shell отключены. Dozzle подключается к Docker через `/var/run/docker.sock`, поэтому предназначен только для доверенного локального окружения и не публикуется во внешнюю сеть.

Нагрузочный observability-контур запускается командой `make loadtest-observability-up`: Prometheus доступен на <http://localhost:9090>, а Grafana с автоматически provisioned dashboard **GoodQueue — нагрузка и конверсия** — на <http://localhost:2002>. Локальные credentials по умолчанию: `admin` / `goodqueue`; для любой внешней среды пароль необходимо переопределить.

### Быстрая проверка для жюри

После запуска Compose проверка занимает несколько минут:

1. Открыть <http://localhost:2000>, выбрать демонстрационного пользователя и товар.
2. Нажать «Купить» и проверить переход на ожидание, приглашение или checkout в зависимости от остатка.
3. На экране ожидания проверить позицию, прошедшее время, отмену и блок похожих товаров.
4. Открыть тот же дефицитный товар от другого пользователя: число активных прав не должно превысить остаток.
5. Проверить возврат к товару, повтор после terminal state и восстановление экрана после обновления страницы.
6. Для backend-контракта открыть Swagger и пройти join с повторным `Idempotency-Key`.
7. На checkout нажать «Оплатить и завершить покупку» и проверить переход в `purchased`; внутренний callback для этого включать не требуется.

Готовые HTTP-команды приведены в разделе «Пример». Для повторяемой демонстрации с исходными seed-данными удалите только локальный Compose volume:

```powershell
docker compose down --volumes
$env:GOODQUEUE_POSTGRES_PASSWORD = "goodqueue-local"
$env:VITE_API_URL = "http://localhost:2001"
$env:GOODQUEUE_CORS_ALLOWED_ORIGINS = "http://localhost:2000,http://127.0.0.1:2000"
docker compose up --build -d
```

Если локальный PostgreSQL уже занимает `5432`, задайте другой внешний порт. Внутренний адрес БД между контейнерами останется `postgres:5432`:

```bash
GOODQUEUE_POSTGRES_PASSWORD=goodqueue-local GOODQUEUE_POSTGRES_PORT=15432 docker compose up --build -d
```

В PowerShell эквивалентная команда выглядит так:

```powershell
$env:GOODQUEUE_POSTGRES_PORT = "15432"
$env:GOODQUEUE_POSTGRES_PASSWORD = "goodqueue-local"
docker compose up --build -d
```

Для запуска backend без Compose сначала поднимите PostgreSQL, экспортируйте переменные из `.env` и примените миграции:

```bash
set -a; . ./.env; set +a
make migrate-up DATABASE_URL="$GOODQUEUE_DATABASE_URL"
make run
```

`make migrate-*` использует `DATABASE_URL`, а приложение — `GOODQUEUE_DATABASE_URL`. Значение по умолчанию для Makefile: `postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable`.

### Конфигурация

Все runtime-переменные перечислены в `.env.example`:

| Группа | Переменные |
|---|---|
| HTTP, CORS и shutdown | `GOODQUEUE_HTTP_ADDRESS`, `GOODQUEUE_HTTP_READ_HEADER_TIMEOUT`, `GOODQUEUE_CORS_ALLOWED_ORIGINS`, `GOODQUEUE_SHUTDOWN_TIMEOUT` |
| Локальные UI | `GOODQUEUE_DOZZLE_PORT` (Dozzle, по умолчанию `9999`); Grafana настраивается через `LOADTEST_GRAFANA_*` в `loadtest/.env` |
| Режим | `GOODQUEUE_MODE` (`postgres` по умолчанию или `mock`) |
| PostgreSQL | `GOODQUEUE_DATABASE_URL` (обязательна в режиме `postgres`), `GOODQUEUE_DATABASE_PING_TIMEOUT`, `GOODQUEUE_DATABASE_MAX_OPEN_CONNS`, `GOODQUEUE_DATABASE_MAX_IDLE_CONNS`, `GOODQUEUE_DATABASE_CONN_MAX_LIFETIME` |
| Очередь | `GOODQUEUE_INVITATION_TTL`, `GOODQUEUE_CHECKOUT_TTL`, `GOODQUEUE_WAITING_BUFFER_PERCENT` |
| Адаптивная очередь | `GOODQUEUE_ADAPTIVE_QUEUE_ENABLED`, `GOODQUEUE_ADAPTIVE_QUEUE_INTERVAL`, `GOODQUEUE_ADAPTIVE_QUEUE_MIN_HTTP_REQUESTS`, `GOODQUEUE_ADAPTIVE_QUEUE_MIN_CHECKOUT_OUTCOMES`, `GOODQUEUE_ADAPTIVE_QUEUE_MIN_HTTP_SUCCESS_PERCENT`, `GOODQUEUE_ADAPTIVE_QUEUE_MIN_BUFFER_PERCENT`, `GOODQUEUE_ADAPTIVE_QUEUE_MAX_BUFFER_PERCENT`, `GOODQUEUE_ADAPTIVE_QUEUE_MAX_STEP_PERCENT` |
| Worker limits | `GOODQUEUE_WORKER_INTERVAL`, `GOODQUEUE_RECONCILIATION_TRANSITION_BATCH_SIZE`, `GOODQUEUE_MAX_PRODUCTS_PER_CYCLE`, `GOODQUEUE_MAX_OUTBOX_ITEMS_PER_CYCLE` |
| Outbox | `GOODQUEUE_OUTBOX_LEASE_DURATION`, `GOODQUEUE_OUTBOX_RETRY_BASE_DURATION`, `GOODQUEUE_OUTBOX_RETRY_MAX_DURATION`, `GOODQUEUE_PUBLISHER_TIMEOUT` |
| AI-рекомендации | `GOODQUEUE_RECOMMENDATIONS_AI_ENABLED`, `GOODQUEUE_OPENAI_API_KEY`, `GOODQUEUE_OPENAI_BASE_URL`, `GOODQUEUE_OPENAI_EMBEDDING_MODEL`, `GOODQUEUE_OPENAI_EMBEDDING_TIMEOUT` |
| Остальное | `GOODQUEUE_LOG_LEVEL`, `GOODQUEUE_UNSAFE_PAYMENT_CALLBACK`, `GOODQUEUE_UNSAFE_STOCK_ADJUSTMENT` |

### Миграции, генерация и проверки

```bash
make build                # собрать backend
make run                  # запустить backend из текущего окружения
make compose-up           # Compose без локальных frontend overrides
make compose-down         # остановить локальный стек

make migrate-status       # состояние Goose
make migrate-up           # применить миграции
make migrate-down         # откатить одну миграцию

make swagger              # обновить Swaggo-файлы
make swagger-check        # проверить отсутствие Swagger drift
make jet-generate         # миграции и обновление Go Jet-кода
make jet-check            # миграции и проверка Go Jet drift
make generate             # Swagger и Go Jet

make test                 # go test ./...
make test-race            # go test -race ./...
make test-e2e             # E2E против уже поднятых backend и PostgreSQL
make test-ac              # только acceptance criteria кейса
make format-check         # проверить gofmt и goimports без изменения файлов
make vet
make lint
make verify               # format, Swagger, test, race, vet, lint, build
make verify-integration   # изолированные миграции, Jet, PostgreSQL и HTTP
make verify-all           # все проверки
```

Для интеграционных repository-тестов используется `GOODQUEUE_TEST_DATABASE_URL`; `make verify-integration` создаёт отдельный Compose project с временными портами и удаляет его после проверки. Если Docker сообщает `no space left on device`, сначала проверьте объём неиспользуемого build cache: это ограничение локального окружения, а не поведение очереди.

E2E-suite использует `GOODQUEUE_E2E_BASE_URL` и `GOODQUEUE_E2E_DATABASE_URL`. Он запускает пользовательские сценарии через реальный HTTP API, а PostgreSQL использует для изоляции seed-данных и проверки финальных инвариантов. `make verify-integration` задаёт эти переменные автоматически и запускает E2E в отдельном временном Compose project.

Acceptance-тесты в том же suite явно проверяют критерии кейса: ровно одно право при конкурентной покупке последней единицы, персональность права и невозможность обойти очередь, наличие однозначных клиентских состояний и альтернатив после `sold_out`. `make test-ac` запускает только эти сценарии; полный `make test-e2e` запускает acceptance и остальные сквозные проверки вместе.

Frontend проверяется отдельно:

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm run steiger
npm run format:check
npm test -- --runInBand
npm run build
```

В актуальном проекте проходят 43 frontend test suites и 261 тест, включая сквозные сценарии каталога, очереди, восстановления после reload, demo-оплаты, повторной покупки, terminal states и рекомендаций. GitHub Actions запускает backend static/unit/race pipeline, отдельную PostgreSQL integration/runtime job и полный frontend quality pipeline.

Выбор линтеров связан с рисками проекта:

- `errcheck`, `errorlint` и `go vet` помогают не потерять ошибки транзакций и SQL;
- `gosec` проверяет типовые проблемы безопасности backend;
- `revive`, `gofmt` и `goimports` поддерживают единый Go-стиль;
- ESLint проверяет TypeScript, React Hooks, TanStack Query, исчерпывающие `switch` по состояниям и порядок импортов;
- Steiger контролирует границы слоёв Feature-Sliced Design;
- Prettier отделяет форматирование frontend от смысловых правил ESLint.

### Конкурентная проверка очереди

HTTP API можно проверить параллельными запросами без сторонних нагрузочных инструментов. Тест требует чистого demo-стека: у товара `11111111-1111-1111-1111-111111111111` должен быть остаток `1` и резерв `0`.

```powershell
docker compose down --volumes
$env:GOODQUEUE_POSTGRES_PASSWORD = "goodqueue-local"
$env:VITE_API_URL = "http://localhost:2001"
$env:GOODQUEUE_CORS_ALLOWED_ORIGINS = "http://localhost:2000,http://127.0.0.1:2000"
docker compose up --build -d
go run scripts/queue_load.go -requests 20 -base-url http://localhost:2001
```

Скрипт одновременно отправляет 20 join-запросов от разных пользователей и проверяет, что выделен ровно один резерв, `reserved` не превышает остаток, остальные допустимые заявки ждут или получают `queue_full`, а повтор с тем же `Idempotency-Key` возвращает ту же попытку. Число запросов и адрес можно изменить:

```bash
go run scripts/queue_load.go -requests 100 -base-url http://localhost:2001
```

Полный нагрузочный контур k6 с профилями smoke/medium/main, исходами purchase/cancel/TTL, постоянными отчётами PostgreSQL и метриками Prometheus описан в [loadtest/README.md](loadtest/README.md).

Backend-конфигурация по умолчанию разрешает dev-server origins `http://localhost:5173` и `http://127.0.0.1:5173`; команды быстрого запуска дополнительно разрешают Docker frontend на `2000`. Для другого origin задайте точный список в `GOODQUEUE_CORS_ALLOWED_ORIGINS`; wildcard намеренно не поддерживается.

## Команда и вклад

Распределение ниже основано на исходных зонах ответственности и подтверждено историей коммитов. В процессе интеграции участники также исправляли смежные части проекта.

| Участник | Основная зона | Реализованный вклад |
|---|---|---|
| [Кочегаров Данил — Woolfer0097](https://github.com/Woolfer0097) | Backend B: бизнес-логика очереди | Инициализация репозитория и backend-каркаса; state machine очереди; двухэтапные `invited → checkout` права; payment inbox и notification outbox; reconciliation worker; интеграционные конкурентные сценарии; CI и production-конфигурация Compose/Caddy |
| [Любченко Влад — Zoom7122](https://github.com/Zoom7122) | Backend C: API и интеграция | HTTP-контракты и Mock API для frontend; единые ошибки и CORS; позиция и размер очереди; k6-контур, seed/verify-команды и профили нагрузки; сценарии purchase outcomes; Prometheus-агрегация метрик; Dozzle; исправление версий миграций |
| [Поздняков Никита — cunofou](https://github.com/cunofou) | Backend A: инфраструктура и данные | PostgreSQL repositories, миграции, seed, индексы и `SELECT ... FOR UPDATE`; интеграция backend MVP; конкурентные, E2E и acceptance-тесты; защита внутренних маршрутов и hardening воркеров; AI-рекомендации и pgvector fallback; Grafana dashboard и адаптивная ёмкость очереди; продуктовая документация |
| [Зубков Иван — Zis1220](https://github.com/Zis1220) | Frontend | React/TypeScript/Vite-приложение и Feature-Sliced Design; Mantine UI; каталог и карточка товара; выбор demo-пользователя; polling очереди; экраны waiting, reservation, checkout и result; таймеры, retry, восстановление после reload и queue-aware навигация; похожие товары; responsive UX; frontend unit/integration-тесты и документация |

## История разработки

Проект развивался небольшими тематическими ветками и pull request-ами. Основные вехи видны в Git-истории:

| Коммит | Результат |
|---|---|
| [`4f0635a`](https://github.com/Woolfer0097/GoodQueue/commit/4f0635a) | базовая backend-структура, Docker, миграции, Swagger и линтер |
| [`3d2e471`](https://github.com/Woolfer0097/GoodQueue/commit/3d2e471) | транзакционные PostgreSQL repositories, индексы и блокировки |
| [`437d67c`](https://github.com/Woolfer0097/GoodQueue/commit/437d67c) | полноценная очередь, payment/outbox, workers и конкурентные integration-тесты |
| [`25f322c`](https://github.com/Woolfer0097/GoodQueue/commit/25f322c) | API-контракт, Mock API и интеграция с frontend |
| [`2e22390`](https://github.com/Woolfer0097/GoodQueue/commit/2e22390) | PostgreSQL/k6 нагрузочный контур |
| [`addf27c`](https://github.com/Woolfer0097/GoodQueue/commit/addf27c) | frontend bootstrap, TypeScript tooling и FSD |
| [`ee5935e`](https://github.com/Woolfer0097/GoodQueue/commit/ee5935e) | backend E2E и acceptance coverage требований кейса |
| [`230f863`](https://github.com/Woolfer0097/GoodQueue/commit/230f863) | AI-powered рекомендации с pgvector и fallback |
| [`af756bb`](https://github.com/Woolfer0097/GoodQueue/commit/af756bb) | provisioned Grafana dashboard |
| [`4494beb`](https://github.com/Woolfer0097/GoodQueue/commit/4494beb) | hardening конкурентности и внутренних маршрутов |
| [`e18d2ac`](https://github.com/Woolfer0097/GoodQueue/commit/e18d2ac) | полный frontend queue flow |
| [`54b278d`](https://github.com/Woolfer0097/GoodQueue/commit/54b278d) | метрико-управляемая адаптивная ёмкость очереди |
| [`8fe7c3e`](https://github.com/Woolfer0097/GoodQueue/commit/8fe7c3e) | рекомендации похожих товаров во всём пользовательском пути |

Полная история доступна в [`git log`](https://github.com/Woolfer0097/GoodQueue/commits/main/); merge-коммиты сохраняют связь изменений с отдельными ветками и pull request-ами.

## Ограничения текущего MVP

- `X-User-ID` имитирует результат внешней аутентификации и не является самостоятельным механизмом защиты;
- публичная demo-оплата имитирует только успешный ответ провайдера и не списывает реальные деньги; внутренний callback без проверки подписи включается только локально для технических сценариев;
- полноценный checkout, списание денег и возвраты принадлежат внешним системам; compensation пока представлена outbox-событием;
- logging publisher подтверждает механику outbox, но не отправляет push, email или сообщения в брокер;
- состояние очереди клиент получает polling-запросами; WebSocket/SSE и реальные пользовательские уведомления не реализованы;
- embeddings обновляются лениво при первом запросе рекомендаций после изменения каталога; отдельного фонового catalog-indexer пока нет;
- адаптивный процент waiting buffer хранится в памяти процесса; multi-instance режим требует общего хранилища состояния контроллера;
- численные production SLO и результаты большого нагрузочного прогона должны быть зафиксированы на целевой инфраструктуре.

Эти ограничения не ослабляют основные инварианты MVP: FIFO внутри товара, персональность права, ограниченные deadlines, атомарное резервирование и отсутствие oversell.

## Использование ИИ

При разработке применялся OpenAI Codex как инженерный ассистент:

- для анализа требований кейса и проверки пограничных конкурентных сценариев;
- для интеграции частей backend, code review и поиска потенциальных нарушений инвариантов;
- для подготовки тестовых сценариев, команд локальной проверки и документации;
- для запуска автоматических проверок и интерпретации их результатов.
- для создания 12 демонстрационных product-shot изображений без брендов и текста; изображения сохранены локально в WebP и не требуют runtime-вызовов ИИ.

В runtime OpenAI embeddings опционально используются только для поиска похожих доступных товаров. ИИ не участвует в распределении очереди, резервировании, проверке права или списании остатка; при его отказе работает детерминированный fallback. Все изменения проверялись исходным кодом, unit/integration-тестами и запуском через Docker Compose; ответственность за итоговые продуктовые и архитектурные решения остаётся у команды.
