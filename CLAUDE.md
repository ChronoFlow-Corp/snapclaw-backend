# CLAUDE.md

Краткий рабочий гайд для Claude Code по этому репозиторию. Полная архитектура,
все flow и HTTP API reference — в [`AGENTS.md`](AGENTS.md); он остаётся источником
истины, а этот файл — быстрый вход и жёсткие правила.

## Что Это

`snapclaw-backend` — backend-платформа для multi-tenant управления пользовательскими
`claw`-инстансами OpenClaw. Go workspace из трёх модулей:

- `simpleClaw/` — control plane: OAuth/JWT, пользователи, каналы, billing, payment
  webhooks, async lifecycle `claw`, OpenRouter, Pub/Sub fan-out, транзакционные email.
- `containerManager/` — execution plane: runtime-конфиги, Docker image OpenClaw,
  идемпотентные `ensure/start/stop/delete`, runtime-state, approve/connect/archive/restore.
- `shared/` — общий контракт control ↔ execution: `hostingapi`, `jwt`, `observability`, `response`.

## Работа С Workspace

- В корне нет `go.mod`. Go-команды запускайте из конкретного модуля (`simpleClaw/`,
  `containerManager/`, `shared/`), не из корня.
- Директория `containerManager/`, но import path — `containermanager/...`. Для
  `go list`/импортов ориентируйтесь на `containermanager`.
- `go.work` подключает все три модуля; `Taskfile.yaml` в корне управляет compose-стеком.

## Технический Стек (реальный, не generic Go)

- HTTP router: `chi`; JSON responses: `shared/pkg/response`; observability: `shared/pkg/observability`.
- config: `cleanenv` + `CONFIG_PATH`, с env-override поверх.
- `simpleClaw`: `gorm` + Postgres (миграции через GORM auto-migration в коде, не отдельной папкой).
- `containerManager`: `pgx` + `golang-migrate`.
- OAuth: `goth`/Google; auth tokens: `shared/pkg/jwt`; provider: OpenRouter; payment: YooKassa;
  email: Resend (опционально, `EMAIL_ENABLED`); runtime: Docker SDK.
- Если задача зависит от поведения внешней библиотеки/SDK — сначала Context7 MCP, потом выводы.

## Где Что Живёт

- user/business/billing/config ownership → `simpleClaw`.
- runtime/container/file-system execution → `containerManager`.
- общий wire contract, DTO, error payload, route constants → `shared/pkg/hostingapi`.

## Жёсткие Правила

- `context.Context` — первый аргумент во всех IO/DB/network операциях.
- Ошибки оборачивайте через `fmt.Errorf("...: %w", err)`.
- Не смешивайте domain entities, HTTP DTO и ORM models.
- В transport layer следуйте `shared/pkg/response` и `respondServiceError`.
- В `simpleClaw` не тащите Docker/runtime/file-system код; в `containerManager` — business/billing/control-plane policy.
- Меняете shared hosting contract (`shared/pkg/hostingapi`) — проверяйте оба сервиса сразу.
- Меняете billing flow — проверяйте service/storage, webhook DTO, controllers и entity mapping.
- Комментарии — только там, где объясняют неочевидное «почему», а не пересказывают код.

## Ключевые Готчи

- `start/stop/restart/delete` у `claw` в `simpleClaw` async-only: HTTP handler валидирует,
  ставит lifecycle operation и возвращает текущее состояние; исход runtime не ждём синхронно.
- Нет legacy `claw.status`; ориентируйтесь на `desired_state`, `observed_state`,
  `lifecycle_status`, `current_operation_id`. Неоднозначный outcome → `reconcile_pending`,
  truth подтягивает reconciler через `GET /claws/state`.
- `containerManager` lifecycle обязан быть идемпотентным; при старте реально собирает Docker image.
- Источник истины доменного config — `openclaw.json` в БД `simpleClaw`; при restore он всегда
  пересобирается из БД, а не берётся из runtime tar.
- Email опционален и по умолчанию выключен (`EMAIL_ENABLED=false`). Письма идут через durable
  outbox (`entities.OutboxEmail` + `service/email`), enqueue best-effort после коммита события;
  маркетинговые письма уважают `MarketingOptOut` и unsubscribe headers. Не шлите inline.

## Команды

Запускать из каждого модуля (`simpleClaw`, `containerManager`, `shared`):

- Формат: `go fmt ./...`
- Тесты: `GOCACHE=/tmp/go-build-cache go test ./... -count=1`
- Линт перед PR (если есть `golangci-lint`): `golangci-lint run ./...`

Compose-стек из корня: `task up`, `task up-obs`, `task down`, `task logs`, `task ps`.
