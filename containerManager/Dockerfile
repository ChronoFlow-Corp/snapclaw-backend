FROM golang:1.25 AS build

WORKDIR /src

COPY go.work go.work.sum ./
COPY shared ./shared
COPY containerManager ./containerManager
COPY simpleClaw ./simpleClaw

WORKDIR /src/containerManager

ENV CGO_ENABLED=0

RUN go mod download
RUN go build -trimpath -o /out/containermanager ./cmd/containerManager

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
