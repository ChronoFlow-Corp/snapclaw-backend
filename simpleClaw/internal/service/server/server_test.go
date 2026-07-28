package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/server/commands"
)

type testStorage struct {
	created  entities.Server
	updated  entities.Server
	servers  []entities.Server
	existing entities.Server
}

func (s *testStorage) Create(_ context.Context, srv entities.Server) error {
	s.created = srv

	return nil
}

func (s *testStorage) GetAll(context.Context) ([]entities.Server, error) {
	return append([]entities.Server(nil), s.servers...), nil
}

func (s *testStorage) GetByID(context.Context, uuid.UUID) (entities.Server, error) {
	return s.existing, nil
}

func (s *testStorage) Update(_ context.Context, srv entities.Server) error {
	s.updated = srv

	return nil
}

func (s *testStorage) Delete(context.Context, uuid.UUID) error {
	return nil
}

type testCapacityResolver struct {
	maxClaws int
	err      error
}

func (r *testCapacityResolver) Capacity(context.Context, entities.Server) (int, error) {
	if r.err != nil {
		return 0, r.err
	}

	return r.maxClaws, nil
}

func TestServiceCreate_SyncsCapacityFromContainerManager(t *testing.T) {
	t.Parallel()

	storage := &testStorage{}
	svc := New(storage, &testCapacityResolver{maxClaws: 7})

	srv, err := svc.Create(context.Background(), commands.CreateServer{
		Name:      "alpha",
		URL:       "http://alpha.internal",
		ProxyURL:  "http://alpha.internal/gmail-pubsub",
		Status:    "ready",
		SecretKey: "secret",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if srv.MaxClaws != 7 {
		t.Fatalf("result maxClaws = %d, want 7", srv.MaxClaws)
	}

	if storage.created.MaxClaws != 7 {
		t.Fatalf("stored maxClaws = %d, want 7", storage.created.MaxClaws)
	}
}

func TestServiceUpdate_RefreshesCapacityFromContainerManager(t *testing.T) {
	t.Parallel()

	serverID := uuid.New()
	storage := &testStorage{
		existing: entities.Server{
			ID:        serverID,
			Name:      "alpha",
			URL:       "http://alpha.internal",
			ProxyURL:  "http://alpha.internal/gmail-pubsub",
			Status:    "ready",
			SecretKey: "secret",
			MaxClaws:  3,
			CreatedAt: time.Now(),
		},
	}
	svc := New(storage, &testCapacityResolver{maxClaws: 11})

	srv, err := svc.Update(context.Background(), commands.UpdateServer{
		ID:        serverID,
		Name:      "alpha-updated",
		URL:       "http://alpha.internal",
		ProxyURL:  "http://alpha.internal/gmail-pubsub",
		Status:    "ready",
		SecretKey: "secret",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if srv.MaxClaws != 11 {
		t.Fatalf("result maxClaws = %d, want 11", srv.MaxClaws)
	}

	if storage.updated.MaxClaws != 11 {
		t.Fatalf("stored maxClaws = %d, want 11", storage.updated.MaxClaws)
	}
}

func TestServiceCreate_FailsWhenCapacitySyncFails(t *testing.T) {
	t.Parallel()

	svc := New(&testStorage{}, &testCapacityResolver{err: errors.New("boom")})

	_, err := svc.Create(context.Background(), commands.CreateServer{
		Name:      "alpha",
		URL:       "http://alpha.internal",
		ProxyURL:  "http://alpha.internal/gmail-pubsub",
		Status:    "ready",
		SecretKey: "secret",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServiceSyncCapacities_RefreshesStoredServers(t *testing.T) {
	t.Parallel()

	serverID := uuid.New()
	storage := &testStorage{
		servers: []entities.Server{{
			ID:        serverID,
			Name:      "alpha",
			URL:       "http://alpha.internal",
			ProxyURL:  "http://alpha.internal/gmail-pubsub",
			Status:    "ready",
			SecretKey: "secret",
			MaxClaws:  2,
			CreatedAt: time.Now(),
		}},
	}
	svc := New(storage, &testCapacityResolver{maxClaws: 9})

	err := svc.SyncCapacities(context.Background())
	if err != nil {
		t.Fatalf("sync capacities: %v", err)
	}

	if storage.updated.MaxClaws != 9 {
		t.Fatalf("stored maxClaws = %d, want 9", storage.updated.MaxClaws)
	}
}
