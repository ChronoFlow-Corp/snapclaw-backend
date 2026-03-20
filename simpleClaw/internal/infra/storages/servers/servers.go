package servers

import (
	"context"
	"fmt"
	"log/slog"
	"shared/pkg/observability"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
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

func (s *Storage) Create(ctx context.Context, srv entities.Server) error {
	const op = "storages.Servers.Create"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.servers", "storage.server.create", "server_registry")
	var err error
	defer func() { finish(err) }()

	err = gorm.G[models.Server](s.db).Create(ctx, toModel(srv))
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) GetAll(ctx context.Context) ([]entities.Server, error) {
	const op = "storages.Servers.GetAll"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.servers", "storage.server.list", "server_registry")
	var err error
	defer func() { finish(err) }()

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
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.servers", "storage.server.get_available", "server_registry")
	var err error
	defer func() { finish(err) }()

	srv, err := gorm.G[models.Server](s.db).Order("created_at ASC").First(ctx)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return toEntity(srv), nil
}

func (s *Storage) GetByID(ctx context.Context, id uuid.UUID) (entities.Server, error) {
	const op = "storages.Servers.GetByID"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.servers", "storage.server.get_by_id", "server_registry")
	var err error
	defer func() { finish(err) }()

	srv, err := gorm.G[models.Server](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return toEntity(srv), nil
}

func (s *Storage) Update(ctx context.Context, srv entities.Server) (err error) {
	const op = "storages.Servers.Update"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.servers", "storage.server.update", "server_registry")
	defer func() { finish(err) }()

	tx := s.db.WithContext(ctx).Model(&models.Server{}).
		Where("id = ?", srv.ID).
		Updates(map[string]any{
			"name":       srv.Name,
			"ip":         srv.IP,
			"url":        srv.URL,
			"proxy_url":  srv.ProxyURL,
			"status":     srv.Status,
			"secret_key": srv.SecretKey,
			"max_claws":  srv.MaxClaws,
		})
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) Delete(ctx context.Context, id uuid.UUID) (err error) {
	const op = "storages.Servers.Delete"
	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), s.metrics, "storage.servers", "storage.server.delete", "server_registry")
	defer func() { finish(err) }()

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
		MaxClaws:  srv.MaxClaws,
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
		MaxClaws:  srv.MaxClaws,
		CreatedAt: srv.CreatedAt,
	}
}
