package paymentmethods

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
	method := newPaymentMethod(user.ID, true, time.Now().UTC())

	if err := store.Create(context.Background(), method); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), method.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	assertPaymentMethodEqual(t, method, got)
}

func TestStorageGetByUserIDFiltersOwnerAndOrdersByDefaultThenCreatedAtDesc(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	userOne := seedUser(t, db)
	userTwo := seedUser(t, db)

	baseTime := time.Now().UTC().Add(-time.Hour)
	defaultMethod := newPaymentMethod(userOne.ID, true, baseTime)
	newerNonDefault := newPaymentMethod(userOne.ID, false, baseTime.Add(10*time.Minute))
	otherUserMethod := newPaymentMethod(userTwo.ID, true, baseTime.Add(20*time.Minute))

	if err := store.Create(context.Background(), defaultMethod); err != nil {
		t.Fatalf("Create() default method error = %v", err)
	}

	if err := store.Create(context.Background(), newerNonDefault); err != nil {
		t.Fatalf("Create() newer non-default method error = %v", err)
	}

	if err := store.Create(context.Background(), otherUserMethod); err != nil {
		t.Fatalf("Create() other user method error = %v", err)
	}

	got, err := store.GetByUserID(context.Background(), userOne.ID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("GetByUserID() len = %d, want 2", len(got))
	}

	assertPaymentMethodEqual(t, defaultMethod, got[0])
	assertPaymentMethodEqual(t, newerNonDefault, got[1])
}

func TestStorageUpdateDefault(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	method := newPaymentMethod(user.ID, false, time.Now().UTC())

	if err := store.Create(context.Background(), method); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.UpdateDefault(context.Background(), method.ID, user.ID, true); err != nil {
		t.Fatalf("UpdateDefault() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), method.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if !got.IsDefault {
		t.Fatal("IsDefault after update = false, want true")
	}
}

func TestStorageClearDefaultByUserID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	methodOne := newPaymentMethod(user.ID, true, time.Now().UTC())
	methodTwo := newPaymentMethod(user.ID, true, time.Now().UTC().Add(time.Minute))

	if err := store.Create(context.Background(), methodOne); err != nil {
		t.Fatalf("Create() methodOne error = %v", err)
	}

	if err := store.Create(context.Background(), methodTwo); err != nil {
		t.Fatalf("Create() methodTwo error = %v", err)
	}

	if err := store.ClearDefaultByUserID(context.Background(), user.ID); err != nil {
		t.Fatalf("ClearDefaultByUserID() error = %v", err)
	}

	got, err := store.GetByUserID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v", err)
	}

	for _, method := range got {
		if method.IsDefault {
			t.Fatalf("method %s remained default", method.ID)
		}
	}
}

func TestStorageCountByUserIDAndDeleteWithOwnerScoping(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	owner := seedUser(t, db)
	otherUser := seedUser(t, db)
	methodOne := newPaymentMethod(owner.ID, true, time.Now().UTC())
	methodTwo := newPaymentMethod(owner.ID, false, time.Now().UTC().Add(time.Minute))

	if err := store.Create(context.Background(), methodOne); err != nil {
		t.Fatalf("Create() methodOne error = %v", err)
	}

	if err := store.Create(context.Background(), methodTwo); err != nil {
		t.Fatalf("Create() methodTwo error = %v", err)
	}

	count, err := store.CountByUserID(context.Background(), owner.ID)
	if err != nil {
		t.Fatalf("CountByUserID() error = %v", err)
	}

	if count != 2 {
		t.Fatalf("CountByUserID() = %d, want 2", count)
	}

	if err := store.Delete(context.Background(), methodOne.ID, otherUser.ID); !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("Delete() wrong owner error = %v, want ErrNotFound", err)
	}

	if err := store.Delete(context.Background(), methodOne.ID, owner.ID); err != nil {
		t.Fatalf("Delete() owner error = %v", err)
	}

	_, err = store.GetByID(context.Background(), methodOne.ID, owner.ID)
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("GetByID() after delete error = %v, want ErrNotFound", err)
	}
}

func TestStorageRejectsInvalidData(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)

	method := newPaymentMethod(user.ID, false, time.Now().UTC())
	method.ID = uuid.Nil
	if err := store.Create(context.Background(), method); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() nil id error = %v, want ErrInvalid", err)
	}

	method = newPaymentMethod(uuid.Nil, false, time.Now().UTC())
	if err := store.Create(context.Background(), method); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() nil user id error = %v, want ErrInvalid", err)
	}

	method = newPaymentMethod(user.ID, false, time.Now().UTC())
	method.Title = ""
	if err := store.Create(context.Background(), method); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() empty title error = %v, want ErrInvalid", err)
	}

	method = newPaymentMethod(user.ID, false, time.Now().UTC())
	if err := store.Create(context.Background(), method); err != nil {
		t.Fatalf("Create() valid method error = %v", err)
	}

	if err := store.UpdateDefault(context.Background(), uuid.Nil, user.ID, true); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("UpdateDefault() nil id error = %v, want ErrInvalid", err)
	}

	if err := store.UpdateDefault(context.Background(), method.ID, uuid.Nil, true); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("UpdateDefault() nil user id error = %v, want ErrInvalid", err)
	}

	if _, err := store.GetByID(context.Background(), uuid.Nil, user.ID); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("GetByID() nil id error = %v, want ErrInvalid", err)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	if err := db.AutoMigrate(&models.User{}, &models.PaymentMethod{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	return db
}

func seedUser(t *testing.T, db *gorm.DB) entities.User {
	t.Helper()

	user := entities.NewUser("Test User", "tester", "", uuid.NewString()+"@example.com", entities.UserRole)
	model := models.User{
		ID:               user.ID,
		Name:             user.Name,
		NickName:         user.Nickname,
		AvatarURL:        user.AvatarURL,
		Email:            user.Email,
		Role:             user.Role,
		OpenRouterKeyID:  user.OpenRouterKeyID,
		OpenRouterApiKey: user.OpenRouterApiKey,
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.CreatedAt,
	}

	if err := db.Create(&model).Error; err != nil {
		t.Fatalf("seed user error = %v", err)
	}

	return user
}

func newPaymentMethod(userID uuid.UUID, isDefault bool, createdAt time.Time) entities.PaymentMethod {
	lastUsedAt := createdAt.Add(5 * time.Minute)

	return entities.PaymentMethod{
		ID:         uuid.New(),
		UserID:     userID,
		Title:      "Primary card " + uuid.NewString(),
		IsDefault:  isDefault,
		CreatedAt:  createdAt,
		LastUsedAt: &lastUsedAt,
	}
}

func assertPaymentMethodEqual(t *testing.T, want, got entities.PaymentMethod) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("ID = %s, want %s", got.ID, want.ID)
	}

	if got.UserID != want.UserID {
		t.Fatalf("UserID = %s, want %s", got.UserID, want.UserID)
	}

	if got.Title != want.Title {
		t.Fatalf("Title = %q, want %q", got.Title, want.Title)
	}

	if got.IsDefault != want.IsDefault {
		t.Fatalf("IsDefault = %t, want %t", got.IsDefault, want.IsDefault)
	}

	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want %s", got.CreatedAt, want.CreatedAt)
	}

	if (got.LastUsedAt == nil) != (want.LastUsedAt == nil) {
		t.Fatalf("LastUsedAt nil mismatch: got %v want %v", got.LastUsedAt, want.LastUsedAt)
	}

	if got.LastUsedAt != nil && !got.LastUsedAt.Equal(*want.LastUsedAt) {
		t.Fatalf("LastUsedAt = %s, want %s", got.LastUsedAt, want.LastUsedAt)
	}
}
