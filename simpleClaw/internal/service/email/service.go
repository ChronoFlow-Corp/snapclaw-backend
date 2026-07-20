package email

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"sync"
	"time"

	"simpleClaw/internal/entities"
	infraemail "simpleClaw/internal/infra/email"
	"simpleClaw/internal/infra/sql"

	"github.com/google/uuid"
)

// ErrInvalidToken marks an unsubscribe request that can never succeed (bad,
// tampered, or stale token, or a user that no longer exists). Callers map it to
// a 4xx; every other error is transient and should surface as a 5xx so RFC 8058
// one-click clients retry instead of reporting the link as broken.
var ErrInvalidToken = errors.New("email: invalid unsubscribe token")

const (
	defaultBatchSize   = 20
	defaultMaxAttempts = 5
	defaultConcurrency = 5
	defaultRetention   = 30 * 24 * time.Hour
	pruneInterval      = time.Hour
	unsubscribePath    = "/email/unsubscribe"
)

type sender interface {
	Send(ctx context.Context, msg infraemail.Message) (string, error)
}

type outboxStore interface {
	Enqueue(ctx context.Context, m entities.OutboxEmail) error
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]entities.OutboxEmail, error)
	MarkSent(ctx context.Context, id uuid.UUID, providerMessageID string, sentAt time.Time) error
	MarkFailed(ctx context.Context, id uuid.UUID, attempts int, lastErr string, nextAttemptAt time.Time, status entities.EmailStatus) error
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

type userReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (entities.User, error)
	SetMarketingOptOut(ctx context.Context, id uuid.UUID, optOut bool) error
}

type Options struct {
	FromName          string
	FromAddress       string
	ReplyToAddress    string
	AppURL            string
	PublicBaseURL     string
	UnsubscribeSecret string
	BatchSize         int
	MaxAttempts       int
	Concurrency       int
	Retention         time.Duration
}

type Service struct {
	outbox            outboxStore
	client            sender
	users             userReader
	tmpl              *template.Template
	fromName          string
	fromAddress       string
	replyTo           string
	appURL            string
	publicBaseURL     string
	unsubscribeSecret string
	batchSize         int
	maxAttempts       int
	concurrency       int
	retention         time.Duration
	lastPruneAt       time.Time
	logger            *slog.Logger
}

type rendered struct {
	subject string
	html    string
	text    string
	headers map[string]string
}

func New(outbox outboxStore, client sender, users userReader, opts Options) *Service {
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}

	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = defaultMaxAttempts
	}

	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultConcurrency
	}

	if opts.Retention <= 0 {
		opts.Retention = defaultRetention
	}

	return &Service{
		outbox:            outbox,
		client:            client,
		users:             users,
		tmpl:              template.Must(template.New("layout").Parse(layoutHTML)),
		fromName:          strings.TrimSpace(opts.FromName),
		fromAddress:       strings.TrimSpace(opts.FromAddress),
		replyTo:           strings.TrimSpace(opts.ReplyToAddress),
		appURL:            strings.TrimRight(strings.TrimSpace(opts.AppURL), "/"),
		publicBaseURL:     strings.TrimRight(strings.TrimSpace(opts.PublicBaseURL), "/"),
		unsubscribeSecret: opts.UnsubscribeSecret,
		batchSize:         opts.BatchSize,
		maxAttempts:       opts.MaxAttempts,
		concurrency:       opts.Concurrency,
		retention:         opts.Retention,
		logger:            slog.Default(),
	}
}

// EnqueueWelcome queues the registration welcome email. The user entity is
// already in hand at sign-up, so no extra lookup is needed.
func (s *Service) EnqueueWelcome(ctx context.Context, u entities.User) error {
	r, err := s.renderWelcome(u.Name)
	if err != nil {
		return err
	}

	return s.enqueue(ctx, u.Email, entities.EmailCategoryWelcome, r)
}

func (s *Service) EnqueuePremiumGranted(ctx context.Context, userID uuid.UUID) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	r, err := s.renderPremiumGranted(u.Name)
	if err != nil {
		return err
	}

	return s.enqueue(ctx, u.Email, entities.EmailCategoryPremiumGranted, r)
}

func (s *Service) EnqueueTopUpConfirmation(
	ctx context.Context,
	userID uuid.UUID,
	amountValue, amountCurrency string,
) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	r, err := s.renderTopUp(u.Name, amountValue, amountCurrency)
	if err != nil {
		return err
	}

	return s.enqueue(ctx, u.Email, entities.EmailCategoryTopUp, r)
}

func (s *Service) EnqueueSupportReply(
	ctx context.Context,
	userID uuid.UUID,
	ticketSubject, replyPreview, ticketURL string,
) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	r, err := s.renderSupportReply(u.Name, ticketSubject, replyPreview, ticketURL)
	if err != nil {
		return err
	}

	return s.enqueue(ctx, u.Email, entities.EmailCategorySupportReply, r)
}

// EnqueueFeatureAnnouncement queues a marketing email. It respects the user's
// marketing opt-out and attaches RFC 8058 one-click unsubscribe headers.
func (s *Service) EnqueueFeatureAnnouncement(
	ctx context.Context,
	userID uuid.UUID,
	featureName, body, ctaURL string,
) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if u.MarketingOptOut {
		return nil
	}

	unsubURL := s.unsubscribeURL(userID)

	r, err := s.renderAnnouncement(u.Name, featureName, body, ctaURL, unsubURL)
	if err != nil {
		return err
	}

	if unsubURL != "" {
		r.headers = map[string]string{
			"List-Unsubscribe":      "<" + unsubURL + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		}
	}

	return s.enqueue(ctx, u.Email, entities.EmailCategoryAnnouncement, r)
}

func (s *Service) enqueue(ctx context.Context, to string, category entities.EmailCategory, r rendered) error {
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("email: empty recipient for category %s", category)
	}

	return s.outbox.Enqueue(ctx, entities.OutboxEmail{
		ID:            uuid.New(),
		ToAddress:     strings.TrimSpace(to),
		Subject:       r.subject,
		HTMLBody:      r.html,
		TextBody:      r.text,
		Headers:       r.headers,
		Category:      category,
		Status:        entities.EmailStatusPending,
		MaxAttempts:   s.maxAttempts,
		NextAttemptAt: time.Now(),
	})
}

func (s *Service) render(subject string, d layoutData) (rendered, error) {
	d.FromName = s.fromName
	if d.FromName == "" {
		d.FromName = "SnapClaw"
	}

	html, err := s.renderHTML(d)
	if err != nil {
		return rendered{}, fmt.Errorf("email: render %q: %w", subject, err)
	}

	return rendered{subject: subject, html: html, text: layoutText(d)}, nil
}

// RunDispatcher polls the outbox and delivers due emails. It mirrors the
// service's other background workers and exits when ctx is cancelled.
func (s *Service) RunDispatcher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.logger.Info("email dispatcher started", slog.Duration("interval", interval))

	// Work once immediately, then on each tick — mirrors RunReconciler so the
	// first welcome emails and receipts aren't delayed a full interval.
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.dispatchBatch(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) dispatchBatch(ctx context.Context) {
	now := time.Now()

	s.prune(ctx, now)

	due, err := s.outbox.ClaimDue(ctx, now, s.batchSize)
	if err != nil {
		s.logger.Error("email dispatcher: claim due failed", slog.Any("err", err))
		return
	}

	if len(due) == 0 {
		return
	}

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup

	for _, m := range due {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(m entities.OutboxEmail) {
			defer wg.Done()
			defer func() { <-sem }()

			s.deliver(ctx, m)
		}(m)
	}

	wg.Wait()
}

func (s *Service) deliver(ctx context.Context, m entities.OutboxEmail) {
	if ctx.Err() != nil {
		return
	}

	providerID, sendErr := s.client.Send(ctx, s.toMessage(m))
	if sendErr != nil {
		s.markFailure(ctx, m, sendErr)
		return
	}

	if err := s.outbox.MarkSent(ctx, m.ID, providerID, time.Now()); err != nil {
		s.logger.Error("email dispatcher: mark sent failed",
			slog.String("email_id", m.ID.String()), slog.Any("err", err))
	}
}

// prune deletes old terminal rows so the outbox doesn't grow unbounded. It is
// throttled to at most once per pruneInterval and runs on the dispatcher's
// goroutine, so lastPruneAt needs no synchronization.
func (s *Service) prune(ctx context.Context, now time.Time) {
	if s.retention <= 0 {
		return
	}

	if !s.lastPruneAt.IsZero() && now.Sub(s.lastPruneAt) < pruneInterval {
		return
	}

	s.lastPruneAt = now

	deleted, err := s.outbox.DeleteExpired(ctx, now.Add(-s.retention))
	if err != nil {
		s.logger.Error("email dispatcher: prune failed", slog.Any("err", err))
		return
	}

	if deleted > 0 {
		s.logger.Info("email dispatcher: pruned terminal rows", slog.Int64("count", deleted))
	}
}

func (s *Service) markFailure(ctx context.Context, m entities.OutboxEmail, sendErr error) {
	attempts := m.Attempts + 1

	maxAttempts := m.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = s.maxAttempts
	}

	status := entities.EmailStatusPending
	if attempts >= maxAttempts {
		status = entities.EmailStatusFailed
	}

	nextAttemptAt := time.Now().Add(backoffFor(attempts))

	s.logger.Warn("email dispatcher: send failed",
		slog.String("email_id", m.ID.String()),
		slog.String("category", string(m.Category)),
		slog.Int("attempts", attempts),
		slog.String("status", string(status)),
		slog.Any("err", sendErr),
	)

	if err := s.outbox.MarkFailed(ctx, m.ID, attempts, sendErr.Error(), nextAttemptAt, status); err != nil {
		s.logger.Error("email dispatcher: mark failed failed",
			slog.String("email_id", m.ID.String()), slog.Any("err", err))
	}
}

func (s *Service) toMessage(m entities.OutboxEmail) infraemail.Message {
	return infraemail.Message{
		From:           s.fromHeader(),
		To:             m.ToAddress,
		Subject:        m.Subject,
		HTML:           m.HTMLBody,
		Text:           m.TextBody,
		ReplyTo:        s.replyTo,
		Headers:        m.Headers,
		IdempotencyKey: m.ID.String(),
	}
}

func (s *Service) fromHeader() string {
	if s.fromName == "" {
		return s.fromAddress
	}

	return fmt.Sprintf("%s <%s>", s.fromName, s.fromAddress)
}

// Unsubscribe verifies a token and opts the user out of marketing email.
func (s *Service) Unsubscribe(ctx context.Context, token string) error {
	userID, err := s.verifyUnsubscribeToken(token)
	if err != nil {
		return err
	}

	if err := s.users.SetMarketingOptOut(ctx, userID, true); err != nil {
		if errors.Is(err, sql.ErrNotFound) {
			return fmt.Errorf("%w: %w", ErrInvalidToken, err)
		}

		return err
	}

	return nil
}

func (s *Service) unsubscribeURL(userID uuid.UUID) string {
	if s.publicBaseURL == "" {
		return ""
	}

	return s.publicBaseURL + unsubscribePath + "?token=" + s.unsubscribeToken(userID)
}

func (s *Service) unsubscribeToken(userID uuid.UUID) string {
	payload := userID.String()
	sig := base64.RawURLEncoding.EncodeToString(s.sign(payload))
	raw := payload + "." + sig

	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func (s *Service) verifyUnsubscribeToken(token string) (uuid.UUID, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: decode: %w", ErrInvalidToken, err)
	}

	parts := strings.SplitN(string(decoded), ".", 2)
	if len(parts) != 2 {
		return uuid.Nil, fmt.Errorf("%w: malformed", ErrInvalidToken)
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: decode signature: %w", ErrInvalidToken, err)
	}

	if !hmac.Equal(sig, s.sign(parts[0])) {
		return uuid.Nil, fmt.Errorf("%w: bad signature", ErrInvalidToken)
	}

	userID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: parse user id: %w", ErrInvalidToken, err)
	}

	return userID, nil
}

func (s *Service) sign(payload string) []byte {
	mac := hmac.New(sha256.New, []byte(s.unsubscribeSecret))
	mac.Write([]byte(payload))

	return mac.Sum(nil)
}

func backoffFor(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return time.Minute
	case attempts == 2:
		return 5 * time.Minute
	case attempts == 3:
		return 30 * time.Minute
	case attempts == 4:
		return 2 * time.Hour
	default:
		return 6 * time.Hour
	}
}
