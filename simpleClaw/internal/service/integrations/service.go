package integrations

import (
	"context"
	"errors"
	"strings"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"

	"github.com/google/uuid"
)

type storage interface {
	Upsert(ctx context.Context, integration entities.AccountIntegration) error
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]entities.AccountIntegration, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (entities.AccountIntegration, error)
}

type Service struct {
	storage storage
}

func NewService(storage storage) *Service {
	return &Service{storage: storage}
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]entities.AccountIntegration, error) {
	if userID == uuid.Nil {
		return nil, sql.ErrInvalid
	}

	return s.storage.ListByUserID(ctx, userID)
}

func (s *Service) Connect(ctx context.Context, cmd ConnectCommand) (entities.AccountIntegration, error) {
	if cmd.UserID == uuid.Nil {
		return entities.AccountIntegration{}, sql.ErrInvalid
	}

	provider := strings.TrimSpace(strings.ToLower(cmd.Provider))
	capabilityID := CapabilityFromProvider(provider)
	if capabilityID == "" {
		return entities.AccountIntegration{}, sql.ErrInvalid
	}

	integrationID := cmd.ID
	now := time.Now().UTC()
	if integrationID == uuid.Nil {
		integrationID = uuid.New()
	}

	integration := entities.AccountIntegration{
		ID:                integrationID,
		UserID:            cmd.UserID,
		CapabilityID:      capabilityID,
		Provider:          provider,
		ExternalAccountID: strings.TrimSpace(cmd.ExternalAccountID),
		DisplayName:       strings.TrimSpace(cmd.DisplayName),
		Status:            entities.AccountIntegrationStatusActive,
		SecretPayload:     cmd.SecretPayload,
		Metadata:          cmd.Metadata,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if existing, err := s.storage.GetByID(ctx, integrationID, cmd.UserID); err == nil {
		integration.CreatedAt = existing.CreatedAt
	} else if !errors.Is(err, sql.ErrNotFound) {
		return entities.AccountIntegration{}, err
	}

	if err := s.storage.Upsert(ctx, integration); err != nil {
		return entities.AccountIntegration{}, err
	}

	return integration, nil
}
