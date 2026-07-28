package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type fakeUnsub struct {
	called bool
	token  string
	err    error
}

func (f *fakeUnsub) Unsubscribe(_ context.Context, token string) error {
	f.called = true
	f.token = token
	return f.err
}

func newEmailRouter(svc emailUnsubscriber) http.Handler {
	r := chi.NewRouter()
	NewEmail(svc).Register(r)
	return r
}

func TestUnsubscribeGetDoesNotMutate(t *testing.T) {
	svc := &fakeUnsub{}
	req := httptest.NewRequest(http.MethodGet, "/email/unsubscribe?token=abc123", nil)
	rec := httptest.NewRecorder()

	newEmailRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if svc.called {
		t.Error("GET must be side-effect-free — scanners and prefetchers issue GETs")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<form") || !strings.Contains(body, `name="token"`) {
		t.Errorf("expected a confirmation form, got %q", body)
	}
	if !strings.Contains(body, "abc123") {
		t.Errorf("expected the token embedded in the form, got %q", body)
	}
}

func TestUnsubscribeGetEmptyTokenIsBadRequest(t *testing.T) {
	svc := &fakeUnsub{}
	req := httptest.NewRequest(http.MethodGet, "/email/unsubscribe", nil)
	rec := httptest.NewRecorder()

	newEmailRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if svc.called {
		t.Error("empty-token GET must not call the service")
	}
}

func TestUnsubscribePostFormWrites(t *testing.T) {
	svc := &fakeUnsub{}
	form := url.Values{"token": {"abc123"}}
	req := httptest.NewRequest(http.MethodPost, "/email/unsubscribe", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	newEmailRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !svc.called || svc.token != "abc123" {
		t.Errorf("expected POST to unsubscribe token abc123, called=%v token=%q", svc.called, svc.token)
	}
}

func TestUnsubscribePostOneClickQueryToken(t *testing.T) {
	svc := &fakeUnsub{}
	// RFC 8058 one-click: token in the query string, body is List-Unsubscribe=One-Click.
	req := httptest.NewRequest(
		http.MethodPost,
		"/email/unsubscribe?token=xyz789",
		strings.NewReader("List-Unsubscribe=One-Click"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	newEmailRouter(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !svc.called || svc.token != "xyz789" {
		t.Errorf("expected one-click POST to unsubscribe token xyz789, called=%v token=%q", svc.called, svc.token)
	}
}
