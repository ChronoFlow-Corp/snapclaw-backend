package users

import (
	"context"
	"errors"
	"fmt"

	"simpleClaw/internal/infra/sql"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Storage struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{
		db: db,
	}
}

func (s *Storage) Create(ctx context.Context, u entities.User) error {
	const op = "storages.Users.Create"

	err := gorm.G[models.User](s.db).Create(ctx, &models.User{
		ID:               u.ID,
		Name:             u.Name,
		Email:            u.Email,
		Role:             u.Role,
		OpenRouterApiKey: u.OpenRouterApiKey,
		OpenRouterKeyID:  u.OpenRouterKeyID,
		CreatedAt:        u.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Storage) Delete(ctx context.Context, id uuid.UUID) error {
	const op = "storages.Users.Delete"

	affected, err := gorm.G[models.User](s.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if affected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) CreateSession(ctx context.Context, session entities.Session) error {
	const op = "storages.Users.CreateSession"

	err := gorm.G[models.Session](s.db).Create(ctx, &models.Session{
		ID:           session.ID,
		UserID:       session.UserID,
		RefreshToken: session.RefreshToken,
		CreatedAt:    session.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Storage) GetByEmail(ctx context.Context, email string) (entities.User, error) {
	const op = "storages.Users.GetByEmail"

	uDB, err := gorm.G[models.User](s.db).Where("email = ?", email).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return entities.User{}, fmt.Errorf("%s: %w: %w", op, sql.ErrNotFound, err)
		}

		return entities.User{}, fmt.Errorf("%s: %w", op, err)
	}

	return entities.User{
		ID:               uDB.ID,
		Name:             uDB.Name,
		Email:            uDB.Email,
		Role:             uDB.Role,
		OpenRouterApiKey: uDB.OpenRouterApiKey,
		OpenRouterKeyID:  uDB.OpenRouterKeyID,
		CreatedAt:        uDB.CreatedAt,
	}, nil
}

func (s *Storage) GetByID(ctx context.Context, id uuid.UUID) (entities.User, error) {
	const op = "storages.Users.GetByID"

	uDB, err := gorm.G[models.User](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return entities.User{}, fmt.Errorf("%s: %w: %w", op, sql.ErrNotFound, err)
		}

		return entities.User{}, fmt.Errorf("%s: %w", op, err)
	}

	return entities.User{
		ID:               uDB.ID,
		Name:             uDB.Name,
		Email:            uDB.Email,
		Role:             uDB.Role,
		OpenRouterApiKey: uDB.OpenRouterApiKey,
		OpenRouterKeyID:  uDB.OpenRouterKeyID,
		CreatedAt:        uDB.CreatedAt,
	}, nil
}

func (s *Storage) DeleteSession(ctx context.Context, session entities.Session) error {
	const op = "storages.Users.DeleteSession"

	_, err := gorm.G[models.Session](
		s.db,
	).Where("id = ? AND user_id = ?", session.ID, session.UserID).
		Delete(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Storage) GetSessions(ctx context.Context, userID uuid.UUID) ([]entities.Session, error) {
	const op = "storages.Users.GetSessions"

	sessions := make([]entities.Session, 0)

	sDB, err := gorm.G[models.Session](s.db).Where("user_id = ?", userID).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	for _, s := range sDB {
		sessions = append(sessions, entities.Session{
			ID:           s.ID,
			UserID:       s.UserID,
			RefreshToken: s.RefreshToken,
			CreatedAt:    s.CreatedAt,
		})
	}

	return sessions, nil
}

func (s *Storage) GetSession(ctx context.Context, id uuid.UUID) (entities.Session, error) {
	const op = "storages.Users.GetSession"

	sDB, err := gorm.G[models.Session](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return entities.Session{}, fmt.Errorf("%s: %w: %w", op, sql.ErrNotFound, err)
		}

		return entities.Session{}, fmt.Errorf("%s: %w", op, err)
	}

	return entities.Session{
		ID:           sDB.ID,
		UserID:       sDB.UserID,
		RefreshToken: sDB.RefreshToken,
		CreatedAt:    sDB.CreatedAt,
	}, nil
}

func (s *Storage) UpdateSessionRefresh(
	ctx context.Context,
	sessionID, userID uuid.UUID,
	refreshToken string,
) error {
	const op = "storages.Users.UpdateSessionRefresh"

	updates := map[string]any{
		"refresh_token": refreshToken,
	}

	tx := s.db.WithContext(ctx).Model(&models.Session{}).
		Where("id = ? AND user_id = ?", sessionID, userID).
		Updates(updates)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) UpdateOpenRouterKey(
	ctx context.Context,
	id uuid.UUID,
	key entities.OpenRouterKey,
) error {
	const op = "storages.Users.UpdateOpenRouterKey"

	updates := map[string]any{
		"open_router_api_key": key.Secret,
		"open_router_key_id":  key.ID,
	}

	tx := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Updates(updates)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}
