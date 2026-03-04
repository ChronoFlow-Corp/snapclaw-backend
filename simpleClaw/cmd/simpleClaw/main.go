package main

import (
	"fmt"
	"shared/pkg/jwt"

	"simpleClaw/internal/api/rest"
	"simpleClaw/internal/api/rest/controllers"

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
	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/google"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg := config.New()

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
	clawService := claw.NewClaw(clawStorage, channelsStorage, userStorage, serversStorage, hostingManager, orManager)

	api := chi.NewRouter()

	uController := controllers.NewUser(cfg.Environment, uService)
	clawController := controllers.NewClaw(clawService)

	uController.Register(api)
	clawController.Register(api)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Mount("/api", api)

	s := rest.NewServer(cfg.Http.Addr, r)

	fmt.Println("Starting server on " + cfg.Http.Addr)

	if err := s.ListenAndServe(); err != nil {
		panic(err)
	}
}
