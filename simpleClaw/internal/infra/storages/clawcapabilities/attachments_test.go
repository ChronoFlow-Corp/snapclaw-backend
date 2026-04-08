package clawcapabilities

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

func TestStorageUpsertListAndDelete(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	owner := seedUser(t, db)
	clawOne := seedClaw(t, db, owner.ID, "claw-one")
	clawTwo := seedClaw(t, db, owner.ID, "claw-two")
	integration := seedIntegration(t, db, owner.ID, entities.CapabilityGmail, "gmail")

	attachmentOne := entities.ClawCapabilityAttachment{
		ID:                   uuid.New(),
		ClawID:               clawOne.ID,
		UserID:               owner.ID,
		CapabilityID:         entities.CapabilityGmail,
		Provider:             "gmail",
		AccountIntegrationID: &integration.ID,
		Enabled:              true,
		Settings: map[string]any{
			"labelIds": []string{"INBOX"},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	attachmentTwo := entities.ClawCapabilityAttachment{
		ID:                   uuid.New(),
		ClawID:               clawTwo.ID,
		UserID:               owner.ID,
		CapabilityID:         entities.CapabilityGmail,
		Provider:             "gmail",
		AccountIntegrationID: &integration.ID,
		Enabled:              true,
		CreatedAt:            time.Now().UTC(),
		UpdatedAt:            time.Now().UTC(),
	}

	if err := store.Upsert(context.Background(), attachmentOne); err != nil {
		t.Fatalf("Upsert() clawOne error = %v", err)
	}

	if err := store.Upsert(context.Background(), attachmentTwo); err != nil {
		t.Fatalf("Upsert() clawTwo error = %v", err)
	}

	got, err := store.ListByClawID(context.Background(), clawOne.ID, owner.ID)
	if err != nil {
		t.Fatalf("ListByClawID() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("ListByClawID() len = %d, want 1", len(got))
	}

	if got[0].AccountIntegrationID == nil || *got[0].AccountIntegrationID != integration.ID {
		t.Fatalf("AccountIntegrationID = %#v, want %s", got[0].AccountIntegrationID, integration.ID)
	}

	if err := store.Delete(context.Background(), clawOne.ID, owner.ID, entities.CapabilityGmail); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got, err = store.ListByClawID(context.Background(), clawOne.ID, owner.ID)
	if err != nil {
		t.Fatalf("ListByClawID() after delete error = %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("ListByClawID() after delete len = %d, want 0", len(got))
	}

	otherClaw, err := store.ListByClawID(context.Background(), clawTwo.ID, owner.ID)
	if err != nil {
		t.Fatalf("ListByClawID() clawTwo error = %v", err)
	}

	if len(otherClaw) != 1 {
		t.Fatalf("ListByClawID() clawTwo len = %d, want 1", len(otherClaw))
	}
}

func TestStorageScopesAttachmentsByOwner(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	owner := seedUser(t, db)
	otherUser := seedUser(t, db)
	claw := seedClaw(t, db, owner.ID, "claw-one")

	attachment := entities.ClawCapabilityAttachment{
		ID:           uuid.New(),
		ClawID:       claw.ID,
		UserID:       owner.ID,
		CapabilityID: entities.CapabilityMemory,
		Enabled:      true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := store.Upsert(context.Background(), attachment); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	got, err := store.ListByClawID(context.Background(), claw.ID, otherUser.ID)
	if err != nil {
		t.Fatalf("ListByClawID() wrong owner error = %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("ListByClawID() wrong owner len = %d, want 0", len(got))
	}

	if err := store.Delete(context.Background(), claw.ID, otherUser.ID, entities.CapabilityMemory); !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("Delete() wrong owner error = %v, want ErrNotFound", err)
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
		`CREATE TABLE claws (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			config JSON,
			user_id TEXT,
			server_id TEXT,
			container_id TEXT,
			desired_state TEXT,
			observed_state TEXT,
			lifecycle_status TEXT,
			last_lifecycle_error TEXT,
			last_runtime_sync_at DATETIME,
			current_operation_id TEXT,
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
		`CREATE TABLE claw_capability_attachments (
			id TEXT PRIMARY KEY,
			claw_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			capability_id TEXT NOT NULL,
			provider TEXT,
			account_integration_id TEXT,
			enabled BOOLEAN NOT NULL,
			settings JSON,
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

func seedClaw(t *testing.T, db *gorm.DB, userID uuid.UUID, name string) entities.Claw {
	t.Helper()

	now := time.Now().UTC()
	claw := entities.Claw{
		ID:                 uuid.New(),
		Name:               name,
		UserID:             userID,
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := db.Exec(
		`INSERT INTO claws (id, name, config, user_id, desired_state, observed_state, lifecycle_status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		claw.ID.String(),
		claw.Name,
		`{}`,
		claw.UserID.String(),
		"stopped",
		"unknown",
		"idle",
		claw.CreatedAt,
		claw.UpdatedAt,
	).Error; err != nil {
		t.Fatalf("seed claw error = %v", err)
	}

	return claw
}

func seedIntegration(
	t *testing.T,
	db *gorm.DB,
	userID uuid.UUID,
	capabilityID entities.CapabilityID,
	provider string,
) entities.AccountIntegration {
	t.Helper()

	now := time.Now().UTC()
	id := uuid.New()
	if err := db.Exec(
		`INSERT INTO account_integrations (id, user_id, capability_id, provider, external_account_id, display_name, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(),
		userID.String(),
		string(capabilityID),
		provider,
		provider+"@example.com",
		provider+" account",
		string(entities.AccountIntegrationStatusActive),
		now,
		now,
	).Error; err != nil {
		t.Fatalf("seed integration error = %v", err)
	}

	return entities.AccountIntegration{
		ID:                id,
		UserID:            userID,
		CapabilityID:      capabilityID,
		Provider:          provider,
		ExternalAccountID: provider + "@example.com",
		DisplayName:       provider + " account",
		Status:            entities.AccountIntegrationStatusActive,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}
