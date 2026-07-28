package hosting

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"simpleClaw/internal/entities"
)

func TestRewriteConfigArchive_ReplacesOpenClawConfigAndRemovesBackups(t *testing.T) {
	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Meta = &entities.ConfigMeta{LastTouchedVersion: "rewritten"}

	var src bytes.Buffer
	writeArchiveFixture(t, &src, "claw-1", map[string]string{
		"openclaw.json":      `{"meta":{"version":"stale"}}`,
		"openclaw.json.bak":  "bak-0",
		"openclaw.json.bak1": "bak-1",
		"session/auth.json":  `{"token":"keep-me"}`,
	})

	var dst bytes.Buffer
	if err := RewriteConfigArchive(&src, "claw-1", cfg, &dst); err != nil {
		t.Fatalf("RewriteConfigArchive() error = %v", err)
	}

	files := readArchiveFixture(t, &dst)

	if _, ok := files["claw-1/openclaw.json.bak"]; ok {
		t.Fatal("expected openclaw.json.bak to be removed")
	}
	if _, ok := files["claw-1/openclaw.json.bak1"]; ok {
		t.Fatal("expected openclaw.json.bak1 to be removed")
	}

	if got := string(files["claw-1/session/auth.json"]); got != `{"token":"keep-me"}` {
		t.Fatalf("session/auth.json = %q, want unchanged payload", got)
	}

	assertArchiveConfigEquals(t, files["claw-1/openclaw.json"], cfg)
}

func TestRewriteConfigArchive_AddsOpenClawConfigWhenMissing(t *testing.T) {
	cfg := entities.NewDefaultClawConfig("openrouter/anthropic/claude-3.5-sonnet")

	var src bytes.Buffer
	writeArchiveFixture(t, &src, "claw-2", map[string]string{
		"session/state.json": `{"ok":true}`,
	})

	var dst bytes.Buffer
	if err := RewriteConfigArchive(&src, "claw-2", cfg, &dst); err != nil {
		t.Fatalf("RewriteConfigArchive() error = %v", err)
	}

	files := readArchiveFixture(t, &dst)
	assertArchiveConfigEquals(t, files["claw-2/openclaw.json"], cfg)

	if got := string(files["claw-2/session/state.json"]); got != `{"ok":true}` {
		t.Fatalf("session/state.json = %q, want unchanged payload", got)
	}
}

func TestRewriteConfigArchive_RejectsEmptyClawID(t *testing.T) {
	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")

	err := RewriteConfigArchive(bytes.NewReader(nil), "", cfg, io.Discard)
	if err == nil {
		t.Fatal("expected error for empty claw id")
	}
}

func TestRewriteConfigArchive_RejectsNilReaderOrWriter(t *testing.T) {
	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")

	if err := RewriteConfigArchive(nil, "claw-3", cfg, io.Discard); err == nil {
		t.Fatal("expected error for nil reader")
	}

	if err := RewriteConfigArchive(bytes.NewReader(nil), "claw-3", cfg, nil); err == nil {
		t.Fatal("expected error for nil writer")
	}
}

func writeArchiveFixture(t *testing.T, w io.Writer, clawID string, files map[string]string) {
	t.Helper()

	tw := tar.NewWriter(w)
	defer func() {
		if err := tw.Close(); err != nil {
			t.Fatalf("close tar writer: %v", err)
		}
	}()

	if err := tw.WriteHeader(&tar.Header{
		Name:     clawID + "/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}); err != nil {
		t.Fatalf("write root dir header: %v", err)
	}

	seenDirs := map[string]bool{clawID: true}
	for relPath, contents := range files {
		fullPath := filepath.ToSlash(filepath.Join(clawID, relPath))
		dir := filepath.Dir(fullPath)
		if dir != "." && dir != clawID && !seenDirs[dir] {
			if err := writeArchiveDir(tw, dir); err != nil {
				t.Fatalf("write dir %q: %v", dir, err)
			}
			seenDirs[dir] = true
		}

		if err := tw.WriteHeader(&tar.Header{
			Name:     fullPath,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(contents)),
		}); err != nil {
			t.Fatalf("write file header %q: %v", fullPath, err)
		}

		if _, err := tw.Write([]byte(contents)); err != nil {
			t.Fatalf("write file %q: %v", fullPath, err)
		}
	}
}

func writeArchiveDir(tw *tar.Writer, path string) error {
	return tw.WriteHeader(&tar.Header{
		Name:     filepath.ToSlash(path) + "/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	})
}

func readArchiveFixture(t *testing.T, r io.Reader) map[string][]byte {
	t.Helper()

	files := make(map[string][]byte)
	tr := tar.NewReader(r)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files
		}
		if err != nil {
			t.Fatalf("next tar header: %v", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %q: %v", hdr.Name, err)
		}
		files[hdr.Name] = data
	}
}

func assertArchiveConfigEquals(t *testing.T, got []byte, want entities.ClawConfig) {
	t.Helper()

	if len(got) == 0 {
		t.Fatal("expected openclaw.json in archive")
	}

	var gotCfg entities.ClawConfig
	if err := json.Unmarshal(got, &gotCfg); err != nil {
		t.Fatalf("unmarshal archive config: %v", err)
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want config: %v", err)
	}

	gotJSON, err := json.Marshal(gotCfg)
	if err != nil {
		t.Fatalf("marshal got config: %v", err)
	}

	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("openclaw.json = %s, want %s", gotJSON, wantJSON)
	}
}

func TestWriteConfigArchive_WritesOpenClawJSON(t *testing.T) {
	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")

	var archive bytes.Buffer
	if err := WriteConfigArchive(cfg, "claw-4", &archive); err != nil {
		t.Fatalf("WriteConfigArchive() error = %v", err)
	}

	files := readArchiveFixture(t, bytes.NewReader(archive.Bytes()))
	assertArchiveConfigEquals(t, files["claw-4/openclaw.json"], cfg)
}
