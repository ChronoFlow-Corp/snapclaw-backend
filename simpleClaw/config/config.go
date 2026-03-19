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
	Environment   string        `yaml:"environment" env:"ENVIRONMENT" env-default:"development"`
	Http          http          `yaml:"http"`
	Database      database      `yaml:"database"`
	Auth          auth          `yaml:"auth"`
	OpenRouter    openrouter    `yaml:"openrouter"`
	Hosting       hosting       `yaml:"hosting"`
	Connect       connect       `yaml:"connect"`
	Proxy         proxy         `yaml:"proxy"`
	Observability observability `yaml:"observability"`
}

type http struct {
	Addr    string   `yaml:"addr"    env-default:":8080"`
	Origins []string `yaml:"origins" env-default:"*"`
}

type database struct {
	Dsn string `yaml:"dsn" env-required:"true"`
}

type auth struct {
	Google google   `yaml:"google"`
	Jwt    Jwt      `yaml:"jwt"`
	Admins []string `yaml:"admins" env:"AUTH_ADMINS" env-separator:","`
}

type google struct {
	ClientID     string `yaml:"client_id"     env-required:"true"`
	ClientSecret string `yaml:"client_secret" env-required:"true"`
	CallbackURL  string `yaml:"callback_url"  env-required:"true"`
	FrontendURL  string `yaml:"frontend_url"  env-required:"true"`
}

type Jwt struct {
	RefreshSecret       string        `yaml:"refresh_secret"        env-required:"true"`
	AccessSecretPublic  string        `yaml:"access_secret_public"  env-required:"true"`
	AccessSecretPrivate string        `yaml:"access_secret_private" env-required:"true"`
	AccessExpire        time.Duration `yaml:"access_expire"         env-required:"true" env-default:"24h"`
	RefreshExpire       time.Duration `yaml:"refresh_expire"        env-required:"true" env-default:"148h"`
}

type openrouter struct {
	BaseURL  string        `yaml:"base_url"  env-required:"true"`
	APIToken string        `yaml:"api_token" env-required:"true"`
	Timeout  time.Duration `yaml:"timeout"                       env-default:"15s"`
}

type hosting struct {
	ContainerManager containerManager `yaml:"container_manager"`
}

type containerManager struct {
	Timeout    time.Duration `yaml:"timeout"     env-default:"15s"`
	BackupPath string        `yaml:"backup_path" env-default:"/tmp/simpleclaw/config-archives" env:"CONTAINER_MANAGER_BACKUP_PATH"`
}

type connect struct {
	Gmail gmail `yaml:"gmail"`
}

type proxy struct {
	Token          string        `yaml:"token" env:"PROXY_TOKEN"`
	ForwardTimeout time.Duration `yaml:"forward_timeout" env:"PROXY_FORWARD_TIMEOUT" env-default:"5s"`
	RetryCount     int           `yaml:"retry_count" env:"PROXY_RETRY_COUNT" env-default:"2"`
	RetryBackoff   time.Duration `yaml:"retry_backoff" env:"PROXY_RETRY_BACKOFF" env-default:"250ms"`
}

type gmail struct {
	ClientID     string `yaml:"client_id"     env-required:"true"`
	ClientSecret string `yaml:"client_secret" env-required:"true"`
	CallbackURL  string `yaml:"callback_url"  env-required:"true"`
	Watch        watch  `yaml:"watch"`
}

type watch struct {
	Topic  string   `yaml:"topic" env:"CONNECT_GMAIL_WATCH_TOPIC"`
	Labels []string `yaml:"labels" env:"CONNECT_GMAIL_WATCH_LABELS" env-separator:"," env-default:"INBOX"`
}

type observability struct {
	Metrics metrics `yaml:"metrics"`
	Tracing tracing `yaml:"tracing"`
}

type metrics struct {
	Enabled bool   `yaml:"enabled" env:"OBS_METRICS_ENABLED" env-default:"true"`
	Path    string `yaml:"path" env:"OBS_METRICS_PATH" env-default:"/metrics"`
}

type tracing struct {
	Enabled     bool    `yaml:"enabled" env:"OBS_TRACING_ENABLED" env-default:"false"`
	Endpoint    string  `yaml:"endpoint" env:"OBS_TRACING_ENDPOINT"`
	Insecure    bool    `yaml:"insecure" env:"OBS_TRACING_INSECURE" env-default:"true"`
	SampleRatio float64 `yaml:"sample_ratio" env:"OBS_TRACING_SAMPLE_RATIO" env-default:"1"`
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
