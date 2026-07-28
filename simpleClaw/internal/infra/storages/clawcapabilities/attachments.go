package clawcapabilities

import (
	"context"
	"encoding/json"
	"fmt"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Storage struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) WithDB(db *gorm.DB) *Storage {
	if db == nil {
		return s
	}

	copy := *s
	copy.db = db

	return &copy
}

func (s *Storage) Upsert(ctx context.Context, attachment entities.ClawCapabilityAttachment) error {
	const op = "storages.ClawCapabilities.Upsert"

	if attachment.ID == uuid.Nil || attachment.ClawID == uuid.Nil || attachment.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	settings, err := marshalJSON(attachment.Settings)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	model := models.ClawCapabilityAttachment{
		ID:                   attachment.ID,
		ClawID:               attachment.ClawID,
		UserID:               attachment.UserID,
		CapabilityID:         string(attachment.CapabilityID),
		Provider:             attachment.Provider,
		AccountIntegrationID: attachment.AccountIntegrationID,
		Enabled:              attachment.Enabled,
		Settings:             settings,
		CreatedAt:            attachment.CreatedAt,
		UpdatedAt:            attachment.UpdatedAt,
	}

	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"claw_id":                model.ClawID,
			"user_id":                model.UserID,
			"capability_id":          model.CapabilityID,
			"provider":               model.Provider,
			"account_integration_id": model.AccountIntegrationID,
			"enabled":                model.Enabled,
			"settings":               model.Settings,
			"updated_at":             model.UpdatedAt,
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) ListByClawID(
	ctx context.Context,
	clawID uuid.UUID,
	userID uuid.UUID,
) ([]entities.ClawCapabilityAttachment, error) {
	const op = "storages.ClawCapabilities.ListByClawID"

	if clawID == uuid.Nil || userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	rows := make([]models.ClawCapabilityAttachment, 0)
	if err := s.db.WithContext(ctx).
		Where("claw_id = ? AND user_id = ?", clawID, userID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	out := make([]entities.ClawCapabilityAttachment, 0, len(rows))
	for _, row := range rows {
		attachment, err := mapAttachment(row)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}

		out = append(out, attachment)
	}

	return out, nil
}

func (s *Storage) Delete(
	ctx context.Context,
	clawID uuid.UUID,
	userID uuid.UUID,
	capabilityID entities.CapabilityID,
) error {
	const op = "storages.ClawCapabilities.Delete"

	if clawID == uuid.Nil || userID == uuid.Nil || capabilityID == "" {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	tx := s.db.WithContext(ctx).
		Where("claw_id = ? AND user_id = ? AND capability_id = ?", clawID, userID, string(capabilityID)).
		Delete(&models.ClawCapabilityAttachment{})
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func mapAttachment(row models.ClawCapabilityAttachment) (entities.ClawCapabilityAttachment, error) {
	settings, err := unmarshalJSON(row.Settings)
	if err != nil {
		return entities.ClawCapabilityAttachment{}, err
	}

	return entities.ClawCapabilityAttachment{
		ID:                   row.ID,
		ClawID:               row.ClawID,
		UserID:               row.UserID,
		CapabilityID:         entities.CapabilityID(row.CapabilityID),
		Provider:             row.Provider,
		AccountIntegrationID: row.AccountIntegrationID,
		Enabled:              row.Enabled,
		Settings:             settings,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}, nil
}

func marshalJSON(payload map[string]any) (datatypes.JSON, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return datatypes.JSON(raw), nil
}

func unmarshalJSON(payload datatypes.JSON) (map[string]any, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}

	return out, nil
}
