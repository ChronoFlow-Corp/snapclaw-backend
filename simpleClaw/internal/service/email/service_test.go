package email

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"simpleClaw/internal/entities"
	infraemail "simpleClaw/internal/infra/email"

	"github.com/google/uuid"
)

type fakeOutbox struct {
	mu     sync.Mutex
	items  map[uuid.UUID]*entities.OutboxEmail
	sent   []uuid.UUID
	failed []uuid.UUID
}

func newFakeOutbox() *fakeOutbox {
	return &fakeOutbox{items: make(map[uuid.UUID]*entities.OutboxEmail)}
}

func (f *fakeOutbox) Enqueue(_ context.Context, m entities.OutboxEmail) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	cp := m
	f.items[m.ID] = &cp
	return nil
}

func (f *fakeOutbox) ClaimDue(_ context.Context, now time.Time, limit int) ([]entities.OutboxEmail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]entities.OutboxEmail, 0)
	for _, m := range f.items {
		if m.Status == entities.EmailStatusPending && !m.NextAttemptAt.After(now) {
			out = append(out, *m)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (f *fakeOutbox) MarkSent(_ context.Context, id uuid.UUID, providerMessageID string, sentAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	m, ok := f.items[id]
	if !ok {
		return errors.New("not found")
	}
	m.Status = entities.EmailStatusSent
	m.ProviderMessageID = providerMessageID
	m.SentAt = &sentAt
	f.sent = append(f.sent, id)
	return nil
}

func (f *fakeOutbox) MarkFailed(_ context.Context, id uuid.UUID, attempts int, lastErr string, nextAttemptAt time.Time, status entities.EmailStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	m, ok := f.items[id]
	if !ok {
		return errors.New("not found")
	}
	m.Status = status
	m.Attempts = attempts
	m.LastError = lastErr
	m.NextAttemptAt = nextAttemptAt
	f.failed = append(f.failed, id)
	return nil
}

func (f *fakeOutbox) DeleteExpired(_ context.Context, before time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var deleted int64
	for id, m := range f.items {
		terminal := m.Status == entities.EmailStatusSent || m.Status == entities.EmailStatusFailed
		if terminal && m.UpdatedAt.Before(before) {
			delete(f.items, id)
			deleted++
		}
	}
	return deleted, nil
}

type fakeSender struct {
	mu       sync.Mutex
	messages []infraemail.Message
	err      error
}

func (f *fakeSender) Send(_ context.Context, msg infraemail.Message) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.messages = append(f.messages, msg)
	if f.err != nil {
		return "", f.err
	}
	return "provider-id-123", nil
}

type fakeUsers struct {
	user   entities.User
	optOut *bool
}

func (f *fakeUsers) GetByID(_ context.Context, _ uuid.UUID) (entities.User, error) {
	return f.user, nil
}

func (f *fakeUsers) SetMarketingOptOut(_ context.Context, _ uuid.UUID, optOut bool) error {
	f.optOut = &optOut
	return nil
}

func newTestService(outbox outboxStore, sender sender, users userReader) *Service {
	return New(outbox, sender, users, Options{
		FromName:          "SnapClaw",
		FromAddress:       "no-reply@send.snapclaw.ru",
		ReplyToAddress:    "support@snapclaw.ru",
		AppURL:            "https://snapclaw.ru",
		PublicBaseURL:     "https://api.snapclaw.ru",
		UnsubscribeSecret: "test-secret",
	})
}

func TestEnqueueAndDispatchWelcome(t *testing.T) {
	outbox := newFakeOutbox()
	sender := &fakeSender{}
	user := entities.User{ID: uuid.New(), Name: "Ada", Email: "ada@example.com"}
	svc := newTestService(outbox, sender, &fakeUsers{user: user})

	if err := svc.EnqueueWelcome(context.Background(), user); err != nil {
		t.Fatalf("EnqueueWelcome: %v", err)
	}

	if len(outbox.items) != 1 {
		t.Fatalf("expected 1 queued email, got %d", len(outbox.items))
	}

	svc.dispatchBatch(context.Background())

	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sender.messages))
	}

	msg := sender.messages[0]
	if msg.To != "ada@example.com" {
		t.Errorf("unexpected recipient: %q", msg.To)
	}
	if msg.From != "SnapClaw <no-reply@send.snapclaw.ru>" {
		t.Errorf("unexpected From: %q", msg.From)
	}
	if msg.ReplyTo != "support@snapclaw.ru" {
		t.Errorf("unexpected ReplyTo: %q", msg.ReplyTo)
	}
	if msg.IdempotencyKey == "" {
		t.Error("expected an idempotency key on the outgoing message")
	}
	if !strings.Contains(msg.HTML, "Добро пожаловать в SnapClaw") {
		t.Errorf("HTML missing welcome heading: %q", msg.HTML)
	}
	if !strings.Contains(msg.Text, "Ada") {
		t.Errorf("text missing recipient name: %q", msg.Text)
	}
	if len(outbox.sent) != 1 {
		t.Errorf("expected email marked sent, got %d", len(outbox.sent))
	}
}

func TestDispatchRetriesOnSendError(t *testing.T) {
	outbox := newFakeOutbox()
	sender := &fakeSender{err: errors.New("resend down")}
	user := entities.User{ID: uuid.New(), Name: "Ada", Email: "ada@example.com"}
	svc := newTestService(outbox, sender, &fakeUsers{user: user})

	if err := svc.EnqueueWelcome(context.Background(), user); err != nil {
		t.Fatalf("EnqueueWelcome: %v", err)
	}

	svc.dispatchBatch(context.Background())

	if len(outbox.failed) != 1 {
		t.Fatalf("expected 1 failed record, got %d", len(outbox.failed))
	}

	var only *entities.OutboxEmail
	for _, m := range outbox.items {
		only = m
	}
	if only.Status != entities.EmailStatusPending {
		t.Errorf("expected still pending for retry, got %q", only.Status)
	}
	if only.Attempts != 1 {
		t.Errorf("expected attempts=1, got %d", only.Attempts)
	}
	if !only.NextAttemptAt.After(time.Now()) {
		t.Errorf("expected next attempt in the future for backoff")
	}
}

func TestDispatchMarksFailedAtMaxAttempts(t *testing.T) {
	outbox := newFakeOutbox()
	sender := &fakeSender{err: errors.New("resend down")}
	svc := newTestService(outbox, sender, &fakeUsers{})

	id := uuid.New()
	_ = outbox.Enqueue(context.Background(), entities.OutboxEmail{
		ID:            id,
		ToAddress:     "ada@example.com",
		Status:        entities.EmailStatusPending,
		MaxAttempts:   1,
		NextAttemptAt: time.Now().Add(-time.Minute),
	})

	svc.dispatchBatch(context.Background())

	got := outbox.items[id]
	if got.Status != entities.EmailStatusFailed {
		t.Errorf("expected status failed at max attempts, got %q", got.Status)
	}
	if got.Attempts != 1 {
		t.Errorf("expected attempts=1, got %d", got.Attempts)
	}
	if len(sender.messages) != 1 {
		t.Errorf("expected one send attempt, got %d", len(sender.messages))
	}
}

func TestPruneDeletesOldTerminalRows(t *testing.T) {
	outbox := newFakeOutbox()
	svc := newTestService(outbox, &fakeSender{}, &fakeUsers{})

	old := uuid.New()
	_ = outbox.Enqueue(context.Background(), entities.OutboxEmail{
		ID:        old,
		Status:    entities.EmailStatusSent,
		UpdatedAt: time.Now().Add(-90 * 24 * time.Hour),
	})
	fresh := uuid.New()
	_ = outbox.Enqueue(context.Background(), entities.OutboxEmail{
		ID:        fresh,
		Status:    entities.EmailStatusPending,
		UpdatedAt: time.Now(),
	})

	svc.prune(context.Background(), time.Now())

	if _, ok := outbox.items[old]; ok {
		t.Error("expected old terminal row to be pruned")
	}
	if _, ok := outbox.items[fresh]; !ok {
		t.Error("expected pending row to be kept")
	}
}

func TestUnsubscribeTokenRoundTrip(t *testing.T) {
	users := &fakeUsers{user: entities.User{ID: uuid.New()}}
	svc := newTestService(newFakeOutbox(), &fakeSender{}, users)

	userID := uuid.New()
	token := svc.unsubscribeToken(userID)

	got, err := svc.verifyUnsubscribeToken(token)
	if err != nil {
		t.Fatalf("verifyUnsubscribeToken: %v", err)
	}
	if got != userID {
		t.Errorf("round-trip mismatch: got %s want %s", got, userID)
	}

	if _, err := svc.verifyUnsubscribeToken(token + "tampered"); err == nil {
		t.Error("expected tampered token to fail verification")
	}

	if err := svc.Unsubscribe(context.Background(), token); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if users.optOut == nil || !*users.optOut {
		t.Error("expected marketing opt-out to be set true")
	}
}

func TestFeatureAnnouncementRespectsOptOut(t *testing.T) {
	outbox := newFakeOutbox()
	optedOut := entities.User{ID: uuid.New(), Email: "ada@example.com", MarketingOptOut: true}
	svc := newTestService(outbox, &fakeSender{}, &fakeUsers{user: optedOut})

	if err := svc.EnqueueFeatureAnnouncement(context.Background(), optedOut.ID, "Dark mode", "It's here.", "https://snapclaw.ru/whats-new"); err != nil {
		t.Fatalf("EnqueueFeatureAnnouncement: %v", err)
	}

	if len(outbox.items) != 0 {
		t.Errorf("expected no email for opted-out user, got %d", len(outbox.items))
	}
}

func TestFeatureAnnouncementAddsUnsubscribeHeaders(t *testing.T) {
	outbox := newFakeOutbox()
	user := entities.User{ID: uuid.New(), Email: "ada@example.com"}
	svc := newTestService(outbox, &fakeSender{}, &fakeUsers{user: user})

	if err := svc.EnqueueFeatureAnnouncement(context.Background(), user.ID, "Dark mode", "It's here.", "https://snapclaw.ru/whats-new"); err != nil {
		t.Fatalf("EnqueueFeatureAnnouncement: %v", err)
	}

	var only *entities.OutboxEmail
	for _, m := range outbox.items {
		only = m
	}
	if only == nil {
		t.Fatal("expected a queued announcement")
	}
	if _, ok := only.Headers["List-Unsubscribe"]; !ok {
		t.Errorf("expected List-Unsubscribe header, got %v", only.Headers)
	}
	if only.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Errorf("expected one-click post header, got %v", only.Headers)
	}
}
