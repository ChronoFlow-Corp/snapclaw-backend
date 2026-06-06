FROM golang:1.25 AS build

WORKDIR /src

ENV CGO_ENABLED=0

# Сначала только манифесты модулей: слой go mod download кэшируется,
# пока не менялись go.mod/go.sum, а не при каждом изменении кода.
COPY go.work go.work.sum ./
COPY shared/go.mod shared/go.sum ./shared/
COPY simpleClaw/go.mod simpleClaw/go.sum ./simpleClaw/
COPY containerManager/go.mod containerManager/go.sum ./containerManager/

# Переиспользуемый между сборками кэш модулей BuildKit:
# даже когда слой инвалидируется, модули не качаются заново.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Исходники — последним шагом: их изменение НЕ инвалидирует go mod download.
COPY shared ./shared
COPY simpleClaw ./simpleClaw
COPY containerManager ./containerManager

WORKDIR /src/containerManager

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o /out/containermanager ./cmd/containerManager

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
RUN adduser -D -g '' app

WORKDIR /app

COPY --from=build /out/containermanager /app/containermanager
COPY containerManager/migrations /app/migrations

ENV CONFIG_PATH=/config/config.yaml

USER app

EXPOSE 8080

ENTRYPOINT ["/app/containermanager"]
