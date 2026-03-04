package claw

import (
	"context"
	"errors"
	"fmt"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/claw/commands"

	"github.com/google/uuid"
)

var (
	errUserIDRequired   = errors.New("user id is required")
	errNameRequired     = errors.New("name is required")
	errModelRequired    = errors.New("model is required")
	errChannelNotFound  = errors.New("channel not found")
	errHostingMissing   = errors.New("hosting manager is not configured")
	errOpenRouterClient = errors.New("openrouter manager is not configured")
)

type Service struct {
	claws    clawStorage
	channels channelStorage
	users    userStorage
	servers  serverStorage
	hosting  hostingManager
	keys     apiKeyManager
}

func NewClaw(
	claws clawStorage,
	channels channelStorage,
	users userStorage,
	servers serverStorage,
	hosting hostingManager,
	keys apiKeyManager,
) *Service {
	return &Service{
		claws:    claws,
		channels: channels,
		users:    users,
		servers:  servers,
		hosting:  hosting,
		keys:     keys,
	}
}

func (s *Service) Create(
	ctx context.Context,
	cm commands.CreateClaw,
) (entities.Claw, error) {
	const op = "service.Claw.Create"

	if cm.UserID == uuid.Nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errUserIDRequired)
	}

	if cm.Name == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errNameRequired)
	}

	if cm.Model == "" {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errModelRequired)
	}

	if s.hosting == nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, errHostingMissing)
	}

	user, err := s.users.GetByID(ctx, cm.UserID)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	server, err := s.servers.GetAvailable(ctx)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	channelIDs := deduplicateUUIDs(cm.ChannelIDs)
	var chs []entities.Channel
	if len(channelIDs) > 0 {
		chs, err = s.channels.GetByIDs(ctx, channelIDs, cm.UserID)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}

		if len(chs) != len(channelIDs) {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, errChannelNotFound)
		}
	}

	keyValue := user.OpenRouterApiKey
	if keyValue == "" || user.OpenRouterKeyID == "" {
		if s.keys == nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, errOpenRouterClient)
		}

		rpm, monthly := normalizeLimits(cm.ApiKeyLimit)
		apiKey, err := s.keys.Create(ctx, user.ID, cm.Name, rpm, monthly)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}

		err = s.users.UpdateOpenRouterKey(ctx, user.ID, apiKey)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}

		keyValue = apiKey.Secret
	}

	primaryModel, err := s.keys.ResolveModel(ctx, cm.Model)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	gatewayToken := generateGatewayToken()
	cfg := buildBaseConfig(primaryModel, keyValue, chs, gatewayToken)

	now := time.Now()
	cl := entities.Claw{
		ID:        uuid.New(),
		Name:      cm.Name,
		UserID:    user.ID,
		ServerID:  server.ID,
		Status:    entities.StatusStop,
		Config:    cfg,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err = s.claws.Create(ctx, cl, channelIDs); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	container, err := s.hosting.Create(ctx, cl, server)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	if container.ServerID != uuid.Nil {
		cl.ServerID = container.ServerID
	}
	cl.ContainerID = container.ID
	if container.Status != "" {
		cl.Status = container.Status
	} else {
		cl.Status = entities.StatusStop
	}

	if err = s.claws.UpdateRuntime(ctx, cl.ID, cl.ServerID, cl.ContainerID, cl.Status); err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
	}

	return cl, nil
}

func (s *Service) Start() {
	const op = "service.Service.Start"
	_ = op
}

func (s *Service) Stop() {
	const op = "service.Service.Stop"
	_ = op
}

func (s *Service) Delete() {
	const op = "service.Service.Delete"
	_ = op
}

func buildBaseConfig(
	primaryModel string,
	apiKey string,
	channels []entities.Channel,
	gatewayToken string,
) entities.ClawConfig {
	return entities.ClawConfig{
		Env: entities.Env{
			OpenRouterAPIKey: apiKey,
		},
		Channels: mergeChannelConfigs(channels),
		Agents: entities.Agents{
			Defaults: entities.AgentDefaults{
				Model: entities.AgentModelSelection{Primary: primaryModel},
			},
		},
		Gateway: entities.GatewayConfig{
			Mode: "token",
			Auth: entities.GatewayAuth{
				Mode:  "token",
				Token: gatewayToken,
			},
		},
	}
}

func mergeChannelConfigs(chs []entities.Channel) entities.ClawChannels {
	var cfg entities.ClawChannels

	for _, ch := range chs {
		switch ch.ChannelType {
		case entities.ChannelTelegramType:
			if ch.Config.Telegram != nil {
				tg := *ch.Config.Telegram
				tg.Enabled = true
				cfg.Telegram = &tg
			}
		case entities.ChannelDiscordType:
			if ch.Config.Discord != nil {
				discord := *ch.Config.Discord
				discord.Enabled = true
				cfg.Discord = &discord
			}
		case entities.ChannelWhatsappType:
			if ch.Config.WhatsApp != nil {
				wa := *ch.Config.WhatsApp
				cfg.WhatsApp = &wa
			}
		case entities.ChannelSlackType:
			if ch.Config.Slack != nil {
				slack := *ch.Config.Slack
				cfg.Slack = &slack
			}
		}
	}

	return cfg
}

func normalizeLimits(l commands.ApiKeyLimits) (int, float64) {
	rpm := l.RequestsPerMinute
	if rpm <= 0 {
		rpm = 60
	}

	monthly := l.MonthlyBudgetUSD
	if monthly <= 0 {
		monthly = 50
	}

	return rpm, monthly
}

func deduplicateUUIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) == 0 {
		return nil
	}

	seen := make(map[uuid.UUID]struct{}, len(ids))
	result := make([]uuid.UUID, 0, len(ids))

	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}

		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		result = append(result, id)
	}

	return result
}

func generateGatewayToken() string {
	return uuid.NewString()
}
