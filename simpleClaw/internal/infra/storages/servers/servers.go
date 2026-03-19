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

func (s *Storage) Create(ctx context.Context, srv entities.Server) error {
	const op = "storages.Servers.Create"

	err := gorm.G[models.Server](s.db).Create(ctx, toModel(srv))
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) GetAll(ctx context.Context) ([]entities.Server, error) {
	const op = "storages.Servers.GetAll"

	rows, err := gorm.G[models.Server](s.db).Order("created_at ASC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	servers := make([]entities.Server, 0, len(rows))
	for _, srv := range rows {
		servers = append(servers, toEntity(srv))
	}

	return servers, nil
}

func (s *Storage) GetAvailable(ctx context.Context) (entities.Server, error) {
	const op = "storages.Servers.GetAvailable"

	srv, err := gorm.G[models.Server](s.db).Order("created_at ASC").First(ctx)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return toEntity(srv), nil
}

func (s *Storage) GetByID(ctx context.Context, id uuid.UUID) (entities.Server, error) {
	const op = "storages.Servers.GetByID"

	srv, err := gorm.G[models.Server](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return toEntity(srv), nil
}

func (s *Storage) Update(ctx context.Context, srv entities.Server) error {
	const op = "storages.Servers.Update"

	tx := s.db.WithContext(ctx).Model(&models.Server{}).
		Where("id = ?", srv.ID).
		Updates(map[string]any{
			"name":       srv.Name,
			"ip":         srv.IP,
			"url":        srv.URL,
			"proxy_url":  srv.ProxyURL,
			"status":     srv.Status,
			"secret_key": srv.SecretKey,
		})
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) Delete(ctx context.Context, id uuid.UUID) error {
	const op = "storages.Servers.Delete"

	affected, err := gorm.G[models.Server](s.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if affected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func toEntity(srv models.Server) entities.Server {
	return entities.Server{
		ID:        srv.ID,
		Name:      srv.Name,
		IP:        srv.Ip,
		URL:       srv.Url,
		ProxyURL:  srv.ProxyUrl,
		Status:    srv.Status,
		SecretKey: srv.SecretKey,
		CreatedAt: srv.CreatedAt,
	}
}

func toModel(srv entities.Server) *models.Server {
	return &models.Server{
		ID:        srv.ID,
		Name:      srv.Name,
		Ip:        srv.IP,
		Url:       srv.URL,
		ProxyUrl:  srv.ProxyURL,
		Status:    srv.Status,
		SecretKey: srv.SecretKey,
		CreatedAt: srv.CreatedAt,
	}
}
