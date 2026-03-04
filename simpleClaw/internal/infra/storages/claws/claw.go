package claws

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
)

type Storage struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) Create(
	ctx context.Context,
	cl entities.Claw,
	channelIDs []uuid.UUID,
) error {
	const op = "storages.Claws.Create"

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
		return fmt.Errorf("%s: %w", op, err)
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
		return fmt.Errorf("%s: %w", op, err)
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

	updates := map[string]any{
		"server_id":    serverID,
		"container_id": containerID,
	}

	if status != "" {
		updates["status"] = status
	}

	tx := s.db.WithContext(ctx).Model(&models.Claw{}).Where("id = ?", clID).Updates(updates)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}
