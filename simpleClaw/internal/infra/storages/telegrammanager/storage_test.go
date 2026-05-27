package telegrammanager

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"simpleClaw/internal/entities"
	infraSQL "simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStorageCreateConsumeAndResolveLink(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()
	linkID := uuid.New()

	err := store.CreateLink(context.Background(), entities.TelegramAccountLink{
		ID:           linkID,
		UserID:       user.ID,
		LinkCodeHash: "hash-1",
		Status:       entities.TelegramAccountLinkStatusPending,
		ExpiresAt:    now.Add(time.Minute),
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("CreateLink() error = %v", err)
	}

	consumed, err := store.ConsumeLink(
		context.Background(),
		"hash-1",
		entities.TelegramLinkActivation{
			TelegramUserID:   1001,
			TelegramChatID:   2002,
			TelegramUsername: "snapclaw_user",
			LinkedAt:         now.Add(10 * time.Second),
		},
	)
	if err != nil {
		t.Fatalf("ConsumeLink() error = %v", err)
	}

	if consumed.Status != entities.TelegramAccountLinkStatusLinked {
		t.Fatalf("status = %q", consumed.Status)
	}

	if consumed.TelegramUserID != 1001 {
		t.Fatalf("telegram user id = %d", consumed.TelegramUserID)
	}

	resolved, err := store.GetByTelegramUserID(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetByTelegramUserID() error = %v", err)
	}

	if resolved.UserID != user.ID {
		t.Fatalf("user id = %s, want %s", resolved.UserID, user.ID)
	}

	if _, err := store.ConsumeLink(
		context.Background(),
		"hash-1",
		entities.TelegramLinkActivation{
			TelegramUserID:   1001,
			TelegramChatID:   2002,
			TelegramUsername: "snapclaw_user",
			LinkedAt:         now.Add(20 * time.Second),
		},
	); !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("second ConsumeLink() error = %v, want ErrNotFound", err)
	}
}

func TestStorageConsumeLinkRejectsExpiredRecord(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()

	err := store.CreateLink(context.Background(), entities.TelegramAccountLink{
		ID:           uuid.New(),
		UserID:       user.ID,
		LinkCodeHash: "expired-hash",
		Status:       entities.TelegramAccountLinkStatusPending,
		ExpiresAt:    now.Add(-time.Minute),
		CreatedAt:    now.Add(-2 * time.Minute),
		UpdatedAt:    now.Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateLink() error = %v", err)
	}

	_, err = store.ConsumeLink(
		context.Background(),
		"expired-hash",
		entities.TelegramLinkActivation{
			TelegramUserID:   1001,
			TelegramChatID:   2002,
			TelegramUsername: "snapclaw_user",
			LinkedAt:         now,
		},
	)
	if !errors.Is(err, infraSQL.ErrNotFound) {
		t.Fatalf("ConsumeLink() error = %v, want ErrNotFound", err)
	}
}

func TestStorageTryRecordWebhookUpdateIsIdempotent(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)

	recorded, err := store.TryRecordWebhookUpdate(context.Background(), 77)
	if err != nil {
		t.Fatalf("TryRecordWebhookUpdate() first error = %v", err)
	}

	if !recorded {
		t.Fatal("expected first record attempt to be true")
	}

	recorded, err = store.TryRecordWebhookUpdate(context.Background(), 77)
	if err != nil {
		t.Fatalf("TryRecordWebhookUpdate() second error = %v", err)
	}

	if recorded {
		t.Fatal("expected second record attempt to be false")
	}
}

func TestStorageUpsertManagedBot(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()
	managedBotID := uuid.New()
	channelID := uuid.New()

	err := store.UpsertManagedBot(context.Background(), entities.TelegramManagedBot{
		ID:                  managedBotID,
		UserID:              user.ID,
		TelegramOwnerUserID: 1001,
		ManagedBotUserID:    3003,
		ManagedBotUsername:  "snapclaw_helper_bot",
		ManagedBotName:      "Snapclaw Helper",
		Status:              entities.TelegramManagedBotStatusWaitingCreation,
		CreatedAt:           now,
		UpdatedAt:           now,
	})
	if err != nil {
		t.Fatalf("UpsertManagedBot() create error = %v", err)
	}

	err = store.UpsertManagedBot(context.Background(), entities.TelegramManagedBot{
		ID:                  managedBotID,
		UserID:              user.ID,
		TelegramOwnerUserID: 1001,
		ManagedBotUserID:    3003,
		ManagedBotUsername:  "snapclaw_helper_bot",
		ManagedBotName:      "Snapclaw Helper",
		ChannelID:           channelID,
		Status:              entities.TelegramManagedBotStatusReady,
		CreatedAt:           now,
		UpdatedAt:           now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("UpsertManagedBot() update error = %v", err)
	}

	got, err := store.GetManagedBotByID(context.Background(), managedBotID, user.ID)
	if err != nil {
		t.Fatalf("GetManagedBotByID() error = %v", err)
	}

	if got.Status != entities.TelegramManagedBotStatusReady {
		t.Fatalf("status = %q", got.Status)
	}

	if got.ChannelID != channelID {
		t.Fatalf("channel id = %s, want %s", got.ChannelID, channelID)
	}
}

func TestStorageUpsertManagedBotPersistsResumeState(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()
	managedBotID := uuid.New()
	clawID := uuid.New()
	channelID := uuid.New()
	linkExpiresAt := now.Add(10 * time.Minute)

	err := store.UpsertManagedBot(context.Background(), entities.TelegramManagedBot{
		ID:                  managedBotID,
		UserID:              user.ID,
		ClawID:              clawID,
		TelegramOwnerUserID: 1001,
		ManagedBotUserID:    3003,
		ManagedBotUsername:  "snapclaw_helper_bot",
		ManagedBotName:      "Snapclaw Helper",
		DeepLinkURL:         "https://t.me/simpleclaw_manager_bot?start=resume-code",
		LinkExpiresAt:       linkExpiresAt,
		ChannelID:           channelID,
		Status:              entities.TelegramManagedBotStatusFailed,
		LastError:           "telegram token exchange failed",
		CreatedAt:           now,
		UpdatedAt:           now,
	})
	if err != nil {
		t.Fatalf("UpsertManagedBot() error = %v", err)
	}

	got, err := store.GetManagedBotByID(context.Background(), managedBotID, user.ID)
	if err != nil {
		t.Fatalf("GetManagedBotByID() error = %v", err)
	}

	if got.ClawID != clawID {
		t.Fatalf("claw id = %s, want %s", got.ClawID, clawID)
	}

	if got.DeepLinkURL != "https://t.me/simpleclaw_manager_bot?start=resume-code" {
		t.Fatalf("deep link = %q", got.DeepLinkURL)
	}

	if !got.LinkExpiresAt.Equal(linkExpiresAt) {
		t.Fatalf("link expires at = %s, want %s", got.LinkExpiresAt, linkExpiresAt)
	}

	if got.ChannelID != channelID {
		t.Fatalf("channel id = %s, want %s", got.ChannelID, channelID)
	}

	if got.LastError != "telegram token exchange failed" {
		t.Fatalf("last error = %q", got.LastError)
	}
}

func TestStorageUpsertManagedBotAllowsPendingLinkWithoutManagedBotUserID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	now := time.Now().UTC()
	firstID := uuid.New()
	secondID := uuid.New()
	firstClawID := uuid.New()
	secondClawID := uuid.New()

	for _, item := range []entities.TelegramManagedBot{
		{
			ID:            firstID,
			UserID:        user.ID,
			ClawID:        firstClawID,
			DeepLinkURL:   "https://t.me/simpleclaw_manager_bot?start=first-code",
			LinkExpiresAt: now.Add(10 * time.Minute),
			Status:        entities.TelegramManagedBotStatusPendingLink,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            secondID,
			UserID:        user.ID,
			ClawID:        secondClawID,
			DeepLinkURL:   "https://t.me/simpleclaw_manager_bot?start=second-code",
			LinkExpiresAt: now.Add(12 * time.Minute),
			Status:        entities.TelegramManagedBotStatusPendingLink,
			CreatedAt:     now.Add(time.Minute),
			UpdatedAt:     now.Add(time.Minute),
		},
	} {
		if err := store.UpsertManagedBot(context.Background(), item); err != nil {
			t.Fatalf("UpsertManagedBot() error = %v", err)
		}
	}

	first, err := store.GetManagedBotByID(context.Background(), firstID, user.ID)
	if err != nil {
		t.Fatalf("GetManagedBotByID() first error = %v", err)
	}

	if first.ManagedBotUserID != 0 {
		t.Fatalf("managed bot user id = %d, want 0", first.ManagedBotUserID)
	}

	if first.DeepLinkURL != "https://t.me/simpleclaw_manager_bot?start=first-code" {
		t.Fatalf("deep link = %q", first.DeepLinkURL)
	}

	second, err := store.GetManagedBotByID(context.Background(), secondID, user.ID)
	if err != nil {
		t.Fatalf("GetManagedBotByID() second error = %v", err)
	}

	if second.ManagedBotUserID != 0 {
		t.Fatalf("managed bot user id = %d, want 0", second.ManagedBotUserID)
	}
}

func TestStorageGetLatestManagedBotByClawID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	store := NewStorage(db)
	user := seedUser(t, db)
	otherUser := seedUser(t, db)
	now := time.Now().UTC()
	clawID := uuid.New()
	otherClawID := uuid.New()
	latestID := uuid.New()
	expectedLink := "https://t.me/simpleclaw_manager_bot?start=fresh-code"

	fixtures := []entities.TelegramManagedBot{
		{
			ID:                  uuid.New(),
			UserID:              user.ID,
			ClawID:              clawID,
			TelegramOwnerUserID: 1001,
			ManagedBotUserID:    3001,
			Status:              entities.TelegramManagedBotStatusPendingLink,
			DeepLinkURL:         "https://t.me/simpleclaw_manager_bot?start=stale-code",
			LinkExpiresAt:       now.Add(5 * time.Minute),
			CreatedAt:           now.Add(-10 * time.Minute),
			UpdatedAt:           now.Add(-10 * time.Minute),
		},
		{
			ID:                  latestID,
			UserID:              user.ID,
			ClawID:              clawID,
			TelegramOwnerUserID: 1001,
			ManagedBotUserID:    3002,
			Status:              entities.TelegramManagedBotStatusWaitingCreation,
			DeepLinkURL:         expectedLink,
			LinkExpiresAt:       now.Add(15 * time.Minute),
			CreatedAt:           now.Add(-5 * time.Minute),
			UpdatedAt:           now,
		},
		{
			ID:                  uuid.New(),
			UserID:              user.ID,
			ClawID:              otherClawID,
			TelegramOwnerUserID: 1001,
			ManagedBotUserID:    3003,
			Status:              entities.TelegramManagedBotStatusReady,
			CreatedAt:           now.Add(-3 * time.Minute),
			UpdatedAt:           now.Add(time.Minute),
		},
		{
			ID:                  uuid.New(),
			UserID:              otherUser.ID,
			ClawID:              clawID,
			TelegramOwnerUserID: 2001,
			ManagedBotUserID:    4001,
			Status:              entities.TelegramManagedBotStatusReady,
			CreatedAt:           now.Add(-2 * time.Minute),
			UpdatedAt:           now.Add(2 * time.Minute),
		},
	}

	for _, item := range fixtures {
		if err := store.UpsertManagedBot(context.Background(), item); err != nil {
			t.Fatalf("UpsertManagedBot() error = %v", err)
		}
	}

	got, err := store.GetLatestManagedBotByClawID(context.Background(), user.ID, clawID)
	if err != nil {
		t.Fatalf("GetLatestManagedBotByClawID() error = %v", err)
	}

	if got.ID != latestID {
		t.Fatalf("id = %s, want %s", got.ID, latestID)
	}

	if got.DeepLinkURL != expectedLink {
		t.Fatalf("deep link = %q, want %q", got.DeepLinkURL, expectedLink)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.TelegramAccountLink{},
		&models.TelegramWebhookUpdate{},
		&models.TelegramManagedBot{},
	); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	return db
}

func seedUser(t *testing.T, db *gorm.DB) entities.User {
	t.Helper()

	user := entities.NewUser("Test User", "tester", "", uuid.NewString()+"@example.com", entities.UserRole)
	if err := db.Create(&models.User{
		ID:               user.ID,
		Name:             user.Name,
		NickName:         user.Nickname,
		AvatarURL:        user.AvatarURL,
		Email:            user.Email,
		Role:             user.Role,
		OpenRouterApiKey: user.OpenRouterApiKey,
		OpenRouterKeyID:  user.OpenRouterKeyID,
		BalanceMinor:     user.BalanceMinor,
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.CreatedAt,
	}).Error; err != nil {
		t.Fatalf("seed user error = %v", err)
	}

	return user
}
