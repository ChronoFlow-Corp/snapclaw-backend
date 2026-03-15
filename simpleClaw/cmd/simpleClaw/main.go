package main

import (
	"log/slog"
	"os"
	"shared/pkg/jwt"
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

	goth.UseProviders(
		google.New(
			cfg.Auth.Google.ClientID,
			cfg.Auth.Google.ClientSecret,
			cfg.Auth.Google.CallbackURL,
		),
	)

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

	userStorage := users.NewStorage(db)
	channelsStorage := channels.NewStorage(db)
	clawStorage := claws.NewStorage(db)
	serversStorage := servers.NewStorage(db)

	j := jwt.New(
		[]byte(cfg.Auth.Jwt.AccessSecretPrivate),
		[]byte(cfg.Auth.Jwt.AccessSecretPublic),
		[]byte(cfg.Auth.Jwt.RefreshSecret),
		cfg.Auth.Jwt.AccessExpire,
		cfg.Auth.Jwt.RefreshExpire,
	)

	orManager, err := openrouter.NewApiKeyManager(openrouter.Options{
		BaseURL:  cfg.OpenRouter.BaseURL,
		APIToken: cfg.OpenRouter.APIToken,
		Timeout:  cfg.OpenRouter.Timeout,
	})
	if err != nil {
		panic(err)
	}

	uService := user.NewUser(j, userStorage, channelsStorage, orManager)
	hostingManager := hosting.NewManager()
	clawService := claw.NewClaw(
		clawStorage,
		channelsStorage,
		userStorage,
		serversStorage,
		hostingManager,
		orManager,
		cfg.Hosting.ContainerManager.BackupPath,
	)

	api := chi.NewRouter()

	uController := controllers.NewUser(cfg.Environment, uService, j, cfg.Auth.Google.FrontendURL)
	clawController := controllers.NewClaw(clawService, j)

	uController.Register(api)
	clawController.Register(api)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(appmw.Logger())
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(corsOptions(cfg)))

	r.Mount("/api", api)

	s := rest.NewServer(cfg.Http.Addr, r)

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
