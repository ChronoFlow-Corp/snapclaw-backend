package clawoperations

import (
	"context"
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

func captureOperationStorageLogger(t *testing.T) (*strings.Builder, func()) {
	t.Helper()

	var buf strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	return &buf, func() {
		slog.SetDefault(prev)
	}
}

func TestStorageCreateAndGetActiveByClawID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	clawID := seedClaw(t, db)
	now := time.Now().UTC()
	op := entities.ClawLifecycleOperation{
		ID:        uuid.New(),
		ClawID:    clawID,
		Type:      entities.ClawLifecycleOperationTypeStart,
		Status:    entities.ClawLifecycleOperationStatusPending,
		Attempt:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.Create(context.Background(), op); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetActiveByClawID(context.Background(), clawID)
	if err != nil {
		t.Fatalf("GetActiveByClawID() error = %v", err)
	}

	assertOperationEqual(t, op, got)
}

func TestStorageCreateRejectsSecondActiveOperationForSameClaw(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	clawID := seedClaw(t, db)
	now := time.Now().UTC()

	first := entities.ClawLifecycleOperation{
		ID:        uuid.New(),
		ClawID:    clawID,
		Type:      entities.ClawLifecycleOperationTypeStart,
		Status:    entities.ClawLifecycleOperationStatusPending,
		Attempt:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	second := entities.ClawLifecycleOperation{
		ID:        uuid.New(),
		ClawID:    clawID,
		Type:      entities.ClawLifecycleOperationTypeStop,
		Status:    entities.ClawLifecycleOperationStatusPending,
		Attempt:   1,
		CreatedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}

	if err := store.Create(context.Background(), first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}

	err := store.Create(context.Background(), second)
	if !errors.Is(err, infraSQL.ErrConflict) {
		t.Fatalf("Create(second) error = %v, want ErrConflict", err)
	}
}

func TestStorageLockNextRunnableReturnsOldestRunnableOperation(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	notReadyClawID := seedClaw(t, db)
	dueRetryClawID := seedClaw(t, db)
	pendingClawID := seedClaw(t, db)
	now := time.Now().UTC()
	future := now.Add(15 * time.Minute)
	past := now.Add(-15 * time.Minute)

	notReady := entities.ClawLifecycleOperation{
		ID:          uuid.New(),
		ClawID:      notReadyClawID,
		Type:        entities.ClawLifecycleOperationTypeReconcile,
		Status:      entities.ClawLifecycleOperationStatusRetryScheduled,
		Attempt:     2,
		NextRetryAt: &future,
		CreatedAt:   now.Add(-2 * time.Minute),
		UpdatedAt:   now.Add(-2 * time.Minute),
	}
	dueRetry := entities.ClawLifecycleOperation{
		ID:          uuid.New(),
		ClawID:      dueRetryClawID,
		Type:        entities.ClawLifecycleOperationTypeStop,
		Status:      entities.ClawLifecycleOperationStatusRetryScheduled,
		Attempt:     3,
		NextRetryAt: &past,
		CreatedAt:   now.Add(-time.Minute),
		UpdatedAt:   now.Add(-time.Minute),
	}
	pending := entities.ClawLifecycleOperation{
		ID:        uuid.New(),
		ClawID:    pendingClawID,
		Type:      entities.ClawLifecycleOperationTypeStart,
		Status:    entities.ClawLifecycleOperationStatusPending,
		Attempt:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	for _, op := range []entities.ClawLifecycleOperation{notReady, dueRetry, pending} {
		if err := store.Create(context.Background(), op); err != nil {
			t.Fatalf("Create(%s) error = %v", op.ID, err)
		}
	}

	got, err := store.LockNextRunnable(context.Background(), now)
	if err != nil {
		t.Fatalf("LockNextRunnable() error = %v", err)
	}

	if got.ID != dueRetry.ID {
		t.Fatalf("LockNextRunnable() id = %s, want %s", got.ID, dueRetry.ID)
	}

	if got.Status != entities.ClawLifecycleOperationStatusRunning {
		t.Fatalf("LockNextRunnable() status = %q, want %q", got.Status, entities.ClawLifecycleOperationStatusRunning)
	}

	next, err := store.LockNextRunnable(context.Background(), now)
	if err != nil {
		t.Fatalf("second LockNextRunnable() error = %v", err)
	}

	if next.ID != pending.ID {
		t.Fatalf("second LockNextRunnable() id = %s, want %s", next.ID, pending.ID)
	}

	var persistedStatus string
	if err := db.Raw("SELECT status FROM claw_lifecycle_operations WHERE id = ?", dueRetry.ID).Scan(&persistedStatus).Error; err != nil {
		t.Fatalf("query claimed status error = %v", err)
	}

	if persistedStatus != string(entities.ClawLifecycleOperationStatusRunning) {
		t.Fatalf("persisted claimed status = %q, want %q", persistedStatus, entities.ClawLifecycleOperationStatusRunning)
	}
}

func TestStorageUpdateRemovesSucceededOperationFromActiveSet(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	clawID := seedClaw(t, db)
	now := time.Now().UTC()
	op := entities.ClawLifecycleOperation{
		ID:        uuid.New(),
		ClawID:    clawID,
		Type:      entities.ClawLifecycleOperationTypeStart,
		Status:    entities.ClawLifecycleOperationStatusPending,
		Attempt:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.Create(context.Background(), op); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	op.Status = entities.ClawLifecycleOperationStatusRunning
	op.Stage = "host.start"
	op.ServerID = uuid.New()
	op.ContainerRecordID = "runtime-record-1"
	op.DockerContainerID = "docker-1"
	op.UpdatedAt = now.Add(time.Minute)
	if err := store.Update(context.Background(), op); err != nil {
		t.Fatalf("Update(running) error = %v", err)
	}

	active, err := store.GetActiveByClawID(context.Background(), clawID)
	if err != nil {
		t.Fatalf("GetActiveByClawID() after running error = %v", err)
	}

	if active.Status != entities.ClawLifecycleOperationStatusRunning {
		t.Fatalf("active.Status = %q, want %q", active.Status, entities.ClawLifecycleOperationStatusRunning)
	}

	op.Status = entities.ClawLifecycleOperationStatusSucceeded
	op.LastError = ""
	op.UpdatedAt = now.Add(2 * time.Minute)
	if err := store.Update(context.Background(), op); err != nil {
		t.Fatalf("Update(succeeded) error = %v", err)
	}

	_, err = store.GetActiveByClawID(context.Background(), clawID)
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("GetActiveByClawID() after succeeded error = %v, want ErrNotFound", err)
	}
}

func TestStorageLockNextRunnableWhenEmptyDoesNotLogIdlePoll(t *testing.T) {
	db := newTestDB(t)
	store := NewStorage(db)
	logBuf, restore := captureOperationStorageLogger(t)
	defer restore()

	_, err := store.LockNextRunnable(context.Background(), time.Now().UTC())
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("LockNextRunnable() error = %v, want ErrNotFound", err)
	}

	if strings.TrimSpace(logBuf.String()) != "" {
		t.Fatalf("expected no idle poll logs, got %q", logBuf.String())
	}
}

func TestStorageCreateAllowsTerminalOperationAlongsideActiveOperation(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	clawID := seedClaw(t, db)
	now := time.Now().UTC()

	active := entities.ClawLifecycleOperation{
		ID:        uuid.New(),
		ClawID:    clawID,
		Type:      entities.ClawLifecycleOperationTypeStart,
		Status:    entities.ClawLifecycleOperationStatusPending,
		Attempt:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	terminal := entities.ClawLifecycleOperation{
		ID:        uuid.New(),
		ClawID:    clawID,
		Type:      entities.ClawLifecycleOperationTypeStop,
		Status:    entities.ClawLifecycleOperationStatusSucceeded,
		Attempt:   1,
		CreatedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}

	if err := store.Create(context.Background(), active); err != nil {
		t.Fatalf("Create(active) error = %v", err)
	}

	if err := store.Create(context.Background(), terminal); err != nil {
		t.Fatalf("Create(terminal) error = %v", err)
	}

	got, err := store.GetActiveByClawID(context.Background(), clawID)
	if err != nil {
		t.Fatalf("GetActiveByClawID() error = %v", err)
	}

	if got.ID != active.ID {
		t.Fatalf("GetActiveByClawID() id = %s, want %s", got.ID, active.ID)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_foreign_keys=on", uuid.NewString())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
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
			marketing_opt_out INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
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
			current_operation_id TEXT NULL,
			vars TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			CONSTRAINT fk_users_claws FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE claw_lifecycle_operations (
			id TEXT PRIMARY KEY,
			claw_id TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			stage TEXT NOT NULL DEFAULT '',
			attempt INTEGER NOT NULL DEFAULT 0,
			last_error TEXT,
			server_id TEXT,
			container_record_id TEXT,
			docker_container_id TEXT,
			next_retry_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			CONSTRAINT fk_claw_lifecycle_operations_claw FOREIGN KEY (claw_id) REFERENCES claws(id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX idx_claw_lifecycle_active_operation ON claw_lifecycle_operations (claw_id)
			WHERE status IN ('pending', 'running', 'retry_scheduled')`,
	}

	for _, stmt := range schema {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("schema setup error = %v", err)
		}
	}

	return db
}

func seedClaw(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()

	user := models.User{
		ID:        uuid.New(),
		Name:      "Test User",
		NickName:  "tester",
		Email:     uuid.NewString() + "@example.com",
		Role:      entities.UserRole,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user error = %v", err)
	}

	clawID := uuid.New()
	if err := db.Exec(
		`INSERT INTO claws (
			id, name, user_id, desired_state, observed_state, lifecycle_status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		clawID,
		"test claw",
		user.ID,
		entities.ClawDesiredStateStopped,
		entities.ClawObservedStateUnknown,
		entities.ClawLifecycleStatusIdle,
		time.Now().UTC(),
		time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed claw error = %v", err)
	}

	return clawID
}

func assertOperationEqual(t *testing.T, want, got entities.ClawLifecycleOperation) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("ID = %s, want %s", got.ID, want.ID)
	}

	if got.ClawID != want.ClawID {
		t.Fatalf("ClawID = %s, want %s", got.ClawID, want.ClawID)
	}

	if got.Type != want.Type {
		t.Fatalf("Type = %q, want %q", got.Type, want.Type)
	}

	if got.Status != want.Status {
		t.Fatalf("Status = %q, want %q", got.Status, want.Status)
	}

	if got.Attempt != want.Attempt {
		t.Fatalf("Attempt = %d, want %d", got.Attempt, want.Attempt)
	}
}
