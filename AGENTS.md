# Snapclaw Backend (Go)

Репозиторий — Go workspace (`go.work`) с несколькими Go-модулями. В корне нет `go.mod`.
Практически все Go-команды (`go test`, `go list`, `golangci-lint`) запускайте из конкретного модуля:
- `simpleClaw/`
- `containerManager/`
- `shared/`

## Общие правила для Go-кода

- Форматирование: всегда `gofmt` (обычно достаточно `go fmt ./...`).
- Импорты: держать в порядке (если в проекте используется `goimports`, применяйте его).
- Ошибки: оборачивать контекстом (`fmt.Errorf("...: %w", err)`), не глотать.
- `context.Context`: принимать первым аргументом там, где есть IO/сеть/БД; прокидывать в нижние слои.
- Пакетные границы: доменную логику держать в `internal/*`, наружу экспортировать только реально нужное.
- Проверка перед PR:
  - `(cd simpleClaw && go test ./... && golangci-lint run ./...)`
  - `(cd containerManager && GOCACHE=/tmp/go-build-cache go test ./... && golangci-lint run ./...)`
  - `(cd shared && go test ./... && golangci-lint run ./...)`

## Модули и пакеты (из `go.work`)

### `simpleClaw/`
Основной API-сервис (OAuth/JWT, управление claw-сущностями, серверный реестр, прокси PubSub).

- `simpleClaw/cmd/simpleClaw/` — entrypoint, wiring зависимостей, инициализация HTTP роутера и middleware.
- `simpleClaw/config/` — загрузка и нормализация конфигурации (включая JWT-ключи, OAuth и OpenRouter).
- `simpleClaw/internal/api/rest/` — HTTP server wrapper.
- `simpleClaw/internal/api/rest/controllers/` — REST endpoints:
  - auth/me, channels, claws CRUD/start/stop/connect/approve;
  - admin-only servers CRUD;
  - `/pubsub` fan-out proxy и `/health`.
- `simpleClaw/internal/api/rest/dto/` — transport DTO для request/response.
- `simpleClaw/internal/api/rest/middleware/` — JWT auth, admin-check, request logging.
- `simpleClaw/internal/entities/` — доменные сущности (user, session, channel, server, claw, claw config, gmail token, openrouter key).
- `simpleClaw/internal/entities/channels/` — channel-specific конфигурации и константы (сейчас Telegram).
- `simpleClaw/internal/service/user/` — use-cases пользователя: signin/refresh, profile, каналы, connect provider.
- `simpleClaw/internal/service/user/commands/` — command-структуры для user use-cases.
- `simpleClaw/internal/service/server/` — бизнес-логика реестра серверов/валидация полей.
- `simpleClaw/internal/service/server/commands/` — command-структуры для server use-cases.
- `simpleClaw/internal/service/claw/` — жизненный цикл claw: create/update/start/stop/delete, pairing/connect, конфиг-архив.
- `simpleClaw/internal/service/claw/commands/` — command-структуры для claw use-cases.
- `simpleClaw/internal/infra/openrouter/` — клиент/менеджер API-ключей и резолв модели в OpenRouter.
- `simpleClaw/internal/infra/hosting/` — HTTP-клиент к `containerManager` (create/start/stop/delete/update/connect/archive/restore).
- `simpleClaw/internal/infra/sql/` — миграции GORM и SQL-ошибки.
- `simpleClaw/internal/infra/sql/models/` — DB-модели (GORM).
- `simpleClaw/internal/infra/storages/channels/` — storage каналов.
- `simpleClaw/internal/infra/storages/claws/` — storage claws + связи `claw_channels`.
- `simpleClaw/internal/infra/storages/servers/` — storage серверов.
- `simpleClaw/internal/infra/storages/users/` — storage пользователей, сессий и Gmail токенов.
- `simpleClaw/internal/pkg/slctx/` — контекстный логгер (`slog`) для request scope.

### `containerManager/`
Сервис-оркестратор контейнеров OpenClaw (Docker + Postgres + конфигурационные файлы инстансов).

- `containerManager/cmd/containerManager/` — entrypoint: конфиг, миграции, pgx pool, Docker manager, REST server.
- `containerManager/config/` — конфигурация Postgres/HTTP/API key/image build/migrations.
- `containerManager/internal/interface/rest/` — HTTP server wrapper.
- `containerManager/internal/interface/rest/controllers/` — REST endpoints управления контейнерами и config archive/restore, approve/connect.
- `containerManager/internal/interface/rest/controllers/dto/` — transport DTO для REST слоя.
- `containerManager/internal/interface/rest/middleware/` — API key auth + request logging.
- `containerManager/internal/service/` — orchestration use-cases контейнеров (create/start/stop/update/delete/approve/connect/archive/restore), порт-аллокатор.
- `containerManager/internal/service/commands/` — command-структуры сервисного слоя.
- `containerManager/internal/entities/` — доменные сущности контейнера и форматы конфиг-файлов.
- `containerManager/internal/infrastucture/pkg/docker/` — обёртка Docker SDK: build image, create/start/stop/remove, exec connect.
- `containerManager/internal/infrastucture/pkg/configurer/` — управление конфигами claw на файловой системе, tar archive/restore, pairing-файлы.
- `containerManager/internal/infrastucture/sql/pgx/` — создание pgx pool.
- `containerManager/internal/infrastucture/sql/storage/` — Postgres-репозиторий контейнеров.
- `containerManager/internal/infrastucture/sql/migrations/` — запуск миграций через `golang-migrate`.
- `containerManager/internal/pkg/logctx/` — контекстный логгер (`slog`) для request scope.
- `containerManager/migrations/` — SQL миграции.
- `containerManager/images/openclaw/` — Docker build context/артефакты образа.

### `shared/`
Переиспользуемые пакеты для других модулей.

- `shared/consts/` — общие константы между сервисами (например, provider identifiers).
- `shared/pkg/jwt/` — генерация/парсинг JWT (access/refresh), типы токенов и ошибки валидации.
- `shared/pkg/response/` — унифицированные HTTP JSON ответы (`RespondOK`, `RespondError`).
