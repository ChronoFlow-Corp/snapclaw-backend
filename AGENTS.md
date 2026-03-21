# Snapclaw Backend (Go)

## Что Это За Проект

`snapclaw-backend` — backend для управления пользовательскими `claw`-инстансами OpenClaw.
Проект разделён на control plane и execution plane:

- `simpleClaw` — внешний API и control plane. Здесь живут OAuth/JWT, пользователи, каналы, реестр серверов, создание и управление claw, интеграции с OpenRouter и проксирование Pub/Sub.
- `containerManager` — execution plane. Этот сервис разворачивает и обслуживает реальные контейнеры OpenClaw, хранит их runtime-состояние, конфиги и выполняет approve/connect/archive/restore, их может быть несколько на разных машинах.
- `shared` — общие контракты и инфраструктурные пакеты, которые используют оба сервиса.

На практике система делает следующее:

- пользователь логинится через Google;
- на пользователя заводится OpenRouter API key и session;
- пользователь подключает каналы и создаёт `claw`;
- `simpleClaw` собирает конфиг `claw`, выбирает хост-сервер и вызывает `containerManager`;
- `containerManager` создаёт/обновляет Docker-контейнер OpenClaw и хранит runtime-запись;
- дальше `simpleClaw` управляет жизненным циклом `claw`, а `containerManager` выполняет низкоуровневые операции.

## Назначение Проекта Для Контекста

Если вы работаете в этом репозитории как агент, думайте о системе так:

- это не просто CRUD API, а backend-платформа для multi-tenant управления AI/automation-агентами;
- `simpleClaw` отвечает за бизнес-правила, владельцев, конфиги, выбор сервера и интеграции;
- `containerManager` отвечает за исполнение, ресурсы хоста, Docker, файловые конфиги и runtime-операции;
- `shared` фиксирует общий контракт между сервисами, чтобы control plane и execution plane не расходились по API и наблюдаемости.

`claw` в контексте проекта — это пользовательский экземпляр OpenClaw с собственным конфигом, каналами, моделью, секретами и Docker-runtime.

## Workspace И Модули

Репозиторий — Go workspace (`go.work`) с несколькими модулями. В корне нет `go.mod`.
Почти все Go-команды запускайте из конкретного модуля:

- `simpleClaw/`
- `containerManager/`
- `shared/`

## Ключевые Сущности

### В `simpleClaw`

- `User` — владелец claw и каналов. Хранит профиль, роль (`user`/`admin`) и привязку к OpenRouter API key.
- `Session` — серверная refresh-session для JWT-пары access/refresh.
- `Channel` — пользовательский канал интеграции, который потом встраивается в `claw`-конфиг. Сейчас основной сценарий — Telegram, но модели уже подготовлены под Discord / WhatsApp / Slack.
- `Claw` — верхнеуровневая доменная сущность пользовательского инстанса. Содержит owner, server binding, container id, статус и runtime-конфиг.
- `ClawConfig` — большой конфиг OpenClaw: env, auth profiles, tools, session policy, channels, hooks, gateway, skills, agents, models, cron.
- `Server` — запись о доступном execution-хосте с URL, `proxy_url`, API key и capacity (`max_claws`).
- `GmailToken` — сохранённый OAuth token для Gmail connect/watch сценариев.
- `OpenRouterKey` — OpenRouter API key, который выделяется на пользователя и используется в конфиге `claw`.

### В `containerManager`

- `Container` — runtime-запись контейнера OpenClaw: кому принадлежит, какой `claw` обслуживает, Docker container id, порт, статус, признак первого запуска.
- `entities.ClawConfig` — файловое представление конфигурации, которое можно записать на файловую систему, архивировать и восстанавливать.

## Основные Flows

### 1. Аутентификация пользователя

- Клиент идёт в `simpleClaw /auth/connect/google`.
- После OAuth callback сервис ищет или создаёт `User`.
- Для нового пользователя сразу создаётся OpenRouter API key.
- Создаётся `Session`, генерируются JWT access/refresh, refresh token хранится в БД.

### 2. Подключение канала

- Пользователь вызывает `/me/channel`.
- `simpleClaw` валидирует payload и создаёт `Channel`, привязанный к `User`.
- Позже этот channel включается в `ClawConfig` при создании или обновлении `claw`.

### 3. Создание `claw`

- Пользователь вызывает `POST /claws`.
- `simpleClaw` валидирует owner/model/channel ids.
- Сервис выбирает доступный `Server` по данным capacity.
- Сервис резолвит модель в OpenRouter, собирает `ClawConfig`, внедряет OpenRouter key и channel config.
- `simpleClaw` сохраняет `Claw` в своей БД.
- Затем control plane вызывает `containerManager`, который пишет конфиги и создаёт Docker-контейнер.
- Возвращённый `container_id` сохраняется обратно в `Claw`.

### 4. Update / Start / Stop / Delete `claw`

- Все пользовательские lifecycle-команды приходят в `simpleClaw`.
- `simpleClaw` проверяет владельца, актуализирует конфиг и вызывает `containerManager`.
- `containerManager` уже делает Docker/file-system операции и обновляет свою runtime-запись.
- При delete/archive/restore участвует backup path и файловый архив конфигов.

### 5. Pairing / Connect flow

- Для interactive-подключений `simpleClaw` вызывает `containerManager` endpoints `approve` и `connect`.
- `containerManager` либо подтверждает pairing-код, либо выполняет runtime connect внутрь контейнера.
- Это execution-level операции, поэтому бизнес-решение остаётся в `simpleClaw`, а фактическое исполнение — в `containerManager`.

### 6. Gmail connect и Pub/Sub fan-out

- Пользователь может подключить Gmail через `/me/connect/gmail`.
- Токены сохраняются в `simpleClaw`, а watch-конфиг попадает в `ClawConfig`.
- Входящий Pub/Sub webhook приходит в `simpleClaw /pubsub`.
- `simpleClaw` не обрабатывает событие сам, а fan-out'ит его на все `Server.ProxyURL`.
- Каждый `containerManager` принимает `/gmail-pubsub`, дедуплицирует события и направляет их в соответствующие runtime-инстансы.

### 7. Реестр серверов и capacity

- Админ управляет списком execution-серверов через `/servers`.
- `simpleClaw` хранит URL, `proxy_url`, auth token и статус сервера.
- При create/update/sync сервис забирает capacity из `containerManager /capacity`.
- Выбор хоста для нового `claw` идёт через этот реестр.

## Как Думать Об Архитектуре

- `simpleClaw` — источник истины для пользователей, каналов, claw-метаданных и бизнес-правил.
- `containerManager` — источник истины для runtime-контейнеров и файловых конфигов на execution-хосте.
- Контракт между ними проходит через HTTP-клиент `simpleClaw/internal/infra/hosting` и DTO/routes из `shared/pkg/hostingapi`.
- У обоих сервисов есть отдельные transport/controller, service и storage/infra слои.
- Наблюдаемость вынесена в `shared/pkg/observability`, чтобы action/flow/metrics были согласованы между сервисами.

## Пакеты И Их Назначение

Ниже перечислены реальные Go-пакеты из `go list ./...`.

### `simpleClaw`

- `simpleClaw/cmd/simpleClaw`
  - Что делает: entrypoint сервиса, wiring зависимостей, bootstrap OAuth providers, DB, observability, HTTP router.
  - Зачем: это composition root, где собирается весь `simpleClaw`.

- `simpleClaw/config`
  - Что делает: загружает и нормализует runtime-конфиг сервиса.
  - Зачем: держит все внешние настройки в одном месте и не размазывает env/config parsing по слоям.

- `simpleClaw/internal/api/rest`
  - Что делает: тонкая обёртка над HTTP server lifecycle.
  - Зачем: изолирует запуск сервера от логики controller'ов и main.

- `simpleClaw/internal/api/rest/controllers`
  - Что делает: HTTP endpoints для auth, me, channels, claws, servers, health, pubsub.
  - Зачем: адаптирует HTTP transport к command/use-case слою, не смешивая transport и бизнес-логику.

- `simpleClaw/internal/api/rest/dto`
  - Что делает: request/response DTO для REST API.
  - Зачем: отделяет transport schema от доменных сущностей и защищает service-слой от HTTP-деталей.

- `simpleClaw/internal/api/rest/middleware`
  - Что делает: JWT auth, admin-only guard, request logging, action/flow classification для metrics.
  - Зачем: общие HTTP concerns должны жить отдельно от controller'ов.

- `simpleClaw/internal/entities`
  - Что делает: доменные сущности `User`, `Session`, `Channel`, `Claw`, `Server`, `ClawConfig`, hooks, gateway, skills, models.
  - Зачем: это главный слой доменной модели control plane.

- `simpleClaw/internal/entities/channels`
  - Что делает: channel-specific конфиг и константы, сейчас в основном Telegram.
  - Зачем: изолирует детали конкретных каналов от остального доменного пакета.

- `simpleClaw/internal/infra/hosting`
  - Что делает: HTTP-клиент и manager для вызовов `containerManager` (`create/start/stop/update/delete/connect/approve/archive/restore/capacity`).
  - Зачем: инкапсулирует внешний execution API и скрывает детали HTTP-контракта от service-слоя.

- `simpleClaw/internal/infra/openrouter`
  - Что делает: управление OpenRouter API keys и резолв моделей через OpenRouter API.
  - Зачем: отдельная интеграция со сторонним провайдером должна быть вынесена из use-case слоя.

- `simpleClaw/internal/infra/sql`
  - Что делает: инициализация SQL/GORM, миграции и общие DB-specific ошибки.
  - Зачем: держит SQL bootstrap и DB plumbing отдельно от домена и storage.

- `simpleClaw/internal/infra/sql/models`
  - Что делает: GORM-модели таблиц и mapping-структуры persistence-слоя.
  - Зачем: ORM-модели не должны протекать в domain entities.

- `simpleClaw/internal/infra/storages/channels`
  - Что делает: persistence для `Channel`.
  - Зачем: даёт service-слою интерфейс работы с каналами без знания GORM/SQL.

- `simpleClaw/internal/infra/storages/claws`
  - Что делает: persistence для `Claw` и связей `claw_channels`.
  - Зачем: хранит агрегат claw и его channel bindings.

- `simpleClaw/internal/infra/storages/servers`
  - Что делает: persistence для реестра execution-серверов.
  - Зачем: отделяет server registry от бизнес-правил выбора capacity.

- `simpleClaw/internal/infra/storages/users`
  - Что делает: persistence для `User`, `Session`, Gmail tokens и OpenRouter key linkage.
  - Зачем: user/auth state — отдельный агрегат со своей DB-логикой.

- `simpleClaw/internal/pkg/slctx`
  - Что делает: request-scoped `slog` logger из контекста.
  - Зачем: даёт единый способ доставать и обогащать logger внутри HTTP/request flow.

- `simpleClaw/internal/service/claw`
  - Что делает: use-cases жизненного цикла `claw`: create, update, start, stop, delete, approve pairing, connect, config/archive sync.
  - Зачем: это ядро бизнес-логики продукта.

- `simpleClaw/internal/service/claw/commands`
  - Что делает: command-структуры для операций с `claw`.
  - Зачем: фиксирует явный контракт между controller'ами и service-слоем.

- `simpleClaw/internal/service/server`
  - Что делает: use-cases реестра серверов, валидация полей, sync capacities.
  - Зачем: правила работы с execution-хостами не должны жить в controller'ах или storage.

- `simpleClaw/internal/service/server/commands`
  - Что делает: command-структуры для server use-cases.
  - Зачем: сохраняет service API явным и устойчивым к transport-изменениям.

- `simpleClaw/internal/service/user`
  - Что делает: auth/sign-in/refresh, profile, channel management, provider connect.
  - Зачем: инкапсулирует пользовательские сценарии и политику ролей.

- `simpleClaw/internal/service/user/commands`
  - Что делает: command-структуры для user use-cases.
  - Зачем: убирает зависимость service-слоя от HTTP DTO.

### `containerManager`

- `containermanager/cmd/containerManager`
  - Что делает: entrypoint сервиса, загрузка конфига, миграции, pgx pool, Docker client/manager, wiring service/controller.
  - Зачем: composition root execution plane.

- `containermanager/config`
  - Что делает: конфиг Postgres, HTTP, API key, image build, pubsub, max claws, observability.
  - Зачем: централизует runtime-настройки orchestration-сервиса.

- `containermanager/internal/entities`
  - Что делает: доменные сущности runtime-слоя, в первую очередь `Container` и файловый `ClawConfig`.
  - Зачем: описывает execution-domain независимо от REST, Docker SDK и SQL.

- `containermanager/internal/infrastucture/pkg/configurer`
  - Что делает: пишет конфиги OpenClaw на файловую систему, управляет pairing-файлами, архивированием и восстановлением.
  - Зачем: файловый layout инстанса — отдельная инфраструктурная ответственность.

- `containermanager/internal/infrastucture/pkg/docker`
  - Что делает: обёртка над Docker SDK: build image, create/start/stop/remove container, exec/connect.
  - Зачем: изолирует Docker API от service-слоя и упрощает тестирование.

- `containermanager/internal/infrastucture/sql/migrations`
  - Что делает: запускает SQL миграции через `golang-migrate`.
  - Зачем: execution DB schema должна подниматься отдельно от main/service логики.

- `containermanager/internal/infrastucture/sql/pgx`
  - Что делает: создаёт и настраивает `pgx` pool.
  - Зачем: инкапсулирует подключение к Postgres.

- `containermanager/internal/infrastucture/sql/storage`
  - Что делает: Postgres persistence для runtime-контейнеров.
  - Зачем: хранит state контейнеров независимо от Docker runtime и HTTP слоя.

- `containermanager/internal/interface/rest`
  - Что делает: HTTP server wrapper.
  - Зачем: отделяет lifecycle веб-сервера от handlers.

- `containermanager/internal/interface/rest/controllers`
  - Что делает: HTTP endpoints для `create/update/start/stop/delete`, config archive/restore, approve, connect, capacity, gmail pubsub.
  - Зачем: transport-адаптер между shared hosting API и execution use-cases.

- `containermanager/internal/interface/rest/middleware`
  - Что делает: API key auth, request logging, action/flow classification.
  - Зачем: инфраструктурные HTTP concerns не должны смешиваться с orchestration-кодом.

- `containermanager/internal/pkg/logctx`
  - Что делает: request-scoped logger helper для execution-сервиса.
  - Зачем: обеспечивает единый structured logging в runtime-flow.

- `containermanager/internal/service`
  - Что делает: orchestration use-cases контейнеров, port allocation, memory/capacity guard, approve/connect, archive/restore.
  - Зачем: это основной execution-domain слой, где принимаются runtime-решения.

- `containermanager/internal/service/commands`
  - Что делает: command-структуры service-слоя.
  - Зачем: стабилизирует контракт между REST controller'ами и orchestration-логикой.

### `shared`

- `shared/consts`
  - Что делает: общие строковые константы, например provider ids.
  - Зачем: чтобы `simpleClaw` и `containerManager` не расходились по базовым идентификаторам.

- `shared/pkg/hostingapi`
  - Что делает: общий HTTP-контракт между control plane и execution plane: routes, query params, DTO, provider mapping, error payloads.
  - Зачем: это единый source of truth для API общения `simpleClaw` ↔ `containerManager`.

- `shared/pkg/jwt`
  - Что делает: генерация и валидация access/refresh JWT, typed errors.
  - Зачем: auth logic должна быть общей и одинаковой во всех сервисах.

- `shared/pkg/observability`
  - Что делает: tracing, Prometheus metrics, HTTP instrumentation, action/flow/component labelling, error classification.
  - Зачем: унифицирует наблюдаемость по всем сервисам и сценариям.

- `shared/pkg/response`
  - Что делает: унифицированные JSON HTTP-ответы и ошибки.
  - Зачем: transport layer должен отдавать одинаковый формат ошибок и success payloads.

## Непакетные Директории И Артефакты

- `deploy/`
  - Что делает: dev/runtime конфиги для `simpleClaw`, `containerManager`, Alloy, Grafana и ключей.
  - Зачем: инфраструктурный bootstrap и локальный/deployment контекст.

- `docs/`
  - Что делает: рабочие планы и внутренние инженерные заметки.
  - Зачем: хранит проектные решения и контекст по крупным изменениям.

- `docker-compose.yml`
  - Что делает: локально поднимает оба сервиса и observability stack.
  - Зачем: удобная интеграционная среда для разработки и проверки.

## Общие Правила Для Go-Кода

- Форматирование: всегда `gofmt` (`go fmt ./...` внутри конкретного модуля).
- Импорты: держать в порядке; если в модуле используется `goimports`, применяйте его.
- Ошибки: оборачивать контекстом через `fmt.Errorf("...: %w", err)`, не глотать.
- `context.Context`: передавать первым аргументом во все IO/network/DB операции и прокидывать вниз по стеку.
- Границы пакетов: доменная логика живёт в `internal/service` и `internal/entities`, инфраструктура — в `internal/infra` или `internal/infrastucture`.
- Не смешивать DTO, ORM models и domain entities.
- Для новых сценариев сначала определяйте, в каком сервисе живёт source of truth:
  - control/data ownership — обычно `simpleClaw`;
  - runtime/container/file-system execution — обычно `containerManager`.

## Проверка Перед PR

- `(cd simpleClaw && GOCACHE=/tmp/go-build-cache go test ./... -count=1 && golangci-lint run ./...)`
- `(cd containerManager && GOCACHE=/tmp/go-build-cache go test ./... -count=1 && golangci-lint run ./...)`
- `(cd shared && GOCACHE=/tmp/go-build-cache go test ./... -count=1 && golangci-lint run ./...)`

## Практические Замечания Для Агентов

- Не запускайте Go-команды из корня workspace, если команда ожидает `go.mod`.
- Если нужно понять поведение создания/обновления `claw`, смотрите сначала:
  - `simpleClaw/internal/service/claw`
  - `simpleClaw/internal/infra/hosting`
  - `containerManager/internal/service`
  - `shared/pkg/hostingapi`
- Если нужно понять auth/user flow, смотрите сначала:
  - `simpleClaw/internal/api/rest/controllers/users.go`
  - `simpleClaw/internal/service/user`
  - `shared/pkg/jwt`
- Если нужно понять Pub/Sub или Gmail flow, смотрите сначала:
  - `simpleClaw/internal/api/rest/controllers/pubsub_proxy.go`
  - `containerManager/internal/interface/rest/controllers/claws.go`
  - `containerManager/internal/service`
