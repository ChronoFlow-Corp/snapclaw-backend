package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"shared/pkg/observability"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/server/commands"

	"github.com/google/uuid"
)

type Service struct {
	storage  storage
	capacity capacityResolver
	metrics  *observability.OperationMetrics
}

func New(
	storage storage,
	capacity capacityResolver,
	metrics ...*observability.OperationMetrics,
) *Service {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Service{storage: storage, capacity: capacity, metrics: opMetrics}
}

func (s *Service) Create(ctx context.Context, cm commands.CreateServer) (entities.Server, error) {
	const op = "service.server.Create"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.server",
		"server.create",
		"server_registry",
	)

	var err error

	defer func() { finish(err) }()

	srv, err := newServer(cm)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.syncCapacity(ctx, &srv); err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.storage.Create(ctx, srv); err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	return srv, nil
}

func (s *Service) GetAll(ctx context.Context) ([]entities.Server, error) {
	const op = "service.server.GetAll"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.server",
		"server.list",
		"server_registry",
	)

	var err error

	defer func() { finish(err) }()

	servers, err := s.storage.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return servers, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (entities.Server, error) {
	const op = "service.server.GetByID"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.server",
		"server.get",
		"server_registry",
	)

	var err error

	defer func() { finish(err) }()

	if id == uuid.Nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, ErrServerIDRequired)
	}

	srv, err := s.storage.GetByID(ctx, id)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	return srv, nil
}

func (s *Service) Update(ctx context.Context, cm commands.UpdateServer) (entities.Server, error) {
	const op = "service.server.Update"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.server",
		"server.update",
		"server_registry",
	)

	var err error

	defer func() { finish(err) }()

	if cm.ID == uuid.Nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, ErrServerIDRequired)
	}

	existing, err := s.storage.GetByID(ctx, cm.ID)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	name, ip, url, proxyURL, status, secretKey, err := normalizeFields(
		cm.Name,
		cm.IP,
		cm.URL,
		cm.ProxyURL,
		cm.Status,
		cm.SecretKey,
	)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	existing.Name = name
	existing.IP = ip
	existing.URL = url
	existing.ProxyURL = proxyURL
	existing.Status = status
	existing.SecretKey = secretKey

	if err := s.syncCapacity(ctx, &existing); err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.storage.Update(ctx, existing); err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	return existing, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) (err error) {
	const op = "service.server.Delete"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.server",
		"server.delete",
		"server_registry",
	)

	defer func() { finish(err) }()

	if id == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrServerIDRequired)
	}

	if err := s.storage.Delete(ctx, id); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) SyncCapacities(ctx context.Context) error {
	const op = "service.server.SyncCapacities"

	servers, err := s.storage.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	for _, srv := range servers {
		tmp := srv
		err := s.syncCapacity(ctx, &tmp)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		if tmp.MaxClaws == srv.MaxClaws {
			continue
		}

		err = s.storage.Update(ctx, tmp)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

func newServer(cm commands.CreateServer) (entities.Server, error) {
	name, ip, url, proxyURL, status, secretKey, err := normalizeFields(
		cm.Name,
		cm.IP,
		cm.URL,
		cm.ProxyURL,
		cm.Status,
		cm.SecretKey,
	)
	if err != nil {
		return entities.Server{}, err
	}

	return entities.NewServer(name, ip, url, proxyURL, status, secretKey), nil
}

func normalizeFields(
	name, ip, url, proxyURL, status, secretKey string,
) (string, string, string, string, string, string, error) {
	name = strings.TrimSpace(name)
	ip = strings.TrimSpace(ip)
	url = strings.TrimSpace(url)
	proxyURL = strings.TrimSpace(proxyURL)
	status = strings.TrimSpace(status)
	secretKey = strings.TrimSpace(secretKey)

	switch {
	case name == "":
		return "", "", "", "", "", "", ErrNameRequired
	case url == "":
		return "", "", "", "", "", "", ErrURLRequired
	case proxyURL != "" && !isHTTPURL(proxyURL):
		return "", "", "", "", "", "", ErrProxyURLInvalid
	case secretKey == "":
		return "", "", "", "", "", "", ErrSecretKeyRequired
	default:
		return name, ip, url, proxyURL, status, secretKey, nil
	}
}

func isHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}

	if !parsed.IsAbs() || parsed.Host == "" {
		return false
	}

	switch parsed.Scheme {
	case "http", "https":
		return true
	default:
		return false
	}
}

func (s *Service) syncCapacity(ctx context.Context, srv *entities.Server) error {
	if srv == nil {
		return fmt.Errorf("%w: server is nil", ErrCapacitySync)
	}

	if s.capacity == nil {
		return fmt.Errorf("%w: resolver is not configured", ErrCapacitySync)
	}

	maxClaws, err := s.capacity.Capacity(ctx, *srv)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCapacitySync, err)
	}

	if maxClaws <= 0 {
		return fmt.Errorf("%w: invalid max claws %d", ErrCapacitySync, maxClaws)
	}

	srv.MaxClaws = maxClaws

	return nil
}
