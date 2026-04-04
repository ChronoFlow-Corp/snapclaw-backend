package balanceentries

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

func TestBalanceEntryHasNoClawDependency(t *testing.T) {
	t.Parallel()

	if _, ok := reflect.TypeOf(entities.UserBalanceEntry{}).FieldByName("ClawID"); ok {
		t.Fatal("entities.UserBalanceEntry must not depend on a specific claw")
	}

	if _, ok := reflect.TypeOf(models.UserBalanceEntry{}).FieldByName("ClawID"); ok {
		t.Fatal("models.UserBalanceEntry must not persist a claw_id reference")
	}
}

func TestStorageApplyCreditCreatesEntryAndIncrementsBalance(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db, 1000)
	entry := newBalanceEntry(user.ID, entities.BalanceEntryTypeTopUpCredit, 250, time.Now().UTC())

	balance, err := store.ApplyCredit(context.Background(), entry, entities.Payment{})
	if err != nil {
		t.Fatalf("ApplyCredit() error = %v", err)
	}

	if balance != 1250 {
		t.Fatalf("ApplyCredit() balance = %d, want 1250", balance)
	}

	assertUserBalance(t, db, user.ID, 1250)
	assertEntryCount(t, db, user.ID, 1)
}

func TestStorageApplyUsageDebitCreatesEntryAndDecrementsBalance(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db, 1000)
	entry := newBalanceEntry(user.ID, entities.BalanceEntryTypeUsageDebit, 400, time.Now().UTC())

	balance, err := store.ApplyUsageDebit(context.Background(), entry)
	if err != nil {
		t.Fatalf("ApplyUsageDebit() error = %v", err)
	}

	if balance != 600 {
		t.Fatalf("ApplyUsageDebit() balance = %d, want 600", balance)
	}

	assertUserBalance(t, db, user.ID, 600)
	assertEntryCount(t, db, user.ID, 1)
}

func TestStorageApplyUsageDebitRejectsInsufficientBalance(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db, 100)
	entry := newBalanceEntry(user.ID, entities.BalanceEntryTypeUsageDebit, 250, time.Now().UTC())

	if _, err := store.ApplyUsageDebit(context.Background(), entry); err == nil {
		t.Fatal("ApplyUsageDebit() error = nil, want non-nil")
	}

	assertUserBalance(t, db, user.ID, 100)
	assertEntryCount(t, db, user.ID, 0)
}

func TestStorageApplyCreditReturnsDetailedEntryValidationError(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)

	_, err := store.ApplyCredit(context.Background(), entities.UserBalanceEntry{
		ID:          uuid.New(),
		Type:        entities.BalanceEntryTypeTopUpCredit,
		AmountMinor: 250,
		CreatedAt:   time.Now().UTC(),
	}, entities.Payment{})
	if err == nil {
		t.Fatal("ApplyCredit() error = nil, want non-nil")
	}

	if !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("ApplyCredit() error = %v, want ErrInvalid", err)
	}

	if !strings.Contains(err.Error(), "entry user_id is required") {
		t.Fatalf("ApplyCredit() error = %q, want to contain %q", err.Error(), "entry user_id is required")
	}
}

func TestStorageApplyCreditReturnsDetailedPaymentSnapshotError(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db, 1000)
	entry := newBalanceEntry(user.ID, entities.BalanceEntryTypeTopUpCredit, 250, time.Now().UTC())

	_, err := store.ApplyCredit(context.Background(), entry, entities.Payment{
		ID:        "pay_invalid",
		UserID:    user.ID,
		Purpose:   entities.PaymentPurposeSubscription,
		Amount:    entities.Amount{Value: "100.00", Currency: entities.RUB},
		CreatedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("ApplyCredit() error = nil, want non-nil")
	}

	if !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("ApplyCredit() error = %v, want ErrInvalid", err)
	}

	if !strings.Contains(err.Error(), "map payment snapshot pay_invalid") {
		t.Fatalf("ApplyCredit() error = %q, want to contain %q", err.Error(), "map payment snapshot pay_invalid")
	}

	if !strings.Contains(err.Error(), "payment status is required") {
		t.Fatalf("ApplyCredit() error = %q, want to contain %q", err.Error(), "payment status is required")
	}
}

func TestStorageApplyUsageDebitOnceIsIdempotent(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db, 100)
	entry := newBalanceEntry(user.ID, entities.BalanceEntryTypeUsageDebit, 12, time.Now().UTC())
	event := entities.OpenRouterUsageEvent{
		ID:         uuid.New(),
		Provider:   "openrouter",
		TraceID:    "trace-1",
		SpanID:     "span-1",
		UserID:     user.ID,
		Model:      "openai/gpt-4.1-mini",
		APIKeyName: "snapclaw+" + user.ID.String(),
		CreatedAt:  time.Now().UTC(),
	}

	balance, applied, err := store.ApplyUsageDebitOnce(context.Background(), entry, event)
	if err != nil {
		t.Fatalf("ApplyUsageDebitOnce() first error = %v", err)
	}

	if !applied {
		t.Fatal("ApplyUsageDebitOnce() first applied = false, want true")
	}

	if balance != 88 {
		t.Fatalf("ApplyUsageDebitOnce() first balance = %d, want 88", balance)
	}

	balance, applied, err = store.ApplyUsageDebitOnce(context.Background(), entry, event)
	if err != nil {
		t.Fatalf("ApplyUsageDebitOnce() second error = %v", err)
	}

	if applied {
		t.Fatal("ApplyUsageDebitOnce() second applied = true, want false")
	}

	if balance != 88 {
		t.Fatalf("ApplyUsageDebitOnce() second balance = %d, want 88", balance)
	}

	assertUserBalance(t, db, user.ID, 88)
	assertEntryCount(t, db, user.ID, 1)
	assertUsageEventCount(t, db, user.ID, 1)
}

func TestStorageApplyUsageDebitOnceRollsBackUsageEventOnInsufficientBalance(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db, 10)
	entry := newBalanceEntry(user.ID, entities.BalanceEntryTypeUsageDebit, 25, time.Now().UTC())
	event := entities.OpenRouterUsageEvent{
		ID:         uuid.New(),
		Provider:   "openrouter",
		TraceID:    "trace-insufficient",
		SpanID:     "span-insufficient",
		UserID:     user.ID,
		Model:      "openai/gpt-4.1-mini",
		APIKeyName: "snapclaw+" + user.ID.String(),
		CreatedAt:  time.Now().UTC(),
	}

	_, _, err := store.ApplyUsageDebitOnce(context.Background(), entry, event)
	if err == nil {
		t.Fatal("ApplyUsageDebitOnce() error = nil, want non-nil")
	}

	assertUserBalance(t, db, user.ID, 10)
	assertEntryCount(t, db, user.ID, 0)
	assertUsageEventCount(t, db, user.ID, 0)
}

func TestStorageListByUserIDOrdersNewestFirst(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db, 1000)
	older := newBalanceEntry(user.ID, entities.BalanceEntryTypeTopUpCredit, 100, time.Now().UTC())
	newer := newBalanceEntry(user.ID, entities.BalanceEntryTypeUsageDebit, 50, older.CreatedAt.Add(time.Minute))

	if _, err := store.ApplyCredit(context.Background(), older, entities.Payment{}); err != nil {
		t.Fatalf("ApplyCredit() error = %v", err)
	}

	if _, err := store.ApplyUsageDebit(context.Background(), newer); err != nil {
		t.Fatalf("ApplyUsageDebit() error = %v", err)
	}

	got, err := store.ListByUserID(context.Background(), user.ID, 10)
	if err != nil {
		t.Fatalf("ListByUserID() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("ListByUserID() len = %d, want 2", len(got))
	}

	if got[0].ID != newer.ID {
		t.Fatalf("first entry = %s, want %s", got[0].ID, newer.ID)
	}

	if got[1].ID != older.ID {
		t.Fatalf("second entry = %s, want %s", got[1].ID, older.ID)
	}
}

func TestStorageSumUsageDebitByUserIDInRangeUsesHalfOpenRange(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db, 1000)

	start := time.Date(2026, time.April, 3, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	excludedBefore := newBalanceEntry(user.ID, entities.BalanceEntryTypeUsageDebit, 100, start.Add(-time.Minute))
	includedAtStart := newBalanceEntry(user.ID, entities.BalanceEntryTypeUsageDebit, 200, start)
	includedInside := newBalanceEntry(user.ID, entities.BalanceEntryTypeUsageDebit, 300, start.Add(12*time.Hour))
	excludedAtEnd := newBalanceEntry(user.ID, entities.BalanceEntryTypeUsageDebit, 400, end)
	excludedType := newBalanceEntry(user.ID, entities.BalanceEntryTypeTopUpCredit, 500, start.Add(time.Hour))

	for _, entry := range []entities.UserBalanceEntry{
		excludedBefore,
		includedAtStart,
		includedInside,
		excludedAtEnd,
		excludedType,
	} {
		if entry.Type == entities.BalanceEntryTypeUsageDebit {
			if _, err := store.ApplyUsageDebit(context.Background(), entry); err != nil {
				t.Fatalf("ApplyUsageDebit() error = %v", err)
			}
			continue
		}

		if _, err := store.ApplyCredit(context.Background(), entry, entities.Payment{}); err != nil {
			t.Fatalf("ApplyCredit() error = %v", err)
		}
	}

	got, err := store.SumUsageDebitByUserIDInRange(context.Background(), user.ID, start, end)
	if err != nil {
		t.Fatalf("SumUsageDebitByUserIDInRange() error = %v", err)
	}

	if got != 500 {
		t.Fatalf("SumUsageDebitByUserIDInRange() = %d, want 500", got)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	if err := db.AutoMigrate(&models.User{}, &models.UserBalanceEntry{}, &models.OpenRouterUsageEvent{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	return db
}

func seedUser(t *testing.T, db *gorm.DB, balanceMinor int64) models.User {
	t.Helper()

	user := models.User{
		ID:           uuid.New(),
		Name:         "Test User",
		NickName:     "tester",
		AvatarURL:    "https://example.com/avatar.png",
		Email:        uuid.NewString() + "@example.com",
		Role:         entities.UserRole,
		BalanceMinor: balanceMinor,
	}

	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("db.Create(user) error = %v", err)
	}

	return user
}

func newBalanceEntry(userID uuid.UUID, entryType string, amountMinor int64, createdAt time.Time) entities.UserBalanceEntry {
	return entities.UserBalanceEntry{
		ID:          uuid.New(),
		UserID:      userID,
		Type:        entryType,
		AmountMinor: amountMinor,
		Description: "test entry",
		CreatedAt:   createdAt,
	}
}

func assertUserBalance(t *testing.T, db *gorm.DB, userID uuid.UUID, want int64) {
	t.Helper()

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatalf("db.First(user) error = %v", err)
	}

	if user.BalanceMinor != want {
		t.Fatalf("BalanceMinor = %d, want %d", user.BalanceMinor, want)
	}
}

func assertEntryCount(t *testing.T, db *gorm.DB, userID uuid.UUID, want int64) {
	t.Helper()

	var count int64
	if err := db.Model(&models.UserBalanceEntry{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("Count() error = %v", err)
	}

	if count != want {
		t.Fatalf("entry count = %d, want %d", count, want)
	}
}

func assertUsageEventCount(t *testing.T, db *gorm.DB, userID uuid.UUID, want int64) {
	t.Helper()

	var count int64
	if err := db.Model(&models.OpenRouterUsageEvent{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("Count() usage event error = %v", err)
	}

	if count != want {
		t.Fatalf("usage event count = %d, want %d", count, want)
	}
}
