package entities

import (
	"time"

	"github.com/google/uuid"
)

type EmailStatus string

const (
	EmailStatusPending EmailStatus = "pending"
	EmailStatusSent    EmailStatus = "sent"
	EmailStatusFailed  EmailStatus = "failed"
)

type EmailCategory string

const (
	EmailCategoryWelcome        EmailCategory = "welcome"
	EmailCategoryPremiumGranted EmailCategory = "premium_granted"
	EmailCategoryTopUp          EmailCategory = "top_up"
	EmailCategorySupportReply   EmailCategory = "support_reply"
	EmailCategoryAnnouncement   EmailCategory = "announcement"
)

// OutboxEmail is a durable, queued email awaiting delivery by the dispatcher.
type OutboxEmail struct {
	ID                uuid.UUID
	ToAddress         string
	Subject           string
	HTMLBody          string
	TextBody          string
	Headers           map[string]string
	Category          EmailCategory
	Status            EmailStatus
	Attempts          int
	MaxAttempts       int
	LastError         string
	ProviderMessageID string
	NextAttemptAt     time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SentAt            *time.Time
}
