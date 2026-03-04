package commands

import "github.com/google/uuid"

type AddChannel struct {
	UserID   uuid.UUID
	Name     string
	Telegram *TelegramChannel
}

type TelegramChannel struct {
	DmPolicy  string
	BotToken  string
	AllowFrom []string
}

type RemoveChannel struct {
	UserID    uuid.UUID
	ChannelID uuid.UUID
}

type UpdateChannel struct {
	UserID    uuid.UUID
	ChannelID uuid.UUID
	Name      string
	Telegram  *TelegramChannel
}
