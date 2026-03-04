package hosting

import (
	"context"

	"simpleClaw/internal/entities"
)

type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Create(ctx context.Context, cl entities.Claw) error {
	const op = "service.Service.Create"

	return nil
}
