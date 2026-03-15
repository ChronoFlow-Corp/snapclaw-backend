FROM golang:1.25 AS build

WORKDIR /src

COPY go.work go.work.sum ./
COPY shared ./shared
COPY simpleClaw ./simpleClaw
COPY containerManager ./containerManager

WORKDIR /src/simpleClaw

ENV CGO_ENABLED=0

RUN go mod download
RUN go build -trimpath -o /out/simpleclaw ./cmd/simpleClaw

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
RUN adduser -D -g '' app

WORKDIR /app

COPY --from=build /out/simpleclaw /app/simpleclaw

ENV CONFIG_PATH=/config/config.yaml

USER app

EXPOSE 1337

ENTRYPOINT ["/app/simpleclaw"]
