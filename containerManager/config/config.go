package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Environment   string        `yaml:"environment" env:"ENVIRONMENT" env-default:"development"`
	Postgres      Postgres      `yaml:"postgres"`
	Http          Http          `yaml:"http"`
	Image         Image         `yaml:"image"`
	MaxClaws      MaxClaws      `yaml:"max_claws" env:"MAX_CLAWS" env-default:"auto"`
	Gog           gog           `yaml:"gog"`
	PubSub        pubSub        `yaml:"pubsub"`
	Migrations    Migrations    `yaml:"migrations"`
	Observability observability `yaml:"observability"`
}

type Http struct {
	Addr         string `env:"HTTP_ADDR"          env-default:"localhost:8080" yaml:"addr"`
	ReadTimeout  int    `env:"HTTP_READ_TIMEOUT"  env-default:"5"              yaml:"read_timeout"`
	WriteTimeout int    `env:"HTTP_WRITE_TIMEOUT" env-default:"10"             yaml:"write_timeout"`
	ApiKey       string `env:"API_KEY"                                         yaml:"api_key"`
}

type Postgres struct {
	URL string `env:"POSTGRES_URL" env-required:"true" yaml:"url"`
}

type Image struct {
	BuildCtx        []string `yaml:"build_context"`
	BasePath        string   `yaml:"base_path"`
	Dockerfile      string   `yaml:"dockerfile"`
	CredentialsPath string   `yaml:"credentials_path" env:"GOG_CREDENTIALS_PATH"`
}

type gog struct {
	KeyringBackend  string `yaml:"keyring_backend" env:"GOG_KEYRING_BACKEND" env-default:"file"`
	KeyringPassword string `yaml:"keyring_password" env:"GOG_KEYRING_PASSWORD"`
}

type pubSub struct {
	ForwardTimeout time.Duration `yaml:"forward_timeout" env:"PUBSUB_FORWARD_TIMEOUT" env-default:"5s"`
	Workers        int           `yaml:"workers" env:"PUBSUB_WORKERS" env-default:"32"`
	DedupTTL       time.Duration `yaml:"dedup_ttl" env:"PUBSUB_DEDUP_TTL" env-default:"10m"`
}

type Migrations struct {
	Auto bool   `env:"MIGRATIONS_AUTO" env-default:"true"       yaml:"auto"`
	Path string `env:"MIGRATIONS_PATH" env-default:"migrations" yaml:"path"`
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

func MustLoadConfig() Config {
	const op = "configurer.MustLoadConfig"

	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		panic(fmt.Sprintf("%s: %s", op, "CONFIG_PATH environment variable is not set"))
	}

	cfg := Config{}

	err := cleanenv.ReadConfig(path, &cfg)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", op, err))
	}

	cfg.normalize()

	return cfg
}

func (c *Config) normalize() {
	c.Observability.Metrics.Path = normalizeMetricsPath(c.Observability.Metrics.Path)
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
