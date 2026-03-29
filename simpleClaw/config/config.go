package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

const (
	defaultHTTPAddr       = ":8080"
	defaultDevHTTPAddr    = ":1337"
	defaultBackendPort    = "1337"
	defaultFrontendPort   = "3000"
	defaultDevHost        = "localhost"
	defaultOpenRouterURL  = "https://openrouter.ai"
	defaultBackupPath     = "/data/simpleclaw/backups"
	defaultJWTRefreshDev  = "simpleclaw-dev-refresh-secret"
	defaultJWTPrivateName = "jwtRS256.key"
	defaultJWTPublicName  = "jwtRS256.key.pub"
	defaultGooglePath     = "/auth/connect/google/callback"
	defaultGmailPath      = "/api/me/connect/gmail/callback"
	defaultJWTAccessTTL   = 24 * time.Hour
	defaultJWTRefreshTTL  = 168 * time.Hour
	defaultORTTimeout     = 15 * time.Second
	defaultCMTimeout      = 15 * time.Second
	defaultProxyTimeout   = 5 * time.Second
	defaultProxyRetries   = 2
	defaultProxyBackoff   = 250 * time.Millisecond
	defaultTracingRatio   = 1.0
)

type Config struct {
	Environment   string        `mapstructure:"environment"`
	Http          http          `mapstructure:"http"`
	Database      database      `mapstructure:"database"`
	Auth          auth          `mapstructure:"auth"`
	OpenRouter    openrouter    `mapstructure:"openrouter"`
	Hosting       hosting       `mapstructure:"hosting"`
	Connect       connect       `mapstructure:"connect"`
	Proxy         proxy         `mapstructure:"proxy"`
	Observability observability `mapstructure:"observability"`
	Payment       payment       `mapstructure:"payment"`
}

type http struct {
	Addr    string   `mapstructure:"addr"`
	Origins []string `mapstructure:"origins"`
}

type database struct {
	Dsn string `mapstructure:"dsn"`
}

type auth struct {
	Google google   `mapstructure:"google"`
	Jwt    Jwt      `mapstructure:"jwt"`
	Admins []string `mapstructure:"admins"`
}

type google struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	CallbackURL  string `mapstructure:"callback_url"`
	FrontendURL  string `mapstructure:"frontend_url"`
}

type Jwt struct {
	RefreshSecret       string        `mapstructure:"refresh_secret"`
	AccessSecretPublic  string        `mapstructure:"access_secret_public"`
	AccessSecretPrivate string        `mapstructure:"access_secret_private"`
	AccessExpire        time.Duration `mapstructure:"access_expire"`
	RefreshExpire       time.Duration `mapstructure:"refresh_expire"`
}

type openrouter struct {
	BaseURL  string        `mapstructure:"base_url"`
	APIToken string        `mapstructure:"api_token"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

type hosting struct {
	ContainerManager containerManager `mapstructure:"container_manager"`
}

type containerManager struct {
	Timeout    time.Duration `mapstructure:"timeout"`
	BackupPath string        `mapstructure:"backup_path"`
}

type connect struct {
	Gmail gmail `mapstructure:"gmail"`
}

type proxy struct {
	Token          string        `mapstructure:"token"`
	ForwardTimeout time.Duration `mapstructure:"forward_timeout"`
	RetryCount     int           `mapstructure:"retry_count"`
	RetryBackoff   time.Duration `mapstructure:"retry_backoff"`
}

type gmail struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	CallbackURL  string `mapstructure:"callback_url"`
	Watch        watch  `mapstructure:"watch"`
}

type watch struct {
	Topic  string   `mapstructure:"topic"`
	Labels []string `mapstructure:"labels"`
}

type observability struct {
	Metrics metrics `mapstructure:"metrics"`
	Tracing tracing `mapstructure:"tracing"`
}

type metrics struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

type tracing struct {
	Enabled     bool    `mapstructure:"enabled"`
	Endpoint    string  `mapstructure:"endpoint"`
	Insecure    bool    `mapstructure:"insecure"`
	SampleRatio float64 `mapstructure:"sample_ratio"`
}

type payment struct {
	Yookassa   yookassa          `mapstructure:"yookassa"`
	OpenRouter openrouterPayment `mapstructure:"openrouter"`
}

type yookassa struct {
	StoreID   string `mapstructure:"store_id"`
	SecretKey string `mapstructure:"secret_key"`
}

type openrouterPayment struct {
	WebhookSecret string `mapstructure:"webhook_secret"`
}

func New() Config {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		panic("CONFIG_PATH environment variable not set")
	}

	cfg, err := load(path)
	if err != nil {
		panic("failed to read config: " + err.Error())
	}

	return cfg
}

func load(path string) (Config, error) {
	cfg := Config{}

	v := newViper(path)

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.applyEnvOverrides(); err != nil {
		return Config{}, err
	}

	if cfg.Environment != EnvDevelopment && cfg.Environment != EnvProduction {
		return Config{}, fmt.Errorf("invalid environment: %s", cfg.Environment)
	}

	cfg.applyDefaults(path)
	cfg.normalize()

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	if err := cfg.ensureJWTKeyPair(path); err != nil {
		return Config{}, err
	}

	if err := cfg.loadJWT(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func newViper(path string) *viper.Viper {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetDefault("environment", EnvDevelopment)
	v.SetDefault("auth.jwt.access_expire", defaultJWTAccessTTL)
	v.SetDefault("auth.jwt.refresh_expire", defaultJWTRefreshTTL)
	v.SetDefault("openrouter.timeout", defaultORTTimeout)
	v.SetDefault("hosting.container_manager.timeout", defaultCMTimeout)
	v.SetDefault("proxy.forward_timeout", defaultProxyTimeout)
	v.SetDefault("proxy.retry_count", defaultProxyRetries)
	v.SetDefault("proxy.retry_backoff", defaultProxyBackoff)
	v.SetDefault("connect.gmail.watch.labels", []string{"INBOX"})
	v.SetDefault("observability.metrics.enabled", true)
	v.SetDefault("observability.metrics.path", "/metrics")
	v.SetDefault("observability.tracing.enabled", false)
	v.SetDefault("observability.tracing.insecure", true)
	v.SetDefault("observability.tracing.sample_ratio", defaultTracingRatio)

	return v
}

func (c *Config) applyEnvOverrides() error {
	applyStringEnv(&c.Environment, "ENVIRONMENT")
	applyStringEnv(&c.Http.Addr, "HTTP_ADDR")
	applyCSVEnv(&c.Http.Origins, "HTTP_ORIGINS")
	applyStringEnv(&c.Database.Dsn, "DATABASE_DSN")
	applyCSVEnv(&c.Auth.Admins, "AUTH_ADMINS")
	applyStringEnv(&c.Auth.Google.ClientID, "GOOGLE_CLIENT_ID")
	applyStringEnv(&c.Auth.Google.ClientSecret, "GOOGLE_CLIENT_SECRET")
	applyStringEnv(&c.Auth.Google.CallbackURL, "GOOGLE_CALLBACK_URL")
	applyStringEnv(&c.Auth.Google.FrontendURL, "FRONTEND_URL")
	applyStringEnv(&c.Auth.Jwt.RefreshSecret, "AUTH_JWT_REFRESH_SECRET")
	applyStringEnv(&c.Auth.Jwt.AccessSecretPublic, "AUTH_JWT_ACCESS_SECRET_PUBLIC")
	applyStringEnv(&c.Auth.Jwt.AccessSecretPrivate, "AUTH_JWT_ACCESS_SECRET_PRIVATE")

	if err := applyDurationEnv(&c.Auth.Jwt.AccessExpire, "AUTH_JWT_ACCESS_EXPIRE"); err != nil {
		return err
	}

	if err := applyDurationEnv(&c.Auth.Jwt.RefreshExpire, "AUTH_JWT_REFRESH_EXPIRE"); err != nil {
		return err
	}

	applyStringEnv(&c.OpenRouter.BaseURL, "OPENROUTER_BASE_URL")
	applyStringEnv(&c.OpenRouter.APIToken, "OPENROUTER_API_TOKEN")

	if err := applyDurationEnv(&c.OpenRouter.Timeout, "OPENROUTER_TIMEOUT"); err != nil {
		return err
	}

	if err := applyDurationEnv(&c.Hosting.ContainerManager.Timeout, "CONTAINER_MANAGER_TIMEOUT"); err != nil {
		return err
	}

	applyStringEnv(&c.Hosting.ContainerManager.BackupPath, "CONTAINER_MANAGER_BACKUP_PATH")
	applyStringEnv(&c.Proxy.Token, "PROXY_TOKEN")

	if err := applyDurationEnv(&c.Proxy.ForwardTimeout, "PROXY_FORWARD_TIMEOUT"); err != nil {
		return err
	}

	if err := applyIntEnv(&c.Proxy.RetryCount, "PROXY_RETRY_COUNT"); err != nil {
		return err
	}

	if err := applyDurationEnv(&c.Proxy.RetryBackoff, "PROXY_RETRY_BACKOFF"); err != nil {
		return err
	}

	applyStringEnv(&c.Connect.Gmail.ClientID, "GMAIL_CLIENT_ID")
	applyStringEnv(&c.Connect.Gmail.ClientSecret, "GMAIL_CLIENT_SECRET")
	applyStringEnv(&c.Connect.Gmail.CallbackURL, "GMAIL_CALLBACK_URL")
	applyStringEnv(&c.Connect.Gmail.Watch.Topic, "CONNECT_GMAIL_WATCH_TOPIC")
	applyCSVEnv(&c.Connect.Gmail.Watch.Labels, "CONNECT_GMAIL_WATCH_LABELS")

	if err := applyBoolEnv(&c.Observability.Metrics.Enabled, "OBS_METRICS_ENABLED"); err != nil {
		return err
	}

	applyStringEnv(&c.Observability.Metrics.Path, "OBS_METRICS_PATH")

	if err := applyBoolEnv(&c.Observability.Tracing.Enabled, "OBS_TRACING_ENABLED"); err != nil {
		return err
	}

	applyStringEnv(&c.Observability.Tracing.Endpoint, "OBS_TRACING_ENDPOINT")

	if err := applyBoolEnv(&c.Observability.Tracing.Insecure, "OBS_TRACING_INSECURE"); err != nil {
		return err
	}

	if err := applyFloatEnv(&c.Observability.Tracing.SampleRatio, "OBS_TRACING_SAMPLE_RATIO"); err != nil {
		return err
	}

	applyStringEnv(&c.Payment.Yookassa.StoreID, "YOOKASSA_STORE_ID")
	applyStringEnv(&c.Payment.Yookassa.SecretKey, "YOOKASSA_SECRET_KEY")
	applyStringEnv(&c.Payment.OpenRouter.WebhookSecret, "OPENROUTER_WEBHOOK_SECRET")

	return nil
}

func (c *Config) applyDefaults(configPath string) {
	if strings.TrimSpace(c.Environment) == "" {
		c.Environment = EnvDevelopment
	}

	if strings.TrimSpace(c.Http.Addr) == "" {
		c.Http.Addr = defaultHTTPAddr
		if c.Environment == EnvDevelopment {
			c.Http.Addr = defaultDevHTTPAddr
		}
	}

	if strings.TrimSpace(c.Hosting.ContainerManager.BackupPath) == "" {
		c.Hosting.ContainerManager.BackupPath = defaultBackupPath
	}

	if strings.TrimSpace(c.OpenRouter.BaseURL) == "" {
		c.OpenRouter.BaseURL = defaultOpenRouterURL
	}

	c.applyJWTDefaults(configPath)

	if c.Environment != EnvDevelopment {
		return
	}

	host := devHost()
	frontendURL := fmt.Sprintf("http://%s:%s", host, defaultFrontendPort)

	if strings.TrimSpace(c.Auth.Google.CallbackURL) == "" {
		c.Auth.Google.CallbackURL = fmt.Sprintf("http://%s:%s%s", host, defaultBackendPort, defaultGooglePath)
	}

	if strings.TrimSpace(c.Auth.Google.FrontendURL) == "" {
		c.Auth.Google.FrontendURL = frontendURL
	}

	if len(c.Http.Origins) == 0 {
		c.Http.Origins = []string{
			"http://" + host,
			frontendURL,
		}
	}

	if strings.TrimSpace(c.Connect.Gmail.ClientID) == "" {
		c.Connect.Gmail.ClientID = c.Auth.Google.ClientID
	}

	if strings.TrimSpace(c.Connect.Gmail.ClientSecret) == "" {
		c.Connect.Gmail.ClientSecret = c.Auth.Google.ClientSecret
	}

	if strings.TrimSpace(c.Connect.Gmail.CallbackURL) == "" {
		c.Connect.Gmail.CallbackURL = fmt.Sprintf("http://%s:%s%s", host, defaultBackendPort, defaultGmailPath)
	}
}

func applyStringEnv(target *string, envName string) {
	value, ok := lookupNonEmptyEnv(envName)
	if !ok {
		return
	}

	*target = value
}

func applyCSVEnv(target *[]string, envName string) {
	value, ok := lookupNonEmptyEnv(envName)
	if !ok {
		return
	}

	*target = strings.Split(value, ",")
}

func applyDurationEnv(target *time.Duration, envName string) error {
	value, ok := lookupNonEmptyEnv(envName)
	if !ok {
		return nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("parse %s: %w", envName, err)
	}

	*target = parsed
	return nil
}

func applyIntEnv(target *int, envName string) error {
	value, ok := lookupNonEmptyEnv(envName)
	if !ok {
		return nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("parse %s: %w", envName, err)
	}

	*target = parsed
	return nil
}

func applyBoolEnv(target *bool, envName string) error {
	value, ok := lookupNonEmptyEnv(envName)
	if !ok {
		return nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("parse %s: %w", envName, err)
	}

	*target = parsed
	return nil
}

func applyFloatEnv(target *float64, envName string) error {
	value, ok := lookupNonEmptyEnv(envName)
	if !ok {
		return nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("parse %s: %w", envName, err)
	}

	*target = parsed
	return nil
}

func lookupNonEmptyEnv(envName string) (string, bool) {
	value, ok := os.LookupEnv(envName)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}

	return value, true
}

func (c *Config) applyJWTDefaults(configPath string) {
	if strings.TrimSpace(c.Auth.Jwt.RefreshSecret) == "" && c.Environment == EnvDevelopment {
		c.Auth.Jwt.RefreshSecret = defaultJWTRefreshDev
	}

	privatePath, publicPath := defaultJWTPaths(configPath)

	if strings.TrimSpace(c.Auth.Jwt.AccessSecretPrivate) == "" {
		c.Auth.Jwt.AccessSecretPrivate = privatePath
	}

	if strings.TrimSpace(c.Auth.Jwt.AccessSecretPublic) == "" {
		c.Auth.Jwt.AccessSecretPublic = publicPath
	}
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Database.Dsn) == "" {
		return fmt.Errorf("database.dsn is required")
	}

	if strings.TrimSpace(c.Auth.Google.ClientID) == "" {
		return fmt.Errorf("auth.google.client_id is required")
	}

	if strings.TrimSpace(c.Auth.Google.ClientSecret) == "" {
		return fmt.Errorf("auth.google.client_secret is required")
	}

	if c.Environment == EnvDevelopment {
		return nil
	}

	if strings.TrimSpace(c.Auth.Google.CallbackURL) == "" {
		return fmt.Errorf("auth.google.callback_url is required")
	}

	if strings.TrimSpace(c.Auth.Google.FrontendURL) == "" {
		return fmt.Errorf("auth.google.frontend_url is required")
	}

	if strings.TrimSpace(c.Auth.Jwt.RefreshSecret) == "" {
		return fmt.Errorf("auth.jwt.refresh_secret is required")
	}

	if strings.TrimSpace(c.Auth.Jwt.AccessSecretPrivate) == "" {
		return fmt.Errorf("auth.jwt.access_secret_private is required")
	}

	if strings.TrimSpace(c.Auth.Jwt.AccessSecretPublic) == "" {
		return fmt.Errorf("auth.jwt.access_secret_public is required")
	}

	if strings.TrimSpace(c.OpenRouter.APIToken) == "" {
		return fmt.Errorf("openrouter.api_token is required")
	}

	if strings.TrimSpace(c.Payment.Yookassa.StoreID) == "" {
		return fmt.Errorf("payment.yookassa.store_id is required")
	}

	if strings.TrimSpace(c.Payment.Yookassa.SecretKey) == "" {
		return fmt.Errorf("payment.yookassa.secret_key is required")
	}

	return nil
}

func (c *Config) ensureJWTKeyPair(configPath string) error {
	privatePath := strings.TrimSpace(c.Auth.Jwt.AccessSecretPrivate)
	publicPath := strings.TrimSpace(c.Auth.Jwt.AccessSecretPublic)

	if privatePath == "" || publicPath == "" {
		return fmt.Errorf("auth.jwt access key paths are required")
	}

	if c.Environment != EnvDevelopment {
		if !fileExists(privatePath) {
			return fmt.Errorf("open access secret private file: %s: %w", privatePath, os.ErrNotExist)
		}

		if !fileExists(publicPath) {
			return fmt.Errorf("open access secret public file: %s: %w", publicPath, os.ErrNotExist)
		}

		return nil
	}

	if fileExists(privatePath) && fileExists(publicPath) {
		return nil
	}

	privatePath, publicPath = defaultOrConfiguredJWTPaths(configPath, privatePath, publicPath)

	if err := writeJWTKeyPair(privatePath, publicPath); err != nil {
		return fmt.Errorf("generate dev jwt key pair: %w", err)
	}

	return nil
}

func (c *Config) loadJWT() error {
	publicKey, err := readFile(c.Auth.Jwt.AccessSecretPublic)
	if err != nil {
		return fmt.Errorf("read access secret public file: %w", err)
	}

	privateKey, err := readFile(c.Auth.Jwt.AccessSecretPrivate)
	if err != nil {
		return fmt.Errorf("read access secret private file: %w", err)
	}

	c.Auth.Jwt.AccessSecretPublic = publicKey
	c.Auth.Jwt.AccessSecretPrivate = privateKey

	return nil
}

func (c *Config) normalize() {
	c.Auth.Admins = normalizeEmails(c.Auth.Admins)
	c.Http.Origins = normalizeList(c.Http.Origins)
	c.Connect.Gmail.Watch.Labels = normalizeList(c.Connect.Gmail.Watch.Labels)
	c.Observability.Metrics.Path = normalizeMetricsPath(c.Observability.Metrics.Path)
}

func normalizeEmails(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, raw := range values {
		email := strings.ToLower(strings.TrimSpace(raw))
		if email == "" {
			continue
		}

		if _, ok := seen[email]; ok {
			continue
		}

		seen[email] = struct{}{}
		out = append(out, email)
	}

	return out
}

func normalizeList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}

		if _, ok := seen[value]; ok {
			continue
		}

		seen[value] = struct{}{}
		out = append(out, value)
	}

	return out
}

func normalizeMetricsPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/metrics"
	}

	if strings.HasPrefix(path, "/") {
		return path
	}

	return "/" + path
}

func devHost() string {
	host := strings.TrimSpace(os.Getenv("DEV_HOST"))
	if host == "" {
		return defaultDevHost
	}

	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimRight(host, "/")
	if host == "" {
		return defaultDevHost
	}

	return host
}

func defaultJWTPaths(configPath string) (privatePath, publicPath string) {
	configDir := filepath.Dir(configPath)
	keysDir := filepath.Join(configDir, "keys")

	return filepath.Join(keysDir, defaultJWTPrivateName), filepath.Join(keysDir, defaultJWTPublicName)
}

func defaultOrConfiguredJWTPaths(configPath, privatePath, publicPath string) (string, string) {
	defaultPrivate, defaultPublic := defaultJWTPaths(configPath)

	if strings.TrimSpace(privatePath) == "" {
		privatePath = defaultPrivate
	}

	if strings.TrimSpace(publicPath) == "" {
		publicPath = defaultPublic
	}

	return privatePath, publicPath
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFile(path string) (string, error) {
	fd, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer fd.Close()

	body, err := io.ReadAll(fd)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func writeJWTKeyPair(privatePath, publicPath string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	publicBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}

	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	publicPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicBytes,
	})

	if err := os.MkdirAll(filepath.Dir(privatePath), 0o755); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(publicPath), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(privatePath, privatePEM, 0o600); err != nil {
		return err
	}

	if err := os.WriteFile(publicPath, publicPEM, 0o644); err != nil {
		return err
	}

	return nil
}
