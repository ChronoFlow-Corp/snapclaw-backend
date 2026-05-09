package claw

import (
	"context"
	"io"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/storages/claws"

	"github.com/google/uuid"
)

type clawStorage interface {
	Create(ctx context.Context, cl entities.Claw, channelIDs []uuid.UUID) error
	GetBySystemID(ctx context.Context, id uuid.UUID) (entities.Claw, error)
	GetNextReconcilePending(ctx context.Context) (entities.Claw, error)
	GetByID(ctx context.Context, id, userID uuid.UUID) (entities.Claw, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Claw, error)
	ListRuntimeSyncCandidates(ctx context.Context, limit int) ([]entities.Claw, error)
	CountOccupiedByServer(ctx context.Context) (map[uuid.UUID]int, error)
	UpdateLifecycle(ctx context.Context, clID uuid.UUID, update claws.LifecycleUpdate) error
	Update(
		ctx context.Context,
		cl entities.Claw,
		channelIDs []uuid.UUID,
		replaceChannels bool,
	) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
	UpdateRuntime(
		ctx context.Context,
		clID uuid.UUID,
		update entities.ClawRuntimeUpdate,
	) error
}

type lifecycleOperationStorage interface {
	Create(ctx context.Context, op entities.ClawLifecycleOperation) error
	GetActiveByClawID(ctx context.Context, clawID uuid.UUID) (entities.ClawLifecycleOperation, error)
	LockNextRunnable(ctx context.Context, now time.Time) (entities.ClawLifecycleOperation, error)
	Update(ctx context.Context, op entities.ClawLifecycleOperation) error
}

type channelStorage interface {
	GetByIDs(ctx context.Context, ids []uuid.UUID, userID uuid.UUID) ([]entities.Channel, error)
}

type userStorage interface {
	GetByID(ctx context.Context, id uuid.UUID) (entities.User, error)
	UpdateOpenRouterKey(ctx context.Context, id uuid.UUID, key entities.OpenRouterKey) error
}

type integrationStorage interface {
	GetByID(ctx context.Context, id, userID uuid.UUID) (entities.AccountIntegration, error)
}

type capabilityAttachmentStorage interface {
	Upsert(ctx context.Context, attachment entities.ClawCapabilityAttachment) error
	ListByClawID(ctx context.Context, clawID, userID uuid.UUID) ([]entities.ClawCapabilityAttachment, error)
}

type serverStorage interface {
	GetAll(ctx context.Context) ([]entities.Server, error)
	GetAvailable(ctx context.Context) (entities.Server, error)
	GetByID(ctx context.Context, id uuid.UUID) (entities.Server, error)
}

type apiKeyManager interface {
	Create(
		ctx context.Context,
		userID uuid.UUID,
		monthlyBudgetUSD float64,
	) (entities.OpenRouterKey, error)
	ResolveModel(ctx context.Context, model string) (string, error)
}

type hostingManager interface {
	Create(ctx context.Context, cl entities.Claw, server entities.Server) (hosting.Container, error)
	ConfigArchive(ctx context.Context, cl entities.Claw, server entities.Server, deleteAfter bool) (io.ReadCloser, error)
	RestoreConfigArchive(ctx context.Context, cl entities.Claw, server entities.Server, body io.Reader) error
	Start(ctx context.Context, cl entities.Claw, server entities.Server) error
	Stop(ctx context.Context, cl entities.Claw, server entities.Server) error
	Delete(ctx context.Context, cl entities.Claw, server entities.Server, deleteConfig bool) error
	State(ctx context.Context, cl entities.Claw, server entities.Server) (hosting.RuntimeState, error)
	ApprovePairing(ctx context.Context, cl entities.Claw, server entities.Server, channelType string, code string) error
	Connect(
		ctx context.Context,
		cl entities.Claw,
		server entities.Server,
		provider string,
		token []byte,
	) error
}
