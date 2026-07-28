package clawcapability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	clawcapabilitystorage "simpleClaw/internal/infra/storages/clawcapabilities"
	clawstorage "simpleClaw/internal/infra/storages/claws"
	integrationstorage "simpleClaw/internal/infra/storages/integrations"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type clawStorage interface {
	GetByID(ctx context.Context, id, userID uuid.UUID) (entities.Claw, error)
	Update(ctx context.Context, cl entities.Claw, channelIDs []uuid.UUID, replaceChannels bool) error
}

type attachmentStorage interface {
	Upsert(ctx context.Context, attachment entities.ClawCapabilityAttachment) error
	ListByClawID(ctx context.Context, clawID, userID uuid.UUID) ([]entities.ClawCapabilityAttachment, error)
	Delete(ctx context.Context, clawID, userID uuid.UUID, capabilityID entities.CapabilityID) error
}

type integrationStorage interface {
	GetByID(ctx context.Context, id, userID uuid.UUID) (entities.AccountIntegration, error)
}

type AttachCommand struct {
	UserID               uuid.UUID
	ClawID               uuid.UUID
	CapabilityID         entities.CapabilityID
	Provider             string
	AccountIntegrationID *uuid.UUID
	Enabled              bool
	Settings             map[string]any
}

type Service struct {
	claws        clawStorage
	attachments  attachmentStorage
	integrations integrationStorage
	tx           *transactionResources
}

type transactionResources struct {
	db           *gorm.DB
	claws        *clawstorage.Storage
	attachments  *clawcapabilitystorage.Storage
	integrations *integrationstorage.Storage
}

func NewService(
	claws *clawstorage.Storage,
	attachments *clawcapabilitystorage.Storage,
	integrations *integrationstorage.Storage,
) *Service {
	return &Service{
		claws:        claws,
		attachments:  attachments,
		integrations: integrations,
		tx: &transactionResources{
			db:           claws.DB(),
			claws:        claws,
			attachments:  attachments,
			integrations: integrations,
		},
	}
}

func newService(
	claws clawStorage,
	attachments attachmentStorage,
	integrations integrationStorage,
) *Service {
	return &Service{
		claws:        claws,
		attachments:  attachments,
		integrations: integrations,
	}
}

func (s *Service) Attach(ctx context.Context, cmd AttachCommand) error {
	return s.runInTransaction(ctx, func(claws clawStorage, attachments attachmentStorage, integrations integrationStorage) error {
		if cmd.UserID == uuid.Nil || cmd.ClawID == uuid.Nil || cmd.CapabilityID == "" {
			return sql.ErrInvalid
		}

		cl, err := claws.GetByID(ctx, cmd.ClawID, cmd.UserID)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		createdAt := now
		attachmentID := uuid.New()
		existing, err := attachments.ListByClawID(ctx, cmd.ClawID, cmd.UserID)
		if err != nil {
			return err
		}

		for _, item := range existing {
			if item.CapabilityID == cmd.CapabilityID {
				attachmentID = item.ID
				createdAt = item.CreatedAt
				break
			}
		}

		provider := strings.TrimSpace(cmd.Provider)
		if requiresAccountIntegration(cmd.CapabilityID) && cmd.Enabled {
			if cmd.AccountIntegrationID == nil || *cmd.AccountIntegrationID == uuid.Nil {
				return sql.ErrInvalid
			}

			integration, err := integrations.GetByID(ctx, *cmd.AccountIntegrationID, cmd.UserID)
			if err != nil {
				return err
			}

			if integration.CapabilityID != cmd.CapabilityID {
				return sql.ErrInvalid
			}

			if provider != "" && !strings.EqualFold(provider, integration.Provider) {
				return sql.ErrInvalid
			}

			provider = integration.Provider
		}

		if err := attachments.Upsert(ctx, entities.ClawCapabilityAttachment{
			ID:                   attachmentID,
			ClawID:               cmd.ClawID,
			UserID:               cmd.UserID,
			CapabilityID:         cmd.CapabilityID,
			Provider:             provider,
			AccountIntegrationID: cmd.AccountIntegrationID,
			Enabled:              cmd.Enabled,
			Settings:             cmd.Settings,
			CreatedAt:            createdAt,
			UpdatedAt:            now,
		}); err != nil {
			return err
		}

		markCapabilityOnConfig(&cl.Config, cmd.CapabilityID, cmd.Enabled)
		cl.UpdatedAt = now

		return claws.Update(ctx, cl, nil, false)
	})
}

func (s *Service) Detach(ctx context.Context, userID, clawID uuid.UUID, capabilityID entities.CapabilityID) error {
	return s.runInTransaction(ctx, func(claws clawStorage, attachments attachmentStorage, integrations integrationStorage) error {
		if userID == uuid.Nil || clawID == uuid.Nil || capabilityID == "" {
			return sql.ErrInvalid
		}

		cl, err := claws.GetByID(ctx, clawID, userID)
		if err != nil {
			return err
		}

		existing, err := attachments.ListByClawID(ctx, clawID, userID)
		if err != nil {
			return err
		}

		attached := false
		for _, item := range existing {
			if item.CapabilityID == capabilityID {
				attached = true
				break
			}
		}

		if !attached {
			return sql.ErrNotFound
		}

		if err := attachments.Delete(ctx, clawID, userID, capabilityID); err != nil {
			return err
		}

		markCapabilityOnConfig(&cl.Config, capabilityID, false)
		cl.UpdatedAt = time.Now().UTC()

		return claws.Update(ctx, cl, nil, false)
	})
}

func ParseCapabilityID(raw string) (entities.CapabilityID, error) {
	switch entities.CapabilityID(raw) {
	case entities.CapabilityWebSearch,
		entities.CapabilityFilesImages,
		entities.CapabilityMemory,
		entities.CapabilityGmail,
		entities.CapabilityGoogleCalendar,
		entities.CapabilityNotion,
		entities.CapabilityGitHub,
		entities.CapabilitySheets,
		entities.CapabilityLinear,
		entities.CapabilityTrello:
		return entities.CapabilityID(raw), nil
	default:
		return "", fmt.Errorf("%w: unsupported capability", sql.ErrInvalid)
	}
}

func requiresAccountIntegration(capabilityID entities.CapabilityID) bool {
	switch capabilityID {
	case entities.CapabilityGmail,
		entities.CapabilityGoogleCalendar,
		entities.CapabilityNotion,
		entities.CapabilityGitHub,
		entities.CapabilitySheets,
		entities.CapabilityLinear,
		entities.CapabilityTrello:
		return true
	default:
		return false
	}
}

func markCapabilityOnConfig(cfg *entities.ClawConfig, capabilityID entities.CapabilityID, enabled bool) {
	if cfg == nil {
		return
	}

	switch capabilityID {
	case entities.CapabilityGmail:
		if enabled {
			if cfg.Hooks == nil {
				cfg.Hooks = &entities.HooksConfig{}
			}

			if cfg.Hooks.Gmail.Serve.Path == "" {
				cfg.Hooks.Gmail.Serve.Path = "/gmail-pubsub"
			}
		} else if cfg.Hooks != nil {
			cfg.Hooks.Gmail = entities.GmailHookConfig{}
			if len(cfg.Hooks.Mappings) == 0 && len(cfg.Hooks.Presets) == 0 &&
				cfg.Hooks.Path == "" && cfg.Hooks.Token == "" && !cfg.Hooks.Enabled {
				cfg.Hooks = nil
			}
		}
	}
}

func (s *Service) runInTransaction(
	ctx context.Context,
	fn func(claws clawStorage, attachments attachmentStorage, integrations integrationStorage) error,
) error {
	if s.tx == nil || s.tx.db == nil {
		return fn(s.claws, s.attachments, s.integrations)
	}

	return s.tx.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(
			s.tx.claws.WithDB(tx),
			s.tx.attachments.WithDB(tx),
			s.tx.integrations.WithDB(tx),
		)
	})
}
