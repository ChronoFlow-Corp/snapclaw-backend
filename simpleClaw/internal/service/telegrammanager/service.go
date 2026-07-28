package telegrammanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"simpleClaw/internal/entities"
	entitychannels "simpleClaw/internal/entities/channels"
	infraSQL "simpleClaw/internal/infra/sql"
	telegraminfra "simpleClaw/internal/infra/telegram"
	userservice "simpleClaw/internal/service/user"
	"simpleClaw/internal/service/user/commands"

	"github.com/google/uuid"
)

const defaultLinkTTL = 10 * time.Minute

type storage interface {
	CreateLink(ctx context.Context, link entities.TelegramAccountLink) error
	ConsumeLink(ctx context.Context, linkCodeHash string, activation entities.TelegramLinkActivation) (entities.TelegramAccountLink, error)
	GetByTelegramUserID(ctx context.Context, telegramUserID int64) (entities.TelegramAccountLink, error)
	TryRecordWebhookUpdate(ctx context.Context, updateID int64) (bool, error)
	UpsertManagedBot(ctx context.Context, bot entities.TelegramManagedBot) error
	GetManagedBotByID(ctx context.Context, id, userID uuid.UUID) (entities.TelegramManagedBot, error)
	GetLatestManagedBotByTelegramOwnerUserID(ctx context.Context, telegramOwnerUserID int64) (entities.TelegramManagedBot, error)
	GetLatestManagedBotByClawID(ctx context.Context, userID, clawID uuid.UUID) (entities.TelegramManagedBot, error)
}

type channelStorage interface {
	Create(ctx context.Context, channel entities.Channel) error
}

type telegramAPI interface {
	GetMe(ctx context.Context) (telegraminfra.User, error)
	DeleteWebhook(ctx context.Context, dropPendingUpdates bool) error
	GetUpdates(ctx context.Context, params telegraminfra.GetUpdatesParams) ([]telegraminfra.Update, error)
	GetManagedBotToken(ctx context.Context, userID int64) (string, error)
	SendMessage(ctx context.Context, chatID int64, text string) error
}

type Options struct {
	ManagerUsername string
	LinkTTL         time.Duration
	Now             func() time.Time
}

type PollingOptions struct {
	TimeoutSeconds int
	AllowedUpdates []string
}

type Service struct {
	st                       storage
	channels                 channelStorage
	telegram                 telegramAPI
	managerUsername          string
	resolvedManagerUsername  string
	managerUsernameResolveMu sync.RWMutex
	linkTTL                  time.Duration
	now                      func() time.Time
}

func New(st storage, channels channelStorage, telegram telegramAPI, opts Options) *Service {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	linkTTL := opts.LinkTTL
	if linkTTL <= 0 {
		linkTTL = defaultLinkTTL
	}

	return &Service{
		st:              st,
		channels:        channels,
		telegram:        telegram,
		managerUsername: strings.TrimSpace(strings.TrimPrefix(opts.ManagerUsername, "@")),
		linkTTL:         linkTTL,
		now:             nowFn,
	}
}

func (s *Service) CreateLink(ctx context.Context, cmd CreateLinkCommand) (CreateLinkResult, error) {
	const op = "service.TelegramManager.CreateLink"

	if cmd.UserID == uuid.Nil {
		return CreateLinkResult{}, fmt.Errorf("%s: user id is required", op)
	}
	if cmd.ClawID == uuid.Nil {
		return CreateLinkResult{}, fmt.Errorf("%s: %w", op, infraSQL.ErrInvalid)
	}

	managerUsername, err := s.resolveManagerUsername(ctx)
	if err != nil {
		return CreateLinkResult{}, fmt.Errorf("%s: %w", op, err)
	}

	now := s.now().UTC()
	existing, err := s.st.GetLatestManagedBotByClawID(ctx, cmd.UserID, cmd.ClawID)
	switch {
	case err == nil && shouldReuseManagedBot(existing, now) && isDeepLinkForManager(existing.DeepLinkURL, managerUsername):
		return CreateLinkResult{
			ManagedBot:  existing,
			DeepLinkURL: existing.DeepLinkURL,
		}, nil
	case err == nil:
	case err != nil && !isNotFound(err):
		return CreateLinkResult{}, fmt.Errorf("%s: %w", op, err)
	}

	id := uuid.New()
	code := uuid.NewString()
	codeHash := hashCode(code)
	deepLinkURL := buildStartLink(managerUsername, code)
	linkExpiresAt := now.Add(s.linkTTL)

	link := entities.TelegramAccountLink{
		ID:           id,
		UserID:       cmd.UserID,
		LinkCodeHash: codeHash,
		Status:       entities.TelegramAccountLinkStatusPending,
		ExpiresAt:    linkExpiresAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.st.CreateLink(ctx, link); err != nil {
		return CreateLinkResult{}, fmt.Errorf("%s: %w", op, err)
	}

	managedBot := entities.TelegramManagedBot{
		ID:            id,
		UserID:        cmd.UserID,
		ClawID:        cmd.ClawID,
		DeepLinkURL:   deepLinkURL,
		LinkExpiresAt: linkExpiresAt,
		Status:        entities.TelegramManagedBotStatusPendingLink,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.st.UpsertManagedBot(ctx, managedBot); err != nil {
		return CreateLinkResult{}, fmt.Errorf("%s: %w", op, err)
	}

	return CreateLinkResult{
		ManagedBot:  managedBot,
		DeepLinkURL: deepLinkURL,
	}, nil
}

func (s *Service) GetManagedBot(ctx context.Context, id, userID uuid.UUID) (entities.TelegramManagedBot, error) {
	return s.st.GetManagedBotByID(ctx, id, userID)
}

func (s *Service) HandleUpdate(ctx context.Context, update telegraminfra.Update) error {
	const op = "service.TelegramManager.HandleUpdate"

	if update.UpdateID == 0 {
		return nil
	}

	recorded, err := s.st.TryRecordWebhookUpdate(ctx, update.UpdateID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if !recorded {
		return nil
	}

	switch {
	case update.Message != nil:
		return s.handleMessageUpdate(ctx, *update.Message)
	case update.ManagedBot != nil:
		return s.handleManagedBotUpdate(ctx, *update.ManagedBot)
	default:
		return nil
	}
}

func (s *Service) RunPolling(ctx context.Context, opts PollingOptions) error {
	if s.telegram == nil {
		return fmt.Errorf("service.TelegramManager.RunPolling: telegram api is required")
	}

	if err := s.telegram.DeleteWebhook(ctx, false); err != nil {
		return err
	}

	offset := int64(0)
	timeoutSeconds := opts.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		updates, err := s.telegram.GetUpdates(ctx, telegraminfra.GetUpdatesParams{
			Offset:         offset,
			TimeoutSeconds: timeoutSeconds,
			AllowedUpdates: append([]string(nil), opts.AllowedUpdates...),
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return err
		}

		for _, update := range updates {
			if err := s.HandleUpdate(ctx, update); err != nil {
				return err
			}

			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
		}
	}
}

func (s *Service) handleMessageUpdate(ctx context.Context, message telegraminfra.Message) error {
	startCode, ok := parseStartCode(message.Text)
	if !ok || message.From == nil {
		return nil
	}

	now := s.now().UTC()
	link, err := s.st.ConsumeLink(ctx, hashCode(startCode), entities.TelegramLinkActivation{
		TelegramUserID:   message.From.ID,
		TelegramChatID:   message.Chat.ID,
		TelegramUsername: message.From.Username,
		LinkedAt:         now,
	})
	if err != nil {
		if isNotFound(err) {
			return nil
		}

		return err
	}

	managedBot, err := s.st.GetManagedBotByID(ctx, link.ID, link.UserID)
	if err != nil && !isNotFound(err) {
		return err
	}

	managedBot.ID = link.ID
	managedBot.UserID = link.UserID
	managedBot.TelegramOwnerUserID = link.TelegramUserID
	managedBot.Status = entities.TelegramManagedBotStatusLinked
	if managedBot.CreatedAt.IsZero() {
		managedBot.CreatedAt = link.CreatedAt
	}
	managedBot.UpdatedAt = now

	if err := s.st.UpsertManagedBot(ctx, managedBot); err != nil {
		return err
	}

	managerUsername := s.currentManagerUsername()
	if managerUsername == "" {
		var err error
		managerUsername, err = s.resolveManagerUsername(ctx)
		if err != nil {
			return err
		}
	}

	if s.telegram != nil && message.Chat.ID != 0 {
		if err := s.telegram.SendMessage(ctx, message.Chat.ID, buildNewBotMessage(managerUsername)); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) handleManagedBotUpdate(ctx context.Context, update telegraminfra.ManagedBotUpdated) error {
	now := s.now().UTC()
	ownerID := update.User.ID
	if ownerID == 0 || update.Bot.ID == 0 {
		return nil
	}

	link, err := s.st.GetByTelegramUserID(ctx, ownerID)
	if err != nil {
		if isNotFound(err) {
			active, activeErr := s.st.GetLatestManagedBotByTelegramOwnerUserID(ctx, ownerID)
			if activeErr == nil {
				active.Status = entities.TelegramManagedBotStatusFailed
				active.LastError = "telegram account link not found"
				active.UpdatedAt = now
				if upsertErr := s.st.UpsertManagedBot(ctx, active); upsertErr != nil {
					return upsertErr
				}
			}

			return nil
		}

		return err
	}

	managedBot, err := s.st.GetLatestManagedBotByTelegramOwnerUserID(ctx, ownerID)
	if err != nil {
		return err
	}

	token, err := s.telegram.GetManagedBotToken(ctx, update.Bot.ID)
	if err != nil {
		managedBot.Status = entities.TelegramManagedBotStatusFailed
		managedBot.LastError = err.Error()
		managedBot.UpdatedAt = now
		_ = s.st.UpsertManagedBot(ctx, managedBot)

		return err
	}

	channelName := strings.TrimSpace(update.Bot.FirstName)
	if channelName == "" {
		channelName = strings.TrimSpace(update.Bot.Username)
	}
	if channelName == "" {
		channelName = "Telegram"
	}

	channel := userservice.NewTelegramChannel(channelName, commands.TelegramChannel{
		DmPolicy:  string(entitychannels.DmAllowList),
		BotToken:  token,
		AllowFrom: []string{strconv.FormatInt(ownerID, 10)},
	}, link.UserID)

	if err := s.channels.Create(ctx, channel); err != nil {
		return err
	}

	managedBot.UserID = link.UserID
	managedBot.TelegramOwnerUserID = ownerID
	managedBot.ManagedBotUserID = update.Bot.ID
	managedBot.ManagedBotUsername = strings.TrimSpace(update.Bot.Username)
	managedBot.ManagedBotName = channelName
	managedBot.ChannelID = channel.ID
	managedBot.Status = entities.TelegramManagedBotStatusReady
	managedBot.LastError = ""
	if managedBot.CreatedAt.IsZero() {
		managedBot.CreatedAt = now
	}
	managedBot.UpdatedAt = now

	if err := s.st.UpsertManagedBot(ctx, managedBot); err != nil {
		return err
	}

	return nil
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

func parseStartCode(text string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) < 2 {
		return "", false
	}

	command := fields[0]
	if !strings.HasPrefix(command, "/start") {
		return "", false
	}

	code := strings.TrimSpace(fields[1])
	if code == "" {
		return "", false
	}

	return code, true
}

func buildStartLink(managerUsername, code string) string {
	return "https://t.me/" + managerUsername + "?start=" + url.QueryEscape(code)
}

func buildNewBotMessage(managerUsername string) string {
	return "Create your managed bot: https://t.me/newbot/" + managerUsername
}

func (s *Service) resolveManagerUsername(ctx context.Context) (string, error) {
	s.managerUsernameResolveMu.RLock()
	if s.resolvedManagerUsername != "" {
		username := s.resolvedManagerUsername
		s.managerUsernameResolveMu.RUnlock()
		return username, nil
	}
	s.managerUsernameResolveMu.RUnlock()

	s.managerUsernameResolveMu.Lock()
	defer s.managerUsernameResolveMu.Unlock()

	if s.resolvedManagerUsername != "" {
		return s.resolvedManagerUsername, nil
	}

	configuredUsername := strings.TrimSpace(strings.TrimPrefix(s.managerUsername, "@"))
	if s.telegram == nil {
		if configuredUsername == "" {
			return "", errors.New("telegram manager username is empty")
		}

		s.resolvedManagerUsername = configuredUsername
		return s.resolvedManagerUsername, nil
	}

	me, err := s.telegram.GetMe(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve telegram manager bot username: %w", err)
	}

	actualUsername := strings.TrimSpace(strings.TrimPrefix(me.Username, "@"))
	if actualUsername == "" {
		return "", errors.New("resolve telegram manager bot username: bot username is empty")
	}

	if configuredUsername != "" && actualUsername != configuredUsername {
		return "", fmt.Errorf(
			"configured manager username %q does not match bot username %q",
			configuredUsername,
			actualUsername,
		)
	}

	s.resolvedManagerUsername = actualUsername
	return s.resolvedManagerUsername, nil
}

func (s *Service) currentManagerUsername() string {
	s.managerUsernameResolveMu.RLock()
	defer s.managerUsernameResolveMu.RUnlock()

	if s.resolvedManagerUsername != "" {
		return s.resolvedManagerUsername
	}

	return strings.TrimSpace(strings.TrimPrefix(s.managerUsername, "@"))
}

func shouldReuseManagedBot(bot entities.TelegramManagedBot, now time.Time) bool {
	switch bot.Status {
	case entities.TelegramManagedBotStatusWaitingCreation:
		return strings.TrimSpace(bot.DeepLinkURL) != ""
	case entities.TelegramManagedBotStatusPendingLink:
		return strings.TrimSpace(bot.DeepLinkURL) != "" &&
			!bot.LinkExpiresAt.IsZero() &&
			bot.LinkExpiresAt.After(now)
	default:
		return false
	}
}

func isDeepLinkForManager(rawURL, managerUsername string) bool {
	if strings.TrimSpace(rawURL) == "" || strings.TrimSpace(managerUsername) == "" {
		return false
	}

	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}

	return strings.EqualFold(strings.TrimPrefix(parsed.Path, "/"), strings.TrimPrefix(managerUsername, "@"))
}

func isNotFound(err error) bool {
	return errors.Is(err, infraSQL.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found")
}
