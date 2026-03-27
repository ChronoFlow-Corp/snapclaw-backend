package plans

import (
	"context"
	"errors"
	"fmt"
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

func TestStorageCreateAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	plan := newPlan("starter", true, time.Now().UTC())

	if err := store.Create(context.Background(), plan); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	assertPlanEqual(t, plan, got)
}

func TestStorageListFiltersInactiveByDefault(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	active := newPlan("active", true, time.Now().UTC())
	inactive := newPlan("inactive", false, time.Now().UTC().Add(time.Minute))

	if err := store.Create(context.Background(), active); err != nil {
		t.Fatalf("Create() active error = %v", err)
	}

	if err := store.Create(context.Background(), inactive); err != nil {
		t.Fatalf("Create() inactive error = %v", err)
	}

	got, err := store.List(context.Background(), false)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("List() len = %d, want 1", len(got))
	}

	assertPlanEqual(t, active, got[0])
}

func TestStorageListIncludesInactiveWhenRequested(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	first := newPlan("first", true, time.Now().UTC())
	second := newPlan("second", false, time.Now().UTC().Add(time.Minute))

	if err := store.Create(context.Background(), first); err != nil {
		t.Fatalf("Create() first error = %v", err)
	}

	if err := store.Create(context.Background(), second); err != nil {
		t.Fatalf("Create() second error = %v", err)
	}

	got, err := store.List(context.Background(), true)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}
}

func TestStorageUpdate(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	plan := newPlan("starter", true, time.Now().UTC())

	if err := store.Create(context.Background(), plan); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	plan.Name = "Starter Plus"
	plan.BillingAmountMinor = 129000
	plan.BalanceCreditMinor = 200000
	plan.UpdatedAt = plan.UpdatedAt.Add(time.Minute)

	if err := store.Update(context.Background(), plan); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	assertPlanEqual(t, plan, got)
}

func TestStorageDeactivate(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	plan := newPlan("starter", true, time.Now().UTC())

	if err := store.Create(context.Background(), plan); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.Deactivate(context.Background(), plan.ID); err != nil {
		t.Fatalf("Deactivate() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.IsActive {
		t.Fatal("expected plan to be inactive after deactivate")
	}
}

func TestStorageCreateRejectsDuplicateCode(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	first := newPlan("starter", true, time.Now().UTC())
	second := newPlan("starter", true, time.Now().UTC().Add(time.Minute))

	if err := store.Create(context.Background(), first); err != nil {
		t.Fatalf("Create() first error = %v", err)
	}

	err := store.Create(context.Background(), second)
	if !errors.Is(err, infraSQL.ErrConflict) {
		t.Fatalf("Create() duplicate error = %v, want ErrConflict", err)
	}
}

func TestStorageRejectsInvalidData(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)

	plan := newPlan("starter", true, time.Now().UTC())
	plan.ID = uuid.Nil
	if err := store.Create(context.Background(), plan); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() nil id error = %v, want ErrInvalid", err)
	}

	plan = newPlan("", true, time.Now().UTC())
	if err := store.Create(context.Background(), plan); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() empty code error = %v, want ErrInvalid", err)
	}

	plan = newPlan("starter", true, time.Now().UTC())
	plan.BillingAmountMinor = 0
	if err := store.Create(context.Background(), plan); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() zero billing error = %v, want ErrInvalid", err)
	}

	if _, err := store.GetByID(context.Background(), uuid.Nil); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("GetByID() nil id error = %v, want ErrInvalid", err)
	}

	if err := store.Deactivate(context.Background(), uuid.Nil); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Deactivate() nil id error = %v, want ErrInvalid", err)
	}
}

func TestStorageCreateReturnsDetailedPlanValidationError(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)

	plan := newPlan("", true, time.Now().UTC())
	err := store.Create(context.Background(), plan)
	if err == nil {
		t.Fatal("Create() error = nil, want non-nil")
	}

	if !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() error = %v, want ErrInvalid", err)
	}

	if !strings.Contains(err.Error(), "plan code is required") {
		t.Fatalf("Create() error = %q, want to contain %q", err.Error(), "plan code is required")
	}
}

func TestStorageDeactivateReturnsDetailedValidationError(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)

	err := store.Deactivate(context.Background(), uuid.Nil)
	if err == nil {
		t.Fatal("Deactivate() error = nil, want non-nil")
	}

	if !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Deactivate() error = %v, want ErrInvalid", err)
	}

	if !strings.Contains(err.Error(), "plan id is required") {
		t.Fatalf("Deactivate() error = %q, want to contain %q", err.Error(), "plan id is required")
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	if err := db.AutoMigrate(&models.Plan{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	return db
}

func newPlan(code string, isActive bool, now time.Time) entities.Plan {
	return entities.Plan{
		ID:                 uuid.New(),
		Code:               code,
		Name:               "Plan " + code,
		BillingAmountMinor: 99000,
		BalanceCreditMinor: 150000,
		Currency:           entities.RUB,
		IsActive:           isActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func assertPlanEqual(t *testing.T, want, got entities.Plan) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("ID = %s, want %s", got.ID, want.ID)
	}

	if got.Code != want.Code {
		t.Fatalf("Code = %s, want %s", got.Code, want.Code)
	}

	if got.Name != want.Name {
		t.Fatalf("Name = %s, want %s", got.Name, want.Name)
	}

	if got.BillingAmountMinor != want.BillingAmountMinor {
		t.Fatalf("BillingAmountMinor = %d, want %d", got.BillingAmountMinor, want.BillingAmountMinor)
	}

	if got.BalanceCreditMinor != want.BalanceCreditMinor {
		t.Fatalf("BalanceCreditMinor = %d, want %d", got.BalanceCreditMinor, want.BalanceCreditMinor)
	}

	if got.Currency != want.Currency {
		t.Fatalf("Currency = %s, want %s", got.Currency, want.Currency)
	}

	if got.IsActive != want.IsActive {
		t.Fatalf("IsActive = %t, want %t", got.IsActive, want.IsActive)
	}
}
