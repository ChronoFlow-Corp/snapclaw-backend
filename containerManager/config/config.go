package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Environment   string        `env:"ENVIRONMENT" env-default:"development" yaml:"environment"`
	Postgres      Postgres      `                                            yaml:"postgres"`
	Http          Http          `                                            yaml:"http"`
	Image         Image         `                                            yaml:"image"`
	MaxClaws      MaxClaws      `env:"MAX_CLAWS"   env-default:"auto"        yaml:"max_claws"`
	Gog           gog           `                                            yaml:"gog"`
	PubSub        pubSub        `                                            yaml:"pubsub"`
	Migrations    Migrations    `                                            yaml:"migrations"`
	Observability observability `                                            yaml:"observability"`
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
	BasePath        string   `yaml:"base_path"        env:"CLAW_CONFIGS"`
	OwnerUID        int      `yaml:"owner_uid"        env:"CLAW_CONFIGS_OWNER_UID" env-default:"-1"`
	OwnerGID        int      `yaml:"owner_gid"        env:"CLAW_CONFIGS_OWNER_GID" env-default:"-1"`
	Dockerfile      string   `yaml:"dockerfile"`
	CredentialsPath string   `yaml:"credentials_path" env:"GOG_CREDENTIALS_PATH"`
}

type gog struct {
	KeyringBackend  string `env:"GOG_KEYRING_BACKEND"  env-default:"file" yaml:"keyring_backend"`
	KeyringPassword string `env:"GOG_KEYRING_PASSWORD"                    yaml:"keyring_password"`
}

type pubSub struct {
	ForwardTimeout time.Duration `env:"PUBSUB_FORWARD_TIMEOUT" env-default:"5s"  yaml:"forward_timeout"`
	Workers        int           `env:"PUBSUB_WORKERS"         env-default:"32"  yaml:"workers"`
	DedupTTL       time.Duration `env:"PUBSUB_DEDUP_TTL"       env-default:"10m" yaml:"dedup_ttl"`
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
	Enabled bool   `env:"OBS_METRICS_ENABLED" env-default:"true"     yaml:"enabled"`
	Path    string `env:"OBS_METRICS_PATH"    env-default:"/metrics" yaml:"path"`
}

type tracing struct {
	Enabled     bool    `env:"OBS_TRACING_ENABLED"      env-default:"false" yaml:"enabled"`
	Endpoint    string  `env:"OBS_TRACING_ENDPOINT"                         yaml:"endpoint"`
	Insecure    bool    `env:"OBS_TRACING_INSECURE"     env-default:"true"  yaml:"insecure"`
	SampleRatio float64 `env:"OBS_TRACING_SAMPLE_RATIO" env-default:"1"     yaml:"sample_ratio"`
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

	if cfg.MaxClaws.Value == 0 && cfg.MaxClaws.Auto == false {
		panic("max_claws must be greater than zero")
	}

	return cfg
}

func (c *Config) normalize() {
	c.Observability.Metrics.Path = normalizeMetricsPath(c.Observability.Metrics.Path)

	if (c.Image.OwnerUID < 0) != (c.Image.OwnerGID < 0) {
		panic("image.owner_uid and image.owner_gid must be configured together")
	}

	if c.Image.OwnerUID < -1 || c.Image.OwnerGID < -1 {
		panic("image.owner_uid and image.owner_gid must be greater than or equal to -1")
	}
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
