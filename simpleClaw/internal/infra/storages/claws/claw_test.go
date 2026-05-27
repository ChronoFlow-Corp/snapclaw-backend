package claws

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"simpleClaw/internal/entities"
	infraSQL "simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"
)

func captureStorageLogger(t *testing.T) (*strings.Builder, func()) {
	t.Helper()

	var buf strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	return &buf, func() {
		slog.SetDefault(prev)
	}
}

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

func TestStorageUpdateLifecycleState(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()
	claw := entities.Claw{
		ID:                 uuid.New(),
		Name:               "lifecycle claw",
		UserID:             user.ID,
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := store.Create(context.Background(), claw, nil); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	opID := uuid.New()
	err := store.UpdateLifecycle(context.Background(), claw.ID, LifecycleUpdate{
		DesiredState:       entities.ClawDesiredStateRunning,
		ObservedState:      entities.ClawObservedStateUnknown,
		LifecycleStatus:    entities.ClawLifecycleStatusStartPending,
		CurrentOperationID: &opID,
		LastLifecycleError: "boot pending",
	})
	if err != nil {
		t.Fatalf("UpdateLifecycle() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), claw.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.DesiredState != entities.ClawDesiredStateRunning {
		t.Fatalf("DesiredState = %q, want %q", got.DesiredState, entities.ClawDesiredStateRunning)
	}

	if got.ObservedState != entities.ClawObservedStateUnknown {
		t.Fatalf("ObservedState = %q, want %q", got.ObservedState, entities.ClawObservedStateUnknown)
	}

	if got.LifecycleStatus != entities.ClawLifecycleStatusStartPending {
		t.Fatalf("LifecycleStatus = %q, want %q", got.LifecycleStatus, entities.ClawLifecycleStatusStartPending)
	}

	if got.CurrentOperationID == nil || *got.CurrentOperationID != opID {
		t.Fatalf("CurrentOperationID = %v, want %s", got.CurrentOperationID, opID)
	}

	if got.LastError != "boot pending" {
		t.Fatalf("LastError = %q, want %q", got.LastError, "boot pending")
	}
}

func TestStorageUpdateLifecycleClearsOperationAndError(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()
	claw := entities.Claw{
		ID:                 uuid.New(),
		Name:               "clear lifecycle claw",
		UserID:             user.ID,
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := store.Create(context.Background(), claw, nil); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	opID := uuid.New()
	if err := store.UpdateLifecycle(context.Background(), claw.ID, LifecycleUpdate{
		DesiredState:       entities.ClawDesiredStateRunning,
		ObservedState:      entities.ClawObservedStateError,
		LifecycleStatus:    entities.ClawLifecycleStatusFailed,
		CurrentOperationID: &opID,
		LastLifecycleError: "boom",
	}); err != nil {
		t.Fatalf("UpdateLifecycle(set) error = %v", err)
	}

	if err := store.UpdateLifecycle(context.Background(), claw.ID, LifecycleUpdate{
		DesiredState:       entities.ClawDesiredStateStopped,
		ObservedState:      entities.ClawObservedStateStopped,
		LifecycleStatus:    entities.ClawLifecycleStatusIdle,
		CurrentOperationID: nil,
		LastLifecycleError: "",
	}); err != nil {
		t.Fatalf("UpdateLifecycle(clear) error = %v", err)
	}

	got, err := store.GetByID(context.Background(), claw.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.CurrentOperationID != nil {
		t.Fatalf("CurrentOperationID = %v, want nil", got.CurrentOperationID)
	}

	if got.LastError != "" {
		t.Fatalf("LastError = %q, want empty string", got.LastError)
	}
}

func TestStorageUpdateRuntimePersistsLifecycleFields(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()
	serverID := uuid.New()
	claw := entities.Claw{
		ID:                 uuid.New(),
		Name:               "runtime claw",
		UserID:             user.ID,
		ClawLifecycleState: entities.NewClawLifecycleState(),
		Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := db.Create(&models.Server{ID: serverID, Name: "srv", Url: "http://example.com"}).Error; err != nil {
		t.Fatalf("seed server error = %v", err)
	}

	if err := store.Create(context.Background(), claw, nil); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err := store.UpdateRuntime(context.Background(), claw.ID, entities.ClawRuntimeUpdate{
		ServerID:           serverID,
		ContainerRecordID:  "runtime-record-1",
		DesiredState:       entities.ClawDesiredStateRunning,
		ObservedState:      entities.ClawObservedStateRunning,
		LifecycleStatus:    entities.ClawLifecycleStatusIdle,
		LastLifecycleError: "",
	})
	if err != nil {
		t.Fatalf("UpdateRuntime() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), claw.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.ServerID != serverID {
		t.Fatalf("ServerID = %s, want %s", got.ServerID, serverID)
	}

	if got.ContainerID != "runtime-record-1" {
		t.Fatalf("ContainerID = %q, want %q", got.ContainerID, "runtime-record-1")
	}

	if got.DesiredState != entities.ClawDesiredStateRunning {
		t.Fatalf("DesiredState = %q, want %q", got.DesiredState, entities.ClawDesiredStateRunning)
	}

	if got.ObservedState != entities.ClawObservedStateRunning {
		t.Fatalf("ObservedState = %q, want %q", got.ObservedState, entities.ClawObservedStateRunning)
	}

	if got.LifecycleStatus != entities.ClawLifecycleStatusIdle {
		t.Fatalf("LifecycleStatus = %q, want %q", got.LifecycleStatus, entities.ClawLifecycleStatusIdle)
	}

	if got.LastError != "" {
		t.Fatalf("LastError = %q, want empty string", got.LastError)
	}
}

func TestStorageGetNextReconcilePendingWhenEmptyDoesNotLogIdlePoll(t *testing.T) {
	db := newTestDB(t)
	store := NewStorage(db)
	logBuf, restore := captureStorageLogger(t)
	defer restore()

	_, err := store.GetNextReconcilePending(context.Background())
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("GetNextReconcilePending() error = %v, want ErrNotFound", err)
	}

	if strings.TrimSpace(logBuf.String()) != "" {
		t.Fatalf("expected no idle poll logs, got %q", logBuf.String())
	}
}

func TestStorageGetNextReconcilePendingReturnsOldestCandidate(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()

	oldest := entities.Claw{
		ID:        uuid.New(),
		Name:      "oldest",
		UserID:    user.ID,
		Config:    entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt: now.Add(-3 * time.Minute),
		UpdatedAt: now.Add(-3 * time.Minute),
		ClawLifecycleState: entities.ClawLifecycleState{
			DesiredState:    entities.ClawDesiredStateRunning,
			ObservedState:   entities.ClawObservedStateUnknown,
			LifecycleStatus: entities.ClawLifecycleStatusReconcilePending,
		},
	}
	newer := entities.Claw{
		ID:        uuid.New(),
		Name:      "newer",
		UserID:    user.ID,
		Config:    entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt: now.Add(-time.Minute),
		UpdatedAt: now.Add(-time.Minute),
		ClawLifecycleState: entities.ClawLifecycleState{
			DesiredState:    entities.ClawDesiredStateStopped,
			ObservedState:   entities.ClawObservedStateUnknown,
			LifecycleStatus: entities.ClawLifecycleStatusReconcilePending,
		},
	}
	idle := entities.Claw{
		ID:                 uuid.New(),
		Name:               "idle",
		UserID:             user.ID,
		Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt:          now,
		UpdatedAt:          now,
		ClawLifecycleState: entities.NewClawLifecycleState(),
	}

	for _, cl := range []entities.Claw{newer, oldest, idle} {
		if err := store.Create(context.Background(), cl, nil); err != nil {
			t.Fatalf("Create(%s) error = %v", cl.Name, err)
		}
	}

	got, err := store.GetNextReconcilePending(context.Background())
	if err != nil {
		t.Fatalf("GetNextReconcilePending() error = %v", err)
	}

	if got.ID != oldest.ID {
		t.Fatalf("GetNextReconcilePending().ID = %s, want %s", got.ID, oldest.ID)
	}
	if got.LifecycleStatus != entities.ClawLifecycleStatusReconcilePending {
		t.Fatalf("LifecycleStatus = %q, want %q", got.LifecycleStatus, entities.ClawLifecycleStatusReconcilePending)
	}
}

func TestStorageListRuntimeSyncCandidatesReturnsBoundClaws(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	serverID := uuid.New()
	now := time.Now().UTC()

	if err := db.Create(&models.Server{ID: serverID, Name: "srv", Url: "http://example.com"}).Error; err != nil {
		t.Fatalf("seed server error = %v", err)
	}

	bound := entities.Claw{
		ID:        uuid.New(),
		Name:      "bound",
		UserID:    user.ID,
		ServerID:  serverID,
		Config:    entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt: now.Add(-time.Minute),
		UpdatedAt: now.Add(-time.Minute),
		ClawLifecycleState: entities.ClawLifecycleState{
			DesiredState:    entities.ClawDesiredStateRunning,
			ObservedState:   entities.ClawObservedStateRunning,
			LifecycleStatus: entities.ClawLifecycleStatusIdle,
		},
	}
	unbound := entities.Claw{
		ID:                 uuid.New(),
		Name:               "unbound",
		UserID:             user.ID,
		Config:             entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini"),
		CreatedAt:          now,
		UpdatedAt:          now,
		ClawLifecycleState: entities.NewClawLifecycleState(),
	}

	for _, cl := range []entities.Claw{bound, unbound} {
		if err := store.Create(context.Background(), cl, nil); err != nil {
			t.Fatalf("Create(%s) error = %v", cl.Name, err)
		}
	}

	if err := store.UpdateRuntime(context.Background(), bound.ID, entities.ClawRuntimeUpdate{
		ServerID:           serverID,
		ContainerRecordID:  "runtime-record-1",
		DesiredState:       entities.ClawDesiredStateRunning,
		ObservedState:      entities.ClawObservedStateRunning,
		LifecycleStatus:    entities.ClawLifecycleStatusIdle,
		LastLifecycleError: "",
	}); err != nil {
		t.Fatalf("UpdateRuntime(bound) error = %v", err)
	}

	got, err := store.ListRuntimeSyncCandidates(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRuntimeSyncCandidates() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}

	if got[0].ID != bound.ID {
		t.Fatalf("got claw id = %s, want %s", got[0].ID, bound.ID)
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
			container_id TEXT,
			desired_state TEXT NOT NULL,
			observed_state TEXT NOT NULL,
			lifecycle_status TEXT NOT NULL,
			last_lifecycle_error TEXT,
			last_runtime_sync_at DATETIME,
			onboarding_complete BOOLEAN NOT NULL DEFAULT 0,
			current_operation_id TEXT NULL,
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
