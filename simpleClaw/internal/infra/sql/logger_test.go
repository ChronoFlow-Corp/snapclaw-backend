package sql

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNewGormLoggerSuppressesRecordNotFoundLogs(t *testing.T) {
	var logBuf bytes.Buffer

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: NewGormLogger(&logBuf),
	})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	type user struct {
		ID   int
		Name string
	}

	if err := db.AutoMigrate(&user{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	err = db.Where("id = ?", 1).First(&user{}).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("First() error = %v, want ErrRecordNotFound", err)
	}

	if strings.TrimSpace(logBuf.String()) != "" {
		t.Fatalf("expected no record-not-found logs, got %q", logBuf.String())
	}
}
