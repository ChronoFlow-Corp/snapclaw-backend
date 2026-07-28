package adminstats

import (
	"context"
	"fmt"
	"testing"
	"time"

	"simpleClaw/internal/entities"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())

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
			balance_minor INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME
		)`,
		`CREATE TABLE claws (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			user_id TEXT,
			desired_state TEXT NOT NULL DEFAULT 'stopped',
			observed_state TEXT NOT NULL DEFAULT 'unknown',
			lifecycle_status TEXT NOT NULL DEFAULT 'idle',
			created_at DATETIME
		)`,
		`CREATE TABLE user_subscriptions (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			status TEXT,
			created_at DATETIME
		)`,
	}

	for _, stmt := range schema {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("schema setup error = %v", err)
		}
	}

	return db
}

func seedUser(t *testing.T, db *gorm.DB, email, role string, balance int64, createdAt time.Time) uuid.UUID {
	t.Helper()

	id := uuid.New()
	err := db.Exec(
		`INSERT INTO users (id, name, nick_name, avatar_url, email, role, balance_minor, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "Name "+email, "nick", "https://example.com/a.png", email, role, balance, createdAt,
	).Error
	if err != nil {
		t.Fatalf("seed user error = %v", err)
	}

	return id
}

func seedClaw(t *testing.T, db *gorm.DB, userID uuid.UUID, observed entities.ClawObservedState, lifecycle entities.ClawLifecycleStatus) {
	t.Helper()

	err := db.Exec(
		`INSERT INTO claws (id, name, user_id, desired_state, observed_state, lifecycle_status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New(), "claw", userID, "stopped", string(observed), string(lifecycle), time.Now().UTC(),
	).Error
	if err != nil {
		t.Fatalf("seed claw error = %v", err)
	}
}

func seedSubscription(t *testing.T, db *gorm.DB, userID uuid.UUID, status string) {
	t.Helper()

	err := db.Exec(
		`INSERT INTO user_subscriptions (id, user_id, status, created_at) VALUES (?, ?, ?, ?)`,
		uuid.New(), userID, status, time.Now().UTC(),
	).Error
	if err != nil {
		t.Fatalf("seed subscription error = %v", err)
	}
}

// scenario: A = admin, no claws, no sub; B = user, active sub, running + error claws;
// C = user, canceled sub, failed-lifecycle claw.
func seedScenario(t *testing.T, db *gorm.DB) (a, b, c uuid.UUID) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	a = seedUser(t, db, "alice@example.com", entities.AdminRole, 500, base)
	b = seedUser(t, db, "bob@example.com", entities.UserRole, 100, base.Add(time.Hour))
	c = seedUser(t, db, "carol@example.com", entities.UserRole, 300, base.Add(2*time.Hour))

	seedSubscription(t, db, b, entities.SubscriptionStatusActive)
	seedSubscription(t, db, c, entities.SubscriptionStatusCanceled)

	seedClaw(t, db, b, entities.ClawObservedStateRunning, entities.ClawLifecycleStatusIdle)
	seedClaw(t, db, b, entities.ClawObservedStateError, entities.ClawLifecycleStatusIdle)
	seedClaw(t, db, c, entities.ClawObservedStateStopped, entities.ClawLifecycleStatusFailed)

	return a, b, c
}

func TestCounts(t *testing.T) {
	db := newTestDB(t)
	seedScenario(t, db)
	store := NewStorage(db)
	ctx := context.Background()

	if got, _ := store.CountUsers(ctx); got != 3 {
		t.Errorf("CountUsers = %d, want 3", got)
	}
	if got, _ := store.CountUsersByRole(ctx, entities.AdminRole); got != 1 {
		t.Errorf("CountUsersByRole(admin) = %d, want 1", got)
	}
	if got, _ := store.CountUsersWithActiveSubscription(ctx); got != 1 {
		t.Errorf("CountUsersWithActiveSubscription = %d, want 1", got)
	}
	if got, _ := store.CountUsersWithIssues(ctx); got != 2 {
		t.Errorf("CountUsersWithIssues = %d, want 2 (bob error + carol failed)", got)
	}
}

func TestListUsersComputedColumns(t *testing.T) {
	db := newTestDB(t)
	a, b, c := seedScenario(t, db)
	store := NewStorage(db)

	items, total, err := store.ListUsers(context.Background(), entities.AdminUserFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}

	byID := map[uuid.UUID]entities.AdminUserListItem{}
	for _, item := range items {
		byID[item.ID] = item
	}

	if item := byID[a]; item.ClawsCount != 0 || item.HasActiveSubscription || item.HasIssues {
		t.Errorf("A = %+v, want claws=0 activeSub=false issues=false", item)
	}
	if item := byID[b]; item.ClawsCount != 2 || !item.HasActiveSubscription || !item.HasIssues {
		t.Errorf("B = %+v, want claws=2 activeSub=true issues=true", item)
	}
	if item := byID[c]; item.ClawsCount != 1 || item.HasActiveSubscription || !item.HasIssues {
		t.Errorf("C = %+v, want claws=1 activeSub=false issues=true", item)
	}
}

func TestListUsersDefaultSortNewestFirst(t *testing.T) {
	db := newTestDB(t)
	a, b, c := seedScenario(t, db)
	store := NewStorage(db)

	items, _, err := store.ListUsers(context.Background(), entities.AdminUserFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}

	wantOrder := []uuid.UUID{c, b, a}
	for i, want := range wantOrder {
		if items[i].ID != want {
			t.Fatalf("order[%d] = %v, want %v (created_at DESC)", i, items[i].ID, want)
		}
	}
}

func TestListUsersSortBalanceAsc(t *testing.T) {
	db := newTestDB(t)
	a, b, c := seedScenario(t, db)
	store := NewStorage(db)

	items, _, err := store.ListUsers(context.Background(), entities.AdminUserFilter{
		Page: 1, PageSize: 20, Sort: entities.AdminUserSortBalanceAsc,
	})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}

	wantOrder := []uuid.UUID{b, c, a} // 100, 300, 500
	for i, want := range wantOrder {
		if items[i].ID != want {
			t.Fatalf("balance order[%d] = %v, want %v", i, items[i].ID, want)
		}
	}
}

func TestListUsersFilters(t *testing.T) {
	db := newTestDB(t)
	a, b, _ := seedScenario(t, db)
	store := NewStorage(db)
	ctx := context.Background()
	truthy := true
	falsy := false

	activeItems, total, err := store.ListUsers(ctx, entities.AdminUserFilter{
		Page: 1, PageSize: 20, HasActiveSubscription: &truthy,
	})
	if err != nil {
		t.Fatalf("ListUsers(active) error = %v", err)
	}
	if total != 1 || len(activeItems) != 1 || activeItems[0].ID != b {
		t.Errorf("has_active_subscription=true => %d rows, want only B", total)
	}

	noIssueItems, total, err := store.ListUsers(ctx, entities.AdminUserFilter{
		Page: 1, PageSize: 20, HasIssues: &falsy,
	})
	if err != nil {
		t.Fatalf("ListUsers(no issues) error = %v", err)
	}
	if total != 1 || len(noIssueItems) != 1 || noIssueItems[0].ID != a {
		t.Errorf("has_issues=false => %d rows, want only A", total)
	}

	adminItems, total, err := store.ListUsers(ctx, entities.AdminUserFilter{
		Page: 1, PageSize: 20, Role: entities.AdminRole,
	})
	if err != nil {
		t.Fatalf("ListUsers(role) error = %v", err)
	}
	if total != 1 || adminItems[0].ID != a {
		t.Errorf("role=admin => want only A")
	}

	queryItems, total, err := store.ListUsers(ctx, entities.AdminUserFilter{
		Page: 1, PageSize: 20, Query: "BOB",
	})
	if err != nil {
		t.Fatalf("ListUsers(q) error = %v", err)
	}
	if total != 1 || queryItems[0].ID != b {
		t.Errorf("q=BOB => want only B (case-insensitive email match)")
	}
}

func TestListUsersPagination(t *testing.T) {
	db := newTestDB(t)
	seedScenario(t, db)
	store := NewStorage(db)
	ctx := context.Background()

	page1, total, err := store.ListUsers(ctx, entities.AdminUserFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListUsers(page1) error = %v", err)
	}
	if total != 3 || len(page1) != 2 {
		t.Errorf("page1: total=%d len=%d, want total=3 len=2", total, len(page1))
	}

	page2, total, err := store.ListUsers(ctx, entities.AdminUserFilter{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("ListUsers(page2) error = %v", err)
	}
	if total != 3 || len(page2) != 1 {
		t.Errorf("page2: total=%d len=%d, want total=3 len=1", total, len(page2))
	}
}
