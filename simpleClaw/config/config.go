package config

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

type Config struct {
	Environment   string        `yaml:"environment"   env:"ENVIRONMENT" env-default:"development"`
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
	Addr    string   `env-default:":8080" yaml:"addr"`
	Origins []string `env-default:"*"     yaml:"origins"`
}

type database struct {
	Dsn string `env-required:"true" yaml:"dsn"`
}

type auth struct {
	Google google   `yaml:"google"`
	Jwt    Jwt      `yaml:"jwt"`
	Admins []string `yaml:"admins" env:"AUTH_ADMINS" env-separator:","`
}

type google struct {
	ClientID     string `env-required:"true" yaml:"client_id"`
	ClientSecret string `env-required:"true" yaml:"client_secret"`
	CallbackURL  string `env-required:"true" yaml:"callback_url"`
	FrontendURL  string `env-required:"true" yaml:"frontend_url"`
}

type Jwt struct {
	RefreshSecret       string        `env-required:"true" yaml:"refresh_secret"`
	AccessSecretPublic  string        `env-required:"true" yaml:"access_secret_public"`
	AccessSecretPrivate string        `env-required:"true" yaml:"access_secret_private"`
	AccessExpire        time.Duration `env-required:"true" yaml:"access_expire"         env-default:"24h"`
	RefreshExpire       time.Duration `env-required:"true" yaml:"refresh_expire"        env-default:"148h"`
}

type openrouter struct {
	BaseURL  string        `env-required:"true" yaml:"base_url"`
	APIToken string        `env-required:"true" yaml:"api_token"`
	Timeout  time.Duration `                    yaml:"timeout"   env-default:"15s"`
}

type hosting struct {
	ContainerManager containerManager `yaml:"container_manager"`
}

type containerManager struct {
	Timeout    time.Duration `env-default:"15s"                             yaml:"timeout"`
	BackupPath string        `env-default:"/tmp/simpleclaw/config-archives" yaml:"backup_path" env:"CONTAINER_MANAGER_BACKUP_PATH"`
}

type connect struct {
	Gmail gmail `yaml:"gmail"`
}

type proxy struct {
	Token          string        `env:"PROXY_TOKEN"           yaml:"token"`
	ForwardTimeout time.Duration `env:"PROXY_FORWARD_TIMEOUT" yaml:"forward_timeout" env-default:"5s"`
	RetryCount     int           `env:"PROXY_RETRY_COUNT"     yaml:"retry_count"     env-default:"2"`
	RetryBackoff   time.Duration `env:"PROXY_RETRY_BACKOFF"   yaml:"retry_backoff"   env-default:"250ms"`
}

type gmail struct {
	ClientID     string `env-required:"true" yaml:"client_id"`
	ClientSecret string `env-required:"true" yaml:"client_secret"`
	CallbackURL  string `env-required:"true" yaml:"callback_url"`
	Watch        watch  `                    yaml:"watch"`
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
	Enabled bool   `env:"OBS_METRICS_ENABLED" env-default:"true"     yaml:"enabled"`
	Path    string `env:"OBS_METRICS_PATH"    env-default:"/metrics" yaml:"path"`
}

type tracing struct {
	Enabled     bool    `env:"OBS_TRACING_ENABLED"      env-default:"false" yaml:"enabled"`
	Endpoint    string  `env:"OBS_TRACING_ENDPOINT"                         yaml:"endpoint"`
	Insecure    bool    `env:"OBS_TRACING_INSECURE"     env-default:"true"  yaml:"insecure"`
	SampleRatio float64 `env:"OBS_TRACING_SAMPLE_RATIO" env-default:"1"     yaml:"sample_ratio"`
}

type payment struct {
	Yookassa   yookassa          `yaml:"yookassa"`
	OpenRouter openrouterPayment `yaml:"openrouter"`
}

type yookassa struct {
	StoreID   string `env-required:"true" yaml:"store_id"`
	SecretKey string `env-required:"true" yaml:"secret_key"`
}

type openrouterPayment struct {
	WebhookSecret string `env:"OPENROUTER_WEBHOOK_SECRET" yaml:"webhook_secret"`
}

func New() Config {
	p := os.Getenv("CONFIG_PATH")
	if p == "" {
		panic("CONFIG_PATH environment variable not set")
	}

	cfg := Config{}

	err := cleanenv.ReadConfig(p, &cfg)
	if err != nil {
		panic("failed to read config: " + err.Error())
	}

	cfg.mustJwtLoad()
	cfg.normalize()

	if cfg.Environment != EnvDevelopment && cfg.Environment != EnvProduction {
		panic("invalid environment: " + cfg.Environment)
	}

	return cfg
}

func (c *Config) mustJwtLoad() {
	pbFd, err := os.Open(c.Auth.Jwt.AccessSecretPublic)
	if err != nil {
		panic(fmt.Sprintf("failed to open access secret public file: %s", err))
	}
	defer pbFd.Close()

	pb, err := io.ReadAll(pbFd)
	if err != nil {
		panic(fmt.Sprintf("failed to read access secret public file: %s", err))
	}

	c.Auth.Jwt.AccessSecretPublic = string(pb)

	prFd, err := os.Open(c.Auth.Jwt.AccessSecretPrivate)
	if err != nil {
		panic(fmt.Sprintf("failed to open access secret private file: %s", err))
	}
	defer prFd.Close()

	pr, err := io.ReadAll(prFd)
	if err != nil {
		panic(fmt.Sprintf("failed to read access secret private file: %s", err))
	}

	c.Auth.Jwt.AccessSecretPrivate = string(pr)
}

func (c *Config) normalize() {
	c.Auth.Admins = normalizeEmails(c.Auth.Admins)
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
