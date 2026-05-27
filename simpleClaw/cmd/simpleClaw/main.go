package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"shared/pkg/jwt"
	"shared/pkg/observability"
	"strings"
	"syscall"
	"time"

	"simpleClaw/internal/infra/convert"

	"simpleClaw/internal/infra/payment"

	"simpleClaw/internal/infra/storages/balanceentries"
	"simpleClaw/internal/infra/storages/paymentmethods"
	"simpleClaw/internal/infra/storages/payments"
	"simpleClaw/internal/infra/storages/plans"
	"simpleClaw/internal/infra/storages/subscriptions"

	"simpleClaw/config"
	"simpleClaw/internal/api/rest"
	"simpleClaw/internal/api/rest/controllers"
	appmw "simpleClaw/internal/api/rest/middleware"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/openrouter"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/storages/channels"
	"simpleClaw/internal/infra/storages/clawcapabilities"
	"simpleClaw/internal/infra/storages/clawoperations"
	"simpleClaw/internal/infra/storages/claws"
	"simpleClaw/internal/infra/storages/integrations"
	"simpleClaw/internal/infra/storages/servers"
	telegrammanagerstorage "simpleClaw/internal/infra/storages/telegrammanager"
	"simpleClaw/internal/infra/storages/users"
	telegraminfra "simpleClaw/internal/infra/telegram"
	billingservice "simpleClaw/internal/service/billing"
	"simpleClaw/internal/service/claw"
	clawcapabilityservice "simpleClaw/internal/service/clawcapability"
	integrationservice "simpleClaw/internal/service/integrations"
	"simpleClaw/internal/service/integrations/googleoauth"
	serverservice "simpleClaw/internal/service/server"
	telegrammanagerservice "simpleClaw/internal/service/telegrammanager"
	"simpleClaw/internal/service/user"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/markbates/goth"
	gothgoogle "github.com/markbates/goth/providers/google"
	"golang.org/x/oauth2"
	oauth2google "golang.org/x/oauth2/google"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type userAPIKeyManager interface {
	Create(
		ctx context.Context,
		userID uuid.UUID,
		monthlyBudgetUSD float64,
	) (entities.OpenRouterKey, error)
	DisableKey(ctx context.Context, keyID string) error
}

type clawAPIKeyManager interface {
	Create(
		ctx context.Context,
		userID uuid.UUID,
		monthlyBudgetUSD float64,
	) (entities.OpenRouterKey, error)
	ResolveModel(ctx context.Context, model string) (string, error)
}

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.New()
	logger := setupLogger(cfg.Environment)

	shutdownTracing, err := observability.SetupTracing(
		rootCtx,
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
		err := shutdownTracing(context.Background())
		if err != nil {
			logger.Error("failed to shutdown tracing provider", slog.Any("err", err))
		}
	}()

	gAuth := gothgoogle.New(
		cfg.Auth.Google.ClientID,
		cfg.Auth.Google.ClientSecret,
		cfg.Auth.Google.CallbackURL,
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/userinfo.email",
		"openid",
	)

	gAuth.SetName("google")

	goth.UseProviders(gAuth)

	db, err := gorm.Open(
		postgres.Open(cfg.Database.Dsn),
		&gorm.Config{
			TranslateError: true,
			Logger:         sql.NewGormLogger(os.Stdout),
		},
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
	clawOperationStorage := clawoperations.NewStorage(db, operationMetrics)
	integrationStorage := integrations.NewStorage(db)
	clawCapabilityStorage := clawcapabilities.NewStorage(db)
	serversStorage := servers.NewStorage(db, operationMetrics)
	paymentMethodStorage := paymentmethods.NewStorage(db)
	paymentStorage := payments.NewStorage(db)
	plansStorage := plans.NewStorage(db)
	subscriptionsStorage := subscriptions.NewStorage(db)
	balanceEntriesStorage := balanceentries.NewStorage(db)
	telegramManagerStorage := telegrammanagerstorage.NewStorage(db)

	j := jwt.New(
		[]byte(cfg.Auth.Jwt.AccessSecretPrivate),
		[]byte(cfg.Auth.Jwt.AccessSecretPublic),
		[]byte(cfg.Auth.Jwt.RefreshSecret),
		cfg.Auth.Jwt.AccessExpire,
		cfg.Auth.Jwt.RefreshExpire,
	)

	var (
		userKeys userAPIKeyManager
		clawKeys clawAPIKeyManager
	)

	if strings.TrimSpace(cfg.OpenRouter.APIToken) != "" {
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

		userKeys = orManager
		clawKeys = orManager
	} else {
		logger.Warn("openrouter is disabled; google oauth works, claw creation requires OPENROUTER_API_TOKEN")
	}

	hostingManager := hosting.NewManager(operationMetrics).
		WithRuntimeSecrets(hosting.RuntimeSecrets{
			BraveAPIKey: cfg.Brave.APIKey,
		})
	paymentManager := payment.NewYooKassa(
		cfg.Environment,
		cfg.Payment.Yookassa.StoreID,
		cfg.Payment.Yookassa.SecretKey,
		cfg.Auth.Google.FrontendURL,
	)
	ctx := rootCtx

	conv := convert.NewAmount(ctx)

	uService := user.NewUser(
		j,
		userStorage,
		channelsStorage,
		paymentMethodStorage,
		userKeys,
		cfg.Auth.Admins,
		operationMetrics,
	)
	billingSvc := billingservice.NewService(
		cfg.Environment,
		plansStorage,
		subscriptionsStorage,
		balanceEntriesStorage,
		userStorage,
		paymentStorage,
		paymentManager,
		conv,
		paymentMethodStorage,
		userKeys,
		operationMetrics,
	)

	serverService := serverservice.New(serversStorage, hostingManager, operationMetrics)
	integrationSvc := integrationservice.NewService(integrationStorage)
	googleOAuthSvc := googleoauth.NewService(googleoauth.ServiceOptions{
		Auth: &oauth2.Config{
			ClientID:     cfg.Auth.Google.ClientID,
			ClientSecret: cfg.Auth.Google.ClientSecret,
			RedirectURL:  cfg.Auth.Google.IntegrationCallbackURL,
			Scopes:       nil,
			Endpoint:     oauth2google.Endpoint,
		},
		Storage: integrationStorage,
		StateCodec: googleoauth.NewStateCodec(
			[]byte(cfg.Auth.Google.IntegrationStateSecret),
			10*time.Minute,
			time.Now,
		),
		Profile: googleoauth.NewHTTPProfileFetcher(&oauth2.Config{
			ClientID:     cfg.Auth.Google.ClientID,
			ClientSecret: cfg.Auth.Google.ClientSecret,
			RedirectURL:  cfg.Auth.Google.IntegrationCallbackURL,
			Endpoint:     oauth2google.Endpoint,
		}),
	})
	clawCapabilitySvc := clawcapabilityservice.NewService(
		clawStorage,
		clawCapabilityStorage,
		integrationStorage,
	)
	if err := serverService.SyncCapacities(context.Background()); err != nil {
		logger.Error("failed to sync server capacities", slog.Any("err", err))
	}

	clawService := claw.NewClaw(
		clawStorage,
		clawOperationStorage,
		channelsStorage,
		userStorage,
		serversStorage,
		hostingManager,
		clawKeys,
		cfg.Hosting.ContainerManager.BackupPath,
		claw.GmailWatchConfig{
			Topic:  cfg.Connect.Gmail.Watch.Topic,
			Labels: cfg.Connect.Gmail.Watch.Labels,
		},
		operationMetrics,
	).WithBraveAPIKey(cfg.Brave.APIKey).WithCapabilityDependencies(clawCapabilityStorage, integrationStorage)
	billingSvc.WithBootstrapClawReader(clawService)
	billingSvc.WithBootstrapManagedBotReader(telegramManagerStorage)
	go clawService.RunLifecycleWorker(rootCtx, 0)
	go clawService.RunReconciler(rootCtx, 0)
	if cfg.Hosting.ContainerManager.RuntimeSync.Enabled {
		go clawService.RunRuntimeSync(
			rootCtx,
			cfg.Hosting.ContainerManager.RuntimeSync.Interval,
			cfg.Hosting.ContainerManager.RuntimeSync.Timeout,
			cfg.Hosting.ContainerManager.RuntimeSync.BatchSize,
			cfg.Hosting.ContainerManager.RuntimeSync.WorkerCount,
		)
	}

	uController := controllers.NewUser(
		cfg.Environment,
		uService,
		integrationSvc,
		googleOAuthSvc,
		j,
		cfg.Auth.Google.FrontendURL,
	)
	clawController := controllers.NewClaw(clawService, clawCapabilitySvc, j)
	serverController := controllers.NewServer(serverService, uService, j)
	var telegramManagerController *controllers.TelegramManager
	var telegramManagerSvc *telegrammanagerservice.Service
	if cfg.TelegramManager.Enabled {
		telegramClient := telegraminfra.New(
			cfg.TelegramManager.BaseURL,
			cfg.TelegramManager.BotToken,
			cfg.TelegramManager.Timeout,
		)
		telegramManagerSvc = telegrammanagerservice.New(
			telegramManagerStorage,
			channelsStorage,
			telegramClient,
			telegrammanagerservice.Options{
				ManagerUsername: cfg.TelegramManager.ManagerUsername,
				LinkTTL:         10 * time.Minute,
			},
		)
		telegramManagerController = controllers.NewTelegramManager(
			telegramManagerSvc,
			j,
			cfg.TelegramManager.WebhookSecret,
		)

		if cfg.TelegramManager.Mode == "polling" {
			go func() {
				err := telegramManagerSvc.RunPolling(rootCtx, telegrammanagerservice.PollingOptions{
					TimeoutSeconds: 30,
					AllowedUpdates: []string{"message", "managed_bot"},
				})
				if err != nil && err != context.Canceled {
					logger.Error("telegram manager polling stopped", slog.Any("err", err))
				}
			}()
		}
	}
	billingController := controllers.NewBilling(
		billingSvc,
		uService,
		j,
		cfg.Auth.Google.FrontendURL,
		cfg.Payment.OpenRouter.WebhookSecret,
	)
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
	if telegramManagerController != nil {
		telegramManagerController.Register(r)
	}
	billingController.Register(r)
	proxyController.Register(r)

	var handler http.Handler = r

	if cfg.Observability.Tracing.Enabled {
		handler = observability.WrapHTTPHandler(handler, "simpleclaw.http")
	}

	s := rest.NewServer(cfg.Http.Addr, handler)

	logger.Info("simpleClaw server starting", slog.String("addr", cfg.Http.Addr))

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- s.ListenAndServe()
	}()

	select {
	case err := <-serverErrCh:
		logger.Error("simpleClaw server stopped", slog.Any("err", err))
		panic(err)
	case <-rootCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.Shutdown(shutdownCtx); err != nil {
			logger.Error("simpleClaw shutdown failed", slog.Any("err", err))
			panic(err)
		}
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
