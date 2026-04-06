package claw

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/claw/commands"
)

type updateTestClawStorage struct {
	existing        entities.Claw
	updateCalled    bool
	updatedClaw     entities.Claw
	updatedChannels []uuid.UUID
	replaceChannels bool
	updateErr       error
	deleteCalled    bool
	deletedID       uuid.UUID
	deletedUserID   uuid.UUID
	runtimeUpdated  entities.Claw
}

func (s *updateTestClawStorage) Create(context.Context, entities.Claw, []uuid.UUID) error {
	return nil
}

func (s *updateTestClawStorage) GetByID(_ context.Context, _, _ uuid.UUID) (entities.Claw, error) {
	if s.existing.ID == uuid.Nil {
		return entities.Claw{}, sql.ErrNotFound
	}

	return s.existing, nil
}

func (s *updateTestClawStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Claw, error) {
	return nil, nil
}

func (s *updateTestClawStorage) CountOccupiedByServer(context.Context) (map[uuid.UUID]int, error) {
	return map[uuid.UUID]int{}, nil
}

func (s *updateTestClawStorage) Update(
	_ context.Context,
	cl entities.Claw,
	channelIDs []uuid.UUID,
	replaceChannels bool,
) error {
	s.updateCalled = true
	s.updatedClaw = cl
	s.updatedChannels = channelIDs
	s.replaceChannels = replaceChannels

	return s.updateErr
}

func (s *updateTestClawStorage) Delete(_ context.Context, id uuid.UUID, userID uuid.UUID) error {
	s.deleteCalled = true
	s.deletedID = id
	s.deletedUserID = userID

	return nil
}

func (s *updateTestClawStorage) UpdateRuntime(
	_ context.Context,
	clID uuid.UUID,
	serverID uuid.UUID,
	containerID string,
	status string,
) error {
	s.runtimeUpdated.ID = clID
	s.runtimeUpdated.ServerID = serverID
	s.runtimeUpdated.ContainerID = containerID
	s.runtimeUpdated.Status = status

	return nil
}

type updateTestChannelStorage struct{}

func (s *updateTestChannelStorage) GetByIDs(context.Context, []uuid.UUID, uuid.UUID) ([]entities.Channel, error) {
	return nil, nil
}

type updateTestUserStorage struct{}

func (s *updateTestUserStorage) GetByID(context.Context, uuid.UUID) (entities.User, error) {
	return entities.User{}, nil
}

func (s *updateTestUserStorage) UpdateOpenRouterKey(context.Context, uuid.UUID, entities.OpenRouterKey) error {
	return nil
}

func (s *updateTestUserStorage) GetGmailToken(context.Context, uuid.UUID) (entities.GmailToken, error) {
	return entities.GmailToken{}, nil
}

type updateTestServerStorage struct {
	server  entities.Server
	servers []entities.Server
}

func (s *updateTestServerStorage) GetAvailable(context.Context) (entities.Server, error) {
	return s.server, nil
}

func (s *updateTestServerStorage) GetAll(context.Context) ([]entities.Server, error) {
	if len(s.servers) > 0 {
		return append([]entities.Server(nil), s.servers...), nil
	}

	if s.server.ID == uuid.Nil {
		return nil, nil
	}

	return []entities.Server{s.server}, nil
}

func (s *updateTestServerStorage) GetByID(_ context.Context, id uuid.UUID) (entities.Server, error) {
	if s.server.ID == id {
		return s.server, nil
	}

	for _, srv := range s.servers {
		if srv.ID == id {
			return srv, nil
		}
	}

	return entities.Server{}, nil
}

type updateTestKeys struct {
	resolveModelResult string
	resolveModelCalls  int
}

func (k *updateTestKeys) Create(context.Context, uuid.UUID, float64) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{}, nil
}

func (k *updateTestKeys) ResolveModel(context.Context, string) (string, error) {
	k.resolveModelCalls++

	return k.resolveModelResult, nil
}

type updateTestHosting struct {
	createContainer hosting.Container
	createErr       error
	updateErr       error
	startErr        error
	deleteErr       error
	restoreErr      error
	updateCalls     int
	createCalls     int
	startCalls      int
	deleteCalls     int
	restoreCalls    int
}

func (h *updateTestHosting) Create(context.Context, entities.Claw, entities.Server) (hosting.Container, error) {
	h.createCalls++

	if h.createErr != nil {
		return hosting.Container{}, h.createErr
	}

	return h.createContainer, nil
}

func (h *updateTestHosting) Start(context.Context, entities.Claw, entities.Server) error {
	h.startCalls++

	return h.startErr
}

func (h *updateTestHosting) Stop(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *updateTestHosting) Delete(context.Context, entities.Claw, entities.Server, bool) error {
	h.deleteCalls++

	return h.deleteErr
}

func (h *updateTestHosting) Update(context.Context, entities.Claw, entities.Server) error {
	h.updateCalls++

	return h.updateErr
}

func (h *updateTestHosting) ApprovePairing(context.Context, entities.Claw, entities.Server, string) error {
	return nil
}

func (h *updateTestHosting) Connect(context.Context, entities.Claw, entities.Server, string, []byte) error {
	return nil
}

func (h *updateTestHosting) ConfigArchive(context.Context, entities.Claw, entities.Server, bool) (io.ReadCloser, error) {
	return nil, nil
}

func (h *updateTestHosting) RestoreConfigArchive(context.Context, entities.Claw, entities.Server, io.Reader) error {
	h.restoreCalls++

	return h.restoreErr
}

func strPtr(v string) *string {
	return &v
}

func TestServiceUpdate_AllowsPartialWithoutNameAndModel(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	keys := &updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		keys,
		t.TempDir(),
		GmailWatchConfig{},
	)

	updated, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}

	if !storage.updateCalled {
		t.Fatalf("expected storage update to be called")
	}

	if storage.replaceChannels {
		t.Fatalf("did not expect channel replacement flag for omitted channelIds")
	}

	if updated.Name != "existing-name" {
		t.Fatalf("expected name to stay unchanged, got %q", updated.Name)
	}

	if keys.resolveModelCalls != 0 {
		t.Fatalf("expected model resolver to not be called, got %d calls", keys.resolveModelCalls)
	}
}

func TestServiceUpdate_EmptyChannelIDsPreserveClearIntent(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	keys := &updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		keys,
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID:     userID,
		ClawID:     clawID,
		Name:       strPtr("updated-name"),
		Model:      strPtr("openai/gpt-4.1-mini"),
		ChannelIDs: []uuid.UUID{},
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}

	if storage.updatedChannels == nil {
		t.Fatalf("expected explicit empty channel list to stay non-nil")
	}

	if len(storage.updatedChannels) != 0 {
		t.Fatalf("expected no channel ids, got %d", len(storage.updatedChannels))
	}

	if !storage.replaceChannels {
		t.Fatalf("expected channel replacement flag for explicit empty channelIds")
	}
}

func TestServiceUpdate_RejectsEmptyNameWhenProvided(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Name:   strPtr("   "),
	})
	if err == nil {
		t.Fatalf("expected validation error for empty name")
	}
}

func TestServiceUpdate_RejectsEmptyModelWhenProvided(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Model:  strPtr(" "),
	})
	if err == nil {
		t.Fatalf("expected validation error for empty model")
	}
}

func TestServiceUpdate_HostingFailureSkipsDBUpdate(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:          clawID,
		UserID:      userID,
		Name:        "existing-name",
		ServerID:    uuid.New(),
		ContainerID: "container-id",
		Config:      cfg,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}}

	hostingStub := &updateTestHosting{updateErr: errors.New("hosting failed")}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Name:   strPtr("updated"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	var stageErr *UpdateStageError
	if !errors.As(err, &stageErr) {
		t.Fatalf("expected UpdateStageError, got %v", err)
	}

	if stageErr.Stage != updateStageHostingUpdate {
		t.Fatalf("unexpected stage: %s", stageErr.Stage)
	}

	if storage.updateCalled {
		t.Fatalf("db update must not be called when hosting update fails")
	}
}

func TestServiceUpdate_DBErrorIsStageAware(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{
		existing: entities.Claw{
			ID:        clawID,
			UserID:    userID,
			Name:      "existing-name",
			Config:    cfg,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		updateErr: sql.ErrConflict,
	}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Name:   strPtr("updated"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	var stageErr *UpdateStageError
	if !errors.As(err, &stageErr) {
		t.Fatalf("expected UpdateStageError, got %v", err)
	}

	if stageErr.Stage != updateStageDBUpdate {
		t.Fatalf("unexpected stage: %s", stageErr.Stage)
	}
}

func TestServiceUpdate_StoppedClawSkipsHostingAndArchiveSync(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	archiveDir := t.TempDir()
	hostingStub := &updateTestHosting{}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		archiveDir,
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Name:   strPtr("updated"),
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}

	if hostingStub.updateCalls != 0 {
		t.Fatalf("expected hosting update to be skipped, got %d calls", hostingStub.updateCalls)
	}

	archivePath := filepath.Join(archiveDir, userID.String(), clawID.String()+".tar")
	if _, err := os.Stat(archivePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no archive to be written, stat err=%v", err)
	}
}

func TestServiceStart_WithoutRuntimeCreatesAndStartsContainer(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	hostingStub := &updateTestHosting{
		createContainer: hosting.Container{ID: "container-id", ServerID: serverID},
	}
	serverStorage := &updateTestServerStorage{
		servers: []entities.Server{{ID: serverID, MaxClaws: 2}},
	}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		serverStorage,
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	cl, err := svc.Start(context.Background(), commands.StartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("start returned error: %v", err)
	}

	if hostingStub.restoreCalls != 0 {
		t.Fatalf("expected no archive restore on first start, got %d calls", hostingStub.restoreCalls)
	}

	if hostingStub.createCalls != 1 {
		t.Fatalf("expected hosting create once, got %d", hostingStub.createCalls)
	}

	if hostingStub.startCalls != 1 {
		t.Fatalf("expected hosting start once, got %d", hostingStub.startCalls)
	}

	if storage.runtimeUpdated.ContainerID != "container-id" {
		t.Fatalf("expected runtime container id to be saved, got %q", storage.runtimeUpdated.ContainerID)
	}

	if storage.runtimeUpdated.ServerID != serverID {
		t.Fatalf("expected runtime server id to be saved, got %s", storage.runtimeUpdated.ServerID)
	}

	if storage.runtimeUpdated.Status != entities.StatusRunning {
		t.Fatalf("expected running status, got %q", storage.runtimeUpdated.Status)
	}

	if cl.ContainerID != "container-id" {
		t.Fatalf("expected returned claw container id, got %q", cl.ContainerID)
	}
}

func TestServiceStart_WithArchiveRestoresBeforeCreate(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars[openRouterAPIKeyVar] = "secret"

	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, userID.String(), clawID.String()+".tar")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}

	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{Name: clawID.String() + "/openclaw.json", Mode: 0o644, Size: int64(len(`{}`))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte(`{}`)); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close archive file: %v", err)
	}

	hostingStub := &updateTestHosting{
		createContainer: hosting.Container{ID: "container-id", ServerID: serverID},
	}
	serverStorage := &updateTestServerStorage{
		servers: []entities.Server{{ID: serverID, MaxClaws: 2}},
	}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		serverStorage,
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		archiveDir,
		GmailWatchConfig{},
	)

	_, err = svc.Start(context.Background(), commands.StartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("start returned error: %v", err)
	}

	if hostingStub.restoreCalls != 1 {
		t.Fatalf("expected archive restore once, got %d", hostingStub.restoreCalls)
	}

	if hostingStub.createCalls != 1 {
		t.Fatalf("expected hosting create once, got %d", hostingStub.createCalls)
	}

	if hostingStub.startCalls != 1 {
		t.Fatalf("expected hosting start once, got %d", hostingStub.startCalls)
	}
}

func TestServiceDelete_WithoutRuntimeDeletesOnlyDBAndArchive(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:        clawID,
		UserID:    userID,
		Name:      "existing-name",
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}}

	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, userID.String(), clawID.String()+".tar")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive file: %v", err)
	}

	hostingStub := &updateTestHosting{}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		archiveDir,
		GmailWatchConfig{},
	)

	err := svc.Delete(context.Background(), commands.DeleteClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("delete returned error: %v", err)
	}

	if hostingStub.deleteCalls != 0 {
		t.Fatalf("expected hosting delete to be skipped, got %d", hostingStub.deleteCalls)
	}

	if !storage.deleteCalled {
		t.Fatalf("expected db delete to be called")
	}

	if _, err := os.Stat(archivePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected archive file to be removed, stat err=%v", err)
	}
}

func TestServiceDelete_WithRuntimeDeletesHostingAndDB(t *testing.T) {
	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	storage := &updateTestClawStorage{existing: entities.Claw{
		ID:          clawID,
		UserID:      userID,
		Name:        "existing-name",
		ServerID:    serverID,
		ContainerID: "container-id",
		Config:      cfg,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}}

	hostingStub := &updateTestHosting{}
	serverStorage := &updateTestServerStorage{server: entities.Server{ID: serverID}}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		serverStorage,
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1-mini"},
		t.TempDir(),
		GmailWatchConfig{},
	)

	err := svc.Delete(context.Background(), commands.DeleteClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("delete returned error: %v", err)
	}

	if hostingStub.deleteCalls != 1 {
		t.Fatalf("expected hosting delete once, got %d", hostingStub.deleteCalls)
	}

	if !storage.deleteCalled {
		t.Fatalf("expected db delete to be called")
	}
}
