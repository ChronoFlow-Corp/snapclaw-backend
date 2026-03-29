package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDevelopmentConfigAppliesDefaultsAndGeneratesJWTKeys(t *testing.T) {
	t.Setenv("DEV_HOST", "")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "dev-config.yaml")

	writeTestConfig(t, configPath, `
environment: development
database:
  dsn: "postgresql://simpleclaw:simpleclaw@simpleclaw-db:5432/simpleclaw?sslmode=disable"
auth:
  google:
    client_id: "google-client-id"
    client_secret: "google-client-secret"
`)

	cfg, err := load(configPath)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	if cfg.Auth.Google.CallbackURL != "http://localhost:1337/auth/connect/google/callback" {
		t.Fatalf("google callback = %q", cfg.Auth.Google.CallbackURL)
	}

	if cfg.Auth.Google.FrontendURL != "http://localhost:3000" {
		t.Fatalf("frontend url = %q", cfg.Auth.Google.FrontendURL)
	}

	if len(cfg.Http.Origins) != 2 {
		t.Fatalf("origins len = %d, want 2", len(cfg.Http.Origins))
	}

	if cfg.Http.Origins[0] != "http://localhost" || cfg.Http.Origins[1] != "http://localhost:3000" {
		t.Fatalf("origins = %#v", cfg.Http.Origins)
	}

	if cfg.Connect.Gmail.ClientID != "google-client-id" {
		t.Fatalf("gmail client id = %q", cfg.Connect.Gmail.ClientID)
	}

	if cfg.Connect.Gmail.ClientSecret != "google-client-secret" {
		t.Fatalf("gmail client secret = %q", cfg.Connect.Gmail.ClientSecret)
	}

	if cfg.Connect.Gmail.CallbackURL != "http://localhost:1337/api/me/connect/gmail/callback" {
		t.Fatalf("gmail callback = %q", cfg.Connect.Gmail.CallbackURL)
	}

	if cfg.Auth.Jwt.RefreshSecret == "" {
		t.Fatal("refresh secret is empty")
	}

	if !strings.Contains(cfg.Auth.Jwt.AccessSecretPrivate, "BEGIN RSA PRIVATE KEY") {
		t.Fatal("private key was not loaded")
	}

	if !strings.Contains(cfg.Auth.Jwt.AccessSecretPublic, "BEGIN PUBLIC KEY") {
		t.Fatal("public key was not loaded")
	}

	privateKeyPath := filepath.Join(dir, "keys", "jwtRS256.key")
	publicKeyPath := filepath.Join(dir, "keys", "jwtRS256.key.pub")

	if _, err := os.Stat(privateKeyPath); err != nil {
		t.Fatalf("private key stat error = %v", err)
	}

	if _, err := os.Stat(publicKeyPath); err != nil {
		t.Fatalf("public key stat error = %v", err)
	}
}

func TestLoadDevelopmentConfigUsesDevHostForDerivedURLs(t *testing.T) {
	t.Setenv("DEV_HOST", "simpleclaw.local")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "dev-config.yaml")

	writeTestConfig(t, configPath, `
environment: development
database:
  dsn: "postgresql://simpleclaw:simpleclaw@simpleclaw-db:5432/simpleclaw?sslmode=disable"
auth:
  google:
    client_id: "google-client-id"
    client_secret: "google-client-secret"
`)

	cfg, err := load(configPath)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	if cfg.Auth.Google.CallbackURL != "http://simpleclaw.local:1337/auth/connect/google/callback" {
		t.Fatalf("google callback = %q", cfg.Auth.Google.CallbackURL)
	}

	if cfg.Auth.Google.FrontendURL != "http://simpleclaw.local:3000" {
		t.Fatalf("frontend url = %q", cfg.Auth.Google.FrontendURL)
	}

	if cfg.Connect.Gmail.CallbackURL != "http://simpleclaw.local:1337/api/me/connect/gmail/callback" {
		t.Fatalf("gmail callback = %q", cfg.Connect.Gmail.CallbackURL)
	}
}

func TestLoadProductionConfigStillRequiresExplicitSecrets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "prod-config.yaml")

	writeTestConfig(t, configPath, `
environment: production
database:
  dsn: "postgresql://simpleclaw:simpleclaw@simpleclaw-db:5432/simpleclaw?sslmode=disable"
auth:
  google:
    client_id: "google-client-id"
    client_secret: "google-client-secret"
`)

	_, err := load(configPath)
	if err == nil {
		t.Fatal("load() error = nil, want non-nil")
	}

	if !strings.Contains(err.Error(), "auth.google.callback_url") {
		t.Fatalf("load() error = %q, want callback_url validation", err.Error())
	}
}

func writeTestConfig(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}
