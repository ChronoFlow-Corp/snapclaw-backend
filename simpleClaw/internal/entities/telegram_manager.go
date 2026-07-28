package entities

import (
	"time"

	"github.com/google/uuid"
)

type TelegramAccountLinkStatus string

const (
	TelegramAccountLinkStatusPending TelegramAccountLinkStatus = "pending"
	TelegramAccountLinkStatusLinked  TelegramAccountLinkStatus = "linked"
)

type TelegramManagedBotStatus string

const (
	TelegramManagedBotStatusPendingLink     TelegramManagedBotStatus = "pending_link"
	TelegramManagedBotStatusLinked          TelegramManagedBotStatus = "linked"
	TelegramManagedBotStatusWaitingCreation TelegramManagedBotStatus = "waiting_creation"
	TelegramManagedBotStatusReady           TelegramManagedBotStatus = "ready"
	TelegramManagedBotStatusFailed          TelegramManagedBotStatus = "failed"
)

type TelegramAccountLink struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	LinkCodeHash     string
	TelegramUserID   int64
	TelegramChatID   int64
	TelegramUsername string
	Status           TelegramAccountLinkStatus
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TelegramLinkActivation struct {
	TelegramUserID   int64
	TelegramChatID   int64
	TelegramUsername string
	LinkedAt         time.Time
}

type TelegramManagedBot struct {
	ID                  uuid.UUID
	UserID              uuid.UUID
	ClawID              uuid.UUID
	TelegramOwnerUserID int64
	ManagedBotUserID    int64
	ManagedBotUsername  string
	ManagedBotName      string
	DeepLinkURL         string
	LinkExpiresAt       time.Time
	ChannelID           uuid.UUID
	Status              TelegramManagedBotStatus
	LastError           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
