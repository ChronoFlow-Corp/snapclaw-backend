package claws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"shared/pkg/observability"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Storage struct {
	db      *gorm.DB
	metrics *observability.OperationMetrics
}

type LifecycleUpdate struct {
	DesiredState       entities.ClawDesiredState
	ObservedState      entities.ClawObservedState
	LifecycleStatus    entities.ClawLifecycleStatus
	CurrentOperationID *uuid.UUID
	LastLifecycleError string
	OnboardingComplete *bool
}

type occupiedServerRow struct {
	ServerID uuid.UUID
	Count    int
}

func NewStorage(db *gorm.DB, metrics ...*observability.OperationMetrics) *Storage {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Storage{db: db, metrics: opMetrics}
}

func (s *Storage) DB() *gorm.DB {
	return s.db
}

func (s *Storage) WithDB(db *gorm.DB) *Storage {
	if db == nil {
		return s
	}

	copy := *s
	copy.db = db

	return &copy
}

func (s *Storage) Create(
	ctx context.Context,
	cl entities.Claw,
	channelIDs []uuid.UUID,
) error {
	const op = "storages.Claws.Create"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.create",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	cfg, err := json.Marshal(cl.Config)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	var serverID *uuid.UUID
	if cl.ServerID != uuid.Nil {
		serverID = &cl.ServerID
	}

	lifecycleState := normalizeLifecycleState(cl.ClawLifecycleState)

	model := models.Claw{
		ID:                 cl.ID,
		Name:               cl.Name,
		Config:             datatypes.JSON(cfg),
		UserID:             cl.UserID,
		ServerID:           serverID,
		ContainerID:        cl.ContainerID,
		DesiredState:       string(lifecycleState.DesiredState),
		ObservedState:      string(lifecycleState.ObservedState),
		LifecycleStatus:    string(lifecycleState.LifecycleStatus),
		LastLifecycleError: cl.LastError,
		OnboardingComplete: cl.OnboardingComplete,
		CurrentOperationID: cl.CurrentOperationID,
		CreatedAt:          cl.CreatedAt,
		UpdatedAt:          cl.UpdatedAt,
	}

	err = gorm.G[models.Claw](s.db).Create(ctx, &model)
	if err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if len(channelIDs) == 0 {
		return nil
	}

	relations := make([]clawChannel, 0, len(channelIDs))
	for _, chID := range channelIDs {
		relations = append(relations, clawChannel{
			ClawID:    cl.ID,
			ChannelID: chID,
		})
	}

	if err = s.db.WithContext(ctx).Table("claw_channels").Create(&relations).Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	return nil
}

func (s *Storage) UpdateLifecycle(ctx context.Context, clID uuid.UUID, update LifecycleUpdate) error {
	const op = "storages.Claws.UpdateLifecycle"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.update_lifecycle",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	updates := map[string]any{
		"desired_state":        string(update.DesiredState),
		"observed_state":       string(update.ObservedState),
		"lifecycle_status":     string(update.LifecycleStatus),
		"current_operation_id": update.CurrentOperationID,
		"last_lifecycle_error": update.LastLifecycleError,
		"updated_at":           nowExpr(s.db),
	}
	if update.OnboardingComplete != nil {
		updates["onboarding_complete"] = *update.OnboardingComplete
	}

	tx := s.db.WithContext(ctx).Model(&models.Claw{}).Where("id = ?", clID).Updates(updates)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) UpdateRuntime(ctx context.Context, clID uuid.UUID, update entities.ClawRuntimeUpdate) error {
	const op = "storages.Claws.UpdateRuntime"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.update_runtime",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	var sID *uuid.UUID

	if update.ServerID != uuid.Nil {
		sID = &update.ServerID
	}

	updates := map[string]any{
		"server_id":            sID,
		"container_id":         update.ContainerRecordID,
		"desired_state":        string(update.DesiredState),
		"observed_state":       string(update.ObservedState),
		"lifecycle_status":     string(update.LifecycleStatus),
		"last_lifecycle_error": update.LastLifecycleError,
		"last_runtime_sync_at": nowExpr(s.db),
		"updated_at":           nowExpr(s.db),
	}

	tx := s.db.WithContext(ctx).Model(&models.Claw{}).Where("id = ?", clID).Updates(updates)
	if err := tx.Error; err != nil {
		return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	if tx.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func (s *Storage) GetByID(
	ctx context.Context,
	id uuid.UUID,
	userID uuid.UUID,
) (entities.Claw, error) {
	const op = "storages.Claws.GetByID"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.get_by_id",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	clDB, err := gorm.G[models.Claw](s.db).Where("id = ? AND user_id = ?", id, userID).First(ctx)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	var cfg entities.ClawConfig

	if len(clDB.Config) > 0 {
		err := json.Unmarshal(clDB.Config, &cfg)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	return mapClawModel(clDB, cfg), nil
}

func (s *Storage) GetBySystemID(ctx context.Context, id uuid.UUID) (entities.Claw, error) {
	const op = "storages.Claws.GetBySystemID"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.get_by_system_id",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	clDB, err := gorm.G[models.Claw](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	var cfg entities.ClawConfig
	if len(clDB.Config) > 0 {
		err := json.Unmarshal(clDB.Config, &cfg)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	return mapClawModel(clDB, cfg), nil
}

func (s *Storage) GetNextReconcilePending(ctx context.Context) (entities.Claw, error) {
	const op = "storages.Claws.GetNextReconcilePending"

	clDB, err := gorm.G[models.Claw](s.db).
		Where("lifecycle_status = ?", entities.ClawLifecycleStatusReconcilePending).
		Order("updated_at ASC").
		First(ctx)
	if err != nil {
		return entities.Claw{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	var cfg entities.ClawConfig
	if len(clDB.Config) > 0 {
		err := json.Unmarshal(clDB.Config, &cfg)
		if err != nil {
			return entities.Claw{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	return mapClawModel(clDB, cfg), nil
}

func (s *Storage) GetByUserID(
	ctx context.Context,
	userID uuid.UUID,
) ([]entities.Claw, error) {
	const op = "storages.Claws.GetByUserID"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.list_by_user",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	clsDB, err := gorm.G[models.Claw](s.db).Where("user_id = ?", userID).Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	cls := make([]entities.Claw, 0, len(clsDB))
	for _, clDB := range clsDB {
		var cfg entities.ClawConfig

		if len(clDB.Config) > 0 {
			err := json.Unmarshal(clDB.Config, &cfg)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", op, err)
			}
		}

		cls = append(cls, mapClawModel(clDB, cfg))
	}

	return cls, nil
}

func (s *Storage) ListRuntimeSyncCandidates(ctx context.Context, limit int) ([]entities.Claw, error) {
	const op = "storages.Claws.ListRuntimeSyncCandidates"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.list_runtime_sync_candidates",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	query := gorm.G[models.Claw](s.db).
		Where("server_id IS NOT NULL OR container_id <> ''").
		Order("updated_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	clsDB, err := query.Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	cls := make([]entities.Claw, 0, len(clsDB))
	for _, clDB := range clsDB {
		var cfg entities.ClawConfig

		if len(clDB.Config) > 0 {
			err := json.Unmarshal(clDB.Config, &cfg)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", op, err)
			}
		}

		cls = append(cls, mapClawModel(clDB, cfg))
	}

	return cls, nil
}

func (s *Storage) CountOccupiedByServer(ctx context.Context) (map[uuid.UUID]int, error) {
	const op = "storages.Claws.CountOccupiedByServer"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.count_occupied_by_server",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	rows := make([]occupiedServerRow, 0)

	err = s.db.WithContext(ctx).
		Model(&models.Claw{}).
		Select("server_id, COUNT(*) AS count").
		Where("server_id <> ?", uuid.Nil).
		Where("container_id <> ''").
		Group("server_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	result := make(map[uuid.UUID]int, len(rows))
	for _, row := range rows {
		if row.ServerID == uuid.Nil {
			continue
		}

		result[row.ServerID] = row.Count
	}

	return result, nil
}

func derefServerID(serverID *uuid.UUID) uuid.UUID {
	if serverID == nil {
		return uuid.Nil
	}

	return *serverID
}

func mapClawModel(clDB models.Claw, cfg entities.ClawConfig) entities.Claw {
	return entities.Claw{
		ID:          clDB.ID,
		Name:        clDB.Name,
		UserID:      clDB.UserID,
		ServerID:    derefServerID(clDB.ServerID),
		ContainerID: clDB.ContainerID,
		ClawLifecycleState: entities.ClawLifecycleState{
			DesiredState:       entities.ClawDesiredState(clDB.DesiredState),
			ObservedState:      entities.ClawObservedState(clDB.ObservedState),
			LifecycleStatus:    entities.ClawLifecycleStatus(clDB.LifecycleStatus),
			CurrentOperationID: clDB.CurrentOperationID,
			LastError:          clDB.LastLifecycleError,
		},
		Config:             cfg,
		OnboardingComplete: clDB.OnboardingComplete,
		CreatedAt:          clDB.CreatedAt,
		UpdatedAt:          clDB.UpdatedAt,
	}
}

func normalizeLifecycleState(state entities.ClawLifecycleState) entities.ClawLifecycleState {
	defaults := entities.NewClawLifecycleState()

	if state.DesiredState == "" {
		state.DesiredState = defaults.DesiredState
	}

	if state.ObservedState == "" {
		state.ObservedState = defaults.ObservedState
	}

	if state.LifecycleStatus == "" {
		state.LifecycleStatus = defaults.LifecycleStatus
	}

	return state
}

func nowExpr(db *gorm.DB) any {
	if db.Dialector.Name() == "sqlite" {
		return time.Now().UTC()
	}

	return gorm.Expr("NOW()")
}

func (s *Storage) Update(
	ctx context.Context,
	cl entities.Claw,
	channelIDs []uuid.UUID,
	replaceChannels bool,
) error {
	const op = "storages.Claws.Update"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.update",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	cfg, err := json.Marshal(cl.Config)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"name":                cl.Name,
			"config":              datatypes.JSON(cfg),
			"onboarding_complete": cl.OnboardingComplete,
			"updated_at":          cl.UpdatedAt,
		}

		res := tx.Model(&models.Claw{}).
			Where("id = ? AND user_id = ?", cl.ID, cl.UserID).
			Updates(updates)
		err := res.Error
		if err != nil {
			return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
		}

		if res.RowsAffected == 0 {
			return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
		}

		if replaceChannels {
			err := tx.Table("claw_channels").
				Where("claw_id = ?", cl.ID).
				Delete(&clawChannel{}).
				Error
			if err != nil {
				return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
			}

			if len(channelIDs) == 0 {
				return nil
			}

			relations := make([]clawChannel, 0, len(channelIDs))
			for _, chID := range channelIDs {
				relations = append(relations, clawChannel{
					ClawID:    cl.ID,
					ChannelID: chID,
				})
			}

			err = tx.Table("claw_channels").Create(&relations).Error
			if err != nil {
				return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
			}
		}

		return nil
	})
}

func (s *Storage) Delete(
	ctx context.Context,
	id uuid.UUID,
	userID uuid.UUID,
) error {
	const op = "storages.Claws.Delete"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"storage.claws",
		"storage.claw.delete",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Table("claw_channels").Where("claw_id = ?", id).Delete(&clawChannel{}).Error
		if err != nil {
			return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
		}

		res := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Claw{})
		err = res.Error
		if err != nil {
			return fmt.Errorf("%s: %w", op, sql.TranslateError(err))
		}

		if res.RowsAffected == 0 {
			return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
		}

		return nil
	})
}
