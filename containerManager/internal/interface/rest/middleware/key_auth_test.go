package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"containermanager/internal/interface/rest/middleware"
)

func TestAuthRejectsMissingAuthorizationHeader(t *testing.T) {
	var called bool

	handler := middleware.Auth("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/claws/start", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Fatalf("next handler must not be called when auth header is missing")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAuthRejectsInvalidAuthorizationHeader(t *testing.T) {
	var called bool

	handler := middleware.Auth("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/claws/start", nil)
	req.Header.Set("Authorization", "wrong-secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Fatalf("next handler must not be called when auth header is invalid")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
