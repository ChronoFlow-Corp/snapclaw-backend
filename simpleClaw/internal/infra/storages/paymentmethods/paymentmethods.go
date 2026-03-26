package paymentmethods

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"
)

type Storage struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) Create(ctx context.Context, method entities.PaymentMethod) error {
	const op = "storages.PaymentMethods.Create"

	if err := validatePaymentMethod(method); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	model := mapToModel(method)
	if err := gorm.G[models.PaymentMethod](s.db).Create(ctx, &model); err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) GetByID(
	ctx context.Context,
	id, userID uuid.UUID,
) (entities.PaymentMethod, error) {
	const op = "storages.PaymentMethods.GetByID"

	if err := validateIdentity(id, userID); err != nil {
		return entities.PaymentMethod{}, fmt.Errorf("%s: %w", op, err)
	}

	method, err := gorm.G[models.PaymentMethod](s.db).
		Where("id = ? AND user_id = ?", id, userID).
		First(ctx)
	if err != nil {
		return entities.PaymentMethod{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapToEntity(method), nil
}

func (s *Storage) GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.PaymentMethod, error) {
	const op = "storages.PaymentMethods.GetByUserID"

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	rows, err := gorm.G[models.PaymentMethod](s.db).
		Where("user_id = ?", userID).
		Order("is_default DESC").
		Order("created_at DESC").
		Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	methods := make([]entities.PaymentMethod, 0, len(rows))
	for _, row := range rows {
		methods = append(methods, mapToEntity(row))
	}

	return methods, nil
}

func (s *Storage) UpdateDefault(
	ctx context.Context,
	id, userID uuid.UUID,
	isDefault bool,
) error {
	const op = "storages.PaymentMethods.UpdateDefault"

	if err := validateIdentity(id, userID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	updateTx := s.db.WithContext(ctx).
		Model(&models.PaymentMethod{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{"is_default": isDefault})
	if updateTx.Error != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(updateTx.Error))
	}

	if updateTx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) Delete(ctx context.Context, id, userID uuid.UUID) error {
	const op = "storages.PaymentMethods.Delete"

	if err := validateIdentity(id, userID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	affected, err := gorm.G[models.PaymentMethod](s.db).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if affected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) ClearDefaultByUserID(ctx context.Context, userID uuid.UUID) error {
	const op = "storages.PaymentMethods.ClearDefaultByUserID"

	if userID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	updateTx := s.db.WithContext(ctx).
		Model(&models.PaymentMethod{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{"is_default": false})
	if updateTx.Error != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(updateTx.Error))
	}

	return nil
}

func (s *Storage) CountByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	const op = "storages.PaymentMethods.CountByUserID"

	if userID == uuid.Nil {
		return 0, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	var count int64
	err := s.db.WithContext(ctx).
		Model(&models.PaymentMethod{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return count, nil
}

func validatePaymentMethod(method entities.PaymentMethod) error {
	switch {
	case method.ID == uuid.Nil:
		return sql.ErrInvalid
	case method.UserID == uuid.Nil:
		return sql.ErrInvalid
	case method.Title == "":
		return sql.ErrInvalid
	default:
		return nil
	}
}

func validateIdentity(id, userID uuid.UUID) error {
	switch {
	case id == uuid.Nil:
		return sql.ErrInvalid
	case userID == uuid.Nil:
		return sql.ErrInvalid
	default:
		return nil
	}
}

func mapToModel(method entities.PaymentMethod) models.PaymentMethod {
	return models.PaymentMethod{
		ID:         method.ID,
		UserID:     method.UserID,
		Title:      method.Title,
		IsDefault:  method.IsDefault,
		CreatedAt:  method.CreatedAt,
		LastUsedAt: method.LastUsedAt,
	}
}

func mapToEntity(method models.PaymentMethod) entities.PaymentMethod {
	return entities.PaymentMethod{
		ID:         method.ID,
		UserID:     method.UserID,
		Title:      method.Title,
		IsDefault:  method.IsDefault,
		CreatedAt:  method.CreatedAt,
		LastUsedAt: method.LastUsedAt,
	}
}
