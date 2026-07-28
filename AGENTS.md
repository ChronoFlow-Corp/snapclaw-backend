# Snapclaw Backend (Go)

## Что Это За Репозиторий

`snapclaw-backend` — backend-платформа для multi-tenant управления пользовательскими `claw`-инстансами OpenClaw.
Репозиторий разделён на control plane, execution plane и общий контракт между ними:

- `simpleClaw/` — внешний API и control plane. Здесь живут OAuth/JWT, пользователи, каналы, реестр execution-серверов, создание и async lifecycle `claw`, OpenRouter, billing, payment webhooks, Pub/Sub fan-out и транзакционные/маркетинговые email.
- `containerManager/` — execution plane. Этот сервис пишет runtime-конфиги, собирает Docker image OpenClaw, идемпотентно `ensure/start/stop/delete` runtime, держит runtime-state, делает approve/connect/archive/restore и принимает Gmail Pub/Sub fan-out.
- `shared/` — общий контракт и инфраструктурные пакеты, которые используют оба сервиса: hosting API, JWT, observability, HTTP response helpers.

На практике система делает не только lifecycle контейнеров, но и пользовательский биллинг:

- пользователь логинится через Google;
- на пользователя заводится OpenRouter API key и server-side session;
- пользователь подключает канал и при необходимости Gmail;
- пользователь создаёт `claw`, а `simpleClaw` сохраняет доменный config в БД;
- при `start` `simpleClaw` принимает lifecycle intent, ставит `claw` в `start_pending`, а background worker добирает `ensure/start` на `containerManager`;
- пользователь оформляет подписку или пополняет баланс;
- OpenRouter usage webhook списывает стоимость usage в user balance;
- Gmail Pub/Sub webhook приходит в `simpleClaw`, а дальше fan-out'ится на execution-сервера;
- ключевые события аккаунта (welcome, активация Premium, пополнение баланса) уходят пользователю транзакционным письмом через durable email outbox.

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
- email provider: Resend (опционально, gated `EMAIL_ENABLED`)
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

- `User` — владелец claw, каналов, баланса, подписки, OpenRouter key linkage и marketing opt-out flag (`MarketingOptOut`).
- `Session` — серверная refresh-session для JWT пары.
- `Channel` — пользовательский канал интеграции. Активный user-facing сценарий сейчас Telegram.
- `Claw` — верхнеуровневая доменная сущность пользовательского инстанса с owner, server binding, runtime record id, lifecycle state (`desired_state`, `observed_state`, `lifecycle_status`, `current_operation_id`, `last_error`), persisted onboarding completion flag и config.
- `ClawConfig` — большой OpenClaw config: env, auth, tools, sandbox, session, channels, hooks, gateway, skills, agents, models, cron, heartbeat.
- `AccountIntegration` — user-scoped подключение внешнего аккаунта (`gmail`, `google_calendar`, `github`, etc.), которое потом можно привязать к нескольким `claw`.
- `ClawCapabilityAttachment` — claw-scoped включение capability с optional ссылкой на `AccountIntegration`.
- `Server` — запись execution-хоста: URL, `proxy_url`, secret/api key, capacity.
- `OpenRouterKey` — OpenRouter API key, который либо создаётся при sign-in, либо при создании claw, если key ещё не связан с user.
- `PaymentMethod` — сохранённый платёжный метод пользователя.
- `Plan` — тариф с ценой, credit amount и активностью.
- `UserSubscription` — текущая или прошлая подписка пользователя на план.
- `UserBalanceEntry` — ledger записи начислений/списаний баланса.
- `Payment` — доменная модель YooKassa платежа и webhook payload.
- `OpenRouterUsageEvent` — нормализованный usage event, из которого списывается стоимость в balance.
- `OutboxEmail` — durable запись транзакционного/маркетингового письма в email outbox: recipient, subject, отрендеренные HTML/text, headers, category, delivery status, attempts и next-attempt schedule.

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
- Для Telegram Managed Bots появился альтернативный control-plane flow:
  - frontend вызывает `POST /me/telegram/manager/link` с `claw_id`;
  - backend создаёт или переиспользует claw-scoped managed-bot resume record и возвращает resumable deep link на platform manager bot;
  - delivery mode manager bot updates configurable: `webhook` через `/telegram/manager/webhook` или `polling` через Telegram `getUpdates`;
  - в `polling` mode `simpleClaw` перед long polling path делает `deleteWebhook`, чтобы Telegram update delivery не была смешанной;
  - `simpleClaw` получает managed bot token через Telegram Bot API и materialize'ит его как обычный user-owned `Channel`.
- Структуры под Discord / WhatsApp / Slack есть в config schema, но user flow сейчас ориентирован на Telegram.

### 3. Payment Method Management

- Пользователь может прочитать, выбрать default и удалить payment method через `/me/payment-method`.
- Создание payment method не делается отдельным user-facing endpoint в `users` controller; не придумывайте его обратно без явной продуктовой задачи.
- Это отдельный user flow и отдельные storage/service ветки, не смешивайте его с подписками или top-up.

### 4. Create / Update / Start / Stop / Delete `claw`

- Пользователь вызывает `POST /claws` или lifecycle endpoints в `simpleClaw`.
- На `create` `simpleClaw` валидирует owner/model/channel ids, резолвит модель через OpenRouter, собирает `ClawConfig` и сохраняет `Claw` только у себя в БД.
- На `create` не выбирается execution-server и не создаётся runtime: `server_id` пустой, `container_id` пустой, `desired_state=stopped`, `observed_state=unknown`, `lifecycle_status=idle`.
- `start`, `stop`, `restart` и `delete` в `simpleClaw` async-only: HTTP handler только валидирует запрос, создаёт lifecycle operation, обновляет `desired_state`/`lifecycle_status` и возвращает текущее состояние `claw`.
- У `claw` есть backend-owned persisted onboarding completion signal: он выставляется, когда onboarding действительно завершён, а не в момент успешной оплаты.
- `OnboardingComplete` sticky: `start/restart` не должны заново вычислять его только из approve-required config. Флаг сбрасывается в `false` только когда runtime-approved state больше не гарантирован, например archive missing, archive corrupted или archive save failed.
- Фактическое выполнение идёт в background через lifecycle worker:
  - `start` делает `select server -> restore archived runtime config when available -> ensure runtime -> start runtime -> sync runtime state`; если локальный archive отсутствует или битый, worker делает fallback на обычный `ensure` из БД;
  - `stop` сначала архивирует полный runtime config tar в control plane, затем делает `delete runtime` и очищает runtime refs в control plane;
  - `restart` сначала архивирует текущий runtime config tar, затем удаляет runtime, после чего идёт через тот же archive-aware `start` path;
  - `delete` сначала добивается удаления runtime, потом удаляет сам `claw` из control plane.
- Если во время runtime-операции outcome неоднозначен, `simpleClaw` переводит `claw` в `reconcile_pending`, а отдельный reconciler подтягивает truth через `GET /claws/state`.
- Источник истины для доменного конфига — `openclaw.json` в `simpleClaw` БД; runtime archive может содержать остальные runtime files, но `openclaw.json` при restore всегда перезаписывается из БД, а execution-plane runtime state не должен подменять control-plane config.

### 5. Pairing / Connect

- Pairing и interactive connect инициируются из `simpleClaw`, но исполняются в `containerManager`.
- `approve` и `connect` — execution-level операции, не переносите бизнес-решение в execution plane.

### 6. Gmail Connect И Pub/Sub

- Для dashboard/manual flows пользователь всё ещё может создать `AccountIntegration` через `/me/integrations/{provider}/connect`, а затем отдельным запросом attach'ить capability к конкретному `claw`.
- Для onboarding Google-backed skills появился отдельный OAuth flow:
  - frontend вызывает `POST /me/integrations/google/oauth/start` с выбранными capability (`gmail`, `google_calendar`, `sheets`) и optional `returnTo`;
  - backend строит union scopes, подписывает state и редиректит пользователя через Google OAuth callback `GET /auth/google/integrations/callback`;
  - backend создаёт `account_integrations` только для capability, реально покрытых granted scopes;
  - onboarding должен завершить этот шаг до `POST /claws`, а `createClaw` теперь принимает explicit integration bindings и сразу сохраняет initial capability attachments.
- Gmail watch state больше не живёт в user storage; `simpleClaw` хранит integration payload и attachment, а runtime-side effects описываются через binding-aware config.
- Pub/Sub webhook приходит в `simpleClaw /pubsub`.
- `simpleClaw` делает fan-out на `Server.ProxyURL`.
- Каждый `containerManager` принимает `/gmail-pubsub`, делает dedup и форвардит событие только если у runtime есть explicit Gmail binding.

### 7. Billing / Plans / Subscriptions

- Админ управляет тарифами через `/plans`.
- Пользователь запрашивает текущее billing summary через `/me/billing`.
- Пользовательский bootstrap/readiness state читается через `GET /me/bootstrap`; backend сам вычисляет `dashboard_allowed`, onboarding `step`, `required` и resume `claw_id`.
- Для managed Telegram bot onboarding `GET /me/bootstrap` также возвращает backend-owned `onboarding.telegram_manager` snapshot: `id`, `status`, `deep_link_url`, `link_expires_at`, `channel_id`, `last_error`.
- Dashboard access принадлежит backend-у: доступ разрешён для `active` и для `canceled` пока не истёк `current_period_end`; `pending`, `past_due`, отсутствующая подписка и истёкший `canceled` период доступа не дают.
- После успешной оплаты backend активирует подписку, но не стартует `claw` автоматически; frontend должен отдельно вызывать `POST /claws/{id}/start`, а bootstrap возвращает следующий onboarding step из persisted state.
- Реальные onboarding step для bootstrap сейчас включают `subscription_required`, `telegram_choice`, `telegram_manager_link`, `telegram_manager_provisioning`, `telegram_manual_connect`, `telegram_confirm`, `dashboard_ready`.
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
- При выборе сервера для `start` worker учитывает статус и capacity.

### 10. Транзакционные И Маркетинговые Email

- Email — опциональная подсистема, целиком выключенная по умолчанию. Она поднимается только когда `EMAIL_ENABLED=true`; иначе `simpleClaw` логирует, что email отключён, и не регистрирует ни dispatcher, ни `/email/unsubscribe`.
- Провайдер доставки — Resend через тонкий HTTP client `simpleClaw/internal/infra/email`.
- Отправка идёт через durable outbox, а не inline: сервисы кладут письмо в таблицу `email_outbox` (`entities.OutboxEmail`), а background dispatcher (`service/email.RunDispatcher`) периодически забирает due-письма, шлёт их и помечает `sent`/`failed` с backoff-ретраями до `MaxAttempts`.
- Idempotency: на каждое письмо в Resend уходит `Idempotency-Key` = outbox row id, чтобы ретрай после успешной отправки, но неудачного `MarkSent`, не задваивал доставку.
- Enqueue сейчас best-effort и происходит после коммита бизнес-события: ошибка постановки в очередь только логируется и не роняет sign-in/оплату. Это осознанный trade-off, а не полноценный transactional outbox.
- Категории писем: `welcome` (первый sign-in), `premium_granted` (переход подписки pending -> active), `top_up` (успешное пополнение), `support_reply` и `announcement` (маркетинг). `welcome`/`premium`/`top_up` подключаются к существующим user/billing flow через `WithNotifier` и не должны блокировать основной flow при ошибке.
- Маркетинговые письма (`announcement`) уважают `User.MarketingOptOut` и несут RFC 8058 one-click unsubscribe headers, когда задан `EMAIL_PUBLIC_BASE_URL`.
- Unsubscribe: `GET /email/unsubscribe` рендерит подтверждающую страницу (side-effect-free, чтобы mail-сканеры и prefetch'еры не отписывали пользователя), а `POST /email/unsubscribe` пишет opt-out и обслуживает как confirm-форму, так и RFC 8058 one-click. Токен — HMAC-SHA256 над user id, подписанный `EMAIL_UNSUBSCRIBE_SECRET`.
- Dispatcher также throttled-prune'ит терминальные (`sent`/`failed`) строки старше retention, чтобы outbox не рос безгранично; pending-строки не трогаются.

## Важные Контракты И Ограничения

- Не меняйте transport contract между `simpleClaw` и `containerManager` в одном сервисе локально. Источник истины — `shared/pkg/hostingapi`.
- В hosting API сейчас целевой lifecycle contract такой:
  - `POST /claws/ensure`
  - `POST /claws/start`
  - `POST /claws/stop`
  - `POST /claws/delete`
  - `GET /claws/state`
  - `GET /approve`
  - `POST /connect`
  - `GET /capacity`
- `POST /claws/start|stop|delete` принимают `LifecycleCommandRequest` и возвращают `202 Accepted` при принятии runtime-команды.
- `POST /claws/ensure` — create-or-get runtime с записью config bundle на execution-host.
- `GET /claws/state` — execution-plane truth для reconcile: runtime record id, docker id, observed/runtime status, port и last error.
- Не "исправляйте REST" без согласованного изменения shared contract и обоих сервисов.
- В user-facing API тоже есть исторические naming choices вроде `/me/topUp`; не переименовывайте их без отдельной миграции API.

## HTTP API Reference

Ниже — краткий operational reference по реальным HTTP path. Контроллеры в сервисах регистрируются от корня сервиса; если снаружи есть gateway или `/api` prefix, это уже внешний routing layer, а не контракт самих сервисов.

### `simpleClaw` user-facing API

#### Auth

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/auth/connect/{provider}` | `GET` | Начать OAuth flow | path `provider` | body нет |
| `/auth/connect/{provider}/callback` | `GET` | Завершить OAuth flow, выдать cookies | path `provider` | body нет |
| `/auth/google/integrations/callback` | `GET` | Завершить отдельный Google integration OAuth flow для onboarding/dashboard и вернуть пользователя на frontend onboarding path | query `state` | query `code`, provider `error` |
| `/auth/refresh` | `POST` | Обновить JWT pair по refresh cookie | cookie `refresh_token` | body нет |

#### Me / User / Integrations

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/me/user-info` | `GET` | Вернуть профиль текущего пользователя | JWT access cookie/header | body нет |
| `/me/channel` | `POST` | Создать channel config; текущий реальный flow — Telegram | body `name`, `telegramChannel.botToken`, `telegramChannel.dmPolicy` | `telegramChannel.allowFrom` |
| `/me/telegram/manager/link` | `POST` | Инициировать claw-scoped Telegram Managed Bot flow и вернуть или переиспользовать deep link на manager bot | JWT, body `claw_id` | нет |
| `/me/telegram/manager/link/{id}` | `GET` | Прочитать provisioning status managed bot request | path `id`, JWT | body нет |
| `/me/integrations` | `GET` | Список user-scoped integrations | JWT | body нет |
| `/me/integrations/google/oauth/start` | `POST` | Начать отдельный Google OAuth flow для onboarding-selected capability bindings | body `capabilities[]` | body `returnTo` |
| `/me/integrations/{provider}/connect` | `POST` | Создать или обновить integration payload для capability binding | path `provider` | body `id`, `externalAccountId`, `displayName`, `secretPayload`, `metadata` — provider-specific, все поля опциональны на transport уровне |

#### Payment Methods

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/me/payment-method` | `GET` | Список сохранённых payment methods | JWT | body нет |
| `/me/payment-method/{id}` | `GET` | Получить payment method по id | path `id` | body нет |
| `/me/payment-method/{id}` | `PATCH` | Поставить или снять default flag | path `id`, body `is_default` | нет |
| `/me/payment-method/{id}` | `DELETE` | Удалить payment method | path `id` | body нет |

#### Claws

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/claws` | `GET` | Список `claw` текущего пользователя | JWT | body нет |
| `/claws/{id}` | `GET` | Получить один `claw` | path `id` | body нет |
| `/claws` | `POST` | Создать control-plane `claw` без немедленного runtime start; onboarding-selected Google-backed capability bindings валидируются и сохраняются сразу | body `name`, `model` | `channelIds`, `apiLimits.monthlyBudgetUsd`, `capabilities.webSearch`, `capabilities.filesImages`, `capabilities.memory`, `capabilities.gmail`, `capabilities.googleCalendar`, `capabilities.sheets` |
| `/claws/{id}` | `PUT` | Частично обновить имя, модель, channel set и api limit | path `id` | body `name`, `model`, `channelIds`, `apiLimits.monthlyBudgetUsd`; все поля optional, но если `channelIds` передан, он заменяет текущий set |
| `/claws/{id}/start` | `POST` | Поставить async intent на start runtime | path `id` | body нет |
| `/claws/{id}/stop` | `POST` | Поставить async intent на stop/delete runtime | path `id` | body нет |
| `/claws/{id}` | `DELETE` | Поставить async intent на delete runtime и control-plane record | path `id` | body нет |
| `/claws/{id}/approve` | `POST` | Завершить interactive pairing confirm step | path `id`, body `code` | body `channelType`; нужен только если approve-кандидатов больше одного |
| `/claws/{id}/connect` | `POST` | Trigger runtime-side account connect для capability | path `id` | query `provider`; по умолчанию `gmail` |
| `/claws/{id}/capabilities/{capability}` | `PUT` | Attach/update capability binding | path `id`, path `capability`, body `enabled` | body `provider`, `accountIntegrationId`, `settings` |
| `/claws/{id}/capabilities/{capability}` | `DELETE` | Detach capability binding | path `id`, path `capability` | body нет |

Правила по `claw` flow:

- Успешная оплата не стартует `claw` автоматически.
- Backend сам вычисляет bootstrap state через `/me/bootstrap`.
- `start`, `stop`, `delete` — async endpoints; не ждите финального runtime outcome в HTTP handler.
- `GET /claws` и `GET /claws/{id}` возвращают только sanitised dashboard-facing Telegram status в поле `telegram` (`connected`, `status`) и не должны раскрывать `Config`, bot tokens, allow-lists или другие secret-bearing channel fields.

#### Billing / Plans

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/plans/public` | `GET` | Публичный список активных plan variants | нет | body нет |
| `/me/bootstrap` | `GET` | Вернуть backend-owned dashboard access и onboarding resume state | JWT | body нет |
| `/me/billing` | `GET` | Billing summary: balance, current subscription, next charge | JWT | body нет |
| `/me/subscription` | `GET` | Текущая подписка пользователя | JWT | body нет |
| `/me/subscription` | `POST` | Создать subscription checkout | body `plan_id` | нет |
| `/me/subscription/{id}/checkout-status` | `GET` | Проверить статус checkout по subscription id | path `id` | body нет |
| `/me/subscription` | `PATCH` | Сменить plan у текущей subscription | body `plan_id` | нет |
| `/me/subscription` | `DELETE` | Отменить subscription | JWT | body нет |
| `/me/topUp` | `POST` | Создать balance top-up payment intent | body `amount` | body `payment_method_id` |
| `/me/expanse` | `GET` | Usage aggregation: today/week/month/day | JWT | body нет |
| `/billing/webhook/yookassa` | `POST` | Provider webhook; backend обновляет payment/subscription/balance | provider-native JSON body | нет |
| `/billing/webhook/openrouter` | `POST` | Usage webhook; backend дебетует balance | `Authorization` secret header, provider-native JSON body | нет |

`/me/bootstrap` response semantics:

- `dashboard_allowed` вычисляется backend-ом.
- `subscription.status` и `subscription.current_period_end` — raw billing state.
- `subscription.access_active` — derived flag, который фронт не должен пересчитывать.
- `onboarding.required`, `onboarding.step`, `onboarding.claw_id` — backend-owned resume contract.
- `onboarding.telegram_manager` присутствует только для claw-scoped managed-bot resume и содержит `id`, `status`, `deep_link_url`, `link_expires_at`, `channel_id`, `last_error`.

#### Admin / Registry

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/plans` | `GET` | Список plan records для admin | JWT admin | query нет |
| `/plans/{id}` | `GET` | Получить plan по id | JWT admin, path `id` | body нет |
| `/plans` | `POST` | Создать plan | body `code`, `name`, `interval`, `billing_amount_minor`, `balance_credit_minor`, `currency` | нет |
| `/plans/{id}` | `PATCH` | Обновить plan | path `id`, body `code`, `name`, `interval`, `billing_amount_minor`, `balance_credit_minor`, `currency`, `is_active` | нет |
| `/plans/{id}` | `DELETE` | Деактивировать plan | path `id` | body нет |
| `/servers` | `GET` | Список execution hosts | JWT admin | body нет |
| `/servers/{id}` | `GET` | Получить execution host | JWT admin, path `id` | body нет |
| `/servers` | `POST` | Зарегистрировать execution host | body `name`, `ip`, `url`, `proxyUrl`, `status`, `secretKey` | нет |
| `/servers/{id}` | `PUT` | Обновить execution host | path `id`, body `name`, `ip`, `url`, `proxyUrl`, `status`, `secretKey` | нет |
| `/servers/{id}` | `DELETE` | Удалить execution host | path `id` | body нет |

#### Admin Console (back-office read API)

Read-only поверхность для внутренней админки (`snapclaw/admin`). Все роуты gated `AuthJwt` + `AdminOnly`
(role `admin`), контроллер `internal/api/rest/controllers/admin.go`, aggregate-service
`internal/service/admin`, статистика — `internal/infra/storages/adminstats`. Ответы users/детали в
snake_case; claw-подресурсы переиспользуют существующие user-facing мапперы. Мутация claw из админки
(`PUT /admin/claws/{id}`) пока намеренно не реализована (нетривиальная reconciliation capability/integration).

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/admin/users` | `GET` | Пагинированный список пользователей с computed `claws_count`, `has_active_subscription`, `has_issues` | JWT admin | query `page`, `page_size`, `sort` (`-created_at`\|`created_at`\|`-balance_minor`\|`balance_minor`\|`email`), `q`, `role`, `has_active_subscription`, `has_issues`, `registered_from`, `registered_to` |
| `/admin/users/counts` | `GET` | Dashboard-счётчики `total`, `with_active_subscription`, `with_issues`, `admins` | JWT admin | нет |
| `/admin/users/{id}` | `GET` | 360° карточка пользователя + агрегированный `summary` (claws total/running/error, subscription, payments_total_minor, usage_month) | JWT admin, path `id` | нет |
| `/admin/users/{id}/claws` | `GET` | Claw'ы пользователя (admin-shape: `serverId`, `createdAt`, enabled `capabilities`) | JWT admin, path `id` | нет |
| `/admin/users/{id}/subscription` | `GET` | История подписок пользователя | JWT admin, path `id` | нет |
| `/admin/users/{id}/payments` | `GET` | Платежи пользователя | JWT admin, path `id` | нет |
| `/admin/users/{id}/payment-methods` | `GET` | Сохранённые payment methods | JWT admin, path `id` | нет |
| `/admin/users/{id}/balance-entries` | `GET` | Записи balance ledger (последние `200`) | JWT admin, path `id` | нет |
| `/admin/users/{id}/integrations` | `GET` | Account integrations (секреты вырезаны) | JWT admin, path `id` | нет |
| `/admin/users/{id}/telegram-bots` | `GET` | Managed Telegram bots пользователя | JWT admin, path `id` | нет |
| `/admin/users/{id}/usage` | `GET` | Usage aggregation (`ExpanseAnalyze` по target user) | JWT admin, path `id` | нет |
| `/admin/claws/{id}` | `GET` | Один claw по id (admin-scope, без owner) + enabled capabilities | JWT admin, path `id` | нет |

Правило "issue": claw считается проблемным, если `observed_state = 'error'` или `lifecycle_status = 'failed'`.
Это единый предикат для `has_issues` (список), `with_issues` (counts) и `claws_error` (summary).

#### Infra / Proxy

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/health` | `GET` | Liveness probe для proxy/controller process | нет | body нет |
| `/pubsub` | `POST` | Ingress webhook для Gmail Pub/Sub fan-out на execution hosts | JSON body, ingress auth token в `Authorization` | нет |
| `/telegram/manager/webhook` | `POST` | Ingress webhook для Telegram manager bot: `/start <code>` linking и `managed_bot` updates | header `X-Telegram-Bot-Api-Secret-Token`, Telegram update JSON | нет |
| `/email/unsubscribe` | `GET` | Отрендерить confirm-страницу отписки от маркетинговых писем (side-effect-free); регистрируется только при `EMAIL_ENABLED=true` | query `token` | нет |
| `/email/unsubscribe` | `POST` | Выполнить marketing opt-out; обслуживает confirm-форму и RFC 8058 one-click | `token` в query или form body | нет |

### `containerManager` execution-plane API

Все path ниже защищены service-to-service API key middleware и не являются user-facing API.

#### Hosting lifecycle contract

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/claws/ensure` | `POST` | Create-or-get runtime record и записать config bundle | body `userId`, `clawId` | body `vars`, `clawConfig[]`; каждый `clawConfig[]` элемент: `name`, `fileType`, `data` |
| `/claws/start` | `POST` | Принять runtime start command | body `userId`, `clawId` | body `operationId`, `idempotencyKey` |
| `/claws/stop` | `POST` | Принять runtime stop command | body `userId`, `clawId` | body `operationId`, `idempotencyKey` |
| `/claws/delete` | `POST` | Принять runtime delete command | body `userId`, `clawId` | body `operationId`, `idempotencyKey` |
| `/claws/state` | `GET` | Вернуть execution-plane truth для reconcile | query `userId`, `clawId` | нет |
| `/capacity` | `GET` | Вернуть host capacity snapshot | API key | body/query нет |
| `/approve` | `GET` | Выполнить runtime approve/pairing | query `userId`, `clawId`, `code`, `channelType` | нет |
| `/connect` | `POST` | Передать runtime provider token/connect payload | query `userId`, `clawId`, `provider`, body token bytes | нет |
| `/gmail-pubsub` | `POST` | Получить fan-out Gmail Pub/Sub payload и разослать по runtime bindings | JSON body | нет |

#### Archive / Restore helpers

| Path | Method | Purpose | Required | Optional |
|---|---|---|---|---|
| `/claws/config` | `GET` | Скачать tar archive runtime config directory | query `userId`, `clawId` | query `deleteAfter=true|false` |
| `/claws/config` | `POST` | Восстановить runtime config directory из request body | query `userId`, `clawId`, tar body | нет |

### Shared transport source of truth

Для service-to-service lifecycle не придумывайте локальные DTO:

- path constants и query names: `shared/pkg/hostingapi/routes.go`
- request/response DTO: `shared/pkg/hostingapi/dto.go`
- error payload: `shared/pkg/hostingapi/errors.go`

Если меняете одно из этих мест, сразу проверяйте `simpleClaw` client-side mapping и `containerManager` controller-side decoding.

## Пакеты И Их Назначение

### `simpleClaw`

- `simpleClaw/cmd/simpleClaw`
  - Composition root: config, DB, OAuth providers, observability, storages, services, controllers.

- `simpleClaw/config`
  - Runtime config `simpleClaw`: HTTP, DB, auth, OpenRouter, hosting, Gmail watch defaults, proxy, observability, payment, email.

- `simpleClaw/internal/api/rest`
  - HTTP server lifecycle wrapper.

- `simpleClaw/internal/api/rest/controllers`
  - Transport layer для auth, me, claws, servers, billing, payment webhooks, OpenRouter webhook, pubsub proxy и email unsubscribe.

- `simpleClaw/internal/api/rest/dto`
  - Request/response DTO для claws, users, billing, payment methods, servers и webhook payloads.

- `simpleClaw/internal/api/rest/middleware`
  - JWT auth, admin guard, request logging, action/flow classification.

- `simpleClaw/internal/entities`
  - Домен control plane: users, channels, claws, config schema, gateway, skills, agents, billing entities.

- `simpleClaw/internal/entities/channels`
  - Channel-specific config и константы, сейчас practically Telegram-first.

- `simpleClaw/internal/infra/hosting`
  - HTTP client/manager для вызовов `containerManager`: ensure runtime, start, stop, delete, read runtime state, approve, connect, capacity и config archive/restore. Control-plane lifecycle теперь использует archive/restore path для `start/stop/restart`, сохраняя при этом `openclaw.json` как DB-owned source of truth.

- `simpleClaw/internal/infra/openrouter`
  - OpenRouter API key management и model resolution.

- `simpleClaw/internal/infra/payment`
  - YooKassa integration и mapping provider objects ↔ domain payment model.

- `simpleClaw/internal/infra/email`
  - Тонкий HTTP client Resend REST API: отправка одного письма (`Send`) с idempotency key и провайдерским message id.

- `simpleClaw/internal/infra/telegram`
  - Минимальный Telegram Bot API client для manager bot flow: `getMe`, `getManagedBotToken`, `replaceManagedBotToken`, webhook DTO и `sendMessage`.

- `simpleClaw/internal/infra/sql`
  - GORM bootstrap, migration helpers, DB-level common errors.

- `simpleClaw/internal/infra/sql/models`
  - GORM models для users, claws, servers, billing, payments, subscriptions и related tables, включая `telegram_account_links`, `telegram_webhook_updates`, `telegram_managed_bots` и `email_outbox`.

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

- `simpleClaw/internal/infra/storages/telegrammanager`
  - Persistence для Telegram account link records, webhook update idempotency и managed bot provisioning state.

- `simpleClaw/internal/infra/storages/emailoutbox`
  - Persistence для email outbox: enqueue, claim-due, mark sent/failed, prune терминальных строк.

- `simpleClaw/internal/pkg/slctx`
  - Request-scoped `slog` logger helper.

- `simpleClaw/internal/service/user`
  - Auth/sign-in/refresh, profile, channels, payment methods, provider connect.

- `simpleClaw/internal/service/user/commands`
  - Command types для user service.

- `simpleClaw/internal/service/claw`
  - Business logic lifecycle `claw`: create/update, async enqueue `start/stop/restart/delete`, lifecycle worker, reconciler, local archive store for runtime config tar, approve pairing, connect, Gmail watch config injection.

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

- `simpleClaw/internal/service/telegrammanager`
  - Control-plane orchestration для Telegram Managed Bots: deep-link creation, `/start` account linking, `managed_bot` webhook handling и materialization managed bot -> обычный Telegram `Channel`.

- `simpleClaw/internal/service/email`
  - Email domain: рендер шаблонов (welcome/premium/top-up/support/announcement), enqueue в outbox, background dispatcher с backoff-ретраями и prune, HMAC unsubscribe token sign/verify.

### `containerManager`

- `containermanager/cmd/containerManager`
  - Composition root execution plane: config, migrations, pgx, configurer, Docker client, service, controllers.

- `containermanager/config`
  - Runtime config: Postgres, HTTP/API key, image build, `max_claws`, `gog`, pubsub, migrations, observability.

- `containermanager/internal/entities`
  - Runtime domain entities, прежде всего `Container` и filesystem-level `ClawConfig`.

- `containermanager/internal/infrastucture/pkg/configurer`
  - Запись config bundle на диск, pairing files, archive/restore runtime config directory.

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
  - Hosting API transport: ensure/state/start/stop/delete, archive/restore, approve, connect, capacity, Gmail Pub/Sub ingress.

- `containermanager/internal/interface/rest/middleware`
  - API key auth, request logging, action/flow classification.

- `containermanager/internal/pkg/logctx`
  - Request-scoped logger helper.

- `containermanager/internal/service`
  - Runtime orchestration: port allocation, capacity/memory guard, idempotent Docker lifecycle, runtime-state reporting, approve/connect, archive/restore runtime files, `gog` import payload normalization, Gmail Pub/Sub forwarding.

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

Если задача про Pub/Sub или Gmail integrations:

- `simpleClaw/internal/api/rest/controllers/pubsub_proxy.go`
- `simpleClaw/internal/service/integrations`
- `simpleClaw/internal/service/clawcapability`
- `containerManager/internal/interface/rest/controllers/claws.go`
- `containerManager/internal/service/gog_payload.go`
- `containerManager/internal/entities/runtime_binding.go`

Если задача про server registry/capacity:

- `simpleClaw/internal/service/server`
- `simpleClaw/internal/infra/storages/servers`
- `containerManager/internal/interface/rest/controllers/claws.go`

Если задача про транзакционные/маркетинговые email:

- `simpleClaw/internal/service/email`
- `simpleClaw/internal/infra/storages/emailoutbox`
- `simpleClaw/internal/infra/email`
- `simpleClaw/internal/api/rest/controllers/email.go`

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
- Если добавляете новое письмо, идите через outbox (`entities.OutboxEmail` + `service/email`), не шлите inline: enqueue должен оставаться best-effort и не ронять основной business flow, а маркетинговые письма обязаны уважать `MarketingOptOut` и unsubscribe headers.

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
- В control plane больше нет legacy `claw.status`; ориентируйтесь на `desired_state`, `observed_state`, `lifecycle_status`, `current_operation_id` и lifecycle operations.
- `simpleClaw` lifecycle handlers не должны синхронно знать исход runtime-операции; неопределённые кейсы переводятся в `reconcile_pending`, а truth подтягивается из execution plane.
- `containerManager` runtime lifecycle должен оставаться идемпотентным: повторные `ensure/start/stop/delete` не должны плодить orphan runtime и не должны ломать reconcile path.
- Archive/restore остаются execution-level механизмом для runtime files и pairing state, но control plane теперь сознательно использует их как часть `start/stop/restart` lifecycle: archive целиком сохраняется в `simpleClaw`, а `openclaw.json` при restore всегда должен пересобираться из БД и не браться из сохранённого tar как source of truth.
- Не привязывайте `OnboardingComplete` напрямую к текущему `openclaw.json`: approve state может жить в runtime files. Если archive runtime state потерян или недостоверен, backend обязан перевести `OnboardingComplete=false` даже если config в БД не менялся.
- Если вносите изменения в OpenClaw config schema, проверьте и доменную сборку config в `simpleClaw`, и файловую запись/restore path в `containerManager`.
- Email-подсистема опциональна и по умолчанию выключена (`EMAIL_ENABLED=false`). При включении `config.validate` требует минимум `RESEND_API_KEY`, `EMAIL_FROM_ADDRESS` и `EMAIL_UNSUBSCRIBE_SECRET`; dispatcher поднимается отдельной goroutine из composition root, а `email_outbox` создаётся GORM auto-migration, как и остальные `simpleClaw` таблицы.
