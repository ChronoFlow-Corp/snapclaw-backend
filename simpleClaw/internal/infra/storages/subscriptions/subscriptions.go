package subscriptions

import (
	"context"
	"fmt"
	"time"

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

func (s *Storage) Create(ctx context.Context, subscription entities.UserSubscription) error {
	const op = "storages.Subscriptions.Create"

	if err := validateSubscription(subscription); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	model := mapToModel(subscription)
	if err := gorm.G[models.UserSubscription](s.db).Create(ctx, &model); err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) GetByID(ctx context.Context, id uuid.UUID) (entities.UserSubscription, error) {
	const op = "storages.Subscriptions.GetByID"

	if id == uuid.Nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, invalidSubscription("subscription id is required"))
	}

	model, err := gorm.G[models.UserSubscription](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapToEntity(model), nil
}

func (s *Storage) GetByIDAndUserID(
	ctx context.Context,
	id, userID uuid.UUID,
) (entities.UserSubscription, error) {
	const op = "storages.Subscriptions.GetByIDAndUserID"

	switch {
	case id == uuid.Nil:
		return entities.UserSubscription{}, fmt.Errorf(
			"%s: %w",
			op,
			invalidSubscription("subscription id is required"),
		)
	case userID == uuid.Nil:
		return entities.UserSubscription{}, fmt.Errorf(
			"%s: %w",
			op,
			invalidSubscription("subscription user_id is required"),
		)
	}

	model, err := gorm.G[models.UserSubscription](s.db).
		Where("id = ? AND user_id = ?", id, userID).
		First(ctx)
	if err != nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapToEntity(model), nil
}

func (s *Storage) GetActiveByUserID(ctx context.Context, userID uuid.UUID) (entities.UserSubscription, error) {
	const op = "storages.Subscriptions.GetActiveByUserID"

	if userID == uuid.Nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, invalidSubscription("subscription user_id is required"))
	}

	model, err := gorm.G[models.UserSubscription](s.db).
		Where("user_id = ? AND status = ?", userID, entities.SubscriptionStatusActive).
		Order("created_at DESC").
		First(ctx)
	if err != nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapToEntity(model), nil
}

func (s *Storage) GetLatestByUserID(ctx context.Context, userID uuid.UUID) (entities.UserSubscription, error) {
	const op = "storages.Subscriptions.GetLatestByUserID"

	if userID == uuid.Nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, invalidSubscription("subscription user_id is required"))
	}

	model, err := gorm.G[models.UserSubscription](s.db).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		First(ctx)
	if err != nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapToEntity(model), nil
}

func (s *Storage) ListByUserID(ctx context.Context, userID uuid.UUID) ([]entities.UserSubscription, error) {
	const op = "storages.Subscriptions.ListByUserID"

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, invalidSubscription("subscription user_id is required"))
	}

	rows, err := gorm.G[models.UserSubscription](s.db).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	out := make([]entities.UserSubscription, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapToEntity(row))
	}

	return out, nil
}

func (s *Storage) Update(ctx context.Context, subscription entities.UserSubscription) error {
	const op = "storages.Subscriptions.Update"

	if err := validateSubscription(subscription); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tx := s.db.WithContext(ctx).Model(&models.UserSubscription{}).
		Where("id = ? AND user_id = ?", subscription.ID, subscription.UserID).
		Updates(map[string]any{
			"plan_id":              subscription.PlanID,
			"status":               subscription.Status,
			"started_at":           subscription.StartedAt,
			"current_period_start": subscription.CurrentPeriodStart,
			"current_period_end":   subscription.CurrentPeriodEnd,
			"canceled_at":          subscription.CanceledAt,
			"updated_at":           subscription.UpdatedAt,
		})
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) Cancel(ctx context.Context, id, userID uuid.UUID, canceledAt time.Time) error {
	const op = "storages.Subscriptions.Cancel"

	if err := validateCancelInput(id, userID, canceledAt); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tx := s.db.WithContext(ctx).Model(&models.UserSubscription{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{
			"status":      entities.SubscriptionStatusCanceled,
			"canceled_at": canceledAt,
			"updated_at":  canceledAt,
		})
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func validateSubscription(subscription entities.UserSubscription) error {
	switch {
	case subscription.ID == uuid.Nil:
		return invalidSubscription("subscription id is required")
	case subscription.UserID == uuid.Nil:
		return invalidSubscription("subscription user_id is required")
	case subscription.PlanID == uuid.Nil:
		return invalidSubscription("subscription plan_id is required")
	case subscription.Status == "":
		return invalidSubscription("subscription status is required")
	case subscription.StartedAt.IsZero():
		return invalidSubscription("subscription started_at is required")
	case subscription.CurrentPeriodStart.IsZero():
		return invalidSubscription("subscription current_period_start is required")
	case subscription.CurrentPeriodEnd.IsZero():
		return invalidSubscription("subscription current_period_end is required")
	case subscription.CreatedAt.IsZero():
		return invalidSubscription("subscription created_at is required")
	case subscription.UpdatedAt.IsZero():
		return invalidSubscription("subscription updated_at is required")
	default:
		return nil
	}
}

func validateCancelInput(id, userID uuid.UUID, canceledAt time.Time) error {
	switch {
	case id == uuid.Nil:
		return invalidSubscription("subscription id is required")
	case userID == uuid.Nil:
		return invalidSubscription("subscription user_id is required")
	case canceledAt.IsZero():
		return invalidSubscription("subscription canceled_at is required")
	default:
		return nil
	}
}

func invalidSubscription(reason string) error {
	return fmt.Errorf("%s: %w", reason, sql.ErrInvalid)
}

func mapToModel(subscription entities.UserSubscription) models.UserSubscription {
	return models.UserSubscription{
		ID:                 subscription.ID,
		UserID:             subscription.UserID,
		PlanID:             subscription.PlanID,
		Status:             subscription.Status,
		StartedAt:          subscription.StartedAt,
		CurrentPeriodStart: subscription.CurrentPeriodStart,
		CurrentPeriodEnd:   subscription.CurrentPeriodEnd,
		CanceledAt:         subscription.CanceledAt,
		CreatedAt:          subscription.CreatedAt,
		UpdatedAt:          subscription.UpdatedAt,
	}
}

func mapToEntity(subscription models.UserSubscription) entities.UserSubscription {
	return entities.UserSubscription{
		ID:                 subscription.ID,
		UserID:             subscription.UserID,
		PlanID:             subscription.PlanID,
		Status:             subscription.Status,
		StartedAt:          subscription.StartedAt,
		CurrentPeriodStart: subscription.CurrentPeriodStart,
		CurrentPeriodEnd:   subscription.CurrentPeriodEnd,
		CanceledAt:         subscription.CanceledAt,
		CreatedAt:          subscription.CreatedAt,
		UpdatedAt:          subscription.UpdatedAt,
	}
}
