# Snapclaw Backend (Go)

Репозиторий — Go workspace (`go.work`) с несколькими Go-модулями. В корне нет `go.mod`, поэтому команды удобнее запускать из корня (с учётом `go.work`) или из конкретного модуля.

## Общие правила для Go-кода

- Форматирование: всегда `gofmt` (обычно достаточно `go fmt ./...`).
- Импорты: держать в порядке (если в проекте используется `goimports`, применяйте его).
- Ошибки: оборачивать контекстом (`fmt.Errorf("...: %w", err)`), не глотать.
- `context.Context`: принимать первым аргументом там, где есть IO/сеть/БД; прокидывать в нижние слои.
- Пакетные границы: доменную логику держать в `internal/*`, наружу экспортировать только реально нужное.
- Проверка перед PR: `go test ./...` и `golangci-lint run ./...` (конфиг линтера — `.golangci.yaml` в корне).

## Модули (из `go.work`)

### `simpleClaw/`
Основной HTTP API сервис.

- `simpleClaw/cmd/simpleClaw/` — точка входа, сборка зависимостей, запуск HTTP сервера.
- `simpleClaw/config/` + `simpleClaw/config.yaml` — конфигурация окружения/HTTP/БД/авторизации.
- `simpleClaw/internal/api/rest/` — REST сервер, middleware, DTO, контроллеры.
- `simpleClaw/internal/service/` — бизнес-логика (use-cases/commands), интерфейсы сервисов.
- `simpleClaw/internal/infra/` — инфраструктура: SQL (GORM/Postgres), storage-реализации, hosting manager, OpenRouter-клиент.
- `simpleClaw/internal/entities/` — доменные сущности (user/session/channel/server/claw и т.п.).

### `containerManager/`
Сервис управления контейнерами/образами (Docker) + хранение данных в Postgres.

- `containerManager/cmd/containerManager/` — точка входа: инициализация БД, Docker client/manager, поднятие REST сервера.
- `containerManager/config/` + `containerManager/config.yaml` — конфигурация Postgres/образов/пути build context.
- `containerManager/internal/interface/rest/` — REST слой (сервер, контроллеры, DTO).
- `containerManager/internal/service/` — сервисный слой (команды/интерфейсы/порты).
- `containerManager/internal/infrastucture/` — инфраструктура (Docker-обёртки, конфигурирование, SQL/pgx + storage).
- `containerManager/migrations/` — миграции БД.

### `shared/`
Переиспользуемые пакеты для других модулей.

- `shared/pkg/jwt/` — генерация/парсинг JWT (access/refresh), ошибки.
- `shared/pkg/response/` — helpers для HTTP JSON ответов (OK/error).
