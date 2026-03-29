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
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
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
)

type Config struct {
	Environment   string        `yaml:"environment" env:"ENVIRONMENT" env-default:"development"`
	Http          http          `yaml:"http"`
	Database      database      `yaml:"database"`
	Auth          auth          `yaml:"auth"`
	OpenRouter    openrouter    `yaml:"openrouter"`
	Hosting       hosting       `yaml:"hosting"`
	Connect       connect       `yaml:"connect"`
	Proxy         proxy         `yaml:"proxy"`
	Observability observability `yaml:"observability"`
	Payment       payment       `yaml:"payment"`
}

type http struct {
	Addr    string   `env:"HTTP_ADDR"    yaml:"addr"`
	Origins []string `env:"HTTP_ORIGINS" yaml:"origins" env-separator:","`
}

type database struct {
	Dsn string `env:"DATABASE_DSN" yaml:"dsn"`
}

type auth struct {
	Google google   `yaml:"google"`
	Jwt    Jwt      `yaml:"jwt"`
	Admins []string `yaml:"admins" env:"AUTH_ADMINS" env-separator:","`
}

type google struct {
	ClientID     string `env:"GOOGLE_CLIENT_ID"     yaml:"client_id"`
	ClientSecret string `env:"GOOGLE_CLIENT_SECRET" yaml:"client_secret"`
	CallbackURL  string `env:"GOOGLE_CALLBACK_URL"  yaml:"callback_url"`
	FrontendURL  string `env:"FRONTEND_URL"         yaml:"frontend_url"`
}

type Jwt struct {
	RefreshSecret       string        `env:"AUTH_JWT_REFRESH_SECRET"        yaml:"refresh_secret"`
	AccessSecretPublic  string        `env:"AUTH_JWT_ACCESS_SECRET_PUBLIC"  yaml:"access_secret_public"`
	AccessSecretPrivate string        `env:"AUTH_JWT_ACCESS_SECRET_PRIVATE" yaml:"access_secret_private"`
	AccessExpire        time.Duration `env:"AUTH_JWT_ACCESS_EXPIRE"         yaml:"access_expire"  env-default:"24h"`
	RefreshExpire       time.Duration `env:"AUTH_JWT_REFRESH_EXPIRE"        yaml:"refresh_expire" env-default:"168h"`
}

type openrouter struct {
	BaseURL  string        `env:"OPENROUTER_BASE_URL"  yaml:"base_url"`
	APIToken string        `env:"OPENROUTER_API_TOKEN" yaml:"api_token"`
	Timeout  time.Duration `env:"OPENROUTER_TIMEOUT"   yaml:"timeout" env-default:"15s"`
}

type hosting struct {
	ContainerManager containerManager `yaml:"container_manager"`
}

type containerManager struct {
	Timeout    time.Duration `env:"CONTAINER_MANAGER_TIMEOUT"     yaml:"timeout" env-default:"15s"`
	BackupPath string        `env:"CONTAINER_MANAGER_BACKUP_PATH" yaml:"backup_path" env-default:"/data/simpleclaw/backups"`
}

type connect struct {
	Gmail gmail `yaml:"gmail"`
}

type proxy struct {
	Token          string        `env:"PROXY_TOKEN"           yaml:"token"`
	ForwardTimeout time.Duration `env:"PROXY_FORWARD_TIMEOUT" yaml:"forward_timeout" env-default:"5s"`
	RetryCount     int           `env:"PROXY_RETRY_COUNT"     yaml:"retry_count" env-default:"2"`
	RetryBackoff   time.Duration `env:"PROXY_RETRY_BACKOFF"   yaml:"retry_backoff" env-default:"250ms"`
}

type gmail struct {
	ClientID     string `env:"GMAIL_CLIENT_ID"     yaml:"client_id"`
	ClientSecret string `env:"GMAIL_CLIENT_SECRET" yaml:"client_secret"`
	CallbackURL  string `env:"GMAIL_CALLBACK_URL"  yaml:"callback_url"`
	Watch        watch  `yaml:"watch"`
}

type watch struct {
	Topic  string   `env:"CONNECT_GMAIL_WATCH_TOPIC"  yaml:"topic"`
	Labels []string `env:"CONNECT_GMAIL_WATCH_LABELS" yaml:"labels" env-default:"INBOX" env-separator:","`
}

type observability struct {
	Metrics metrics `yaml:"metrics"`
	Tracing tracing `yaml:"tracing"`
}

type metrics struct {
	Enabled bool   `env:"OBS_METRICS_ENABLED" env-default:"true" yaml:"enabled"`
	Path    string `env:"OBS_METRICS_PATH"    env-default:"/metrics" yaml:"path"`
}

type tracing struct {
	Enabled     bool    `env:"OBS_TRACING_ENABLED"      env-default:"false" yaml:"enabled"`
	Endpoint    string  `env:"OBS_TRACING_ENDPOINT"     yaml:"endpoint"`
	Insecure    bool    `env:"OBS_TRACING_INSECURE"     env-default:"true" yaml:"insecure"`
	SampleRatio float64 `env:"OBS_TRACING_SAMPLE_RATIO" env-default:"1" yaml:"sample_ratio"`
}

type payment struct {
	Yookassa   yookassa          `yaml:"yookassa"`
	OpenRouter openrouterPayment `yaml:"openrouter"`
}

type yookassa struct {
	StoreID   string `env:"YOOKASSA_STORE_ID"   yaml:"store_id"`
	SecretKey string `env:"YOOKASSA_SECRET_KEY" yaml:"secret_key"`
}

type openrouterPayment struct {
	WebhookSecret string `env:"OPENROUTER_WEBHOOK_SECRET" yaml:"webhook_secret"`
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

	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
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

func (c *Config) applyDefaults(configPath string) {
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
