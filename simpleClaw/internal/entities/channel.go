package entities

import (
	"time"

	"github.com/google/uuid"
)

const (
	ChannelTelegramType = "telegram"
	ChannelDiscordType  = "discord"
	ChannelWhatsappType = "whatsapp"
	ChannelSlackType    = "slack"
)

type Channel struct {
	ID          uuid.UUID
	ChannelType string
	Name        string
	Config      ClawChannels
	UserID      uuid.UUID
	CreatedAt   time.Time
}

func NewChannel(channelType string, name string, config ClawChannels, userID uuid.UUID) Channel {
	return Channel{
		ID:          uuid.New(),
		ChannelType: channelType,
		Name:        name,
		Config:      config,
		UserID:      userID,
		CreatedAt:   time.Now(),
	}
}
