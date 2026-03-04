package config

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

type Config struct {
	Environment string   `yaml:"environment" env:"ENVIRONMENT" env-default:"development"`
	Http        http     `yaml:"http"`
	Database    database `yaml:"database"`
	Auth        auth     `yaml:"auth"`
}

type http struct {
	Addr string `yaml:"addr" env-default:":8080"`
}

type database struct {
	Dsn string `yaml:"dsn" env-required:"true"`
}

type auth struct {
	Google google `yaml:"google"`
	Jwt    Jwt    `yaml:"jwt"`
}

type google struct {
	ClientID     string `yaml:"client_id"     env-required:"true"`
	ClientSecret string `yaml:"client_secret" env-required:"true"`
	CallbackURL  string `yaml:"callback_url"  env-required:"true"`
}

type Jwt struct {
	RefreshSecret       string        `yaml:"refresh_secret"        env-required:"true"`
	AccessSecretPublic  string        `yaml:"access_secret_public"  env-required:"true"`
	AccessSecretPrivate string        `yaml:"access_secret_private" env-required:"true"`
	AccessExpire        time.Duration `yaml:"access_expire"         env-required:"true" env-default:"24h"`
	RefreshExpire       time.Duration `yaml:"refresh_expire"        env-required:"true" env-default:"148h"`
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
