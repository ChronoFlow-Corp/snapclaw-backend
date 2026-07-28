package clawcapability

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
)

type testClawStorage struct {
	claw        entities.Claw
	updateErr   error
	updatedClaw entities.Claw
}

func (s *testClawStorage) GetByID(context.Context, uuid.UUID, uuid.UUID) (entities.Claw, error) {
	return s.claw, nil
}

func (s *testClawStorage) Update(_ context.Context, cl entities.Claw, _ []uuid.UUID, _ bool) error {
	s.updatedClaw = cl
	return s.updateErr
}

type testAttachmentStorage struct {
	items       map[entities.CapabilityID]entities.ClawCapabilityAttachment
	deleteCalls int
	upsertCalls int
}

func newTestAttachmentStorage(items ...entities.ClawCapabilityAttachment) *testAttachmentStorage {
	store := &testAttachmentStorage{
		items: make(map[entities.CapabilityID]entities.ClawCapabilityAttachment, len(items)),
	}

	for _, item := range items {
		store.items[item.CapabilityID] = item
	}

	return store
}

func (s *testAttachmentStorage) Upsert(_ context.Context, attachment entities.ClawCapabilityAttachment) error {
	s.upsertCalls++
	s.items[attachment.CapabilityID] = attachment
	return nil
}

func (s *testAttachmentStorage) ListByClawID(context.Context, uuid.UUID, uuid.UUID) ([]entities.ClawCapabilityAttachment, error) {
	out := make([]entities.ClawCapabilityAttachment, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}

	return out, nil
}

func (s *testAttachmentStorage) Delete(_ context.Context, _ uuid.UUID, _ uuid.UUID, capabilityID entities.CapabilityID) error {
	s.deleteCalls++

	if _, ok := s.items[capabilityID]; !ok {
		return sql.ErrNotFound
	}

	delete(s.items, capabilityID)
	return nil
}

type testIntegrationStorage struct {
	integration entities.AccountIntegration
	err         error
}

func (s *testIntegrationStorage) GetByID(context.Context, uuid.UUID, uuid.UUID) (entities.AccountIntegration, error) {
	if s.err != nil {
		return entities.AccountIntegration{}, s.err
	}

	return s.integration, nil
}

func TestServiceAttach_ValidatesIntegrationCapability(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	integrationID := uuid.New()

	service := newService(
		&testClawStorage{claw: entities.Claw{ID: clawID, UserID: userID}},
		newTestAttachmentStorage(),
		&testIntegrationStorage{integration: entities.AccountIntegration{
			ID:           integrationID,
			UserID:       userID,
			CapabilityID: entities.CapabilityGoogleCalendar,
			Provider:     "google_calendar",
		}},
	)

	err := service.Attach(context.Background(), AttachCommand{
		UserID:               userID,
		ClawID:               clawID,
		CapabilityID:         entities.CapabilityGmail,
		Provider:             "gmail",
		AccountIntegrationID: &integrationID,
		Enabled:              true,
	})
	if !errors.Is(err, sql.ErrInvalid) {
		t.Fatalf("Attach() error = %v, want sql.ErrInvalid", err)
	}
}

func TestServiceAttach_DisabledCapabilityDoesNotEnableRuntimeHook(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	store := &testClawStorage{claw: entities.Claw{ID: clawID, UserID: userID}}

	service := newService(
		store,
		newTestAttachmentStorage(),
		&testIntegrationStorage{},
	)

	err := service.Attach(context.Background(), AttachCommand{
		UserID:       userID,
		ClawID:       clawID,
		CapabilityID: entities.CapabilityGmail,
		Provider:     "gmail",
		Enabled:      false,
	})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}

	if store.updatedClaw.Config.Hooks != nil {
		t.Fatalf("hooks = %#v, want nil", store.updatedClaw.Config.Hooks)
	}
}
