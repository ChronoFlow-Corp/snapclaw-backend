package plans

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

func (s *Storage) Create(ctx context.Context, plan entities.Plan) error {
	const op = "storages.Plans.Create"

	if err := validatePlan(plan); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	model := mapToModel(plan)
	if err := s.db.WithContext(ctx).Select("*").Create(&model).Error; err != nil {
		return fmt.Errorf("%s: %w", op, translatePlanError(err))
	}

	return nil
}

func (s *Storage) GetByID(ctx context.Context, id uuid.UUID) (entities.Plan, error) {
	const op = "storages.Plans.GetByID"

	if id == uuid.Nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, invalidPlan("plan id is required"))
	}

	model, err := gorm.G[models.Plan](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.Plan{}, fmt.Errorf("%s: %w", op, translatePlanError(err))
	}

	return mapToEntity(model), nil
}

func (s *Storage) List(ctx context.Context, includeInactive bool) ([]entities.Plan, error) {
	const op = "storages.Plans.List"

	query := s.db.WithContext(ctx).Model(&models.Plan{})
	if !includeInactive {
		query = query.Where("is_active = ?", true)
	}

	var rows []models.Plan
	if err := query.Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("%s: %w", op, translatePlanError(err))
	}

	plans := make([]entities.Plan, 0, len(rows))
	for _, row := range rows {
		plans = append(plans, mapToEntity(row))
	}

	return plans, nil
}

func (s *Storage) Update(ctx context.Context, plan entities.Plan) error {
	const op = "storages.Plans.Update"

	if err := validatePlan(plan); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tx := s.db.WithContext(ctx).Model(&models.Plan{}).
		Where("id = ?", plan.ID).
		Updates(map[string]any{
			"code":                 plan.Code,
			"name":                 plan.Name,
			"billing_amount_minor": plan.BillingAmountMinor,
			"balance_credit_minor": plan.BalanceCreditMinor,
			"currency":             plan.Currency,
			"is_active":            plan.IsActive,
			"updated_at":           plan.UpdatedAt,
		})
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, translatePlanError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) Deactivate(ctx context.Context, id uuid.UUID) error {
	const op = "storages.Plans.Deactivate"

	if id == uuid.Nil {
		return fmt.Errorf("%s: %w", op, invalidPlan("plan id is required"))
	}

	tx := s.db.WithContext(ctx).Model(&models.Plan{}).
		Where("id = ?", id).
		Updates(map[string]any{"is_active": false})
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, translatePlanError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func validatePlan(plan entities.Plan) error {
	switch {
	case plan.ID == uuid.Nil:
		return invalidPlan("plan id is required")
	case strings.TrimSpace(plan.Code) == "":
		return invalidPlan("plan code is required")
	case strings.TrimSpace(plan.Name) == "":
		return invalidPlan("plan name is required")
	case plan.BillingAmountMinor <= 0:
		return invalidPlan("plan billing_amount_minor must be greater than 0")
	case plan.BalanceCreditMinor <= 0:
		return invalidPlan("plan balance_credit_minor must be greater than 0")
	case strings.TrimSpace(plan.Currency) == "":
		return invalidPlan("plan currency is required")
	case plan.CreatedAt.IsZero():
		return invalidPlan("plan created_at is required")
	case plan.UpdatedAt.IsZero():
		return invalidPlan("plan updated_at is required")
	default:
		return nil
	}
}

func invalidPlan(reason string) error {
	return fmt.Errorf("%s: %w", reason, sql.ErrInvalid)
}

func translatePlanError(err error) error {
	translated := sql.TranslateError(err)
	if translated != err {
		return translated
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return sql.ErrConflict
	}

	if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
		return sql.ErrConflict
	}

	return translated
}

func mapToModel(plan entities.Plan) models.Plan {
	return models.Plan{
		ID:                 plan.ID,
		Code:               plan.Code,
		Name:               plan.Name,
		BillingAmountMinor: plan.BillingAmountMinor,
		BalanceCreditMinor: plan.BalanceCreditMinor,
		Currency:           plan.Currency,
		IsActive:           plan.IsActive,
		CreatedAt:          plan.CreatedAt,
		UpdatedAt:          plan.UpdatedAt,
	}
}

func mapToEntity(plan models.Plan) entities.Plan {
	return entities.Plan{
		ID:                 plan.ID,
		Code:               plan.Code,
		Name:               plan.Name,
		BillingAmountMinor: plan.BillingAmountMinor,
		BalanceCreditMinor: plan.BalanceCreditMinor,
		Currency:           plan.Currency,
		IsActive:           plan.IsActive,
		CreatedAt:          plan.CreatedAt,
		UpdatedAt:          plan.UpdatedAt,
	}
}
