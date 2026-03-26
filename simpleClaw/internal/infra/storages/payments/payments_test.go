package payments

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
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
	payment := newPayment(user.ID)

	if err := store.Create(context.Background(), payment); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), payment.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	assertPaymentEqual(t, payment, got)
}

func TestStorageCreateAndGetByIDPersistsPurpose(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	payment := newPayment(user.ID)
	payment.Purpose = entities.PaymentPurposeSubscription

	if err := store.Create(context.Background(), payment); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), payment.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.Purpose != entities.PaymentPurposeSubscription {
		t.Fatalf("Purpose = %q, want %q", got.Purpose, entities.PaymentPurposeSubscription)
	}
}

func TestStoragePersistsSubscriptionID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	payment := newPayment(user.ID)
	subscriptionID := uuid.New()
	payment.SubscriptionID = &subscriptionID

	if err := store.Create(context.Background(), payment); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), payment.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if got.SubscriptionID == nil || *got.SubscriptionID != subscriptionID {
		t.Fatalf("SubscriptionID after create = %v, want %v", got.SubscriptionID, subscriptionID)
	}

	updated := payment
	updatedSubscriptionID := uuid.New()
	updated.SubscriptionID = &updatedSubscriptionID
	if err := store.Update(context.Background(), updated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err = store.GetByID(context.Background(), payment.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() after update error = %v", err)
	}

	if got.SubscriptionID == nil || *got.SubscriptionID != updatedSubscriptionID {
		t.Fatalf("SubscriptionID after update = %v, want %v", got.SubscriptionID, updatedSubscriptionID)
	}
}

func TestStorageGetByUserIDFiltersOwnerAndOrdersByCreatedAtDesc(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	userOne := seedUser(t, db)
	userTwo := seedUser(t, db)

	olderPayment := newPayment(userOne.ID)
	newerPayment := newPayment(userOne.ID)
	newerPayment.CreatedAt = olderPayment.CreatedAt.Add(10 * time.Minute)
	otherUserPayment := newPayment(userTwo.ID)

	if err := store.Create(context.Background(), olderPayment); err != nil {
		t.Fatalf("Create() older payment error = %v", err)
	}

	if err := store.Create(context.Background(), newerPayment); err != nil {
		t.Fatalf("Create() newer payment error = %v", err)
	}

	if err := store.Create(context.Background(), otherUserPayment); err != nil {
		t.Fatalf("Create() other user payment error = %v", err)
	}

	got, err := store.GetByUserID(context.Background(), userOne.ID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("GetByUserID() len = %d, want 2", len(got))
	}

	assertPaymentEqual(t, newerPayment, got[0])
	assertPaymentEqual(t, olderPayment, got[1])
}

func TestStorageUpdateMutableFieldsPreservesImmutableAmounts(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	payment := newPayment(user.ID)

	if err := store.Create(context.Background(), payment); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated := payment
	updated.Status = entities.Succeeded
	updated.Paid = true
	updated.Description = "updated description"
	expiresAt := payment.CreatedAt.Add(4 * time.Hour)
	updated.ExpiresAt = &expiresAt
	updated.AuthorizationDetails = &entities.AuthorizationDetails{
		RRN:      "updated-rrn",
		AuthCode: "updated-auth",
	}
	updated.Metadata = map[string]interface{}{
		"attempt": float64(2),
		"source":  "webhook",
	}
	updated.PaymentMethod = &entities.PaymentMethodDetails{
		Type:  "bank_card",
		ID:    "pm_updated",
		Saved: false,
		Title: "Mastercard",
	}
	updated.Recipient = &entities.Recipient{
		AccountID: "account-updated",
		GatewayID: "gateway-updated",
	}
	updated.Refundable = false
	updated.Test = false
	updated.IncomeAmount = &entities.Amount{
		Value:    "950.00",
		Currency: "RUB",
	}
	updated.Amount = entities.Amount{
		Value:    "1.00",
		Currency: "USD",
	}
	updated.CreatedAt = payment.CreatedAt.Add(24 * time.Hour)

	if err := store.Update(context.Background(), updated); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := store.GetByID(context.Background(), payment.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() after update error = %v", err)
	}

	if got.Amount != payment.Amount {
		t.Fatalf("Amount changed after update: got %+v want %+v", got.Amount, payment.Amount)
	}

	if !got.CreatedAt.Equal(payment.CreatedAt) {
		t.Fatalf("CreatedAt changed after update: got %s want %s", got.CreatedAt, payment.CreatedAt)
	}

	if got.Status != updated.Status {
		t.Fatalf("Status after update = %q, want %q", got.Status, updated.Status)
	}

	if !got.Paid {
		t.Fatal("Paid after update = false, want true")
	}

	if got.Description != updated.Description {
		t.Fatalf("Description after update = %q, want %q", got.Description, updated.Description)
	}

	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("ExpiresAt after update = %v, want %v", got.ExpiresAt, expiresAt)
	}

	assertMetadataEqual(t, updated.Metadata, got.Metadata)

	if got.AuthorizationDetails == nil || got.AuthorizationDetails.RRN != "updated-rrn" {
		t.Fatalf("AuthorizationDetails after update = %+v", got.AuthorizationDetails)
	}

	if got.PaymentMethod == nil || got.PaymentMethod.ID != "pm_updated" {
		t.Fatalf("PaymentMethod after update = %+v", got.PaymentMethod)
	}

	if got.Recipient == nil || got.Recipient.AccountID != "account-updated" {
		t.Fatalf("Recipient after update = %+v", got.Recipient)
	}

	if got.Refundable {
		t.Fatal("Refundable after update = true, want false")
	}

	if got.Test {
		t.Fatal("Test after update = true, want false")
	}

	if got.IncomeAmount == nil || *got.IncomeAmount != *updated.IncomeAmount {
		t.Fatalf("IncomeAmount after update = %+v, want %+v", got.IncomeAmount, updated.IncomeAmount)
	}
}

func TestStorageUpdateRejectsPartialSnapshot(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	original := newPayment(user.ID)

	if err := store.Create(context.Background(), original); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	partial := entities.Payment{
		ID:     original.ID,
		UserID: user.ID,
		Status: entities.Succeeded,
		Paid:   false,
	}

	err := store.Update(context.Background(), partial)
	if !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Update() partial snapshot error = %v, want ErrInvalid", err)
	}

	got, err := store.GetByID(context.Background(), original.ID, user.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	assertPaymentEqual(t, original, got)
}

func TestStorageDelete(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	payment := newPayment(user.ID)

	if err := store.Create(context.Background(), payment); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.Delete(context.Background(), payment.ID, user.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := store.GetByID(context.Background(), payment.ID, user.ID)
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("GetByID() after delete error = %v, want ErrNotFound", err)
	}
}

func TestStorageGetLatestSucceededByPurpose(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)

	topUp := newPayment(user.ID)
	topUp.Purpose = entities.PaymentPurposeTopUp
	topUp.Status = entities.Succeeded
	topUp.Paid = true
	topUp.CreatedAt = topUp.CreatedAt.Add(2 * time.Hour)

	oldSubscription := newPayment(user.ID)
	oldSubscription.Purpose = entities.PaymentPurposeSubscription
	oldSubscription.Status = entities.Succeeded
	oldSubscription.Paid = true

	newSubscription := newPayment(user.ID)
	newSubscription.Purpose = entities.PaymentPurposeSubscription
	newSubscription.Status = entities.Succeeded
	newSubscription.Paid = true
	newSubscription.CreatedAt = oldSubscription.CreatedAt.Add(90 * time.Minute)

	for _, payment := range []entities.Payment{oldSubscription, topUp, newSubscription} {
		if err := store.Create(context.Background(), payment); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	got, err := store.GetLatestSucceededByPurpose(
		context.Background(),
		user.ID,
		entities.PaymentPurposeSubscription,
	)
	if err != nil {
		t.Fatalf("GetLatestSucceededByPurpose() error = %v", err)
	}

	assertPaymentEqual(t, newSubscription, got)
}

func TestStorageGetLatestSucceededByPurposeReturnsNotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	payment := newPayment(user.ID)
	payment.Purpose = entities.PaymentPurposeTopUp
	payment.Status = entities.Succeeded
	payment.Paid = true

	if err := store.Create(context.Background(), payment); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err := store.GetLatestSucceededByPurpose(
		context.Background(),
		user.ID,
		entities.PaymentPurposeSubscription,
	)
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("GetLatestSucceededByPurpose() error = %v, want ErrNotFound", err)
	}
}

func TestStorageWrongOwnerReturnsNotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	owner := seedUser(t, db)
	otherUser := seedUser(t, db)
	payment := newPayment(owner.ID)

	if err := store.Create(context.Background(), payment); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err := store.GetByID(context.Background(), payment.ID, otherUser.ID)
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("GetByID() wrong owner error = %v, want ErrNotFound", err)
	}

	payment.Description = "wrong-owner-update"
	err = store.Update(context.Background(), payment)
	if err != nil {
		t.Fatalf("Update() owner update error = %v", err)
	}

	wrongOwnerPayment := payment
	wrongOwnerPayment.UserID = otherUser.ID
	err = store.Update(context.Background(), wrongOwnerPayment)
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("Update() wrong owner error = %v, want ErrNotFound", err)
	}

	err = store.Delete(context.Background(), payment.ID, otherUser.ID)
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("Delete() wrong owner error = %v, want ErrNotFound", err)
	}
}

func TestStorageRejectsInvalidPaymentData(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	payment := newPayment(user.ID)

	payment.ID = ""
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() empty id error = %v, want ErrInvalid", err)
	}

	payment = newPayment(uuid.Nil)
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() nil user id error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.Status = ""
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() empty status error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.Amount.Value = ""
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() empty amount value error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.Amount.Currency = ""
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() empty amount currency error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.CreatedAt = time.Time{}
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() zero created_at error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	if err := store.Create(context.Background(), payment); err != nil {
		t.Fatalf("Create() valid payment error = %v", err)
	}

	payment.ID = ""
	if err := store.Update(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Update() empty id error = %v, want ErrInvalid", err)
	}

	payment = newPayment(uuid.Nil)
	if err := store.Update(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Update() nil user id error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.Status = ""
	if err := store.Update(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Update() empty status error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.PaymentMethod = &entities.PaymentMethodDetails{}
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() empty payment method type error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.Recipient = &entities.Recipient{}
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() empty recipient account id error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.IncomeAmount = &entities.Amount{Currency: "RUB"}
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() income amount missing value error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.IncomeAmount = &entities.Amount{Value: "10.00"}
	if err := store.Create(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() income amount missing currency error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.PaymentMethod = &entities.PaymentMethodDetails{}
	if err := store.Update(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Update() empty payment method type error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.Recipient = &entities.Recipient{}
	if err := store.Update(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Update() empty recipient account id error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.IncomeAmount = &entities.Amount{Currency: "RUB"}
	if err := store.Update(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Update() income amount missing value error = %v, want ErrInvalid", err)
	}

	payment = newPayment(user.ID)
	payment.IncomeAmount = &entities.Amount{Value: "10.00"}
	if err := store.Update(context.Background(), payment); !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Update() income amount missing currency error = %v, want ErrInvalid", err)
	}
}

func TestStorageCreateRejectsUnknownUserID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	payment := newPayment(uuid.New())

	err := store.Create(context.Background(), payment)
	if !errors.Is(err, infraSQL.ErrInvalid) {
		t.Fatalf("Create() unknown user error = %v, want ErrInvalid", err)
	}
}

func TestTranslatePaymentError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "pq foreign key",
			err:  &pq.Error{Code: "23503"},
			want: infraSQL.ErrInvalid,
		},
		{
			name: "pgx duplicate",
			err:  &pgconn.PgError{Code: "23505"},
			want: infraSQL.ErrConflict,
		},
		{
			name: "sqlite foreign key text",
			err:  errors.New("FOREIGN KEY constraint failed"),
			want: infraSQL.ErrInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translatePaymentError(tt.err)
			if !errors.Is(got, tt.want) {
				t.Fatalf("translatePaymentError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("PRAGMA foreign_keys = ON error = %v", err)
	}

	if err := db.AutoMigrate(&models.User{}, &models.Payment{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	return db
}

func TestPaymentModelsAutoMigrateEnforcesForeignKeyAndCascadeDelete(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	user := seedUser(t, db)

	payment := models.Payment{
		ID:             "pay_" + uuid.NewString(),
		UserID:         user.ID,
		Status:         string(entities.Pending),
		Paid:           false,
		AmountValue:    "1000.00",
		AmountCurrency: "RUB",
		CreatedAt:      time.Now().UTC(),
	}

	if err := db.Create(&payment).Error; err != nil {
		t.Fatalf("db.Create(payment) error = %v", err)
	}

	var count int64
	if err := db.Model(&models.Payment{}).Where("id = ?", payment.ID).Count(&count).Error; err != nil {
		t.Fatalf("Count() error = %v", err)
	}

	if count != 1 {
		t.Fatalf("payment count = %d, want 1", count)
	}

	if err := db.Delete(&user).Error; err != nil {
		t.Fatalf("db.Delete(user) error = %v", err)
	}

	if err := db.Model(&models.Payment{}).Where("id = ?", payment.ID).Count(&count).Error; err != nil {
		t.Fatalf("Count() after delete error = %v", err)
	}

	if count != 0 {
		t.Fatalf("payment count after user delete = %d, want 0", count)
	}
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

func newPayment(userID uuid.UUID) entities.Payment {
	createdAt := time.Date(2026, time.March, 23, 10, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(2 * time.Hour)

	return entities.Payment{
		ID:      "pay_" + uuid.NewString(),
		UserID:  userID,
		Purpose: entities.PaymentPurposeTopUp,
		Status:  entities.Pending,
		Paid:    false,
		Amount: entities.Amount{
			Value:    "1000.00",
			Currency: "RUB",
		},
		AuthorizationDetails: &entities.AuthorizationDetails{
			RRN:      "rrn-1",
			AuthCode: "auth-1",
			ThreeDSecure: &entities.ThreeDSecure{
				Applied: true,
			},
		},
		CreatedAt:   createdAt,
		Description: "subscription",
		ExpiresAt:   &expiresAt,
		Metadata: map[string]interface{}{
			"order_id": uuid.NewString(),
			"retry":    false,
		},
		PaymentMethod: &entities.PaymentMethodDetails{
			Type:  "bank_card",
			ID:    "pm_" + uuid.NewString(),
			Saved: true,
			Card: &entities.Card{
				First6:      "555555",
				Last4:       "4444",
				ExpiryMonth: "12",
				ExpiryYear:  "2030",
				CardType:    "Mastercard",
				CardProduct: &entities.CardProduct{
					Code: "MCPP",
					Name: "Premium",
				},
				IssuerCountry: "RU",
				IssuerName:    "Bank",
			},
			Title: "Primary card",
		},
		Recipient: &entities.Recipient{
			AccountID: "account-1",
			GatewayID: "gateway-1",
		},
		Refundable: true,
		Test:       true,
		IncomeAmount: &entities.Amount{
			Value:    "970.00",
			Currency: "RUB",
		},
	}
}

func assertPaymentEqual(t *testing.T, want, got entities.Payment) {
	t.Helper()

	if got.ID != want.ID {
		t.Fatalf("ID = %q, want %q", got.ID, want.ID)
	}

	if got.UserID != want.UserID {
		t.Fatalf("UserID = %s, want %s", got.UserID, want.UserID)
	}

	if (got.SubscriptionID == nil) != (want.SubscriptionID == nil) {
		t.Fatalf("SubscriptionID nil mismatch: got %v want %v", got.SubscriptionID, want.SubscriptionID)
	}

	if got.SubscriptionID != nil && *got.SubscriptionID != *want.SubscriptionID {
		t.Fatalf("SubscriptionID = %s, want %s", *got.SubscriptionID, *want.SubscriptionID)
	}

	if got.Purpose != want.Purpose {
		t.Fatalf("Purpose = %q, want %q", got.Purpose, want.Purpose)
	}

	if got.Status != want.Status {
		t.Fatalf("Status = %q, want %q", got.Status, want.Status)
	}

	if got.Paid != want.Paid {
		t.Fatalf("Paid = %t, want %t", got.Paid, want.Paid)
	}

	if got.Amount != want.Amount {
		t.Fatalf("Amount = %+v, want %+v", got.Amount, want.Amount)
	}

	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("CreatedAt = %s, want %s", got.CreatedAt, want.CreatedAt)
	}

	if got.Description != want.Description {
		t.Fatalf("Description = %q, want %q", got.Description, want.Description)
	}

	if (got.ExpiresAt == nil) != (want.ExpiresAt == nil) {
		t.Fatalf("ExpiresAt nil mismatch: got %v want %v", got.ExpiresAt, want.ExpiresAt)
	}

	if got.ExpiresAt != nil && !got.ExpiresAt.Equal(*want.ExpiresAt) {
		t.Fatalf("ExpiresAt = %s, want %s", got.ExpiresAt, want.ExpiresAt)
	}

	if got.Refundable != want.Refundable {
		t.Fatalf("Refundable = %t, want %t", got.Refundable, want.Refundable)
	}

	if got.Test != want.Test {
		t.Fatalf("Test = %t, want %t", got.Test, want.Test)
	}

	if !equalAuthorizationDetails(got.AuthorizationDetails, want.AuthorizationDetails) {
		t.Fatalf("AuthorizationDetails = %+v, want %+v", got.AuthorizationDetails, want.AuthorizationDetails)
	}

	assertMetadataEqual(t, want.Metadata, got.Metadata)

	if !equalPaymentMethod(got.PaymentMethod, want.PaymentMethod) {
		t.Fatalf("PaymentMethod = %+v, want %+v", got.PaymentMethod, want.PaymentMethod)
	}

	if !equalRecipient(got.Recipient, want.Recipient) {
		t.Fatalf("Recipient = %+v, want %+v", got.Recipient, want.Recipient)
	}

	if !equalAmountPtr(got.IncomeAmount, want.IncomeAmount) {
		t.Fatalf("IncomeAmount = %+v, want %+v", got.IncomeAmount, want.IncomeAmount)
	}
}

func assertMetadataEqual(t *testing.T, want, got interface{}) {
	t.Helper()

	wantMap, wantOK := want.(map[string]interface{})
	gotMap, gotOK := got.(map[string]interface{})

	if !wantOK || !gotOK {
		t.Fatalf("Metadata types = %T and %T, want map[string]interface{}", got, want)
	}

	if len(gotMap) != len(wantMap) {
		t.Fatalf("Metadata len = %d, want %d", len(gotMap), len(wantMap))
	}

	for key, wantValue := range wantMap {
		gotValue, ok := gotMap[key]
		if !ok {
			t.Fatalf("Metadata missing key %q", key)
		}

		if gotValue != wantValue {
			t.Fatalf("Metadata[%q] = %v, want %v", key, gotValue, wantValue)
		}
	}
}

func equalAuthorizationDetails(left, right *entities.AuthorizationDetails) bool {
	if left == nil || right == nil {
		return left == right
	}

	if left.RRN != right.RRN || left.AuthCode != right.AuthCode {
		return false
	}

	if left.ThreeDSecure == nil || right.ThreeDSecure == nil {
		return left.ThreeDSecure == right.ThreeDSecure
	}

	return left.ThreeDSecure.Applied == right.ThreeDSecure.Applied
}

func equalPaymentMethod(left, right *entities.PaymentMethodDetails) bool {
	if left == nil || right == nil {
		return left == right
	}

	if left.Type != right.Type || left.ID != right.ID || left.Saved != right.Saved || left.Title != right.Title {
		return false
	}

	return equalCard(left.Card, right.Card)
}

func equalCard(left, right *entities.Card) bool {
	if left == nil || right == nil {
		return left == right
	}

	if left.First6 != right.First6 ||
		left.Last4 != right.Last4 ||
		left.ExpiryMonth != right.ExpiryMonth ||
		left.ExpiryYear != right.ExpiryYear ||
		left.CardType != right.CardType ||
		left.IssuerCountry != right.IssuerCountry ||
		left.IssuerName != right.IssuerName {
		return false
	}

	if left.CardProduct == nil || right.CardProduct == nil {
		return left.CardProduct == right.CardProduct
	}

	return left.CardProduct.Code == right.CardProduct.Code && left.CardProduct.Name == right.CardProduct.Name
}

func equalRecipient(left, right *entities.Recipient) bool {
	if left == nil || right == nil {
		return left == right
	}

	return left.AccountID == right.AccountID && left.GatewayID == right.GatewayID
}

func equalAmountPtr(left, right *entities.Amount) bool {
	if left == nil || right == nil {
		return left == right
	}

	return *left == *right
}
