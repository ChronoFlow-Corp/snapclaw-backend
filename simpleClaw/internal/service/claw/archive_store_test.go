package claw

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestFileArchiveStore_SaveAndOpen(t *testing.T) {
	root := t.TempDir()
	store := newFileArchiveStore(root)
	userID := uuid.New()
	clawID := uuid.New()
	want := []byte("archive-payload")

	if err := store.Save(context.Background(), userID, clawID, bytes.NewReader(want)); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	rc, err := store.Open(userID, clawID)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("archive bytes = %q, want %q", got, want)
	}

	expectedPath := filepath.Join(root, userID.String(), clawID.String()+".tar")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected saved archive at %s: %v", expectedPath, err)
	}
}

func TestFileArchiveStore_Exists(t *testing.T) {
	root := t.TempDir()
	store := newFileArchiveStore(root)
	userID := uuid.New()
	clawID := uuid.New()

	exists, err := store.Exists(userID, clawID)
	if err != nil {
		t.Fatalf("Exists() before save error = %v", err)
	}
	if exists {
		t.Fatal("expected archive to be missing before save")
	}

	if err := store.Save(context.Background(), userID, clawID, bytes.NewReader([]byte("ok"))); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	exists, err = store.Exists(userID, clawID)
	if err != nil {
		t.Fatalf("Exists() after save error = %v", err)
	}
	if !exists {
		t.Fatal("expected archive to exist after save")
	}
}

func TestFileArchiveStore_SaveIsAtomic(t *testing.T) {
	root := t.TempDir()
	store := newFileArchiveStore(root)
	userID := uuid.New()
	clawID := uuid.New()

	err := store.Save(context.Background(), userID, clawID, errReader{
		err:   errors.New("copy failed"),
		chunk: []byte("partial"),
	})
	if err == nil {
		t.Fatal("expected Save() to fail")
	}

	finalPath := filepath.Join(root, userID.String(), clawID.String()+".tar")
	if _, err := os.Stat(finalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no final archive file, stat err = %v", err)
	}

	tempPattern := filepath.Join(root, userID.String(), clawID.String()+".*.tmp")
	matches, globErr := filepath.Glob(tempPattern)
	if globErr != nil {
		t.Fatalf("Glob() error = %v", globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("expected temp files to be cleaned up, found %v", matches)
	}
}

type errReader struct {
	read  bool
	err   error
	chunk []byte
}

func (r errReader) Read(p []byte) (int, error) {
	if !r.read {
		n := copy(p, r.chunk)
		r.read = true
		return n, r.err
	}

	return 0, io.EOF
}
