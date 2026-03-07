package migrations

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func Run(ctx context.Context, dsn string, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("migrations context: %w", err)
	}

	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("migrations path is empty")
	}

	sourceURL, err := buildSourceURL(path)
	if err != nil {
		return fmt.Errorf("build migrations source url: %w", err)
	}

	m, err := migrate.New(sourceURL, dsn)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("migrations context: %w", err)
	}

	return nil
}

func buildSourceURL(path string) (string, error) {
	if strings.Contains(path, "://") {
		return path, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	return "file://" + absPath, nil
}
