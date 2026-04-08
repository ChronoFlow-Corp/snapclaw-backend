package claw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type archiveStore interface {
	Save(ctx context.Context, userID, clawID uuid.UUID, src io.Reader) error
	Open(userID, clawID uuid.UUID) (io.ReadCloser, error)
	Exists(userID, clawID uuid.UUID) (bool, error)
	Delete(userID, clawID uuid.UUID) error
}

var ErrArchiveStoreUnavailable = errors.New("archive store is not configured")

type fileArchiveStore struct {
	root string
}

func newFileArchiveStore(root string) *fileArchiveStore {
	return &fileArchiveStore{root: strings.TrimSpace(root)}
}

func (s *fileArchiveStore) Save(ctx context.Context, userID, clawID uuid.UUID, src io.Reader) error {
	const op = "claw.fileArchiveStore.Save"

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if src == nil {
		return fmt.Errorf("%s: source reader is required", op)
	}

	finalPath, err := s.archivePath(userID, clawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(finalPath), clawID.String()+".*.tmp")
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tempPath := tempFile.Name()
	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}

	if _, err := io.Copy(tempFile, src); err != nil {
		cleanup()
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := ctx.Err(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *fileArchiveStore) Open(userID, clawID uuid.UUID) (io.ReadCloser, error) {
	const op = "claw.fileArchiveStore.Open"

	archivePath, err := s.archivePath(userID, clawID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	fd, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}

	return fd, nil
}

func (s *fileArchiveStore) Exists(userID, clawID uuid.UUID) (bool, error) {
	const op = "claw.fileArchiveStore.Exists"

	archivePath, err := s.archivePath(userID, clawID)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	_, err = os.Stat(archivePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}

	return false, fmt.Errorf("%s: %w", op, err)
}

func (s *fileArchiveStore) Delete(userID, clawID uuid.UUID) error {
	const op = "claw.fileArchiveStore.Delete"

	archivePath, err := s.archivePath(userID, clawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *fileArchiveStore) archivePath(userID, clawID uuid.UUID) (string, error) {
	if strings.TrimSpace(s.root) == "" {
		return "", ErrArchiveStoreUnavailable
	}
	if userID == uuid.Nil {
		return "", fmt.Errorf("user id is required")
	}
	if clawID == uuid.Nil {
		return "", fmt.Errorf("claw id is required")
	}

	return filepath.Join(s.root, userID.String(), clawID.String()+".tar"), nil
}
