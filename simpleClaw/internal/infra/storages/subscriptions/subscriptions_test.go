package subscriptions

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
	"simpleClaw/internal/infra/sql/models"
)

func TestStorageCreateAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	plan := seedPlan(t, db, "starter")
	subscription := newSubscription(user.ID, plan.ID, entities.SubscriptionStatusActive, time.Now().UTC())

	if err := store.Create(context.Background(), subscription); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), subscription.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	assertSubscriptionEqual(t, subscription, got)
}

func TestStorageGetActiveByUserID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	plan := seedPlan(t, db, "starter")
	active := newSubscription(user.ID, plan.ID, entities.SubscriptionStatusActive, time.Now().UTC())
	canceled := newSubscription(user.ID, plan.ID, entities.SubscriptionStatusCanceled, time.Now().UTC().Add(time.Minute))

	if err := store.Create(context.Background(), canceled); err != nil {
		t.Fatalf("Create() canceled error = %v", err)
	}

	if err := store.Create(context.Background(), active); err != nil {
		t.Fatalf("Create() active error = %v", err)
	}

	got, err := store.GetActiveByUserID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetActiveByUserID() error = %v", err)
	}

	assertSubscriptionEqual(t, active, got)
}

func TestStorageUpdate(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	originalPlan := seedPlan(t, db, "starter")
	newPlan := seedPlan(t, db, "pro")
	subscription := newSubscription(user.ID, originalPlan.ID, entities.SubscriptionStatusActive, time.Now().UTC())

	if err := store.Create(context.Background(), subscription); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	subscription.PlanID = newPlan.ID
	subscription.Status = entities.SubscriptionStatusPastDue
	subscription.CurrentPeriodEnd = subscription.CurrentPeriodEnd.Add(24 * time.Hour)
	subscription.UpdatedAt = subscription.UpdatedAt.Add(time.Minute)

	if err := store.Update(context.Background(), subscription); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), subscription.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	assertSubscriptionEqual(t, subscription, got)
}

func TestStorageCancel(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	plan := seedPlan(t, db, "starter")
	subscription := newSubscription(user.ID, plan.ID, entities.SubscriptionStatusActive, time.Now().UTC())

	if err := store.Create(context.Background(), subscription); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	canceledAt := time.Now().UTC().Add(time.Hour)
	if err := store.Cancel(context.Background(), subscription.ID, user.ID, canceledAt); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), subscription.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.Status != entities.SubscriptionStatusCanceled {
		t.Fatalf("Status = %q, want %q", got.Status, entities.SubscriptionStatusCanceled)
	}

	if got.CanceledAt == nil || !got.CanceledAt.Equal(canceledAt) {
		t.Fatalf("CanceledAt = %v, want %v", got.CanceledAt, canceledAt)
	}
}

func TestStorageGetActiveByUserIDReturnsNotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)

	_, err := store.GetActiveByUserID(context.Background(), user.ID)
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("GetActiveByUserID() error = %v, want ErrNotFound", err)
	}
}

func TestStorageRejectsInvalidData(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	plan := seedPlan(t, db, "starter")
	subscription := newSubscription(user.ID, plan.ID, entities.SubscriptionStatusActive, time.Now().UTC())

	subscription.ID = uuid.Nil
	if err := store.Create(context.Background(), subscription); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() nil id error = %v, want ErrInvalid", err)
	}

	subscription = newSubscription(uuid.Nil, plan.ID, entities.SubscriptionStatusActive, time.Now().UTC())
	if err := store.Create(context.Background(), subscription); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() nil user id error = %v, want ErrInvalid", err)
	}

	subscription = newSubscription(user.ID, uuid.Nil, entities.SubscriptionStatusActive, time.Now().UTC())
	if err := store.Create(context.Background(), subscription); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() nil plan id error = %v, want ErrInvalid", err)
	}

	if _, err := store.GetByID(context.Background(), uuid.Nil); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("GetByID() nil id error = %v, want ErrInvalid", err)
	}

	if err := store.Cancel(context.Background(), uuid.Nil, user.ID, time.Now().UTC()); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Cancel() nil id error = %v, want ErrInvalid", err)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	if err := db.AutoMigrate(&models.User{}, &models.Plan{}, &models.UserSubscription{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
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

func seedPlan(t *testing.T, db *gorm.DB, code string) models.Plan {
	t.Helper()

	now := time.Now().UTC()
	plan := models.Plan{
		ID:                 uuid.New(),
		Code:               code,
		Name:               "Plan " + code,
		BillingAmountMinor: 99000,
		BalanceCreditMinor: 150000,
		Currency:           entities.RUB,
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("db.Create(plan) error = %v", err)
	}

	return plan
}

func newSubscription(userID, planID uuid.UUID, status string, now time.Time) entities.UserSubscription {
	return entities.UserSubscription{
		ID:                 uuid.New(),
		UserID:             userID,
		PlanID:             planID,
		Status:             status,
		StartedAt:          now,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.Add(30 * 24 * time.Hour),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func assertSubscriptionEqual(t *testing.T, want, got entities.UserSubscription) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("ID = %s, want %s", got.ID, want.ID)
	}

	if got.UserID != want.UserID {
		t.Fatalf("UserID = %s, want %s", got.UserID, want.UserID)
	}

	if got.PlanID != want.PlanID {
		t.Fatalf("PlanID = %s, want %s", got.PlanID, want.PlanID)
	}

	if got.Status != want.Status {
		t.Fatalf("Status = %s, want %s", got.Status, want.Status)
	}

	if !got.StartedAt.Equal(want.StartedAt) {
		t.Fatalf("StartedAt = %s, want %s", got.StartedAt, want.StartedAt)
	}

	if !got.CurrentPeriodStart.Equal(want.CurrentPeriodStart) {
		t.Fatalf("CurrentPeriodStart = %s, want %s", got.CurrentPeriodStart, want.CurrentPeriodStart)
	}

	if !got.CurrentPeriodEnd.Equal(want.CurrentPeriodEnd) {
		t.Fatalf("CurrentPeriodEnd = %s, want %s", got.CurrentPeriodEnd, want.CurrentPeriodEnd)
	}
}
