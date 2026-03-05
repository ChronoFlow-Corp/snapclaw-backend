package hosting

import (
	"context"
	"encoding/json"
	"fmt"

	"simpleClaw/internal/entities"

	"github.com/google/uuid"
)

// Manager orchestrates container lifecycle interactions with containerManager over HTTP.
type Manager struct {
	client *client
}

func NewManager() *Manager {
	return &Manager{
		client: newClient(defaultHTTPTimeout),
	}
}

func (m *Manager) Create(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) (Container, error) {
	const op = "infra.hosting.Manager.Create"

	if m == nil || m.client == nil {
		return Container{}, fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return Container{}, fmt.Errorf("%s: user id is required", op)
	}

	if server.URL == "" {
		return Container{}, fmt.Errorf("%s: server url is required", op)
	}

	configFiles, err := buildConfigFiles(cl.Config)
	if err != nil {
		return Container{}, fmt.Errorf("%s: %w", op, err)
	}

	resp, err := m.client.createClaw(ctx, server.URL, createClawRequest{
		UserID:     cl.UserID.String(),
		ClawConfig: configFiles,
	})
	if err != nil {
		return Container{}, fmt.Errorf("%s: %w", op, err)
	}

	if resp.ContainerID == "" {
		return Container{}, fmt.Errorf("%s: empty container id received", op)
	}

	container := Container{
		ID:     resp.ContainerID,
		Status: entities.StatusStop,
	}

	if server.ID != uuid.Nil {
		container.ServerID = server.ID
	} else if cl.ServerID != uuid.Nil {
		container.ServerID = cl.ServerID
	}

	if resp.ServerID != "" {
		if srvID, err := uuid.Parse(resp.ServerID); err == nil {
			container.ServerID = srvID
		}
	}

	if resp.Status != "" {
		container.Status = resp.Status
	}

	return container, nil
}

func (m *Manager) Delete(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) error {
	const op = "infra.hosting.Manager.Delete"

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cl.ContainerID == "" {
		return nil
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	_ = m.Stop(ctx, cl, server)

	if err := m.client.deleteClaw(ctx, server.URL, cl.UserID.String(), cl.ContainerID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Start(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) error {
	const op = "infra.hosting.Manager.Start"

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cl.ContainerID == "" {
		return nil
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	if err := m.client.startClaw(ctx, server.URL, cl.UserID.String(), cl.ContainerID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Stop(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) error {
	const op = "infra.hosting.Manager.Stop"

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cl.ContainerID == "" {
		return nil
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	if err := m.client.stopClaw(ctx, server.URL, cl.UserID.String(), cl.ContainerID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Update(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) error {
	const op = "infra.hosting.Manager.Update"

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cl.ContainerID == "" {
		return nil
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	configFiles, err := buildConfigFiles(cl.Config)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := m.client.updateClaw(ctx, server.URL, updateClawRequest{
		UserID:      cl.UserID.String(),
		ContainerID: cl.ContainerID,
		ClawConfig:  configFiles,
	}); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func buildConfigFiles(cfg entities.ClawConfig) ([]clawConfigFile, error) {
	data, err := json.Marshal(buildOpenClawConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("marshal openclaw config: %w", err)
	}

	return []clawConfigFile{
		{
			Name:     openClawConfigName,
			FileType: fileTypeJSON,
			Data:     string(data),
		},
	}, nil
}
