# GoodQueue

GoodQueue — full-stack MVP пользовательской очереди для покупки дефицитных товаров на классифайде. Сервис ставит покупателей в FIFO-очередь перед checkout, выдаёт персональное право на покупку с ограниченным сроком и не позволяет продать одну единицу товара нескольким пользователям.

Если товар закончился, GoodQueue предлагает доступные похожие объявления — с AI-ранжированием при наличии OpenAI API и детерминированным fallback без внешней зависимости.

## Демо

MVP развёрнут и доступен для проверки:

| Сервис | Адрес |
|---|---|
| Пользовательский интерфейс | <https://mscontractor.golddraft.ru/> |
| Backend readiness | <https://mscontractor.golddraft.ru/api/readyz> |
| Grafana — нагрузка и метрики | <https://mscontractor.golddraft.ru/grafana/> |
| Dozzle — логи контейнеров | <https://mscontractor.golddraft.ru/dozzle/> |
| Load-test runner health | <https://mscontractor.golddraft.ru/loadtest-runner/healthz> |
| Репозиторий | <https://github.com/Woolfer0097/GoodQueue> |

Для демонстрации регистрация и настоящая авторизация заменены выбором тестового пользователя, а реальный платёж — безопасной demo-оплатой.

Grafana, Dozzle и load-test runner являются служебными интерфейсами. На внешнем стенде они должны быть закрыты аутентификацией и сетевыми ограничениями; Prometheus наружу не проксируется.

## Команда и вклад

История коммитов показывает разделение проекта на четыре устойчивые зоны ответственности. При интеграции участники также проводили ревью и исправляли смежные части.

| Участник | Роль и результат |
|---|---|
| [Кочегаров Данил — Woolfer0097](https://github.com/Woolfer0097) | Backend и интеграция: заложил каркас сервиса и state machine, реализовал двухэтапное право, payment inbox/outbox и workers; связывал изменения команды через PR, CI и production Compose/Caddy |
| [Любченко Влад — Zoom7122](https://github.com/Zoom7122) | Backend API и нагрузка: развивал HTTP-контракты, Mock API, обработку ошибок и позицию очереди; собрал k6-контур, Prometheus-метрики, Grafana runner и Dozzle |
| [Поздняков Никита — cunofou](https://github.com/cunofou) | Backend и данные: реализовал PostgreSQL repositories, миграции, транзакционные блокировки и hardening; добавил E2E/AC, AI-рекомендации, pgvector fallback, Grafana, адаптивную очередь и продуктовую документацию |
| [Зубков Иван — Zis1220](https://github.com/Zis1220) | Frontend: построил React/FSD-приложение и весь пользовательский путь от каталога до результата; реализовал polling, восстановление после reload, рекомендации, responsive UX и frontend-тесты |

## Проблема и ценность решения

При ажиотажном спросе несколько покупателей могут одновременно дойти до оплаты последней единицы. Часть пользователей тратит время и только в конце узнаёт, что товар уже продан. Для площадки это означает поздние отказы, лишнюю нагрузку и риск некорректных заказов.

GoodQueue переносит конкуренцию за товар в прозрачный управляемый процесс:

- товар резервируется атомарно и не продаётся дважды;
- пользователь всегда видит своё состояние и следующий шаг;
- забытый резерв автоматически освобождается по TTL;
- следующий участник приглашается в порядке FIFO;
- неудовлетворённый спрос остаётся на платформе благодаря похожим товарам;
- метрики позволяют настраивать вместимость очереди на основе фактической конверсии.

## Что реализовано в MVP

- каталог и карточки лимитированных товаров;
- очередь со статусом, позицией и возможностью выйти;
- два временных этапа права: `invited → checkout`;
- demo-оплата с атомарным списанием остатка;
- повторная покупка товара после завершённой покупки;
- восстановление активного сценария после обновления страницы;
- финальные состояния для отмены, expiry, ошибки оплаты и sold out;
- рекомендации только доступных похожих товаров;
- опциональные OpenAI embeddings и поиск по векторам `pgvector`;
- адаптивная вместимость waiting-очереди по метрикам HTTP и покупок;
- Prometheus, provisioned Grafana dashboard и просмотр логов через Dozzle;
- k6-нагрузочные сценарии, unit, integration, E2E и acceptance-тесты;
- Swagger, линтеры и GitHub Actions CI.

Регистрация, настоящая авторизация, полноценное оформление заказа и реальное списание денег считаются существующими внешними системами Авито и не дублируются в MVP.

## Пользовательский путь

1. Покупатель выбирает товар и нажимает «Купить».
2. Backend создаёт персональную попытку. При свободном остатке пользователь сразу получает резерв; иначе занимает место в `waiting`.
3. Когда резерв освобождается, первый пользователь получает `invited` и 10 минут на переход к checkout.
4. Перед checkout backend повторно проверяет пользователя, товар, состояние и deadline.
5. На оплату отводится 5 минут. Успешная demo-оплата переводит попытку в `purchased` и атомарно уменьшает остаток.
6. Отмена или истечение срока освобождает резерв и продвигает следующего пользователя.
7. При `sold_out` интерфейс показывает похожие доступные товары.

Переход на рекомендованный товар сам по себе не удаляет пользователя из текущей очереди: он может вернуться к активной попытке или явно отменить её.

### Состояния очереди

| Состояние | Что происходит | Следующий шаг |
|---|---|---|
| `waiting` | пользователь ожидает резерв | следить за позицией или выйти |
| `invited` | товар временно выделен пользователю | перейти к checkout до deadline |
| `checkout` | резерв удерживается на время оплаты | завершить оплату |
| `purchased` | покупка подтверждена | завершить путь или купить ещё |
| `invite_expired` | срок приглашения истёк | начать новую попытку |
| `checkout_expired` | срок checkout истёк | начать заново или выбрать аналог |
| `payment_failed` | оплата отклонена | повторить путь или выбрать аналог |
| `cancelled` | пользователь вышел из очереди | вернуться к товару |
| `sold_out` | доступный остаток исчерпан | выбрать похожий товар |

## Критические технические решения

### Защита от oversell

PostgreSQL является единственным источником истины. Все изменения одного товара проходят в транзакции с `SELECT ... FOR UPDATE`: попытка, резерв, остаток, payment inbox и notification outbox меняются согласованно. Ограничения и индексы базы дублируют ключевые инварианты приложения.

Поэтому два конкурентных запроса не могут получить право на последнюю единицу: транзакции будут обработаны последовательно, а вторая увидит уже занятый резерв.

### Персональное и одноразовое право

Frontend передаёт пользователя через демонстрационный `X-User-ID`, но решение всегда принимает backend. Начать checkout или оплатить можно только для своей активной попытки, нужного товара, разрешённого состояния и непросроченного deadline.

Идемпотентные ключи защищают повторные HTTP-запросы и платёжные события от двойного применения. Использованное право нельзя передать или применить ещё раз. После завершения пользователь может создать новую независимую попытку и купить ещё одну доступную единицу.

### Очередь и фоновые процессы

Waiting-часть работает по FIFO. Reconciliation worker завершает просроченные попытки, освобождает резерв и продвигает следующих пользователей. Ошибка обработки одного товара не останавливает остальные товары.

Notification outbox записывается в той же транзакции, а отдельный worker забирает события с lease/fencing token и retry. В MVP события публикуются в структурированные логи; в production publisher можно заменить брокером или push-сервисом.

### Рекомендации

`GET /api/v1/products/{productID}/alternatives` исключает исходный и недоступные товары. При включённом AI текстовые описания преобразуются OpenAI embeddings и сравниваются через `pgvector`. Если AI отключён или недоступен, используется воспроизводимый ranking по категории, цене и доступности. Очередь и резервирование от AI не зависят.

### Адаптивная очередь

Стартовая waiting-ёмкость рассчитывается как процент от `allocatable_stock`. Экспериментальный контроллер может корректировать этот процент по данным Prometheus:

- учитывает только достаточную выборку HTTP-запросов и завершённых checkout;
- проверяет технический success rate;
- использует конверсию `purchased / (purchased + cancelled + checkout_expired)`;
- ограничивает минимальное и максимальное значение и размер одного шага;
- при отсутствии или сомнительности метрик сохраняет последнее безопасное значение.

По умолчанию адаптивный режим выключен и используется статический `GOODQUEUE_WAITING_BUFFER_PERCENT`.

## Архитектура

```mermaid
flowchart LR
    USER[Пользователь] --> CADDY[Caddy :8088]
    CADDY --> UI[React frontend]
    UI -->|HTTP + X-User-ID| API[Gin API]
    API --> UC[Use cases]
    UC --> REPO[PostgreSQL repositories]
    REPO --> DB[(PostgreSQL + pgvector)]
    WORKER[Reconciliation и outbox workers] --> REPO
    GRAFANA[Grafana] -->|запуск профиля| RUNNER[Load-test runner]
    RUNNER --> K6[k6 + verifier]
    K6 --> API
    K6 --> PROM[Prometheus]
    API -->|/metrics| PROM
    PROM --> GRAFANA
    PROM --> ADAPT[Adaptive queue controller]
    UC -. optional embeddings .-> OPENAI[OpenAI API]
```

Backend использует ручной dependency injection: зависимости создаются в composition root и передаются через конструкторы. Это сохраняет явные связи между HTTP, use case и repository-слоями и упрощает unit-тестирование без DI-фреймворка.

Frontend организован по Feature-Sliced Design и использует серверное состояние как источник истины. Polling восстанавливает актуальный статус после reload и переводит пользователя между страницами сценария.

### Структура репозитория

```text
.
├── cmd/                         # backend и команды нагрузочного контура
├── internal/
│   ├── app/                     # конфигурация, DI, HTTP и lifecycle
│   ├── pkg/domain/              # доменные модели и ошибки
│   ├── usecase/                 # очередь, checkout, оплата и каталог
│   ├── repository/postgres/     # транзакции и PostgreSQL repositories
│   ├── worker/                  # reconciliation и outbox
│   ├── adaptivequeue/           # контроллер вместимости очереди
│   ├── observability/           # Prometheus HTTP и business metrics
│   ├── loadtestrunner/          # запуск k6 и verifier из Grafana
│   ├── recommendation/openai/   # клиент embeddings
│   ├── e2e/                     # E2E и acceptance criteria
│   └── mockapi/                 # сценарии без PostgreSQL
├── frontend/src/                # FSD: app, entities, features, pages, shared, widgets
├── migrations/                  # Goose-миграции и demo seed
├── loadtest/                    # k6, runner image, Prometheus и Grafana
├── scripts/                     # интеграционные и конкурентные проверки
├── docs/                        # подробная документация frontend и Mock API
├── compose.yaml                 # локальный full-stack
├── Caddyfile                    # единый reverse proxy для UI и dev-tools
└── Makefile                     # единые команды разработки и CI
```

## Технологии

| Область | Стек |
|---|---|
| Backend | Go 1.25.7, Gin, pgx, Zap, Swaggo |
| Данные | PostgreSQL 18, pgvector, Goose, Go Jet |
| Frontend | React 19, TypeScript 6, Vite 8, React Router, Mantine, TanStack Query, Zod |
| Тестирование | Go testing, race detector, Jest, React Testing Library, PostgreSQL integration, HTTP E2E/AC, k6 |
| Качество | golangci-lint, go vet, ESLint, Prettier, TypeScript, Steiger |
| Наблюдаемость | Prometheus, Grafana, Dozzle, Zap JSON logs |
| Инфраструктура | Docker Compose, Nginx, Caddy, GitHub Actions |

## Локальный запуск

Требуется Docker Desktop с Docker Compose.

PowerShell:

```powershell
$env:GOODQUEUE_POSTGRES_PASSWORD = "goodqueue-local"
$env:VITE_API_URL = "http://localhost:8088"
$env:GOODQUEUE_CORS_ALLOWED_ORIGINS = "http://localhost:8088,http://127.0.0.1:8088"
docker compose up --build -d
```

Bash:

```bash
GOODQUEUE_POSTGRES_PASSWORD=goodqueue-local \
VITE_API_URL=http://localhost:8088 \
GOODQUEUE_CORS_ALLOWED_ORIGINS=http://localhost:8088,http://127.0.0.1:8088 \
docker compose up --build -d
```

Compose ждёт PostgreSQL, применяет миграции и запускает backend, frontend, Caddy и Dozzle. Backend и frontend не публикуют отдельные host-порты: Caddy предоставляет единый origin на `8088`.

| Компонент | Локальный адрес |
|---|---|
| Frontend | <http://localhost:8088/> |
| Backend readiness | <http://localhost:8088/api/readyz> |
| Dozzle через Caddy | <http://localhost:8088/dozzle/> |
| Dozzle напрямую | <http://localhost:9999/dozzle/> |

Проверить и остановить:

```bash
docker compose ps
docker compose down
```

Если `5432` занят, задайте `GOODQUEUE_POSTGRES_PORT=15432`. Это меняет только внешний порт; контейнеры продолжат использовать `postgres:5432`.

### Мониторинг

Скопируйте локальные настройки нагрузочного контура и поднимите служебные сервисы:

```powershell
Copy-Item loadtest/.env.example loadtest/.env
make loadtest-runner-up
```

- Grafana: <http://localhost:8088/grafana/>;
- load-test runner: <http://localhost:8088/loadtest-runner/healthz>;
- Prometheus: <http://localhost:9090>;
- backend `/metrics` собирается внутри Compose, наружу через Caddy не публикуется.

Dashboard **GoodQueue — нагрузка и конверсия** импортируется автоматически и показывает RPS, p95/p99, 4xx/5xx, состояния очереди, конверсию и результат verifier. Для запуска кнопками `SMOKE`, `MEDIUM` или `MAIN / 1000 USERS` сначала подготовьте fixture с тем же `Run ID` и сценарием:

```powershell
$env:LOADTEST_RUN_ID = "demo-smoke-01"
$env:LOADTEST_PROFILE = "smoke"
$env:LOADTEST_SCENARIO = "queue_join_polling"
make loadtest-seed
```

Runner принимает только один активный прогон, проверяет fixture, запускает k6, затем verifier и публикует статус и метрики. Полные инструкции и CLI-профили находятся в [loadtest/README.md](loadtest/README.md#запуск-теста-из-grafana).

## API

Swagger-контракт генерируется вместе с backend и хранится в `internal/app/http/docs`. Основные маршруты:

| Метод | Маршрут | Назначение |
|---|---|---|
| `GET` | `/api/v1/products` | каталог |
| `GET` | `/api/v1/products/{id}` | карточка товара |
| `GET` | `/api/v1/products/{id}/alternatives` | похожие доступные товары |
| `POST` | `/api/v1/products/{id}/queue-entries` | встать в очередь |
| `GET` | `/api/v1/products/{id}/queue-entry` | получить активную попытку |
| `DELETE` | `/api/v1/products/{id}/queue-entry` | выйти из очереди |
| `POST` | `/api/v1/queue-attempts/{id}/checkout` | начать checkout |
| `POST` | `/api/v1/products/{id}/queue-attempts/{attemptID}/demo-payment` | завершить demo-покупку |
| `GET` | `/api/v1/demo/users` | тестовые пользователи |

Изменяющие запросы используют `X-User-ID`, а создание попытки и платёж — `Idempotency-Key`. Небезопасные внутренние stock/payment endpoints по умолчанию вообще не регистрируются и включаются только отдельными demo-флагами.

## Проверки качества

Backend:

```bash
make verify             # format, Swagger drift, unit, race, vet, lint, build
make verify-integration # migrations up/down/up, Jet drift, PostgreSQL, runtime, E2E/AC
make verify-all
```

Frontend:

```bash
cd frontend
npm ci
npm run format:check
npm run typecheck
npm run lint
npm run steiger
npm test -- --runInBand
npm run build
```

Acceptance-тесты проверяют главные требования кейса: единственное право на последнюю единицу, персональность права, невозможность обхода очереди, TTL, понятные terminal states, рекомендации и повторную покупку новой попыткой. GitHub Actions выполняет backend static/unit/race, frontend quality pipeline и отдельный PostgreSQL integration/runtime job на каждом push и pull request.

## История разработки

Проект создавался тематическими ветками и pull request-ами. Ключевые вехи:

| Коммит | Результат |
|---|---|
| [`4f0635a`](https://github.com/Woolfer0097/GoodQueue/commit/4f0635a) | базовая backend-структура, Docker, миграции и Swagger |
| [`437d67c`](https://github.com/Woolfer0097/GoodQueue/commit/437d67c) | state machine, payment/outbox, workers и конкурентные тесты |
| [`2e22390`](https://github.com/Woolfer0097/GoodQueue/commit/2e22390) | PostgreSQL/k6 нагрузочный контур |
| [`230f863`](https://github.com/Woolfer0097/GoodQueue/commit/230f863) | AI-рекомендации с pgvector и fallback |
| [`af756bb`](https://github.com/Woolfer0097/GoodQueue/commit/af756bb) | provisioned Grafana dashboard |
| [`e18d2ac`](https://github.com/Woolfer0097/GoodQueue/commit/e18d2ac) | полный frontend queue flow |
| [`54b278d`](https://github.com/Woolfer0097/GoodQueue/commit/54b278d) | адаптивная вместимость очереди |
| [`2f3a34b`](https://github.com/Woolfer0097/GoodQueue/commit/2f3a34b) | безопасное завершение demo-checkout |
| [`2bf5f1a`](https://github.com/Woolfer0097/GoodQueue/commit/2bf5f1a) | повторные покупки отдельными попытками |
| [`78f2e19`](https://github.com/Woolfer0097/GoodQueue/commit/78f2e19) | запуск k6 из Grafana, runner и backend Prometheus metrics |

[Полная история коммитов](https://github.com/Woolfer0097/GoodQueue/commits/main/) сохраняет связь с ветками и pull request-ами.

## Ограничения MVP

- `X-User-ID` имитирует результат внешней аутентификации;
- demo-оплата не списывает реальные деньги и моделирует успешный ответ провайдера;
- outbox publisher пишет события в логи вместо брокера или push-сервиса;
- клиент получает состояние через polling, без WebSocket/SSE;
- AI embeddings обновляются лениво, отдельного catalog-indexer пока нет;
- адаптивный коэффициент хранится в памяти одного процесса;
- production SLO необходимо подтвердить большим прогоном на целевой инфраструктуре.

Эти ограничения не затрагивают основные инварианты: FIFO внутри товара, персональность и срок действия права, атомарный резерв и отсутствие oversell.

## Использование ИИ

Команда использовала нейросетевые инструменты без привязки к одному сервису. В кодовой базе с их помощью создавался только boilerplate — шаблонные заготовки, которые затем проверялись и дорабатывались участниками. Бизнес-логика очереди, архитектурные решения, конкурентные инварианты и критерии тестирования определены командой. Демонстрационные изображения товаров также сгенерированы нейросетью без брендов и текста и сохранены локально в WebP.

В runtime OpenAI API опционально используется только для embeddings похожих товаров. ИИ не принимает решений о позиции, резерве, праве на покупку или списании остатка; при его недоступности работает обычный алгоритмический fallback.

## Дополнительная документация

- [Frontend README](frontend/README.md)
- [Пользовательский frontend-flow](docs/frontend-flow.md)
- [UI/UX-правила](docs/frontend-ui.md)
- [Mock API](docs/mock-api.md)
- [Нагрузочное тестирование и метрики](loadtest/README.md)
