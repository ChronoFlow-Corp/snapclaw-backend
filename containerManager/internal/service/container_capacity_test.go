package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"containermanager/internal/entities"
	"containermanager/internal/infrastucture/pkg/configurer"
	"containermanager/internal/infrastucture/pkg/docker"
	"containermanager/internal/infrastucture/sql/storage"
	"containermanager/internal/service/commands"
	"github.com/google/uuid"
)

type fakeClawRepository struct {
	getByUserClawIDResult entities.Container
	getByUserClawIDErr    error
	getAllResult          []entities.Container
	updateCalled          bool
	updated               entities.Container
	containersByClawID    map[string]entities.Container
}

func (r *fakeClawRepository) Create(context.Context, entities.Container) error {
	return nil
}

func (r *fakeClawRepository) GetByUserClawID(_ context.Context, _ string, clawID string) (entities.Container, error) {
	if len(r.containersByClawID) > 0 {
		if cl, ok := r.containersByClawID[clawID]; ok {
			return cl, nil
		}
	}

	if r.getByUserClawIDErr != nil {
		return entities.Container{}, r.getByUserClawIDErr
	}

	return r.getByUserClawIDResult, nil
}

func (r *fakeClawRepository) GetAll(context.Context) ([]entities.Container, error) {
	return append([]entities.Container(nil), r.getAllResult...), nil
}

func (r *fakeClawRepository) Update(_ context.Context, cl entities.Container) error {
	r.updateCalled = true
	r.updated = cl

	return nil
}

func (r *fakeClawRepository) Remove(context.Context, entities.Container) error {
	return nil
}

func (r *fakeClawRepository) GetByID(context.Context, string) (entities.Container, error) {
	return entities.Container{}, nil
}

type fakeRuntime struct {
	createCalled bool
	startCalled  bool
	startFunc    func(containerID string) error
}

func (r *fakeRuntime) Create(context.Context, docker.CreateOptions) (string, error) {
	r.createCalled = true

	return "container-1", nil
}

func (r *fakeRuntime) Start(_ context.Context, containerID string) error {
	r.startCalled = true
	if r.startFunc != nil {
		return r.startFunc(containerID)
	}

	return nil
}

func (r *fakeRuntime) Stop(context.Context, string) error   { return nil }
func (r *fakeRuntime) Remove(context.Context, string) error { return nil }
func (r *fakeRuntime) ExecGmail(context.Context, string, []byte, docker.ExecGmailOptions) error {
	return nil
}

func (r *fakeRuntime) StartGmailWatch(context.Context, string, docker.ExecGmailWatchStartOptions) error {
	return nil
}

func (r *fakeRuntime) StartGmailWatcher(context.Context, string, docker.ExecGmailWatcherOptions) error {
	return nil
}

func TestContainerCreate_ReturnsCapacityErrorWhenMaxClawsReached(t *testing.T) {
	t.Parallel()

	runtime := &fakeRuntime{}
	repo := &fakeClawRepository{
		getByUserClawIDErr: storage.ErrNotFound,
		getAllResult: []entities.Container{
			{ID: uuid.New()},
		},
	}
	svc := &Container{
		clRepo:   repo,
		manager:  runtime,
		cfg:      configurer.NewClawConfigurer(t.TempDir(), "", -1, -1),
		p:        NewPorter(),
		maxClaws: 1,
	}

	_, err := svc.Create(context.Background(), commands.CreateClaw{
		UserID: "user-1",
		ClawID: "claw-1",
	})
	if err == nil {
		t.Fatal("expected capacity error")
	}

	if !errors.Is(err, ErrServerCapacityExceeded) {
		t.Fatalf("expected ErrServerCapacityExceeded, got %v", err)
	}

	if runtime.createCalled {
		t.Fatal("docker create should not be called when capacity is exhausted")
	}
}

func TestContainerStart_ColdStartRequiresEnoughMemory(t *testing.T) {
	t.Parallel()

	runtime := &fakeRuntime{}
	repo := &fakeClawRepository{
		getByUserClawIDResult: entities.Container{
			ID:             uuid.New(),
			UserID:         "user-1",
			ClawID:         "claw-1",
			ContainerID:    "container-1",
			Status:         entities.ContainerStatusStop,
			HasStartedOnce: false,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
	}
	svc := &Container{
		clRepo:              repo,
		manager:             runtime,
		readAvailableMemory: func() (uint64, error) { return coldStartMinAvailableBytes - 1, nil },
		coldStartMinBytes:   coldStartMinAvailableBytes,
		warmStartMinBytes:   warmStartMinAvailableBytes,
	}

	err := svc.Start(context.Background(), commands.StartClaw{UserID: "user-1", ClawID: "claw-1"})
	if err == nil {
		t.Fatal("expected memory admission error")
	}

	if !errors.Is(err, ErrServerMemoryUnavailable) {
		t.Fatalf("expected ErrServerMemoryUnavailable, got %v", err)
	}

	if runtime.startCalled {
		t.Fatal("docker start should not be called when memory is insufficient")
	}
}

func TestContainerStart_WarmRestartUsesWarmThreshold(t *testing.T) {
	t.Parallel()

	runtime := &fakeRuntime{}
	repo := &fakeClawRepository{
		getByUserClawIDResult: entities.Container{
			ID:             uuid.New(),
			UserID:         "user-1",
			ClawID:         "claw-1",
			ContainerID:    "container-1",
			Status:         entities.ContainerStatusStop,
			HasStartedOnce: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
	}
	svc := &Container{
		clRepo:              repo,
		manager:             runtime,
		readAvailableMemory: func() (uint64, error) { return warmStartMinAvailableBytes, nil },
		coldStartMinBytes:   coldStartMinAvailableBytes,
		warmStartMinBytes:   warmStartMinAvailableBytes,
	}

	err := svc.Start(context.Background(), commands.StartClaw{UserID: "user-1", ClawID: "claw-1"})
	if err != nil {
		t.Fatalf("start warm container: %v", err)
	}

	if !runtime.startCalled {
		t.Fatal("expected docker start to be called")
	}

	if !repo.updateCalled {
		t.Fatal("expected repository update")
	}

	if repo.updated.Status != entities.ContainerStatusRunning {
		t.Fatalf("status = %q, want %q", repo.updated.Status, entities.ContainerStatusRunning)
	}

	if !repo.updated.HasStartedOnce {
		t.Fatal("expected has_started_once to stay true")
	}
}

func TestContainerStart_SerializesConcurrentStarts(t *testing.T) {
	t.Parallel()

	firstStarted := make(chan struct{}, 1)
	secondStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})

	runtime := &fakeRuntime{}

	var startMu sync.Mutex

	startCalls := 0
	runtime.startFunc = func(containerID string) error {
		startMu.Lock()
		startCalls++
		call := startCalls
		startMu.Unlock()

		if call == 1 {
			firstStarted <- struct{}{}

			<-releaseFirst

			return nil
		}

		secondStarted <- struct{}{}

		return nil
	}

	repo := &fakeClawRepository{
		containersByClawID: map[string]entities.Container{
			"claw-1": {
				ID:             uuid.New(),
				UserID:         "user-1",
				ClawID:         "claw-1",
				ContainerID:    "container-1",
				Status:         entities.ContainerStatusStop,
				HasStartedOnce: true,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
			"claw-2": {
				ID:             uuid.New(),
				UserID:         "user-1",
				ClawID:         "claw-2",
				ContainerID:    "container-2",
				Status:         entities.ContainerStatusStop,
				HasStartedOnce: true,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
		},
	}
	svc := &Container{
		clRepo:              repo,
		manager:             runtime,
		readAvailableMemory: func() (uint64, error) { return warmStartMinAvailableBytes * 2, nil },
		coldStartMinBytes:   coldStartMinAvailableBytes,
		warmStartMinBytes:   warmStartMinAvailableBytes,
	}

	errCh := make(chan error, 2)

	go func() {
		errCh <- svc.Start(context.Background(), commands.StartClaw{UserID: "user-1", ClawID: "claw-1"})
	}()

	select {
	case <-firstStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first start did not reach runtime")
	}

	go func() {
		errCh <- svc.Start(context.Background(), commands.StartClaw{UserID: "user-1", ClawID: "claw-2"})
	}()

	select {
	case <-secondStarted:
		t.Fatal("second start reached runtime before first was released")
	case <-time.After(150 * time.Millisecond):
	}

	close(releaseFirst)

	for range 2 {
		err := <-errCh
		if err != nil {
			t.Fatalf("start returned error: %v", err)
		}
	}

	select {
	case <-secondStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second start never reached runtime after release")
	}
}
