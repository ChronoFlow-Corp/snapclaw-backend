package googleoauth

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	CapabilityGmail          = "gmail"
	CapabilityGoogleCalendar = "google_calendar"
	CapabilitySheets         = "sheets"
)

var (
	ErrUnsupportedCapability = errors.New("unsupported capability")
	ErrStateSecretRequired   = errors.New("state secret is required")
	ErrStateExpired          = errors.New("state expired")
	ErrStateInvalid          = errors.New("state is invalid")
	ErrReturnToInvalid       = errors.New("return_to is invalid")
)

type StatePayload struct {
	UserID       uuid.UUID `json:"userId"`
	Capabilities []string  `json:"capabilities"`
	ReturnTo     string    `json:"returnTo,omitempty"`
	IssuedAt     time.Time `json:"issuedAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
