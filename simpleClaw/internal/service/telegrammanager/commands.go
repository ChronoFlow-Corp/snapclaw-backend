package telegrammanager

import (
	"simpleClaw/internal/entities"

	"github.com/google/uuid"
)

type CreateLinkCommand struct {
	UserID uuid.UUID
	ClawID uuid.UUID
}

type CreateLinkResult struct {
	ManagedBot  entities.TelegramManagedBot
	DeepLinkURL string
}
