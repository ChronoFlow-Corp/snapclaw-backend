package claw

import (
	"context"
	"io"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"

	"github.com/google/uuid"
)

type clawStorage interface {
	Create(ctx context.Context, cl entities.Claw, channelIDs []uuid.UUID) error
	GetByID(ctx context.Context, id, userID uuid.UUID) (entities.Claw, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Claw, error)
	Update(ctx context.Context, cl entities.Claw, channelIDs []uuid.UUID) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	UpdateRuntime(
		ctx context.Context,
		clID uuid.UUID,
		serverID uuid.UUID,
		containerID string,
		status string,
	) error
}

type channelStorage interface {
	GetByIDs(ctx context.Context, ids []uuid.UUID, userID uuid.UUID) ([]entities.Channel, error)
}

type userStorage interface {
	GetByID(ctx context.Context, id uuid.UUID) (entities.User, error)
	UpdateOpenRouterKey(ctx context.Context, id uuid.UUID, key entities.OpenRouterKey) error
}

type serverStorage interface {
	GetAvailable(ctx context.Context) (entities.Server, error)
	GetByID(ctx context.Context, id uuid.UUID) (entities.Server, error)
}

type apiKeyManager interface {
	Create(
		ctx context.Context,
		userID uuid.UUID,
		label string,
		monthlyBudgetUSD float64,
	) (entities.OpenRouterKey, error)
	ResolveModel(ctx context.Context, model string) (string, error)
}

type hostingManager interface {
	Create(ctx context.Context, cl entities.Claw, server entities.Server) (hosting.Container, error)
	Start(ctx context.Context, cl entities.Claw, server entities.Server) error
	Stop(ctx context.Context, cl entities.Claw, server entities.Server) error
	Delete(ctx context.Context, cl entities.Claw, server entities.Server, deleteConfig bool) error
	Update(ctx context.Context, cl entities.Claw, server entities.Server) error
	ConfigArchive(
		ctx context.Context,
		cl entities.Claw,
		server entities.Server,
		deleteAfter bool,
	) (io.ReadCloser, error)
	RestoreConfigArchive(
		ctx context.Context,
		cl entities.Claw,
		server entities.Server,
		body io.Reader,
	) error
}
