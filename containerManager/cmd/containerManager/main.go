package main

import (
	"context"
	"log/slog"
	"os"
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

	st := storage.NewContainer(pool)

	c := configurer.NewClawConfigurer(cfg.Image.BasePath)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	m := docker.NewManager(ctx, cli)

	err = m.Build(ctx, cfg.Image.BuildCtx)
	if err != nil {
		panic(err)
	}

	s := service.NewContainer(c, st, m)

	cl := controllers.NewClaw(s, cfg.Http.ApiKey)

	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(middleware.RealIP)
	mux.Use(restmw.Logger())
	mux.Use(middleware.Recoverer)

	cl.Register(mux)

	server := rest.NewServer(cfg.Http.Addr, mux)

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
