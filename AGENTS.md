# Snapclaw Backend (Go)

## Что Это За Репозиторий

`snapclaw-backend` — backend-платформа для multi-tenant управления пользовательскими `claw`-инстансами OpenClaw.
Репозиторий разделён на control plane, execution plane и общий контракт между ними:

- `simpleClaw/` — внешний API и control plane. Здесь живут OAuth/JWT, пользователи, каналы, реестр execution-серверов, создание и lifecycle `claw`, OpenRouter, billing, payment webhooks и Pub/Sub fan-out.
- `containerManager/` — execution plane. Этот сервис пишет runtime-конфиги, собирает Docker image OpenClaw, создаёт/обновляет контейнеры, держит runtime-state, делает approve/connect/archive/restore и принимает Gmail Pub/Sub fan-out.
- `shared/` — общий контракт и инфраструктурные пакеты, которые используют оба сервиса: hosting API, JWT, observability, HTTP response helpers.

На практике система делает не только lifecycle контейнеров, но и пользовательский биллинг:

- пользователь логинится через Google;
- на пользователя заводится OpenRouter API key и server-side session;
- пользователь подключает канал и при необходимости Gmail;
- пользователь создаёт `claw`, а `simpleClaw` собирает OpenClaw config и выбирает execution-хост;
- `containerManager` создаёт/обновляет реальный Docker container OpenClaw;
- пользователь оформляет подписку или пополняет баланс;
- OpenRouter usage webhook списывает стоимость usage в user balance;
- Gmail Pub/Sub webhook приходит в `simpleClaw`, а дальше fan-out'ится на execution-сервера.

## Как Читать Workspace

Это Go workspace с несколькими модулями. В корне нет `go.mod`, поэтому Go-команды почти всегда запускайте из конкретного модуля:

- `simpleClaw/`
- `containerManager/`
- `shared/`

`go.work` подключает именно эти три модуля.

Важная деталь: директория называется `containerManager/`, но module import path — `containermanager/...`.
Если меняете импорты или ищете пакет через `go list`, ориентируйтесь на `containermanager`, а не на `containerManager`.

Практически:

- root `Taskfile.yaml` управляет `docker compose` стеком;
- `simpleClaw/Taskfile.yaml` и `containerManager/Taskfile.yaml` дают локальные `up`, `test`, `fmt`;
- Go-команды из корня репозитория не запускайте, если им нужен `go.mod`.

## Реальный Технический Стек

Не ориентируйтесь на generic Go defaults, ориентируйтесь на код проекта:

- HTTP router: `chi`
- middleware и observability: `shared/pkg/observability`
- JSON responses: `shared/pkg/response`
- config loading: `cleanenv` + `CONFIG_PATH`
- `simpleClaw` persistence: `gorm` + Postgres
- `containerManager` persistence: `pgx` + `golang-migrate`
- OAuth: `goth` / Google provider
- auth tokens: `shared/pkg/jwt`
- external model/provider integration: OpenRouter
- payment provider: YooKassa
- container runtime: Docker SDK
- metrics/tracing: Prometheus-style metrics + tracing wrappers из `shared/pkg/observability`

Если задача зависит от текущего поведения внешней библиотеки или SDK, сначала используйте Context7 MCP и только потом делайте выводы по памяти.

## Архитектурная Картина

Думайте о системе так:

- `simpleClaw` — источник истины для пользователей, каналов, billing-сущностей, `claw`-метаданных и бизнес-правил.
- `containerManager` — источник истины для runtime-контейнеров, файловых конфигов, Docker-операций и host-level ограничений.
- `shared/pkg/hostingapi` — единый контракт общения между control plane и execution plane.

Если нужно понять, где должен жить код:

- user/business/billing/config ownership — обычно `simpleClaw`;
- runtime/container/file-system execution — обычно `containerManager`;
- общий wire contract, DTO, error payload, route constants — `shared`.

## Ключевые Сущности

### В `simpleClaw/internal/entities`

- `User` — владелец claw, каналов, баланса, подписки и OpenRouter key linkage.
- `Session` — серверная refresh-session для JWT пары.
- `Channel` — пользовательский канал интеграции. Активный user-facing сценарий сейчас Telegram.
- `Claw` — верхнеуровневая доменная сущность пользовательского инстанса с owner, server binding, container id, status и config.
- `ClawConfig` — большой OpenClaw config: env, auth, tools, sandbox, session, channels, hooks, gateway, skills, agents, models, cron, heartbeat.
- `Server` — запись execution-хоста: URL, `proxy_url`, secret/api key, capacity.
- `GmailToken` — сохранённый OAuth token для Gmail connect/watch сценариев.
- `OpenRouterKey` — OpenRouter API key, который либо создаётся при sign-in, либо при создании claw, если key ещё не связан с user.
- `PaymentMethod` — сохранённый платёжный метод пользователя.
- `Plan` — тариф с ценой, credit amount и активностью.
- `UserSubscription` — текущая или прошлая подписка пользователя на план.
- `UserBalanceEntry` — ledger записи начислений/списаний баланса.
- `Payment` — доменная модель YooKassa платежа и webhook payload.
- `OpenRouterUsageEvent` — нормализованный usage event, из которого списывается стоимость в balance.

### В `containerManager/internal/entities`

- `Container` — runtime-запись контейнера OpenClaw: user, claw, docker container id, port, status, `HasStartedOnce`.
- `ClawConfig` — файловое представление config bundle, которое записывается на диск, архивируется и восстанавливается.

### В `shared`

- `hostingapi` — routes, DTO, query params, provider mapping и error payloads для `simpleClaw` ↔ `containerManager`.
- `jwt` — общая генерация и валидация access/refresh JWT.
- `observability` — HTTP metrics, operation metrics, tracing, error classification, Pub/Sub fan-out metrics.
- `response` — единый формат success/error HTTP responses.

## Основные Flows

### 1. User Auth

- Клиент идёт в `simpleClaw /auth/connect/google`.
- После Google OAuth callback сервис ищет или создаёт `User`.
- Для нового пользователя сразу создаётся OpenRouter API key.
- Создаётся `Session`, генерируется JWT access/refresh pair.
- Токены пишутся в cookies, refresh session хранится в БД.

### 2. Channel Connect

- Пользователь вызывает `POST /me/channel`.
- Сейчас сервис реально создаёт Telegram channel config.
- Структуры под Discord / WhatsApp / Slack есть в config schema, но user flow сейчас ориентирован на Telegram.

### 3. Payment Method Management

- Пользователь может создать, прочитать, выбрать default и удалить payment method через `/me/payment-method`.
- Это отдельный user flow и отдельные storage/service ветки, не смешивайте его с подписками или top-up.

### 4. Create / Update / Start / Stop / Delete `claw`

- Пользователь вызывает `POST /claws` или lifecycle endpoints в `simpleClaw`.
- `simpleClaw` валидирует owner/model/channel ids, выбирает `Server`, резолвит модель через OpenRouter, собирает `ClawConfig`.
- `simpleClaw` сохраняет `Claw` у себя в БД.
- Затем control plane вызывает `containerManager` по `shared/pkg/hostingapi`.
- `containerManager` пишет конфиги, создаёт или обновляет Docker container и возвращает runtime metadata.
- `simpleClaw` синхронизирует container/server/status обратно в свою БД.

### 5. Pairing / Connect

- Pairing и interactive connect инициируются из `simpleClaw`, но исполняются в `containerManager`.
- `approve` и `connect` — execution-level операции, не переносите бизнес-решение в execution plane.

### 6. Gmail Connect И Pub/Sub

- Пользователь может пройти `/me/connect/gmail`.
- Gmail refresh token сохраняется в `simpleClaw`, а watch config попадает в `ClawConfig`.
- Pub/Sub webhook приходит в `simpleClaw /pubsub`.
- `simpleClaw` делает fan-out на `Server.ProxyURL`.
- Каждый `containerManager` принимает `/gmail-pubsub`, делает dedup, готовит `gog`-payload и форвардит событие в нужный runtime-instance.

### 7. Billing / Plans / Subscriptions

- Админ управляет тарифами через `/plans`.
- Пользователь запрашивает текущее billing summary через `/me/billing`.
- Подписка создаётся, меняется и отменяется через `/me/subscription`.
- Для оплаты используются YooKassa payment flows и webhook `POST /billing/webhook/yookassa`.

### 8. Balance Top-Up И OpenRouter Usage Debit

- Пользователь может пополнить баланс через `POST /me/topUp`.
- `simpleClaw` создаёт payment intent через YooKassa и ждёт webhook.
- OpenRouter usage webhook приходит в `POST /billing/webhook/openrouter`.
- Сервис извлекает trace/span usage cost и пишет debit в balance ledger.

### 9. Server Registry И Capacity

- Админ управляет execution host registry через `/servers`.
- `simpleClaw` хранит host URL, `proxy_url`, `secret_key`, статус и `max_claws`.
- Capacity подтягивается из `containerManager /capacity`.
- При выборе сервера для нового `claw` учитываются статус и capacity.

## Важные Контракты И Ограничения

- Не меняйте transport contract между `simpleClaw` и `containerManager` в одном сервисе локально. Источник истины — `shared/pkg/hostingapi`.
- В hosting API уже есть неочевидные verb choices:
  - `GET /claws/start`
  - `GET /claws/stop`
  - `GET /approve`
  - `POST /connect`
  - `GET /capacity`
- Не "исправляйте REST" без согласованного изменения shared contract и обоих сервисов.
- В user-facing API тоже есть исторические naming choices вроде `/me/topUp`; не переименовывайте их без отдельной миграции API.

## Пакеты И Их Назначение

### `simpleClaw`

- `simpleClaw/cmd/simpleClaw`
  - Composition root: config, DB, OAuth providers, observability, storages, services, controllers.

- `simpleClaw/config`
  - Runtime config `simpleClaw`: HTTP, DB, auth, OpenRouter, hosting, Gmail connect/watch, proxy, observability, payment.

- `simpleClaw/internal/api/rest`
  - HTTP server lifecycle wrapper.

- `simpleClaw/internal/api/rest/controllers`
  - Transport layer для auth, me, claws, servers, billing, payment webhooks, OpenRouter webhook, pubsub proxy.

- `simpleClaw/internal/api/rest/dto`
  - Request/response DTO для claws, users, billing, payment methods, servers и webhook payloads.

- `simpleClaw/internal/api/rest/middleware`
  - JWT auth, admin guard, request logging, action/flow classification.

- `simpleClaw/internal/entities`
  - Домен control plane: users, channels, claws, config schema, gateway, skills, agents, billing entities.

- `simpleClaw/internal/entities/channels`
  - Channel-specific config и константы, сейчас practically Telegram-first.

- `simpleClaw/internal/infra/hosting`
  - HTTP client/manager для вызовов `containerManager`: create, update, start, stop, delete, approve, connect, archive, restore, capacity.

- `simpleClaw/internal/infra/openrouter`
  - OpenRouter API key management и model resolution.

- `simpleClaw/internal/infra/payment`
  - YooKassa integration и mapping provider objects ↔ domain payment model.

- `simpleClaw/internal/infra/sql`
  - GORM bootstrap, migration helpers, DB-level common errors.

- `simpleClaw/internal/infra/sql/models`
  - GORM models для users, claws, servers, billing, payments, subscriptions и related tables.

- `simpleClaw/internal/infra/storages/channels`
  - Persistence для `Channel`.

- `simpleClaw/internal/infra/storages/claws`
  - Persistence для `Claw` и `claw_channels`.

- `simpleClaw/internal/infra/storages/servers`
  - Persistence для server registry.

- `simpleClaw/internal/infra/storages/users`
  - Persistence для `User`, `Session`, Gmail tokens и OpenRouter key linkage.

- `simpleClaw/internal/infra/storages/paymentmethods`
  - Persistence для `PaymentMethod`.

- `simpleClaw/internal/infra/storages/payments`
  - Persistence для `Payment`.

- `simpleClaw/internal/infra/storages/plans`
  - Persistence для `Plan`.

- `simpleClaw/internal/infra/storages/subscriptions`
  - Persistence для `UserSubscription`.

- `simpleClaw/internal/infra/storages/balanceentries`
  - Persistence для balance ledger entries.

- `simpleClaw/internal/pkg/slctx`
  - Request-scoped `slog` logger helper.

- `simpleClaw/internal/service/user`
  - Auth/sign-in/refresh, profile, channels, payment methods, provider connect.

- `simpleClaw/internal/service/user/commands`
  - Command types для user service.

- `simpleClaw/internal/service/claw`
  - Business logic lifecycle `claw`: create/update/start/stop/delete, approve pairing, connect, archive sync, Gmail watch config injection.

- `simpleClaw/internal/service/claw/commands`
  - Command types для claw service.

- `simpleClaw/internal/service/server`
  - Server registry, validation, capacity sync.

- `simpleClaw/internal/service/server/commands`
  - Command types для server service.

- `simpleClaw/internal/service/billing`
  - Plans, subscriptions, billing summary, top-up, payment webhook handling, OpenRouter usage debit.

- `simpleClaw/internal/service/billing/commands`
  - Command types для billing service.

### `containerManager`

- `containermanager/cmd/containerManager`
  - Composition root execution plane: config, migrations, pgx, configurer, Docker client, service, controllers.

- `containermanager/config`
  - Runtime config: Postgres, HTTP/API key, image build, `max_claws`, `gog`, pubsub, migrations, observability.

- `containermanager/internal/entities`
  - Runtime domain entities, прежде всего `Container` и filesystem-level `ClawConfig`.

- `containermanager/internal/infrastucture/pkg/configurer`
  - Запись config bundle на диск, pairing files, archive/restore.

- `containermanager/internal/infrastucture/pkg/docker`
  - Обёртка над Docker SDK: build image, create/start/stop/remove container, exec/connect.

- `containermanager/internal/infrastucture/sql/migrations`
  - Run SQL migrations.

- `containermanager/internal/infrastucture/sql/pgx`
  - pgx pool bootstrap.

- `containermanager/internal/infrastucture/sql/storage`
  - Postgres persistence для runtime containers.

- `containermanager/internal/interface/rest`
  - HTTP server wrapper.

- `containermanager/internal/interface/rest/controllers`
  - Hosting API transport: create/update/start/stop/delete, archive/restore, approve, connect, capacity, Gmail Pub/Sub ingress.

- `containermanager/internal/interface/rest/middleware`
  - API key auth, request logging, action/flow classification.

- `containermanager/internal/pkg/logctx`
  - Request-scoped logger helper.

- `containermanager/internal/service`
  - Runtime orchestration: port allocation, capacity/memory guard, Docker lifecycle, approve/connect, archive/restore, `gog` import payload normalization, Gmail Pub/Sub forwarding.

- `containermanager/internal/service/commands`
  - Command types execution-layer service.

### `shared`

- `shared/consts`
  - Shared constants вроде provider ids.

- `shared/pkg/hostingapi`
  - Общий control-plane ↔ execution-plane contract.

- `shared/pkg/jwt`
  - Access/refresh JWT.

- `shared/pkg/observability`
  - Metrics, tracing, error attrs, HTTP instrumentation, Pub/Sub fan-out metrics.

- `shared/pkg/response`
  - Единые JSON responses.

## Непакетные Директории И Артефакты

- `deploy/`
  - Dev/runtime configs для `simpleClaw`, `containerManager`, Alloy, Grafana и JWT keys.

- `containerManager/migrations/`
  - SQL migrations execution-plane БД.

- `containerManager/images/openclaw/`
  - Docker build assets для runtime OpenClaw image.

- `docker-compose.yml`
  - Локальный compose stack для обоих сервисов и observability.

- `README.md`
  - Deploy notes по bind mounts, config paths и локальному запуску compose stack.

- `Taskfile.yaml`
  - Root tasks для compose stack.

## Где Смотреть Сначала

Если задача про lifecycle `claw`:

- `simpleClaw/internal/service/claw`
- `simpleClaw/internal/infra/hosting`
- `shared/pkg/hostingapi`
- `containerManager/internal/service`

Если задача про auth/user/connect:

- `simpleClaw/internal/api/rest/controllers/users.go`
- `simpleClaw/internal/service/user`
- `shared/pkg/jwt`

Если задача про billing/payments/subscriptions:

- `simpleClaw/internal/api/rest/controllers/billing.go`
- `simpleClaw/internal/service/billing`
- `simpleClaw/internal/infra/payment`
- `simpleClaw/internal/infra/storages/payments`
- `simpleClaw/internal/infra/storages/plans`
- `simpleClaw/internal/infra/storages/subscriptions`
- `simpleClaw/internal/infra/storages/balanceentries`

Если задача про Pub/Sub или Gmail connect:

- `simpleClaw/internal/api/rest/controllers/pubsub_proxy.go`
- `containerManager/internal/interface/rest/controllers/claws.go`
- `containerManager/internal/service/gog_payload.go`
- `containerManager/internal/service/gog_watch_port.go`

Если задача про server registry/capacity:

- `simpleClaw/internal/service/server`
- `simpleClaw/internal/infra/storages/servers`
- `containerManager/internal/interface/rest/controllers/claws.go`

## Общие Правила Для Агентов

- Всегда передавайте `context.Context` первым аргументом во всех IO/DB/network операциях.
- Ошибки оборачивайте через `fmt.Errorf("...: %w", err)`.
- Не смешивайте domain entities, HTTP DTO и ORM models.
- В transport layer следуйте существующему паттерну `shared/pkg/response` и `respondServiceError`.
- В `simpleClaw` не тащите Docker/runtime/file-system код.
- В `containerManager` не переносите business ownership, user/billing policy и control-plane state.
- Если меняете shared hosting contract, одновременно проверяйте оба сервиса.
- Если меняете billing flow, проверяйте не только service/storage, но и webhook DTO, controllers и entity mapping.
- Если меняете Gmail/PubSub flow, учитывайте ingress auth token, fan-out retry/backoff, dedup и downstream payload normalization.

## Команды И Проверка

Форматирование:

- `cd simpleClaw && go fmt ./...`
- `cd containerManager && go fmt ./...`
- `cd shared && go fmt ./...`

Тесты:

- `cd simpleClaw && GOCACHE=/tmp/go-build-cache go test ./... -count=1`
- `cd containerManager && GOCACHE=/tmp/go-build-cache go test ./... -count=1`
- `cd shared && GOCACHE=/tmp/go-build-cache go test ./... -count=1`

Линтинг перед PR, если `golangci-lint` установлен:

- `cd simpleClaw && golangci-lint run ./...`
- `cd containerManager && golangci-lint run ./...`
- `cd shared && golangci-lint run ./...`

Compose stack:

- `task up`
- `task down`
- `task logs`
- `task ps`

## Практические Замечания

- Не запускайте Go-команды из корня workspace, если команда ожидает `go.mod`.
- Для `simpleClaw` migrations живут в коде через GORM bootstrap, а не отдельной папкой миграций.
- `containerManager` при старте реально собирает Docker image; не считайте запуск сервиса cheap operation.
- `max_claws` в `containerManager` может резолвиться автоматически на Linux, а не только читаться как статичное число.
- У execution-plane есть memory guard для cold start / warm start. Если меняете start semantics, проверьте `containerManager/internal/service/container.go`.
- Если вносите изменения в OpenClaw config schema, проверьте и доменную сборку config в `simpleClaw`, и файловую запись/restore path в `containerManager`.
