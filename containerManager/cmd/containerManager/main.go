package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"shared/pkg/observability"
	"strings"

	"containermanager/config"
	"containermanager/internal/infrastucture/pkg/configurer"
	"containermanager/internal/infrastucture/pkg/docker"
	"containermanager/internal/infrastucture/sql/migrations"
	"containermanager/internal/infrastucture/sql/pgx"
	"containermanager/internal/infrastucture/sql/storage"
	"containermanager/internal/interface/rest"
	"containermanager/internal/interface/rest/controllers"
	restmw "containermanager/internal/interface/rest/middleware"
	"containermanager/internal/service"

	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.MustLoadConfig()
	logger := setupLogger()
	maxClaws, err := cfg.MaxClaws.ResolveLinux()
	if err != nil {
		panic(err)
	}
	shutdownTracing, err := observability.SetupTracing(context.Background(), observability.TracingConfig{
		ServiceName: "containermanager",
		Environment: cfg.Environment,
		Enabled:     cfg.Observability.Tracing.Enabled,
		Endpoint:    cfg.Observability.Tracing.Endpoint,
		Insecure:    cfg.Observability.Tracing.Insecure,
		SampleRatio: cfg.Observability.Tracing.SampleRatio,
	})
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := shutdownTracing(context.Background()); err != nil {
			logger.Error("failed to shutdown tracing provider", slog.Any("err", err))
		}
	}()

	ctx := context.Background()

	if cfg.Migrations.Auto {
		if err := migrations.Run(ctx, cfg.Postgres.URL, cfg.Migrations.Path); err != nil {
			panic(err)
		}
	}

	pool, err := pgx.New(ctx, cfg.Postgres.URL)
	if err != nil {
		panic(err)
	}

	metricsRegistry := observability.NewPrometheusRegistry()
	httpMetrics, err := observability.NewHTTPMetrics(metricsRegistry)
	if err != nil {
		panic(err)
	}
	operationMetrics, err := observability.NewOperationMetrics(metricsRegistry, "containermanager")
	if err != nil {
		panic(err)
	}
	pubSubMetrics, err := observability.NewPubSubFanoutMetrics(metricsRegistry, "containermanager")
	if err != nil {
		panic(err)
	}

	st := storage.NewContainer(pool, operationMetrics)

	c := configurer.NewClawConfigurer(cfg.Image.BasePath, cfg.Image.CredentialsPath)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	m, err := docker.NewManager(ctx, cli, operationMetrics)
	if err != nil {
		panic(err)
	}

	err = m.Build(ctx, cfg.Image.Dockerfile, cfg.Image.BuildCtx)
	if err != nil {
		panic(err)
	}

	s, err := service.NewContainer(c, st, m, maxClaws, service.GogConfig{
		KeyringBackend:  cfg.Gog.KeyringBackend,
		KeyringPassword: cfg.Gog.KeyringPassword,
	}, operationMetrics)
	if err != nil {
		panic(err)
	}

	cl := controllers.NewClaw(s, cfg.Http.ApiKey, controllers.ClawOptions{
		PubSubForwardTimeout: cfg.PubSub.ForwardTimeout,
		PubSubWorkers:        cfg.PubSub.Workers,
		PubSubDedupTTL:       cfg.PubSub.DedupTTL,
		MaxClaws:             maxClaws,
		Metrics:              pubSubMetrics,
	})

	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(middleware.RealIP)
	mux.Use(restmw.Logger())
	mux.Use(middleware.Recoverer)
	mux.Use(httpMetrics.Middleware(restmw.ClassifyActionFlow, restmw.RoutePattern))
	if cfg.Observability.Metrics.Enabled {
		mux.Handle(cfg.Observability.Metrics.Path, observability.Handler(metricsRegistry))
	}

	cl.Register(mux)

	var handler http.Handler = mux
	if cfg.Observability.Tracing.Enabled {
		handler = observability.WrapHTTPHandler(handler, "containermanager.http")
	}

	server := rest.NewServer(cfg.Http.Addr, handler)

	logger.Info("containerManager server starting", slog.String("addr", cfg.Http.Addr))

	if err := server.Start(); err != nil {
		logger.Error("containerManager server stopped", slog.Any("err", err))
		panic(err)
	}
}

func setupLogger() *slog.Logger {
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)

	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "info":
		level.Set(slog.LevelInfo)
	case "warn", "warning":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	}

	logger := slog.New(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}),
	).With("service", "containermanager")

	slog.SetDefault(logger)

	return logger
}
