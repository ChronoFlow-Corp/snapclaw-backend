package integrations

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"simpleClaw/internal/entities"
	infraSQL "simpleClaw/internal/infra/sql"
)

func TestStorageUpsertListAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	owner := seedUser(t, db)
	otherUser := seedUser(t, db)

	integration := entities.AccountIntegration{
		ID:                uuid.New(),
		UserID:            owner.ID,
		CapabilityID:      entities.CapabilityGmail,
		Provider:          "gmail",
		ExternalAccountID: "me@example.com",
		DisplayName:       "Primary Gmail",
		Status:            entities.AccountIntegrationStatusActive,
		SecretPayload: map[string]any{
			"refresh_token": "secret-refresh-token",
		},
		Metadata: map[string]any{
			"email": "me@example.com",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := store.Upsert(context.Background(), integration); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), integration.ID, owner.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.DisplayName != integration.DisplayName {
		t.Fatalf("DisplayName = %q, want %q", got.DisplayName, integration.DisplayName)
	}

	list, err := store.ListByUserID(context.Background(), owner.ID)
	if err != nil {
		t.Fatalf("ListByUserID() error = %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("ListByUserID() len = %d, want 1", len(list))
	}

	if _, err := store.GetByID(context.Background(), integration.ID, otherUser.ID); !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("GetByID() wrong owner error = %v, want ErrNotFound", err)
	}
}

func TestStorageUpsertUpdatesExistingIntegration(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	owner := seedUser(t, db)

	integration := entities.AccountIntegration{
		ID:                uuid.New(),
		UserID:            owner.ID,
		CapabilityID:      entities.CapabilityGoogleCalendar,
		Provider:          "google_calendar",
		ExternalAccountID: "calendar@example.com",
		DisplayName:       "Calendar One",
		Status:            entities.AccountIntegrationStatusActive,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	if err := store.Upsert(context.Background(), integration); err != nil {
		t.Fatalf("Upsert() create error = %v", err)
	}

	integration.DisplayName = "Calendar Updated"
	integration.Metadata = map[string]any{"timezone": "Europe/Moscow"}
	integration.UpdatedAt = time.Now().UTC().Add(time.Minute)

	if err := store.Upsert(context.Background(), integration); err != nil {
		t.Fatalf("Upsert() update error = %v", err)
	}

	got, err := store.GetByID(context.Background(), integration.ID, owner.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.DisplayName != "Calendar Updated" {
		t.Fatalf("DisplayName = %q, want %q", got.DisplayName, "Calendar Updated")
	}

	if got.Metadata["timezone"] != "Europe/Moscow" {
		t.Fatalf("timezone = %#v, want %q", got.Metadata["timezone"], "Europe/Moscow")
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	statements := []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			name TEXT,
			nick_name TEXT,
			avatar_url TEXT,
			email TEXT,
			role TEXT,
			open_router_api_key TEXT,
			open_router_key_id TEXT,
			balance_minor INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE account_integrations (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			capability_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			external_account_id TEXT,
			display_name TEXT,
			status TEXT NOT NULL,
			secret_payload JSON,
			metadata JSON,
			created_at DATETIME,
			updated_at DATETIME
		)`,
	}

	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table error = %v", err)
		}
	}

	return db
}

func seedUser(t *testing.T, db *gorm.DB) entities.User {
	t.Helper()

	user := entities.NewUser("Test User", "tester", "", uuid.NewString()+"@example.com", entities.UserRole)
	if err := db.Exec(
		`INSERT INTO users (id, name, nick_name, avatar_url, email, role, open_router_api_key, open_router_key_id, balance_minor, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID.String(),
		user.Name,
		user.Nickname,
		user.AvatarURL,
		user.Email,
		user.Role,
		user.OpenRouterApiKey,
		user.OpenRouterKeyID,
		user.BalanceMinor,
		user.CreatedAt,
		user.CreatedAt,
	).Error; err != nil {
		t.Fatalf("seed user error = %v", err)
	}

	return user
}
