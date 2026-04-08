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

	if cfg.Connect.Gmail.Watch.Topic != "" {
		t.Fatalf("gmail watch topic = %q, want empty", cfg.Connect.Gmail.Watch.Topic)
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

	if cfg.Connect.Gmail.Watch.Topic != "" {
		t.Fatalf("gmail watch topic = %q, want empty", cfg.Connect.Gmail.Watch.Topic)
	}
}

func TestLoadConfigUsesBraveAPIKeyFromEnv(t *testing.T) {
	t.Setenv("BRAVE_API_KEY", "brave-api-key-1")

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

	if cfg.Brave.APIKey != "brave-api-key-1" {
		t.Fatalf("brave api key = %q, want %q", cfg.Brave.APIKey, "brave-api-key-1")
	}
}

func TestLoadProductionConfigKeepsFileOpenRouterTokenWhenEnvIsEmpty(t *testing.T) {
	t.Setenv("OPENROUTER_API_TOKEN", "")

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
    callback_url: "https://simpleclaw.example/auth/connect/google/callback"
    frontend_url: "https://simpleclaw.example"
  jwt:
    refresh_secret: "refresh-secret"
    access_secret_private: "`+filepath.ToSlash(filepath.Join(dir, "keys", "jwtRS256.key"))+`"
    access_secret_public: "`+filepath.ToSlash(filepath.Join(dir, "keys", "jwtRS256.key.pub"))+`"
openrouter:
  api_token: "file-openrouter-token"
payment:
  yookassa:
    store_id: "store-id"
    secret_key: "secret-key"
`)

	if err := os.MkdirAll(filepath.Join(dir, "keys"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "keys", "jwtRS256.key"), []byte("test-private"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() private key error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "keys", "jwtRS256.key.pub"), []byte("test-public"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() public key error = %v", err)
	}

	cfg, err := load(configPath)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	if cfg.OpenRouter.APIToken != "file-openrouter-token" {
		t.Fatalf("openrouter api token = %q", cfg.OpenRouter.APIToken)
	}
}

func TestLoadProductionConfigUsesOpenRouterTokenFromEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_TOKEN", "env-openrouter-token")

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
    callback_url: "https://simpleclaw.example/auth/connect/google/callback"
    frontend_url: "https://simpleclaw.example"
  jwt:
    refresh_secret: "refresh-secret"
    access_secret_private: "`+filepath.ToSlash(filepath.Join(dir, "keys", "jwtRS256.key"))+`"
    access_secret_public: "`+filepath.ToSlash(filepath.Join(dir, "keys", "jwtRS256.key.pub"))+`"
openrouter:
  api_token: "file-openrouter-token"
payment:
  yookassa:
    store_id: "store-id"
    secret_key: "secret-key"
`)

	if err := os.MkdirAll(filepath.Join(dir, "keys"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "keys", "jwtRS256.key"), []byte("test-private"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() private key error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "keys", "jwtRS256.key.pub"), []byte("test-public"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() public key error = %v", err)
	}

	cfg, err := load(configPath)
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	if cfg.OpenRouter.APIToken != "env-openrouter-token" {
		t.Fatalf("openrouter api token = %q", cfg.OpenRouter.APIToken)
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

func TestLoadDevelopmentConfigAppliesRuntimeSyncDefaults(t *testing.T) {
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

	if !cfg.Hosting.ContainerManager.RuntimeSync.Enabled {
		t.Fatal("expected runtime sync to be enabled by default")
	}

	if cfg.Hosting.ContainerManager.RuntimeSync.Interval <= 0 {
		t.Fatalf("interval = %s, want positive duration", cfg.Hosting.ContainerManager.RuntimeSync.Interval)
	}

	if cfg.Hosting.ContainerManager.RuntimeSync.Timeout <= 0 {
		t.Fatalf("timeout = %s, want positive duration", cfg.Hosting.ContainerManager.RuntimeSync.Timeout)
	}

	if cfg.Hosting.ContainerManager.RuntimeSync.BatchSize <= 0 {
		t.Fatalf("batch size = %d, want positive number", cfg.Hosting.ContainerManager.RuntimeSync.BatchSize)
	}

	if cfg.Hosting.ContainerManager.RuntimeSync.WorkerCount <= 0 {
		t.Fatalf("worker count = %d, want positive number", cfg.Hosting.ContainerManager.RuntimeSync.WorkerCount)
	}
}

func writeTestConfig(t *testing.T, path, body string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}
