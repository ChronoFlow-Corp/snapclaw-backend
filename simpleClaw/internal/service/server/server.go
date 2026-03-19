package server

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/server/commands"

	"github.com/google/uuid"
)

type Service struct {
	storage storage
}

func New(storage storage) *Service {
	return &Service{storage: storage}
}

func (s *Service) Create(ctx context.Context, cm commands.CreateServer) (entities.Server, error) {
	const op = "service.server.Create"

	srv, err := newServer(cm)
	if err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.storage.Create(ctx, srv); err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	return srv, nil
}

func (s *Service) GetAll(ctx context.Context) ([]entities.Server, error) {
	const op = "service.server.GetAll"

	servers, err := s.storage.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return servers, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (entities.Server, error) {
	const op = "service.server.GetByID"

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

	if err := s.storage.Update(ctx, existing); err != nil {
		return entities.Server{}, fmt.Errorf("%s: %w", op, err)
	}

	return existing, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	const op = "service.server.Delete"

	if id == uuid.Nil {
		return fmt.Errorf("%s: %w", op, ErrServerIDRequired)
	}

	if err := s.storage.Delete(ctx, id); err != nil {
		return fmt.Errorf("%s: %w", op, err)
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
