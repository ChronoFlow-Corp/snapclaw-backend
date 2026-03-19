package claws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"shared/pkg/observability"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
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

	return &Storage{db: db, metrics: opMetrics}
}

func (s *Storage) Create(
	ctx context.Context,
	cl entities.Claw,
	channelIDs []uuid.UUID,
) error {
	const op = "storages.Claws.Create"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.claws", "storage.claw.create", "claw_lifecycle")
	var err error
	defer func() { finish(err) }()

	cfg, err := json.Marshal(cl.Config)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	model := models.Claw{
		ID:          cl.ID,
		Name:        cl.Name,
		Config:      datatypes.JSON(cfg),
		UserID:      cl.UserID,
		Status:      cl.Status,
		ServerID:    cl.ServerID,
		ContainerID: cl.ContainerID,
		CreatedAt:   cl.CreatedAt,
		UpdatedAt:   cl.UpdatedAt,
	}

	err = gorm.G[models.Claw](s.db).Create(ctx, &model)
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if len(channelIDs) == 0 {
		return nil
	}

	relations := make([]clawChannel, 0, len(channelIDs))
	for _, chID := range channelIDs {
		relations = append(relations, clawChannel{
			ClawID:    cl.ID,
			ChannelID: chID,
		})
	}

	if err = s.db.WithContext(ctx).Table("claw_channels").Create(&relations).Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) UpdateRuntime(
	ctx context.Context,
	clID uuid.UUID,
	serverID uuid.UUID,
	containerID string,
	status string,
) error {
	const op = "storages.Claws.UpdateRuntime"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.claws", "storage.claw.update_runtime", "claw_lifecycle")
	var err error
	defer func() { finish(err) }()

	var sID *uuid.UUID

	if serverID != uuid.Nil {
		sID = &serverID
	}

	updates := map[string]any{
		"server_id":    sID,
		"container_id": containerID,
		"updated_at":   gorm.Expr("NOW()"),
	}

	if status != "" {
		updates["status"] = status
	}

	tx := s.db.WithContext(ctx).Model(&models.Claw{}).Where("id = ?", clID).Updates(updates)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) GetByID(
	ctx context.Context,
	id uuid.UUID,
	userID uuid.UUID,
) (entities.Claw, error) {
	const op = "storages.Claws.GetByID"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.claws", "storage.claw.get_by_id", "claw_lifecycle")
	var err error
	defer func() { finish(err) }()

	clDB, err := gorm.G[models.Claw](s.db).Where("id = ? AND user_id = ?", id, userID).First(ctx)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	var cfg entities.ClawConfig
	if len(clDB.Config) > 0 {
		if err := json.Unmarshal(clDB.Config, &cfg); err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	return entities.Claw{
		ID:          clDB.ID,
		Name:        clDB.Name,
		UserID:      clDB.UserID,
		ServerID:    clDB.ServerID,
		Status:      clDB.Status,
		ContainerID: clDB.ContainerID,
		Config:      cfg,
		CreatedAt:   clDB.CreatedAt,
		UpdatedAt:   clDB.UpdatedAt,
	}, nil
}

func (s *Storage) GetByUserID(
	ctx context.Context,
	userID uuid.UUID,
) ([]entities.Claw, error) {
	const op = "storages.Claws.GetByUserID"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.claws", "storage.claw.list_by_user", "claw_lifecycle")
	var err error
	defer func() { finish(err) }()

	clsDB, err := gorm.G[models.Claw](s.db).Where("user_id = ?", userID).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	cls := make([]entities.Claw, 0, len(clsDB))
	for _, clDB := range clsDB {
		var cfg entities.ClawConfig
		if len(clDB.Config) > 0 {
			if err := json.Unmarshal(clDB.Config, &cfg); err != nil {
				return nil, fmt.Errorf("%s: %w", op, err)
			}
		}

		cls = append(cls, entities.Claw{
			ID:          clDB.ID,
			Name:        clDB.Name,
			UserID:      clDB.UserID,
			ServerID:    clDB.ServerID,
			Status:      clDB.Status,
			ContainerID: clDB.ContainerID,
			Config:      cfg,
			CreatedAt:   clDB.CreatedAt,
			UpdatedAt:   clDB.UpdatedAt,
		})
	}

	return cls, nil
}

func (s *Storage) Update(
	ctx context.Context,
	cl entities.Claw,
	channelIDs []uuid.UUID,
	replaceChannels bool,
) error {
	const op = "storages.Claws.Update"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.claws", "storage.claw.update", "claw_lifecycle")
	var err error
	defer func() { finish(err) }()

	cfg, err := json.Marshal(cl.Config)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"name":       cl.Name,
			"config":     datatypes.JSON(cfg),
			"updated_at": cl.UpdatedAt,
		}

		res := tx.Model(&models.Claw{}).
			Where("id = ? AND user_id = ?", cl.ID, cl.UserID).
			Updates(updates)
		if err := res.Error; err != nil {
			return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
		}

		if res.RowsAffected == 0 {
			return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
		}

		if replaceChannels {
			if err := tx.Table("claw_channels").Where("claw_id = ?", cl.ID).Delete(&clawChannel{}).Error; err != nil {
				return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
			}

			if len(channelIDs) == 0 {
				return nil
			}

			relations := make([]clawChannel, 0, len(channelIDs))
			for _, chID := range channelIDs {
				relations = append(relations, clawChannel{
					ClawID:    cl.ID,
					ChannelID: chID,
				})
			}

			if err := tx.Table("claw_channels").Create(&relations).Error; err != nil {
				return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
			}
		}

		return nil
	})
}

func (s *Storage) Delete(
	ctx context.Context,
	id uuid.UUID,
	userID uuid.UUID,
) error {
	const op = "storages.Claws.Delete"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.claws", "storage.claw.delete", "claw_lifecycle")
	var err error
	defer func() { finish(err) }()

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("claw_channels").Where("claw_id = ?", id).Delete(&clawChannel{}).Error; err != nil {
			return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
		}

		res := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Claw{})
		if err := res.Error; err != nil {
			return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
		}

		if res.RowsAffected == 0 {
			return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
		}

		return nil
	})
}
