package googleoauth

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"simpleClaw/internal/entities"
)

type fakeStorage struct {
	items   map[uuid.UUID]entities.AccountIntegration
	upserts []entities.AccountIntegration
}

func newFakeStorage(items ...entities.AccountIntegration) *fakeStorage {
	storage := &fakeStorage{items: make(map[uuid.UUID]entities.AccountIntegration, len(items))}
	for _, item := range items {
		storage.items[item.ID] = item
	}

	return storage
}

func (s *fakeStorage) Upsert(_ context.Context, integration entities.AccountIntegration) error {
	s.items[integration.ID] = integration
	s.upserts = append(s.upserts, integration)
	return nil
}

func (s *fakeStorage) ListByUserID(_ context.Context, userID uuid.UUID) ([]entities.AccountIntegration, error) {
	out := make([]entities.AccountIntegration, 0, len(s.items))
	for _, item := range s.items {
		if item.UserID == userID {
			out = append(out, item)
		}
	}

	return out, nil
}

type fakeAuthProvider struct {
	lastState string
	lastOpts  []oauth2.AuthCodeOption
	token     *oauth2.Token
	err       error
}

func (f *fakeAuthProvider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	f.lastState = state
	f.lastOpts = append([]oauth2.AuthCodeOption(nil), opts...)
	return "https://accounts.google.com/o/oauth2/v2/auth?state=" + url.QueryEscape(state)
}

func (f *fakeAuthProvider) Exchange(context.Context, string, ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.token, nil
}

type fakeProfileFetcher struct {
	profile GoogleProfile
	err     error
}

func (f *fakeProfileFetcher) FetchProfile(context.Context, *oauth2.Token) (GoogleProfile, error) {
	if f.err != nil {
		return GoogleProfile{}, f.err
	}

	return f.profile, nil
}

func TestServiceStartRejectsEmptyCapabilities(t *testing.T) {
	svc := NewService(ServiceOptions{
		Auth:       &fakeAuthProvider{},
		Storage:    newFakeStorage(),
		StateCodec: NewStateCodec([]byte("secret"), 10*time.Minute, time.Now),
		Profile:    &fakeProfileFetcher{},
	})

	if _, err := svc.Start(context.Background(), StartCommand{UserID: uuid.New()}); err == nil {
		t.Fatal("Start() error = nil, want error")
	}
}

func TestServiceStartBuildsExpectedAuthURL(t *testing.T) {
	auth := &fakeAuthProvider{}
	svc := NewService(ServiceOptions{
		Auth:       auth,
		Storage:    newFakeStorage(),
		StateCodec: NewStateCodec([]byte("secret"), 10*time.Minute, func() time.Time { return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC) }),
		Profile:    &fakeProfileFetcher{},
	})

	got, err := svc.Start(context.Background(), StartCommand{
		UserID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Capabilities: []string{"google_calendar", "gmail"},
		ReturnTo:     "/onboard",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	wantCapabilities := []string{"gmail", "google_calendar"}
	if !reflect.DeepEqual(got.Capabilities, wantCapabilities) {
		t.Fatalf("Start().Capabilities = %#v, want %#v", got.Capabilities, wantCapabilities)
	}

	if got.AuthURL == "" {
		t.Fatal("Start().AuthURL is empty")
	}

	if auth.lastState == "" {
		t.Fatal("AuthCodeURL state is empty")
	}
}

func TestServiceCompleteUpsertsOnlyGrantedCapabilities(t *testing.T) {
	auth := &fakeAuthProvider{
		token: (&oauth2.Token{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			TokenType:    "Bearer",
			Expiry:       time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		}).WithExtra(map[string]any{
			"scope": strings.Join([]string{
				"https://www.googleapis.com/auth/gmail.modify",
				"https://www.googleapis.com/auth/userinfo.email",
			}, " "),
		}),
	}
	storage := newFakeStorage()
	svc := NewService(ServiceOptions{
		Auth:       auth,
		Storage:    storage,
		StateCodec: NewStateCodec([]byte("secret"), 10*time.Minute, func() time.Time { return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC) }),
		Profile: &fakeProfileFetcher{profile: GoogleProfile{
			ID:      "google-user-1",
			Email:   "user@example.com",
			Name:    "Test User",
			Picture: "https://example.com/avatar.png",
		}},
	})

	rawState, err := svc.state.Sign(StatePayload{
		UserID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Capabilities: []string{"gmail", "google_calendar"},
		ReturnTo:     "/onboard",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	got, err := svc.Complete(context.Background(), CompleteCommand{
		RawState: rawState,
		Code:     "oauth-code",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if got.ReturnTo != "/onboard" {
		t.Fatalf("Complete().ReturnTo = %q", got.ReturnTo)
	}

	if len(got.Integrations) != 1 {
		t.Fatalf("Complete().Integrations len = %d, want 1", len(got.Integrations))
	}

	if got.Integrations[0].CapabilityID != entities.CapabilityGmail {
		t.Fatalf("Complete().Integrations[0].CapabilityID = %q", got.Integrations[0].CapabilityID)
	}
}

func TestServiceCompleteRejectsPartialConsentWithoutCreatingRows(t *testing.T) {
	auth := &fakeAuthProvider{
		token: (&oauth2.Token{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			TokenType:    "Bearer",
			Expiry:       time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		}).WithExtra(map[string]any{
			"scope": "https://www.googleapis.com/auth/userinfo.email",
		}),
	}
	storage := newFakeStorage()
	svc := NewService(ServiceOptions{
		Auth:       auth,
		Storage:    storage,
		StateCodec: NewStateCodec([]byte("secret"), 10*time.Minute, func() time.Time { return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC) }),
		Profile: &fakeProfileFetcher{profile: GoogleProfile{
			ID:    "google-user-1",
			Email: "user@example.com",
		}},
	})

	rawState, err := svc.state.Sign(StatePayload{
		UserID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Capabilities: []string{"gmail"},
		ReturnTo:     "/onboard",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	got, err := svc.Complete(context.Background(), CompleteCommand{
		RawState: rawState,
		Code:     "oauth-code",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if len(got.Integrations) != 0 {
		t.Fatalf("Complete().Integrations len = %d, want 0", len(got.Integrations))
	}

	if len(storage.upserts) != 0 {
		t.Fatalf("storage.Upsert calls = %d, want 0", len(storage.upserts))
	}
}

func TestServiceCompletePreservesRefreshTokenAndExistingIDOnReauth(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	existingID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	storage := newFakeStorage(entities.AccountIntegration{
		ID:                existingID,
		UserID:            userID,
		CapabilityID:      entities.CapabilityGmail,
		Provider:          "gmail",
		ExternalAccountID: "user@example.com",
		DisplayName:       "Old Name",
		Status:            entities.AccountIntegrationStatusActive,
		SecretPayload: map[string]any{
			"refresh_token": "old-refresh-token",
		},
		Metadata: map[string]any{
			"email": "user@example.com",
		},
		CreatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
	})
	auth := &fakeAuthProvider{
		token: (&oauth2.Token{
			AccessToken: "new-access-token",
			TokenType:   "Bearer",
			Expiry:      time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		}).WithExtra(map[string]any{
			"scope": "https://www.googleapis.com/auth/gmail.modify",
		}),
	}
	svc := NewService(ServiceOptions{
		Auth:       auth,
		Storage:    storage,
		StateCodec: NewStateCodec([]byte("secret"), 10*time.Minute, func() time.Time { return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC) }),
		Profile: &fakeProfileFetcher{profile: GoogleProfile{
			ID:      "google-user-1",
			Email:   "user@example.com",
			Name:    "New Name",
			Picture: "https://example.com/avatar.png",
		}},
	})

	rawState, err := svc.state.Sign(StatePayload{
		UserID:       userID,
		Capabilities: []string{"gmail"},
		ReturnTo:     "/onboard",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	got, err := svc.Complete(context.Background(), CompleteCommand{
		RawState: rawState,
		Code:     "oauth-code",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if len(got.Integrations) != 1 {
		t.Fatalf("Complete().Integrations len = %d, want 1", len(got.Integrations))
	}

	if got.Integrations[0].ID != existingID {
		t.Fatalf("Complete().Integrations[0].ID = %s, want %s", got.Integrations[0].ID, existingID)
	}

	if got.Integrations[0].SecretPayload["refresh_token"] != "old-refresh-token" {
		t.Fatalf("refresh_token = %#v, want old-refresh-token", got.Integrations[0].SecretPayload["refresh_token"])
	}

	if got.Integrations[0].SecretPayload["access_token"] != "new-access-token" {
		t.Fatalf("access_token = %#v, want new-access-token", got.Integrations[0].SecretPayload["access_token"])
	}
}

func TestServiceCompleteReturnsExchangeFailure(t *testing.T) {
	svc := NewService(ServiceOptions{
		Auth:    &fakeAuthProvider{err: errors.New("exchange failed")},
		Storage: newFakeStorage(),
		StateCodec: NewStateCodec([]byte("secret"), 10*time.Minute, func() time.Time {
			return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
		}),
		Profile: &fakeProfileFetcher{},
	})

	rawState, err := svc.state.Sign(StatePayload{
		UserID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Capabilities: []string{"gmail"},
		ReturnTo:     "/onboard",
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	if _, err := svc.Complete(context.Background(), CompleteCommand{
		RawState: rawState,
		Code:     "oauth-code",
	}); err == nil {
		t.Fatal("Complete() error = nil, want error")
	}
}
