package googleoauth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type storage interface {
	Upsert(ctx context.Context, integration entities.AccountIntegration) error
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]entities.AccountIntegration, error)
}

type authProvider interface {
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
	Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
}

type profileFetcher interface {
	FetchProfile(ctx context.Context, token *oauth2.Token) (GoogleProfile, error)
}

type GoogleProfile struct {
	ID      string
	Email   string
	Name    string
	Picture string
}

type ServiceOptions struct {
	Auth       authProvider
	Storage    storage
	StateCodec *StateCodec
	Profile    profileFetcher
	Now        func() time.Time
}

type Service struct {
	auth    authProvider
	storage storage
	state   *StateCodec
	profile profileFetcher
	now     func() time.Time
}

type StartCommand struct {
	UserID       uuid.UUID
	Capabilities []string
	ReturnTo     string
}

type StartResult struct {
	AuthURL      string
	Capabilities []string
}

type CompleteCommand struct {
	RawState string
	Code     string
}

type CompleteResult struct {
	ReturnTo     string
	Integrations []entities.AccountIntegration
}

func NewService(opts ServiceOptions) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		auth:    opts.Auth,
		storage: opts.Storage,
		state:   opts.StateCodec,
		profile: opts.Profile,
		now:     now,
	}
}

func (s *Service) Start(_ context.Context, cmd StartCommand) (StartResult, error) {
	if s.auth == nil || s.storage == nil || s.state == nil || s.profile == nil {
		return StartResult{}, sql.ErrUnavailable
	}

	if cmd.UserID == uuid.Nil {
		return StartResult{}, sql.ErrInvalid
	}

	capabilities, err := NormalizeCapabilities(cmd.Capabilities)
	if err != nil {
		return StartResult{}, err
	}

	if len(capabilities) == 0 {
		return StartResult{}, sql.ErrInvalid
	}

	state, err := s.state.Sign(StatePayload{
		UserID:       cmd.UserID,
		Capabilities: capabilities,
		ReturnTo:     cmd.ReturnTo,
	})
	if err != nil {
		return StartResult{}, err
	}

	authURL := s.auth.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	return StartResult{
		AuthURL:      authURL,
		Capabilities: capabilities,
	}, nil
}

func (s *Service) Complete(ctx context.Context, cmd CompleteCommand) (CompleteResult, error) {
	if s.auth == nil || s.storage == nil || s.state == nil || s.profile == nil {
		return CompleteResult{}, sql.ErrUnavailable
	}

	if strings.TrimSpace(cmd.RawState) == "" || strings.TrimSpace(cmd.Code) == "" {
		return CompleteResult{}, sql.ErrInvalid
	}

	state, err := s.state.Parse(cmd.RawState)
	if err != nil {
		return CompleteResult{}, err
	}

	token, err := s.auth.Exchange(ctx, strings.TrimSpace(cmd.Code))
	if err != nil {
		return CompleteResult{}, err
	}

	profile, err := s.profile.FetchProfile(ctx, token)
	if err != nil {
		return CompleteResult{}, err
	}

	grantedCapabilities := intersectCapabilities(
		state.Capabilities,
		CapabilitiesFromGrantedScopes(scopeValues(token)),
	)

	existing, err := s.storage.ListByUserID(ctx, state.UserID)
	if err != nil {
		return CompleteResult{}, err
	}

	integrations := make([]entities.AccountIntegration, 0, len(grantedCapabilities))
	for _, capability := range grantedCapabilities {
		integration := s.buildIntegration(state.UserID, capability, profile, token, existing)
		if err := s.storage.Upsert(ctx, integration); err != nil {
			return CompleteResult{}, err
		}

		integrations = append(integrations, integration)
	}

	return CompleteResult{
		ReturnTo:     state.ReturnTo,
		Integrations: integrations,
	}, nil
}

func (s *Service) buildIntegration(
	userID uuid.UUID,
	capability string,
	profile GoogleProfile,
	token *oauth2.Token,
	existing []entities.AccountIntegration,
) entities.AccountIntegration {
	now := s.now().UTC()
	capabilityID := entities.CapabilityID(capability)
	integrationID := uuid.New()
	createdAt := now
	refreshToken := strings.TrimSpace(token.RefreshToken)

	for _, item := range existing {
		if item.UserID != userID || item.CapabilityID != capabilityID {
			continue
		}

		if profile.Email != "" && !strings.EqualFold(item.ExternalAccountID, profile.Email) {
			continue
		}

		integrationID = item.ID
		createdAt = item.CreatedAt
		if refreshToken == "" {
			refreshToken = stringFromMap(item.SecretPayload, "refresh_token")
		}

		break
	}

	payload := map[string]any{
		"access_token":  strings.TrimSpace(token.AccessToken),
		"refresh_token": refreshToken,
		"token_type":    strings.TrimSpace(token.TokenType),
		"expiry":        token.Expiry.UTC().Format(time.RFC3339Nano),
	}

	metadata := map[string]any{
		"grantedScopes": scopeValues(token),
		"googleUserID":  strings.TrimSpace(profile.ID),
		"email":         strings.TrimSpace(profile.Email),
		"avatarURL":     strings.TrimSpace(profile.Picture),
	}

	return entities.AccountIntegration{
		ID:                integrationID,
		UserID:            userID,
		CapabilityID:      capabilityID,
		Provider:          capability,
		ExternalAccountID: strings.TrimSpace(profile.Email),
		DisplayName:       strings.TrimSpace(profile.Name),
		Status:            entities.AccountIntegrationStatusActive,
		SecretPayload:     payload,
		Metadata:          metadata,
		CreatedAt:         createdAt,
		UpdatedAt:         now,
	}
}

func scopeValues(token *oauth2.Token) []string {
	if token == nil {
		return nil
	}

	raw, _ := token.Extra("scope").(string)
	if strings.TrimSpace(raw) == "" {
		raw, _ = token.Extra("granted_scopes").(string)
	}

	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Fields(raw)
	slices.Sort(parts)

	return slices.Compact(parts)
}

func intersectCapabilities(requested, granted []string) []string {
	if len(requested) == 0 || len(granted) == 0 {
		return nil
	}

	grantedSet := make(map[string]struct{}, len(granted))
	for _, capability := range granted {
		grantedSet[capability] = struct{}{}
	}

	out := make([]string, 0, len(requested))
	for _, capability := range requested {
		if _, ok := grantedSet[capability]; ok {
			out = append(out, capability)
		}
	}

	return out
}

func stringFromMap(payload map[string]any, key string) string {
	if len(payload) == 0 {
		return ""
	}

	value, ok := payload[key]
	if !ok {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

var errNoGrantedCapabilities = errors.New("no granted capabilities")
