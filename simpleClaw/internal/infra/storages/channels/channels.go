package channels

import (
	"context"
	"encoding/json"
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
	return &Storage{
		db: db,
	}
}

func (s *Storage) Create(ctx context.Context, ch entities.Channel) error {
	const op = "service.Service.Create"

	raw, err := json.Marshal(ch.Config)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	err = gorm.G[models.Channel](s.db).Create(ctx, &models.Channel{
		ID:             ch.ID,
		Name:           ch.Name,
		UserID:         ch.UserID,
		ChannelType:    ch.ChannelType,
		OpenClawConfig: raw,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) GetByID(ctx context.Context, id, userID uuid.UUID) (entities.Channel, error) {
	const op = "service.Service.GetByID"

	chDB, err := gorm.G[models.Channel](s.db).Where("id = ? AND user_id = ?", id, userID).First(ctx)
	if err != nil {
		return entities.Channel{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	ch := entities.Channel{
		ID:          chDB.ID,
		Name:        chDB.Name,
		UserID:      chDB.UserID,
		ChannelType: chDB.ChannelType,
		CreatedAt:   chDB.CreatedAt,
	}

	err = json.Unmarshal(chDB.OpenClawConfig, &ch.Config)
	if err != nil {
		return entities.Channel{}, fmt.Errorf("%s: %w", op, err)
	}

	return ch, nil
}

func (s *Storage) Update(ctx context.Context, ch entities.Channel) error {
	const op = "service.Service.Update"

	raw, err := json.Marshal(ch.Config)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	affected, err := gorm.G[models.Channel](
		s.db,
	).Where("id = ? AND user_id = ?", ch.ID, ch.UserID).
		Updates(ctx, models.Channel{
			Name:           ch.Name,
			ChannelType:    ch.ChannelType,
			OpenClawConfig: raw,
		})
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if affected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Channel, error) {
	const op = "service.Service.GetByUserID"

	chsDB, err := gorm.G[models.Channel](s.db).Where("user_id = ?", userID).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	chs := make([]entities.Channel, 0, len(chsDB))

	for _, chDB := range chsDB {
		var cfg entities.ClawChannels

		err = json.Unmarshal(chDB.OpenClawConfig, &cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}

		chs = append(chs, entities.Channel{
			ID:          chDB.ID,
			Name:        chDB.Name,
			UserID:      chDB.UserID,
			ChannelType: chDB.ChannelType,
			CreatedAt:   chDB.CreatedAt,
			Config:      cfg,
		})
	}

	return chs, nil
}

func (s *Storage) GetByIDs(
	ctx context.Context,
	ids []uuid.UUID,
	userID uuid.UUID,
) ([]entities.Channel, error) {
	const op = "storages.Channels.GetByIDs"

	if len(ids) == 0 {
		return []entities.Channel{}, nil
	}

	chsDB, err := gorm.G[models.Channel](s.db).
		Where("user_id = ? AND id IN ?", userID, ids).
		Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	chs := make([]entities.Channel, 0, len(chsDB))

	for _, chDB := range chsDB {
		var cfg entities.ClawChannels

		err = json.Unmarshal(chDB.OpenClawConfig, &cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}

		chs = append(chs, entities.Channel{
			ID:          chDB.ID,
			Name:        chDB.Name,
			UserID:      chDB.UserID,
			ChannelType: chDB.ChannelType,
			CreatedAt:   chDB.CreatedAt,
			Config:      cfg,
		})
	}

	return chs, nil
}

func (s *Storage) Delete(ctx context.Context, id, userID uuid.UUID) error {
	const op = "service.Service.Delete"

	affected, err := gorm.G[models.Channel](
		s.db,
	).Where("id = ? AND user_id = ?", id, userID).
		Delete(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if affected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}
