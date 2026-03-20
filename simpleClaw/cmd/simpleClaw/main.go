package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"shared/pkg/jwt"
	"shared/pkg/observability"
	"strings"

	"simpleClaw/internal/api/rest"
	"simpleClaw/internal/api/rest/controllers"
	appmw "simpleClaw/internal/api/rest/middleware"

	"simpleClaw/config"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/openrouter"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/storages/channels"
	"simpleClaw/internal/infra/storages/claws"
	"simpleClaw/internal/infra/storages/servers"
	"simpleClaw/internal/infra/storages/users"
	"simpleClaw/internal/service/claw"
	serverservice "simpleClaw/internal/service/server"
	"simpleClaw/internal/service/user"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/google"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg := config.New()
	logger := setupLogger(cfg.Environment)
	shutdownTracing, err := observability.SetupTracing(
		context.Background(),
		observability.TracingConfig{
			ServiceName: "simpleclaw",
			Environment: cfg.Environment,
			Enabled:     cfg.Observability.Tracing.Enabled,
			Endpoint:    cfg.Observability.Tracing.Endpoint,
			Insecure:    cfg.Observability.Tracing.Insecure,
			SampleRatio: cfg.Observability.Tracing.SampleRatio,
		},
	)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := shutdownTracing(context.Background()); err != nil {
			logger.Error("failed to shutdown tracing provider", slog.Any("err", err))
		}
	}()

	gAuth := google.New(
		cfg.Auth.Google.ClientID,
		cfg.Auth.Google.ClientSecret,
		cfg.Auth.Google.CallbackURL,
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/userinfo.email",
		"openid",
	)

	gmailConnect := google.New(
		cfg.Connect.Gmail.ClientID,
		cfg.Connect.Gmail.ClientSecret,
		cfg.Connect.Gmail.CallbackURL,
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/userinfo.email",
		"openid",
		"https://www.googleapis.com/auth/gmail.send",
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/gmail.modify",
		"https://www.googleapis.com/auth/gmail.labels",
	)

	gmailConnect.SetName("gmail")
	gmailConnect.SetPrompt("consent")
	gAuth.SetName("google")

	goth.UseProviders(gAuth, gmailConnect)

	db, err := gorm.Open(
		postgres.Open(cfg.Database.Dsn),
		&gorm.Config{},
	)
	if err != nil {
		panic(err)
	}

	err = sql.Migration(db)
	if err != nil {
		panic(err)
	}

	metricsRegistry := observability.NewPrometheusRegistry()
	httpMetrics, err := observability.NewHTTPMetrics(metricsRegistry)
	if err != nil {
		panic(err)
	}
	operationMetrics, err := observability.NewOperationMetrics(metricsRegistry, "simpleclaw")
	if err != nil {
		panic(err)
	}
	pubSubMetrics, err := observability.NewPubSubFanoutMetrics(metricsRegistry, "simpleclaw")
	if err != nil {
		panic(err)
	}

	userStorage := users.NewStorage(db, operationMetrics)
	channelsStorage := channels.NewStorage(db)
	clawStorage := claws.NewStorage(db, operationMetrics)
	serversStorage := servers.NewStorage(db, operationMetrics)

	j := jwt.New(
		[]byte(cfg.Auth.Jwt.AccessSecretPrivate),
		[]byte(cfg.Auth.Jwt.AccessSecretPublic),
		[]byte(cfg.Auth.Jwt.RefreshSecret),
		cfg.Auth.Jwt.AccessExpire,
		cfg.Auth.Jwt.RefreshExpire,
	)

	orManager, err := openrouter.NewApiKeyManager(openrouter.Options{
		BaseURL:          cfg.OpenRouter.BaseURL,
		APIToken:         cfg.OpenRouter.APIToken,
		Timeout:          cfg.OpenRouter.Timeout,
		HTTPClient:       observability.NewHTTPClient(cfg.OpenRouter.Timeout),
		OperationMetrics: operationMetrics,
	})
	if err != nil {
		panic(err)
	}

	hostingManager := hosting.NewManager(operationMetrics)

	uService := user.NewUser(
		j,
		userStorage,
		channelsStorage,
		orManager,
		cfg.Auth.Admins,
		operationMetrics,
	)
	serverService := serverservice.New(serversStorage, hostingManager, operationMetrics)
	if err := serverService.SyncCapacities(context.Background()); err != nil {
		logger.Error("failed to sync server capacities", slog.Any("err", err))
	}
	clawService := claw.NewClaw(
		clawStorage,
		channelsStorage,
		userStorage,
		serversStorage,
		hostingManager,
		orManager,
		cfg.Hosting.ContainerManager.BackupPath,
		claw.GmailWatchConfig{
			Topic:  cfg.Connect.Gmail.Watch.Topic,
			Labels: cfg.Connect.Gmail.Watch.Labels,
		},
		operationMetrics,
	)

	uController := controllers.NewUser(cfg.Environment, uService, j, cfg.Auth.Google.FrontendURL)
	clawController := controllers.NewClaw(clawService, j)
	serverController := controllers.NewServer(serverService, uService, j)
	proxyController := controllers.NewPubSubProxy(serverService, controllers.PubSubProxyOptions{
		Token:          cfg.Proxy.Token,
		ForwardTimeout: cfg.Proxy.ForwardTimeout,
		RetryCount:     cfg.Proxy.RetryCount,
		RetryBackoff:   cfg.Proxy.RetryBackoff,
		Metrics:        pubSubMetrics,
	})

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(appmw.Logger())
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(corsOptions(cfg)))
	r.Use(httpMetrics.Middleware(appmw.ClassifyActionFlow, appmw.RoutePattern))
	if cfg.Observability.Metrics.Enabled {
		r.Handle(cfg.Observability.Metrics.Path, observability.Handler(metricsRegistry))
	}

	uController.Register(r)
	clawController.Register(r)
	serverController.Register(r)
	proxyController.Register(r)

	var handler http.Handler = r
	if cfg.Observability.Tracing.Enabled {
		handler = observability.WrapHTTPHandler(handler, "simpleclaw.http")
	}

	s := rest.NewServer(cfg.Http.Addr, handler)

	logger.Info("simpleClaw server starting", slog.String("addr", cfg.Http.Addr))

	if err := s.ListenAndServe(); err != nil {
		logger.Error("simpleClaw server stopped", slog.Any("err", err))
		panic(err)
	}
}

func setupLogger(env string) *slog.Logger {
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
	).With(
		"service", "simpleclaw",
		"env", env,
	)

	slog.SetDefault(logger)

	return logger
}

func corsOptions(cfg config.Config) cors.Options {
	allowedOrigins := []string{cfg.Auth.Google.FrontendURL}
	allowedOrigins = append(allowedOrigins, cfg.Http.Origins...)

	return cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}
}

func splitCommaList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
