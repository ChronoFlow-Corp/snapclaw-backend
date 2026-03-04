package hosting

import (
	"context"

	"simpleClaw/internal/entities"
)

// Manager manager container running on server entities
type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Create(ctx context.Context, cl entities.Claw) (Container, error) {
	return Container{}, nil
}
