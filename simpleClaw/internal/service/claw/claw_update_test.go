package claw

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/claw/commands"

	"github.com/google/uuid"
)

type updateTestClawStorage struct {
	existing        entities.Claw
	updateCalled    bool
	updatedClaw     entities.Claw
	updatedChannels []uuid.UUID
	replaceChannels bool
	updateErr       error
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

func (s *updateTestClawStorage) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (s *updateTestClawStorage) UpdateRuntime(context.Context, uuid.UUID, uuid.UUID, string, string) error {
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

type updateTestServerStorage struct{}

func (s *updateTestServerStorage) GetAvailable(context.Context) (entities.Server, error) {
	return entities.Server{}, nil
}

func (s *updateTestServerStorage) GetAll(context.Context) ([]entities.Server, error) {
	return nil, nil
}

func (s *updateTestServerStorage) GetByID(context.Context, uuid.UUID) (entities.Server, error) {
	return entities.Server{}, nil
}

type updateTestKeys struct {
	resolveModelResult string
	resolveModelCalls  int
}

func (k *updateTestKeys) Create(context.Context, uuid.UUID, string, float64) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{}, nil
}

func (k *updateTestKeys) ResolveModel(context.Context, string) (string, error) {
	k.resolveModelCalls++
	return k.resolveModelResult, nil
}

func (h *updateTestHosting) Create(context.Context, entities.Claw, entities.Server) (hosting.Container, error) {
	return hosting.Container{}, nil
}

func (h *updateTestHosting) Start(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *updateTestHosting) Stop(context.Context, entities.Claw, entities.Server) error {
	return nil
}

func (h *updateTestHosting) Delete(context.Context, entities.Claw, entities.Server, bool) error {
	return nil
}

type updateTestHosting struct {
	updateErr error
	calls     int
}

func (h *updateTestHosting) Update(context.Context, entities.Claw, entities.Server) error {
	h.calls++
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
	return nil
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

func TestServiceUpdate_ArchiveErrorIsStageAware(t *testing.T) {
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
		"",
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

	if stageErr.Stage != updateStageArchiveSync {
		t.Fatalf("unexpected stage: %s", stageErr.Stage)
	}
}

func TestServiceUpdate_WritesArchiveFromDesiredConfig(t *testing.T) {
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

	archiveDir := t.TempDir()
	hostingStub := &updateTestHosting{}

	svc := NewClaw(
		storage,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		hostingStub,
		&updateTestKeys{resolveModelResult: "openrouter/openai/gpt-4.1"},
		archiveDir,
		GmailWatchConfig{},
	)

	_, err := svc.Update(context.Background(), commands.UpdateClaw{
		UserID: userID,
		ClawID: clawID,
		Model:  strPtr("openai/gpt-4.1"),
	})
	if err != nil {
		t.Fatalf("update returned error: %v", err)
	}

	if hostingStub.calls != 1 {
		t.Fatalf("expected hosting update to be called once, got %d", hostingStub.calls)
	}

	archived, err := readArchivedClawConfig(archiveDir, userID, clawID)
	if err != nil {
		t.Fatalf("read archive config: %v", err)
	}

	gotModel := strings.TrimSpace(currentPrimaryModel(archived))
	if gotModel != "openrouter/openai/gpt-4.1" {
		t.Fatalf("unexpected archived model: %s", gotModel)
	}
}

func readArchivedClawConfig(archiveRoot string, userID, clawID uuid.UUID) (entities.ClawConfig, error) {
	archivePath := archiveRoot + "/" + userID.String() + "/" + clawID.String() + ".tar"
	f, err := os.Open(archivePath)
	if err != nil {
		return entities.ClawConfig{}, err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return entities.ClawConfig{}, io.EOF
			}
			return entities.ClawConfig{}, err
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		var cfg entities.ClawConfig
		if err := json.NewDecoder(tr).Decode(&cfg); err != nil {
			return entities.ClawConfig{}, err
		}

		return cfg, nil
	}
}
