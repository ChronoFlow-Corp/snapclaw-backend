package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMustLoadConfigAppliesRuntimeWatcherDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := os.WriteFile(path, []byte("postgres:\n  url: postgres://localhost/test\n"), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("CONFIG_PATH", path)

	cfg := MustLoadConfig()

	if !cfg.RuntimeWatcher.Enabled {
		t.Fatal("expected runtime watcher to be enabled by default")
	}

	if cfg.RuntimeWatcher.Interval != 15*time.Second {
		t.Fatalf("interval = %s, want %s", cfg.RuntimeWatcher.Interval, 15*time.Second)
	}

	if cfg.RuntimeWatcher.InspectTimeout != 3*time.Second {
		t.Fatalf("inspect timeout = %s, want %s", cfg.RuntimeWatcher.InspectTimeout, 3*time.Second)
	}
}
