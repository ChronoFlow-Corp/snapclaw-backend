package integrations

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

func (s *Storage) Upsert(ctx context.Context, integration entities.AccountIntegration) error {
	const op = "storages.Integrations.Upsert"

	if integration.ID == uuid.Nil || integration.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	secretPayload, err := marshalJSON(integration.SecretPayload)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	metadata, err := marshalJSON(integration.Metadata)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	model := models.AccountIntegration{
		ID:                integration.ID,
		UserID:            integration.UserID,
		CapabilityID:      string(integration.CapabilityID),
		Provider:          integration.Provider,
		ExternalAccountID: integration.ExternalAccountID,
		DisplayName:       integration.DisplayName,
		Status:            string(integration.Status),
		SecretPayload:     secretPayload,
		Metadata:          metadata,
		CreatedAt:         integration.CreatedAt,
		UpdatedAt:         integration.UpdatedAt,
	}

	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"user_id":             model.UserID,
			"capability_id":       model.CapabilityID,
			"provider":            model.Provider,
			"external_account_id": model.ExternalAccountID,
			"display_name":        model.DisplayName,
			"status":              model.Status,
			"secret_payload":      model.SecretPayload,
			"metadata":            model.Metadata,
			"updated_at":          model.UpdatedAt,
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) ListByUserID(ctx context.Context, userID uuid.UUID) ([]entities.AccountIntegration, error) {
	const op = "storages.Integrations.ListByUserID"

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	rows := make([]models.AccountIntegration, 0)
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	out := make([]entities.AccountIntegration, 0, len(rows))
	for _, row := range rows {
		integration, err := mapIntegration(row)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}

		out = append(out, integration)
	}

	return out, nil
}

func (s *Storage) GetByID(ctx context.Context, id, userID uuid.UUID) (entities.AccountIntegration, error) {
	const op = "storages.Integrations.GetByID"

	if id == uuid.Nil || userID == uuid.Nil {
		return entities.AccountIntegration{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	var row models.AccountIntegration
	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&row).Error; err != nil {
		return entities.AccountIntegration{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	integration, err := mapIntegration(row)
	if err != nil {
		return entities.AccountIntegration{}, fmt.Errorf("%s: %w", op, err)
	}

	return integration, nil
}

func mapIntegration(row models.AccountIntegration) (entities.AccountIntegration, error) {
	secretPayload, err := unmarshalJSON(row.SecretPayload)
	if err != nil {
		return entities.AccountIntegration{}, err
	}

	metadata, err := unmarshalJSON(row.Metadata)
	if err != nil {
		return entities.AccountIntegration{}, err
	}

	return entities.AccountIntegration{
		ID:                row.ID,
		UserID:            row.UserID,
		CapabilityID:      entities.CapabilityID(row.CapabilityID),
		Provider:          row.Provider,
		ExternalAccountID: row.ExternalAccountID,
		DisplayName:       row.DisplayName,
		Status:            entities.AccountIntegrationStatus(row.Status),
		SecretPayload:     secretPayload,
		Metadata:          metadata,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
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
