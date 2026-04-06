package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"containermanager/internal/infrastucture/pkg/configurer"
)

func TestContainerApprove_InvalidCode(t *testing.T) {
	t.Parallel()

	const (
		userID = "user-1"
		clawID = "claw-1"
	)

	basePath := t.TempDir()

	credentialsDir := filepath.Join(basePath, userID, clawID, ".openclaw", "credentials")
	if err := os.MkdirAll(credentialsDir, 0o755); err != nil {
		t.Fatalf("mkdir credentials dir: %v", err)
	}

	pendingPairing := configurer.PairingTelegramConfig{
		Version: 1,
		Requests: []configurer.Request{
			{
				ID:        "100500",
				Code:      "123456",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
		},
	}

	fd, err := os.Create(filepath.Join(credentialsDir, "telegram-pairing.json"))
	if err != nil {
		t.Fatalf("create pairing file: %v", err)
	}

	if err := json.NewEncoder(fd).Encode(pendingPairing); err != nil {
		_ = fd.Close()

		t.Fatalf("encode pairing file: %v", err)
	}

	if err := fd.Close(); err != nil {
		t.Fatalf("close pairing file: %v", err)
	}

	svc := &Container{
		cfg: configurer.NewClawConfigurer(basePath, "", -1, -1),
	}

	err = svc.Approve(clawID, userID, "wrong-code")
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected ErrInvalidCode, got: %v", err)
	}
}
