package telegrammanager

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"simpleClaw/internal/entities"
	entitychannels "simpleClaw/internal/entities/channels"
	telegraminfra "simpleClaw/internal/infra/telegram"
	"simpleClaw/internal/service/user/commands"

	"github.com/google/uuid"
)

func TestServiceCreateLinkBuildsDeepLink(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)
	store := newServiceStorage()
	channels := &serviceChannelStorage{}
	telegramAPI := &serviceTelegramAPI{
		managerUser: telegraminfra.User{
			ID:       42,
			IsBot:    true,
			Username: "simpleclaw_manager_bot",
		},
	}

	svc := New(store, channels, telegramAPI, Options{
		ManagerUsername: "simpleclaw_manager_bot",
		LinkTTL:         time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	userID := uuid.New()
	clawID := uuid.New()
	result, err := svc.CreateLink(context.Background(), CreateLinkCommand{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("CreateLink() error = %v", err)
	}

	if result.ManagedBot.Status != entities.TelegramManagedBotStatusPendingLink {
		t.Fatalf("status = %q", result.ManagedBot.Status)
	}

	if !strings.HasPrefix(result.DeepLinkURL, "https://t.me/simpleclaw_manager_bot?start=") {
		t.Fatalf("deep link = %q", result.DeepLinkURL)
	}

	if telegramAPI.getMeCalls != 1 {
		t.Fatalf("getMe calls = %d, want 1", telegramAPI.getMeCalls)
	}

	if len(store.links) != 1 {
		t.Fatalf("links len = %d, want 1", len(store.links))
	}

	if len(store.managedBots) != 1 {
		t.Fatalf("managed bots len = %d, want 1", len(store.managedBots))
	}

	managedBot, err := store.GetManagedBotByID(context.Background(), result.ManagedBot.ID, userID)
	if err != nil {
		t.Fatalf("GetManagedBotByID() error = %v", err)
	}

	if managedBot.ClawID != clawID {
		t.Fatalf("claw id = %s, want %s", managedBot.ClawID, clawID)
	}

	if managedBot.DeepLinkURL != result.DeepLinkURL {
		t.Fatalf("deep link = %q, want %q", managedBot.DeepLinkURL, result.DeepLinkURL)
	}

	if !managedBot.LinkExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("link expires at = %s, want %s", managedBot.LinkExpiresAt, now.Add(time.Minute))
	}
}

func TestServiceCreateLinkRejectsManagerUsernameMismatch(t *testing.T) {
	t.Parallel()

	svc := New(newServiceStorage(), &serviceChannelStorage{}, &serviceTelegramAPI{
		managerUser: telegraminfra.User{
			ID:       42,
			IsBot:    true,
			Username: "actual_manager_bot",
		},
	}, Options{
		ManagerUsername: "configured_manager_bot",
	})

	_, err := svc.CreateLink(context.Background(), CreateLinkCommand{
		UserID: uuid.New(),
		ClawID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error for manager username mismatch")
	}

	if !strings.Contains(err.Error(), "does not match bot username") {
		t.Fatalf("error = %q", err)
	}
}

func TestServiceCreateLinkRequiresClawID(t *testing.T) {
	t.Parallel()

	svc := New(newServiceStorage(), &serviceChannelStorage{}, &serviceTelegramAPI{}, Options{
		ManagerUsername: "simpleclaw_manager_bot",
	})

	_, err := svc.CreateLink(context.Background(), CreateLinkCommand{
		UserID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error for missing claw id")
	}
}

func TestServiceCreateLinkReusesActiveLinkForClaw(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)

	testCases := []struct {
		name      string
		status    entities.TelegramManagedBotStatus
		expiresAt time.Time
	}{
		{
			name:      "pending link",
			status:    entities.TelegramManagedBotStatusPendingLink,
			expiresAt: now.Add(5 * time.Minute),
		},
		{
			name:      "waiting creation",
			status:    entities.TelegramManagedBotStatusWaitingCreation,
			expiresAt: now.Add(-time.Minute),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newServiceStorage()
			svc := New(store, &serviceChannelStorage{}, &serviceTelegramAPI{
				managerUser: telegraminfra.User{
					ID:       42,
					IsBot:    true,
					Username: "simpleclaw_manager_bot",
				},
			}, Options{
				ManagerUsername: "simpleclaw_manager_bot",
				LinkTTL:         10 * time.Minute,
				Now: func() time.Time {
					return now
				},
			})

			userID := uuid.New()
			clawID := uuid.New()
			existingID := uuid.New()
			store.managedBots[existingID] = entities.TelegramManagedBot{
				ID:            existingID,
				UserID:        userID,
				ClawID:        clawID,
				Status:        tc.status,
				DeepLinkURL:   "https://t.me/simpleclaw_manager_bot?start=existing-code",
				LinkExpiresAt: tc.expiresAt,
				CreatedAt:     now.Add(-time.Minute),
				UpdatedAt:     now.Add(-time.Minute),
			}

			result, err := svc.CreateLink(context.Background(), CreateLinkCommand{
				UserID: userID,
				ClawID: clawID,
			})
			if err != nil {
				t.Fatalf("CreateLink() error = %v", err)
			}

			if result.ManagedBot.ID != existingID {
				t.Fatalf("managed bot id = %s, want %s", result.ManagedBot.ID, existingID)
			}

			if result.DeepLinkURL != "https://t.me/simpleclaw_manager_bot?start=existing-code" {
				t.Fatalf("deep link = %q", result.DeepLinkURL)
			}

			if len(store.links) != 0 {
				t.Fatalf("links len = %d, want 0", len(store.links))
			}
		})
	}
}

func TestServiceCreateLinkDoesNotReuseLinkForDifferentManagerUsername(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)
	store := newServiceStorage()
	userID := uuid.New()
	clawID := uuid.New()
	existingID := uuid.New()
	store.managedBots[existingID] = entities.TelegramManagedBot{
		ID:            existingID,
		UserID:        userID,
		ClawID:        clawID,
		Status:        entities.TelegramManagedBotStatusPendingLink,
		DeepLinkURL:   "https://t.me/old_manager_bot?start=existing-code",
		LinkExpiresAt: now.Add(5 * time.Minute),
		CreatedAt:     now.Add(-time.Minute),
		UpdatedAt:     now.Add(-time.Minute),
	}

	svc := New(store, &serviceChannelStorage{}, &serviceTelegramAPI{
		managerUser: telegraminfra.User{
			ID:       42,
			IsBot:    true,
			Username: "simpleclaw_manager_bot",
		},
	}, Options{
		ManagerUsername: "simpleclaw_manager_bot",
		LinkTTL:         10 * time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	result, err := svc.CreateLink(context.Background(), CreateLinkCommand{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("CreateLink() error = %v", err)
	}

	if result.ManagedBot.ID == existingID {
		t.Fatalf("managed bot id = %s, want new id", result.ManagedBot.ID)
	}

	if !strings.HasPrefix(result.DeepLinkURL, "https://t.me/simpleclaw_manager_bot?start=") {
		t.Fatalf("deep link = %q", result.DeepLinkURL)
	}
}

func TestServiceCreateLinkCreatesFreshLinkAfterFailureOrExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)

	testCases := []struct {
		name     string
		existing entities.TelegramManagedBot
	}{
		{
			name: "failed record creates new link",
			existing: entities.TelegramManagedBot{
				ID:            uuid.New(),
				Status:        entities.TelegramManagedBotStatusFailed,
				DeepLinkURL:   "https://t.me/simpleclaw_manager_bot?start=failed-code",
				LinkExpiresAt: now.Add(5 * time.Minute),
				CreatedAt:     now.Add(-2 * time.Minute),
				UpdatedAt:     now.Add(-2 * time.Minute),
			},
		},
		{
			name: "expired pending link creates new link",
			existing: entities.TelegramManagedBot{
				ID:            uuid.New(),
				Status:        entities.TelegramManagedBotStatusPendingLink,
				DeepLinkURL:   "https://t.me/simpleclaw_manager_bot?start=expired-code",
				LinkExpiresAt: now.Add(-time.Minute),
				CreatedAt:     now.Add(-2 * time.Minute),
				UpdatedAt:     now.Add(-2 * time.Minute),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newServiceStorage()
			svc := New(store, &serviceChannelStorage{}, &serviceTelegramAPI{}, Options{
				ManagerUsername: "simpleclaw_manager_bot",
				LinkTTL:         10 * time.Minute,
				Now: func() time.Time {
					return now
				},
			})

			userID := uuid.New()
			clawID := uuid.New()
			tc.existing.UserID = userID
			tc.existing.ClawID = clawID
			store.managedBots[tc.existing.ID] = tc.existing

			result, err := svc.CreateLink(context.Background(), CreateLinkCommand{
				UserID: userID,
				ClawID: clawID,
			})
			if err != nil {
				t.Fatalf("CreateLink() error = %v", err)
			}

			if result.ManagedBot.ID == tc.existing.ID {
				t.Fatalf("managed bot id = %s, want new id", result.ManagedBot.ID)
			}

			if len(store.links) != 1 {
				t.Fatalf("links len = %d, want 1", len(store.links))
			}
		})
	}
}

func TestServiceHandleUpdate_StartLinksTelegramUserAndSendsNewBotLink(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)
	store := newServiceStorage()
	channels := &serviceChannelStorage{}
	telegramAPI := &serviceTelegramAPI{
		managerUser: telegraminfra.User{
			ID:       42,
			IsBot:    true,
			Username: "simpleclaw_manager_bot",
		},
	}

	svc := New(store, channels, telegramAPI, Options{
		ManagerUsername: "simpleclaw_manager_bot",
		LinkTTL:         time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	userID := uuid.New()
	result, err := svc.CreateLink(context.Background(), CreateLinkCommand{UserID: userID, ClawID: uuid.New()})
	if err != nil {
		t.Fatalf("CreateLink() error = %v", err)
	}

	startCode := startCodeFromURL(t, result.DeepLinkURL)

	err = svc.HandleUpdate(context.Background(), telegraminfra.Update{
		UpdateID: 1,
		Message: &telegraminfra.Message{
			Text: "/start " + startCode,
			Chat: telegraminfra.Chat{ID: 2002},
			From: &telegraminfra.User{ID: 1001, Username: "snapclaw_user"},
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	link, err := store.GetByTelegramUserID(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetByTelegramUserID() error = %v", err)
	}

	if link.UserID != userID {
		t.Fatalf("user id = %s, want %s", link.UserID, userID)
	}

	if len(telegramAPI.sentMessages) != 1 {
		t.Fatalf("sent messages len = %d, want 1", len(telegramAPI.sentMessages))
	}

	if !strings.Contains(telegramAPI.sentMessages[0].Text, "https://t.me/newbot/simpleclaw_manager_bot") {
		t.Fatalf("message text = %q", telegramAPI.sentMessages[0].Text)
	}

	managedBot, err := store.GetManagedBotByID(context.Background(), result.ManagedBot.ID, userID)
	if err != nil {
		t.Fatalf("GetManagedBotByID() error = %v", err)
	}

	if managedBot.Status != entities.TelegramManagedBotStatusLinked {
		t.Fatalf("status = %q, want linked", managedBot.Status)
	}
}

func TestServiceHandleUpdate_ManagedBotCreatesChannel(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)
	store := newServiceStorage()
	channels := &serviceChannelStorage{}
	telegramAPI := &serviceTelegramAPI{
		managerUser: telegraminfra.User{
			ID:       42,
			IsBot:    true,
			Username: "simpleclaw_manager_bot",
		},
		managedBotToken: "3003:managed-token",
	}

	svc := New(store, channels, telegramAPI, Options{
		ManagerUsername: "simpleclaw_manager_bot",
		LinkTTL:         time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	userID := uuid.New()
	result, err := svc.CreateLink(context.Background(), CreateLinkCommand{UserID: userID, ClawID: uuid.New()})
	if err != nil {
		t.Fatalf("CreateLink() error = %v", err)
	}

	startCode := startCodeFromURL(t, result.DeepLinkURL)
	if err := svc.HandleUpdate(context.Background(), telegraminfra.Update{
		UpdateID: 1,
		Message: &telegraminfra.Message{
			Text: "/start " + startCode,
			Chat: telegraminfra.Chat{ID: 2002},
			From: &telegraminfra.User{ID: 1001, Username: "snapclaw_user"},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate() link error = %v", err)
	}

	err = svc.HandleUpdate(context.Background(), telegraminfra.Update{
		UpdateID: 2,
		ManagedBot: &telegraminfra.ManagedBotUpdated{
			User: telegraminfra.User{ID: 1001, Username: "snapclaw_user"},
			Bot: telegraminfra.User{
				ID:        3003,
				IsBot:     true,
				Username:  "snapclaw_helper_bot",
				FirstName: "Snapclaw Helper",
			},
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() managed bot error = %v", err)
	}

	if telegramAPI.managedBotTokenRequests != 1 {
		t.Fatalf("managed bot token requests = %d, want 1", telegramAPI.managedBotTokenRequests)
	}

	if len(channels.created) != 1 {
		t.Fatalf("created channels = %d, want 1", len(channels.created))
	}

	created := channels.created[0]
	if created.ChannelType != entities.ChannelTelegramType {
		t.Fatalf("channel type = %q", created.ChannelType)
	}

	if created.Config.Telegram == nil {
		t.Fatal("expected telegram config")
	}

	if created.Config.Telegram.BotToken != "3003:managed-token" {
		t.Fatalf("bot token = %q", created.Config.Telegram.BotToken)
	}

	if created.Config.Telegram.DmPolicy != entitychannels.DmAllowList {
		t.Fatalf("dm policy = %q", created.Config.Telegram.DmPolicy)
	}

	if len(created.Config.Telegram.AllowFrom) != 1 || created.Config.Telegram.AllowFrom[0] != "1001" {
		t.Fatalf("allow from = %#v", created.Config.Telegram.AllowFrom)
	}

	managedBot, err := store.GetManagedBotByID(context.Background(), result.ManagedBot.ID, userID)
	if err != nil {
		t.Fatalf("GetManagedBotByID() error = %v", err)
	}

	if managedBot.Status != entities.TelegramManagedBotStatusReady {
		t.Fatalf("status = %q, want ready", managedBot.Status)
	}

	if managedBot.ChannelID == uuid.Nil {
		t.Fatal("expected channel id to be set")
	}
}

func TestServiceHandleUpdate_IgnoresDuplicateUpdateID(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)
	store := newServiceStorage()
	channels := &serviceChannelStorage{}
	telegramAPI := &serviceTelegramAPI{
		managerUser: telegraminfra.User{
			ID:       42,
			IsBot:    true,
			Username: "simpleclaw_manager_bot",
		},
	}

	svc := New(store, channels, telegramAPI, Options{
		ManagerUsername: "simpleclaw_manager_bot",
		LinkTTL:         time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	store.recordedUpdates[1] = true

	err := svc.HandleUpdate(context.Background(), telegraminfra.Update{
		UpdateID: 1,
		Message: &telegraminfra.Message{
			Text: "/start ignored",
			Chat: telegraminfra.Chat{ID: 2002},
			From: &telegraminfra.User{ID: 1001, Username: "snapclaw_user"},
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	if len(telegramAPI.sentMessages) != 0 {
		t.Fatalf("sent messages len = %d, want 0", len(telegramAPI.sentMessages))
	}
}

func TestServiceHandleUpdate_UnknownManagedBotOwnerMarksRequestFailed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)
	store := newServiceStorage()
	channels := &serviceChannelStorage{}
	telegramAPI := &serviceTelegramAPI{
		managerUser: telegraminfra.User{
			ID:       42,
			IsBot:    true,
			Username: "simpleclaw_manager_bot",
		},
		managedBotToken: "3003:managed-token",
	}
	requestID := uuid.New()

	store.managedBots[requestID] = entities.TelegramManagedBot{
		ID:                  requestID,
		UserID:              uuid.New(),
		TelegramOwnerUserID: 1001,
		Status:              entities.TelegramManagedBotStatusLinked,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	svc := New(store, channels, telegramAPI, Options{
		ManagerUsername: "simpleclaw_manager_bot",
		LinkTTL:         time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	err := svc.HandleUpdate(context.Background(), telegraminfra.Update{
		UpdateID: 2,
		ManagedBot: &telegraminfra.ManagedBotUpdated{
			User: telegraminfra.User{ID: 1001, Username: "snapclaw_user"},
			Bot: telegraminfra.User{
				ID:        3003,
				IsBot:     true,
				Username:  "snapclaw_helper_bot",
				FirstName: "Snapclaw Helper",
			},
		},
	})
	if err != nil {
		t.Fatalf("HandleUpdate() error = %v", err)
	}

	managedBot, err := store.GetManagedBotByID(context.Background(), requestID, store.managedBots[requestID].UserID)
	if err != nil {
		t.Fatalf("GetManagedBotByID() error = %v", err)
	}

	if managedBot.Status != entities.TelegramManagedBotStatusFailed {
		t.Fatalf("status = %q, want failed", managedBot.Status)
	}

	if len(channels.created) != 0 {
		t.Fatalf("created channels = %d, want 0", len(channels.created))
	}
}

func TestServiceRunPolling_DisablesWebhookAndProcessesUpdates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 8, 15, 0, 0, 0, time.UTC)
	store := newServiceStorage()
	channels := &serviceChannelStorage{}
	telegramAPI := &serviceTelegramAPI{
		managerUser: telegraminfra.User{
			ID:       42,
			IsBot:    true,
			Username: "simpleclaw_manager_bot",
		},
		managedBotToken: "3003:managed-token",
	}

	svc := New(store, channels, telegramAPI, Options{
		ManagerUsername: "simpleclaw_manager_bot",
		LinkTTL:         time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	userID := uuid.New()
	result, err := svc.CreateLink(context.Background(), CreateLinkCommand{UserID: userID, ClawID: uuid.New()})
	if err != nil {
		t.Fatalf("CreateLink() error = %v", err)
	}
	startCode := startCodeFromURL(t, result.DeepLinkURL)
	telegramAPI.updates = []telegraminfra.Update{
		{
			UpdateID: 1,
			Message: &telegraminfra.Message{
				Text: "/start " + startCode,
				Chat: telegraminfra.Chat{ID: 2002},
				From: &telegraminfra.User{ID: 1001, Username: "snapclaw_user"},
			},
		},
		{
			UpdateID: 2,
			ManagedBot: &telegraminfra.ManagedBotUpdated{
				User: telegraminfra.User{ID: 1001, Username: "snapclaw_user"},
				Bot: telegraminfra.User{
					ID:        3003,
					IsBot:     true,
					Username:  "snapclaw_helper_bot",
					FirstName: "Snapclaw Helper",
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	telegramAPI.cancel = cancel

	err = svc.RunPolling(ctx, PollingOptions{
		TimeoutSeconds: 30,
		AllowedUpdates: []string{"message", "managed_bot"},
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("RunPolling() error = %v", err)
	}

	if !telegramAPI.deleteWebhookCalled {
		t.Fatal("expected DeleteWebhook to be called")
	}

	if telegramAPI.lastGetUpdates.Offset != 0 {
		t.Fatalf("offset = %d, want 0", telegramAPI.lastGetUpdates.Offset)
	}

	managedBot, err := store.GetManagedBotByID(context.Background(), result.ManagedBot.ID, userID)
	if err != nil {
		t.Fatalf("GetManagedBotByID() error = %v", err)
	}

	if managedBot.Status != entities.TelegramManagedBotStatusReady {
		t.Fatalf("status = %q, want ready", managedBot.Status)
	}
}

type serviceStorage struct {
	links           map[uuid.UUID]entities.TelegramAccountLink
	managedBots     map[uuid.UUID]entities.TelegramManagedBot
	recordedUpdates map[int64]bool
}

func newServiceStorage() *serviceStorage {
	return &serviceStorage{
		links:           make(map[uuid.UUID]entities.TelegramAccountLink),
		managedBots:     make(map[uuid.UUID]entities.TelegramManagedBot),
		recordedUpdates: make(map[int64]bool),
	}
}

func (s *serviceStorage) CreateLink(_ context.Context, link entities.TelegramAccountLink) error {
	s.links[link.ID] = link
	return nil
}

func (s *serviceStorage) ConsumeLink(
	_ context.Context,
	linkCodeHash string,
	activation entities.TelegramLinkActivation,
) (entities.TelegramAccountLink, error) {
	for id, link := range s.links {
		if link.LinkCodeHash == linkCodeHash && link.Status == entities.TelegramAccountLinkStatusPending && link.ExpiresAt.After(activation.LinkedAt) {
			link.TelegramUserID = activation.TelegramUserID
			link.TelegramChatID = activation.TelegramChatID
			link.TelegramUsername = activation.TelegramUsername
			link.Status = entities.TelegramAccountLinkStatusLinked
			link.UpdatedAt = activation.LinkedAt
			s.links[id] = link
			return link, nil
		}
	}

	return entities.TelegramAccountLink{}, errors.New("not found")
}

func (s *serviceStorage) GetByTelegramUserID(_ context.Context, telegramUserID int64) (entities.TelegramAccountLink, error) {
	for _, link := range s.links {
		if link.TelegramUserID == telegramUserID {
			return link, nil
		}
	}

	return entities.TelegramAccountLink{}, errors.New("not found")
}

func (s *serviceStorage) TryRecordWebhookUpdate(_ context.Context, updateID int64) (bool, error) {
	if s.recordedUpdates[updateID] {
		return false, nil
	}

	s.recordedUpdates[updateID] = true
	return true, nil
}

func (s *serviceStorage) UpsertManagedBot(_ context.Context, bot entities.TelegramManagedBot) error {
	s.managedBots[bot.ID] = bot
	return nil
}

func (s *serviceStorage) GetManagedBotByID(_ context.Context, id, userID uuid.UUID) (entities.TelegramManagedBot, error) {
	bot, ok := s.managedBots[id]
	if !ok || bot.UserID != userID {
		return entities.TelegramManagedBot{}, errors.New("not found")
	}

	return bot, nil
}

func (s *serviceStorage) GetLatestManagedBotByTelegramOwnerUserID(
	_ context.Context,
	telegramOwnerUserID int64,
) (entities.TelegramManagedBot, error) {
	var found entities.TelegramManagedBot
	for _, bot := range s.managedBots {
		if bot.TelegramOwnerUserID == telegramOwnerUserID && (found.ID == uuid.Nil || bot.UpdatedAt.After(found.UpdatedAt)) {
			found = bot
		}
	}

	if found.ID == uuid.Nil {
		return entities.TelegramManagedBot{}, errors.New("not found")
	}

	return found, nil
}

func (s *serviceStorage) GetLatestManagedBotByClawID(
	_ context.Context,
	userID uuid.UUID,
	clawID uuid.UUID,
) (entities.TelegramManagedBot, error) {
	var found entities.TelegramManagedBot
	for _, bot := range s.managedBots {
		if bot.UserID == userID && bot.ClawID == clawID && (found.ID == uuid.Nil || bot.UpdatedAt.After(found.UpdatedAt)) {
			found = bot
		}
	}

	if found.ID == uuid.Nil {
		return entities.TelegramManagedBot{}, errors.New("not found")
	}

	return found, nil
}

type serviceChannelStorage struct {
	created []entities.Channel
}

func (s *serviceChannelStorage) Create(_ context.Context, channel entities.Channel) error {
	s.created = append(s.created, channel)
	return nil
}

type serviceTelegramAPI struct {
	managerUser             telegraminfra.User
	getMeCalls              int
	managedBotToken         string
	managedBotTokenRequests int
	sentMessages            []serviceSentMessage
	updates                 []telegraminfra.Update
	lastGetUpdates          telegraminfra.GetUpdatesParams
	deleteWebhookCalled     bool
	cancel                  context.CancelFunc
}

type serviceSentMessage struct {
	ChatID int64
	Text   string
}

func (s *serviceTelegramAPI) GetMe(_ context.Context) (telegraminfra.User, error) {
	s.getMeCalls++
	if strings.TrimSpace(s.managerUser.Username) == "" {
		return telegraminfra.User{
			ID:       42,
			IsBot:    true,
			Username: "simpleclaw_manager_bot",
		}, nil
	}

	return s.managerUser, nil
}

func (s *serviceTelegramAPI) GetManagedBotToken(_ context.Context, userID int64) (string, error) {
	s.managedBotTokenRequests++
	if userID == 0 {
		return "", errors.New("invalid user id")
	}

	return s.managedBotToken, nil
}

func (s *serviceTelegramAPI) SendMessage(_ context.Context, chatID int64, text string) error {
	s.sentMessages = append(s.sentMessages, serviceSentMessage{
		ChatID: chatID,
		Text:   text,
	})
	return nil
}

func (s *serviceTelegramAPI) DeleteWebhook(_ context.Context, _ bool) error {
	s.deleteWebhookCalled = true
	return nil
}

func (s *serviceTelegramAPI) GetUpdates(_ context.Context, params telegraminfra.GetUpdatesParams) ([]telegraminfra.Update, error) {
	s.lastGetUpdates = params
	updates := append([]telegraminfra.Update(nil), s.updates...)
	s.updates = nil
	if s.cancel != nil {
		s.cancel()
	}

	return updates, nil
}

func startCodeFromURL(t *testing.T, raw string) string {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	code := u.Query().Get("start")
	if code == "" {
		t.Fatalf("start code missing in %q", raw)
	}

	return code
}

var _ channelStorage = (*serviceChannelStorage)(nil)
var _ telegramAPI = (*serviceTelegramAPI)(nil)
var _ storage = (*serviceStorage)(nil)
var _ = commands.TelegramChannel{}
