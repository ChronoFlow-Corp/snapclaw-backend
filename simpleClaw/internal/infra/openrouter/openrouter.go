package openrouter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"shared/pkg/observability"
	"strings"
	"time"

	"simpleClaw/internal/entities"

	"github.com/google/uuid"
	gopenrouter "github.com/revrost/go-openrouter"
)

const (
	PrefixKeyName = "claw"
)

// ApiKeyManager manages OpenRouter API keys through the go-openrouter client.
type ApiKeyManager struct {
	client  *gopenrouter.Client
	metrics *observability.OperationMetrics
}

// Options configure ApiKeyManager creation.
type Options struct {
	BaseURL          string
	APIToken         string
	Timeout          time.Duration
	HTTPClient       *http.Client
	Referer          string
	Title            string
	OperationMetrics *observability.OperationMetrics
}

// NewApiKeyManager constructs a manager backed by github.com/revrost/go-openrouter.
func NewApiKeyManager(opts Options) (*ApiKeyManager, error) {
	if opts.BaseURL == "" {
		return nil, ErrMissingBaseURL
	}

	if opts.APIToken == "" {
		return nil, ErrMissingAPIToken
	}

	cfg := gopenrouter.DefaultConfig(opts.APIToken)
	cfg.BaseURL = normalizeBaseURL(opts.BaseURL)

	switch {
	case opts.HTTPClient != nil:
		cfg.HTTPClient = opts.HTTPClient
	default:
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = 15 * time.Second
		}

		cfg.HTTPClient = observability.NewHTTPClient(timeout)
	}

	if opts.Referer != "" {
		cfg.HttpReferer = opts.Referer
	}

	if opts.Title != "" {
		cfg.XTitle = opts.Title
	}

	return &ApiKeyManager{
		client:  gopenrouter.NewClientWithConfig(*cfg),
		metrics: opts.OperationMetrics,
	}, nil
}

// Create issues a new API key for the provided user with requested limits.
func (m *ApiKeyManager) Create(
	ctx context.Context,
	userID uuid.UUID,
	monthlyBudgetUSD float64,
) (entities.OpenRouterKey, error) {
	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		m.metrics,
		"infra.openrouter",
		"openrouter.key.create",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	name := makeKeyName(userID)
	limit := normalizeBudget(monthlyBudgetUSD)

	req := gopenrouter.APIKeyCreateRequest{
		Name:       name,
		Limit:      limit,
		LimitReset: gopenrouter.KeyLimitResetMonthly,
	}

	resp, err := m.client.CreateAPIKey(ctx, req)
	if err != nil {
		return entities.OpenRouterKey{}, wrapError(err)
	}

	if resp.Data.Hash == "" {
		return entities.OpenRouterKey{}, errors.New("no key created")
	}

	return entities.OpenRouterKey{
		ID:     resp.Data.Hash,
		Secret: resp.Key,
	}, nil
}

// UpdateLimits updates the limit for the specified key.
func (m *ApiKeyManager) UpdateLimits(
	ctx context.Context,
	keyID string,
	monthlyBudgetUSD float64,
) (err error) {
	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		m.metrics,
		"infra.openrouter",
		"openrouter.key.update_limits",
		"claw_lifecycle",
	)

	defer func() { finish(err) }()

	limit := normalizeBudget(monthlyBudgetUSD)
	req := gopenrouter.APIKeyUpdateRequest{
		Limit:      floatPtr(limit),
		LimitReset: keyResetPtr(gopenrouter.KeyLimitResetMonthly),
	}

	_, err = m.client.UpdateAPIKey(ctx, keyID, req)

	return wrapError(err)
}

// Delete removes the key from OpenRouter.
func (m *ApiKeyManager) Delete(ctx context.Context, keyID string) (err error) {
	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		m.metrics,
		"infra.openrouter",
		"openrouter.key.delete",
		"claw_lifecycle",
	)

	defer func() { finish(err) }()

	_, err = m.client.DeleteAPIKey(ctx, keyID)

	return wrapError(err)
}

// ResolveModel returns the fully-qualified model slug for OpenRouter agents config.
func (m *ApiKeyManager) ResolveModel(ctx context.Context, model string) (string, error) {
	const prefix = "openrouter/"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		m.metrics,
		"infra.openrouter",
		"openrouter.model.resolve",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	model, err = normalizeModelName(model)
	if err != nil {
		return "", err
	}

	slug := normalizeModelSlug(model)
	if slug == "" {
		return "", ErrModelRequired
	}

	models, err := m.client.ListModels(ctx)
	if err != nil {
		return "", wrapError(err)
	}

	for _, mdl := range models {
		if matchesModelSlug(mdl, slug) {
			return prefix + mdl.ID, nil
		}
	}

	return "", fmt.Errorf("%w: %s", ErrModelNotFound, model)
}

func (m *ApiKeyManager) DisableKey(ctx context.Context, keyID string) error {
	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		m.metrics,
		"infra.openrouter",
		"openrouter.key.disable",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	disabled := true

	rs, err := m.client.UpdateAPIKey(ctx, keyID, gopenrouter.APIKeyUpdateRequest{
		Disabled: &disabled,
	})
	if err != nil {
		return wrapError(err)
	}

	if rs.Data.Disabled != disabled {
		return errors.New("key is not disabled")
	}

	return nil
}

func normalizeBaseURL(base string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasSuffix(base, "/api/v1") {
		base = base + "/api/v1"
	}

	return base
}

func makeKeyName(userID uuid.UUID) string {
	return fmt.Sprintf("%s-%s", PrefixKeyName, userID.String())
}

func normalizeBudget(budget float64) float64 {
	if budget <= 0 {
		return 50
	}

	return budget
}

func floatPtr(v float64) *float64 {
	return &v
}

func keyResetPtr(v gopenrouter.KeyLimitReset) *gopenrouter.KeyLimitReset {
	return &v
}

func normalizeModelName(model string) (string, error) {
	for _, v := range models {
		if strings.Contains(model, v.contain) {
			return v.openrouterName, nil
		}
	}

	return "", ErrModelNotFound
}

func normalizeModelSlug(slug string) string {
	trimmed := strings.TrimSpace(slug)
	trimmed = strings.TrimPrefix(trimmed, "openrouter/")

	return trimmed
}

func matchesModelSlug(model gopenrouter.Model, candidate string) bool {
	if strings.EqualFold(model.ID, candidate) {
		return true
	}

	if model.CanonicalSlug != nil && strings.EqualFold(*model.CanonicalSlug, candidate) {
		return true
	}

	if strings.EqualFold(model.Name, candidate) {
		return true
	}

	parts := strings.Split(model.ID, "/")
	if len(parts) == 2 && strings.EqualFold(parts[1], candidate) {
		return true
	}

	return false
}
