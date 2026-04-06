package configurer

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"containermanager/internal/entities"
	"containermanager/internal/service/commands"
)

type chownCall struct {
	path string
	uid  int
	gid  int
}

func TestClawConfigurerConfigureAppliesOwnership(t *testing.T) {
	basePath := t.TempDir()
	cfg := NewClawConfigurer(basePath, "", 1000, 1000)

	calls := stubChown(t)

	gotPath, err := cfg.Configure(commands.CreateClaw{
		UserID: "user-1",
		ClawID: "claw-1",
		Config: []entities.ClawConfig{
			{
				Name:     "nested",
				FileType: entities.ClawConfigTypeDir,
				ClawConfig: []entities.ClawConfig{
					{
						Name:     "openclaw",
						FileType: entities.ClawConfigTypeJson,
						Data:     []byte(`{"ok":true}`),
					},
				},
			},
			{
				Name:     "README",
				FileType: entities.ClawConfigTypeMd,
				Data:     []byte("hello"),
			},
		},
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	wantPath := filepath.Join(basePath, "user-1", "claw-1")
	if gotPath != wantPath {
		t.Fatalf("Configure() path = %q, want %q", gotPath, wantPath)
	}

	assertPathsChowned(t, *calls, 1000, 1000, wantPath, []string{
		".",
		"nested",
		"nested/openclaw.json",
		"README.md",
	})
}

func TestClawConfigurerUpdateAppliesOwnership(t *testing.T) {
	basePath := t.TempDir()
	cfg := NewClawConfigurer(basePath, "", 1001, 1002)

	calls := stubChown(t)

	if _, err := cfg.Update(commands.UpdateClaw{
		UserID: "user-2",
		ClawID: "claw-2",
		Config: []entities.ClawConfig{
			{
				Name:     "config",
				FileType: entities.ClawConfigTypeJson,
				Data:     []byte(`{"version":2}`),
			},
		},
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	wantPath := filepath.Join(basePath, "user-2", "claw-2")
	assertPathsChowned(t, *calls, 1001, 1002, wantPath, []string{
		".",
		"config.json",
	})
}

func TestClawConfigurerRestoreClawConfigAppliesOwnership(t *testing.T) {
	basePath := t.TempDir()
	cfg := NewClawConfigurer(basePath, "", 2000, 2001)

	calls := stubChown(t)

	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)

	headers := []struct {
		name string
		mode int64
		body []byte
		typ  byte
	}{
		{name: "claw-3/", mode: 0o755, typ: tar.TypeDir},
		{name: "claw-3/.openclaw/", mode: 0o755, typ: tar.TypeDir},
		{name: "claw-3/.openclaw/state.json", mode: 0o644, typ: tar.TypeReg, body: []byte(`{"offset":1}`)},
	}

	for _, header := range headers {
		if err := tw.WriteHeader(&tar.Header{
			Name:     header.name,
			Mode:     header.mode,
			Size:     int64(len(header.body)),
			Typeflag: header.typ,
		}); err != nil {
			t.Fatalf("WriteHeader(%q) error = %v", header.name, err)
		}

		if len(header.body) > 0 {
			if _, err := tw.Write(header.body); err != nil {
				t.Fatalf("Write(%q) error = %v", header.name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("Close archive error = %v", err)
	}

	if err := cfg.RestoreClawConfig("user-3", "claw-3", &archive); err != nil {
		t.Fatalf("RestoreClawConfig() error = %v", err)
	}

	wantPath := filepath.Join(basePath, "user-3", "claw-3")
	assertPathsChowned(t, *calls, 2000, 2001, wantPath, []string{
		".",
		".openclaw",
		".openclaw/state.json",
	})
}

func TestClawConfigurerConfigureSkipsOwnershipWithoutOwnerConfig(t *testing.T) {
	basePath := t.TempDir()
	cfg := NewClawConfigurer(basePath, "", -1, -1)

	calls := stubChown(t)

	if _, err := cfg.Configure(commands.CreateClaw{
		UserID: "user-4",
		ClawID: "claw-4",
		Config: []entities.ClawConfig{
			{
				Name:     "config",
				FileType: entities.ClawConfigTypeJson,
				Data:     []byte(`{"enabled":true}`),
			},
		},
	}); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	if len(*calls) != 0 {
		t.Fatalf("Configure() chown calls = %d, want 0", len(*calls))
	}
}

func stubChown(t *testing.T) *[]chownCall {
	t.Helper()

	calls := make([]chownCall, 0)
	previous := chownPath

	chownPath = func(path string, uid, gid int) error {
		calls = append(calls, chownCall{path: filepath.Clean(path), uid: uid, gid: gid})
		return nil
	}

	t.Cleanup(func() {
		chownPath = previous
	})

	return &calls
}

func assertPathsChowned(
	t *testing.T,
	calls []chownCall,
	uid int,
	gid int,
	basePath string,
	relativePaths []string,
) {
	t.Helper()

	if len(calls) != len(relativePaths) {
		t.Fatalf("chown calls = %d, want %d", len(calls), len(relativePaths))
	}

	got := make([]string, 0, len(calls))
	for _, call := range calls {
		if call.uid != uid || call.gid != gid {
			t.Fatalf("chown(%q) uid/gid = %d:%d, want %d:%d", call.path, call.uid, call.gid, uid, gid)
		}

		got = append(got, call.path)
	}

	want := make([]string, 0, len(relativePaths))
	for _, rel := range relativePaths {
		if rel == "." {
			want = append(want, filepath.Clean(basePath))
			continue
		}

		want = append(want, filepath.Join(basePath, filepath.FromSlash(rel)))
	}

	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Fatalf("chown paths = %v, want %v", got, want)
	}

	for _, rel := range relativePaths[1:] {
		fullPath := filepath.Join(basePath, filepath.FromSlash(rel))
		if _, err := os.Stat(fullPath); err != nil {
			t.Fatalf("Stat(%q) error = %v", fullPath, err)
		}
	}
}
