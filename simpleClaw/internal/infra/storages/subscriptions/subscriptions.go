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
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	model, err := gorm.G[models.UserSubscription](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return mapToEntity(model), nil
}

func (s *Storage) GetActiveByUserID(ctx context.Context, userID uuid.UUID) (entities.UserSubscription, error) {
	const op = "storages.Subscriptions.GetActiveByUserID"

	if userID == uuid.Nil {
		return entities.UserSubscription{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
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

	if id == uuid.Nil || userID == uuid.Nil || canceledAt.IsZero() {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
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
		return sql.ErrInvalid
	case subscription.UserID == uuid.Nil:
		return sql.ErrInvalid
	case subscription.PlanID == uuid.Nil:
		return sql.ErrInvalid
	case subscription.Status == "":
		return sql.ErrInvalid
	case subscription.StartedAt.IsZero():
		return sql.ErrInvalid
	case subscription.CurrentPeriodStart.IsZero():
		return sql.ErrInvalid
	case subscription.CurrentPeriodEnd.IsZero():
		return sql.ErrInvalid
	case subscription.CreatedAt.IsZero():
		return sql.ErrInvalid
	case subscription.UpdatedAt.IsZero():
		return sql.ErrInvalid
	default:
		return nil
	}
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
