package claws

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql/models"
)

func TestStorageCreate_AllowsNilServerID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()
	claw := entities.Claw{
		ID:        uuid.New(),
		Name:      "draft claw",
		UserID:    user.ID,
		Status:    entities.StatusStop,
		Config:    entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.Create(context.Background(), claw, nil); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var rawServerID sql.NullString
	if err := db.Raw("SELECT server_id FROM claws WHERE id = ?", claw.ID).Scan(&rawServerID).Error; err != nil {
		t.Fatalf("query server_id error = %v", err)
	}

	if rawServerID.Valid {
		t.Fatalf("server_id stored = %q, want NULL", rawServerID.String)
	}

	got, err := store.GetByID(context.Background(), claw.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.ServerID != uuid.Nil {
		t.Fatalf("GetByID().ServerID = %s, want uuid.Nil", got.ServerID)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_foreign_keys=on", uuid.NewString())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	schema := []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			name TEXT,
			nick_name TEXT,
			avatar_url TEXT,
			email TEXT UNIQUE,
			role TEXT,
			open_router_key_id TEXT,
			open_router_api_key TEXT,
			balance_minor INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE servers (
			id TEXT PRIMARY KEY,
			name TEXT,
			ip TEXT,
			url TEXT,
			proxy_url TEXT,
			status TEXT,
			secret_key TEXT,
			max_claws INTEGER DEFAULT 0,
			created_at DATETIME
		)`,
		`CREATE TABLE claws (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			config JSON,
			user_id TEXT,
			server_id TEXT NULL,
			status TEXT NOT NULL,
			container_id TEXT,
			vars TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			CONSTRAINT fk_users_claws FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			CONSTRAINT fk_servers_claws FOREIGN KEY (server_id) REFERENCES servers(id)
		)`,
	}

	for _, stmt := range schema {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("schema setup error = %v", err)
		}
	}

	return db
}

func seedUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()

	user := models.User{
		ID:        uuid.New(),
		Name:      "Test User",
		NickName:  "tester",
		AvatarURL: "https://example.com/avatar.png",
		Email:     uuid.NewString() + "@example.com",
		Role:      entities.UserRole,
	}

	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("db.Create(user) error = %v", err)
	}

	return user
}
