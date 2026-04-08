package clawoperations

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"shared/pkg/observability"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Storage struct {
	db      *gorm.DB
	metrics *observability.OperationMetrics
}

func NewStorage(db *gorm.DB, metrics ...*observability.OperationMetrics) *Storage {
	var opMetrics *observability.OperationMetrics
	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Storage{
		db:      db,
		metrics: opMetrics,
	}
}

func (s *Storage) Create(ctx context.Context, op entities.ClawLifecycleOperation) error {
	const operation = "storages.ClawOperations.Create"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claw_operations",
		"storage.claw_operation.create",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	model := mapOperationEntity(op)

	err = sql.TranslateError(s.db.WithContext(ctx).Create(&model).Error)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

func (s *Storage) LockNextRunnable(ctx context.Context, now time.Time) (entities.ClawLifecycleOperation, error) {
	const operation = "storages.ClawOperations.LockNextRunnable"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claw_operations",
		"storage.claw_operation.lock_next_runnable",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	var model models.ClawLifecycleOperation

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&models.ClawLifecycleOperation{}).
			Where(
				"status = ? OR (status = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ?)",
				entities.ClawLifecycleOperationStatusPending,
				entities.ClawLifecycleOperationStatusRetryScheduled,
				now,
			).
			Order("created_at ASC")

		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}

		if err := query.First(&model).Error; err != nil {
			return sql.TranslateError(err)
		}

		claimedAt := time.Now().UTC()
		res := tx.Model(&models.ClawLifecycleOperation{}).
			Where("id = ? AND status = ?", model.ID, model.Status).
			Updates(map[string]any{
				"status":        string(entities.ClawLifecycleOperationStatusRunning),
				"updated_at":    claimedAt,
				"next_retry_at": nil,
			})
		if err := res.Error; err != nil {
			return sql.TranslateError(err)
		}

		if res.RowsAffected == 0 {
			return sql.ErrNotFound
		}

		model.Status = string(entities.ClawLifecycleOperationStatusRunning)
		model.UpdatedAt = claimedAt
		model.NextRetryAt = nil

		return nil
	})
	if err != nil {
		return entities.ClawLifecycleOperation{}, fmt.Errorf("%s: %w", operation, err)
	}

	return mapOperationModel(model), nil
}

func (s *Storage) Update(ctx context.Context, op entities.ClawLifecycleOperation) error {
	const operation = "storages.ClawOperations.Update"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claw_operations",
		"storage.claw_operation.update",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	model := mapOperationEntity(op)
	updates := map[string]any{
		"status":              model.Status,
		"stage":               model.Stage,
		"attempt":             model.Attempt,
		"last_error":          model.LastError,
		"server_id":           model.ServerID,
		"container_record_id": model.ContainerRecordID,
		"docker_container_id": model.DockerContainerID,
		"next_retry_at":       model.NextRetryAt,
		"updated_at":          model.UpdatedAt,
	}

	tx := s.db.WithContext(ctx).
		Model(&models.ClawLifecycleOperation{}).
		Where("id = ?", op.ID).
		Updates(updates)
	if err = tx.Error; err != nil {
		return fmt.Errorf("%s: %w", operation, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", operation, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) GetActiveByClawID(ctx context.Context, clawID uuid.UUID) (entities.ClawLifecycleOperation, error) {
	const operation = "storages.ClawOperations.GetActiveByClawID"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claw_operations",
		"storage.claw_operation.get_active_by_claw",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	modelsList, err := gorm.G[models.ClawLifecycleOperation](s.db).
		Where("claw_id = ?", clawID).
		Where("status IN ?", activeOperationStatuses()).
		Order("created_at DESC").
		Limit(2).
		Find(ctx)
	if err != nil {
		return entities.ClawLifecycleOperation{}, fmt.Errorf("%s: %w", operation, sql.TranslateError(err))
	}

	switch len(modelsList) {
	case 0:
		return entities.ClawLifecycleOperation{}, fmt.Errorf("%s: %w", operation, sql.ErrNotFound)
	case 1:
		return mapOperationModel(modelsList[0]), nil
	default:
		return entities.ClawLifecycleOperation{}, fmt.Errorf("%s: %w", operation, sql.ErrConflict)
	}
}

func mapOperationEntity(op entities.ClawLifecycleOperation) models.ClawLifecycleOperation {
	return models.ClawLifecycleOperation{
		ID:                op.ID,
		ClawID:            op.ClawID,
		Type:              string(op.Type),
		Status:            string(op.Status),
		Stage:             op.Stage,
		Attempt:           op.Attempt,
		LastError:         op.LastError,
		ServerID:          nilUUID(op.ServerID),
		ContainerRecordID: op.ContainerRecordID,
		DockerContainerID: op.DockerContainerID,
		NextRetryAt:       op.NextRetryAt,
		CreatedAt:         op.CreatedAt,
		UpdatedAt:         op.UpdatedAt,
	}
}

func mapOperationModel(model models.ClawLifecycleOperation) entities.ClawLifecycleOperation {
	return entities.ClawLifecycleOperation{
		ID:                model.ID,
		ClawID:            model.ClawID,
		Type:              entities.ClawLifecycleOperationType(model.Type),
		Status:            entities.ClawLifecycleOperationStatus(model.Status),
		Stage:             model.Stage,
		Attempt:           model.Attempt,
		LastError:         model.LastError,
		ServerID:          derefUUID(model.ServerID),
		ContainerRecordID: model.ContainerRecordID,
		DockerContainerID: model.DockerContainerID,
		NextRetryAt:       model.NextRetryAt,
		CreatedAt:         model.CreatedAt,
		UpdatedAt:         model.UpdatedAt,
	}
}

func nilUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}

func derefUUID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}

	return *id
}

func activeOperationStatuses() []string {
	return []string{
		string(entities.ClawLifecycleOperationStatusPending),
		string(entities.ClawLifecycleOperationStatusRunning),
		string(entities.ClawLifecycleOperationStatusRetryScheduled),
	}
}
