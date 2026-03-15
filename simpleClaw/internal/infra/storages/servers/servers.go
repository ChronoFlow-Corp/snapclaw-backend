package servers

import (
	"context"
	"fmt"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Storage struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) GetAvailable(ctx context.Context) (entities.Server, error) {
	const op = "storages.Servers.GetAvailable"

	srv, err := gorm.G[models.Server](s.db).Order("created_at ASC").First(ctx)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return entities.Server{
		ID:        srv.ID,
		Name:      srv.Name,
		IP:        srv.Ip,
		URL:       srv.Url,
		Status:    srv.Status,
		SecretKey: srv.SecretKey,
		CreatedAt: srv.CreatedAt,
	}, nil
}

func (s *Storage) GetByID(ctx context.Context, id uuid.UUID) (entities.Server, error) {
	const op = "storages.Servers.GetByID"

	srv, err := gorm.G[models.Server](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return entities.Server{
		ID:        srv.ID,
		Name:      srv.Name,
		IP:        srv.Ip,
		URL:       srv.Url,
		Status:    srv.Status,
		SecretKey: srv.SecretKey,
		CreatedAt: srv.CreatedAt,
	}, nil
}
