package config

import (
	"fmt"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Postgres   Postgres   `yaml:"postgres"`
	Http       Http       `yaml:"http"`
	Image      Image      `yaml:"image"`
	Migrations Migrations `yaml:"migrations"`
}

type Http struct {
	Addr         string `env:"HTTP_ADDR"          env-default:"localhost:8080" yaml:"addr"`
	ReadTimeout  int    `env:"HTTP_READ_TIMEOUT"  env-default:"5"              yaml:"read_timeout"`
	WriteTimeout int    `env:"HTTP_WRITE_TIMEOUT" env-default:"10"             yaml:"write_timeout"`
}

type Postgres struct {
	URL string `env:"POSTGRES_URL" env-required:"true" yaml:"url"`
}

type Image struct {
	BuildCtx []string `yaml:"build_context"`
	BasePath string   `yaml:"base_path"`
}

type Migrations struct {
	Auto bool   `env:"MIGRATIONS_AUTO" env-default:"true" yaml:"auto"`
	Path string `env:"MIGRATIONS_PATH" env-default:"migrations" yaml:"path"`
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

	return cfg
}
