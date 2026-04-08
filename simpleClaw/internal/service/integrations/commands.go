package integrations

import (
	"simpleClaw/internal/entities"

	"github.com/google/uuid"
)

type ConnectCommand struct {
	UserID            uuid.UUID
	ID                uuid.UUID
	Provider          string
	ExternalAccountID string
	DisplayName       string
	SecretPayload     map[string]any
	Metadata          map[string]any
}

func CapabilityFromProvider(provider string) entities.CapabilityID {
	switch provider {
	case "gmail":
		return entities.CapabilityGmail
	case "google_calendar":
		return entities.CapabilityGoogleCalendar
	case "notion":
		return entities.CapabilityNotion
	case "github":
		return entities.CapabilityGitHub
	case "sheets":
		return entities.CapabilitySheets
	case "linear":
		return entities.CapabilityLinear
	case "trello":
		return entities.CapabilityTrello
	default:
		return ""
	}
}
