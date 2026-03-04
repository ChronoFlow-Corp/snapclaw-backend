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
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
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
		ID:        session.ID,
		UserID:    session.UserID,
		CreatedAt: session.CreatedAt,
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
		ID:        uDB.ID,
		Name:      uDB.Name,
		Email:     uDB.Email,
		Role:      uDB.Role,
		CreatedAt: uDB.CreatedAt,
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
			ID:        s.ID,
			UserID:    s.UserID,
			CreatedAt: s.CreatedAt,
		})
	}

	return sessions, nil
}

func (s *Storage) GetSession(ctx context.Context, id uuid.UUID) (entities.Session, error) {
	const op = "storages.Users.GetSession"

	sDB, err := gorm.G[models.Session](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.Session{}, fmt.Errorf("%s: %w", op, err)
	}

	return entities.Session{
		ID:        sDB.ID,
		UserID:    sDB.UserID,
		CreatedAt: sDB.CreatedAt,
	}, nil
}
