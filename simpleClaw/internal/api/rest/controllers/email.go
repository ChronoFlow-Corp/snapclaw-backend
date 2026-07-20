package controllers

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type emailUnsubscriber interface {
	Unsubscribe(ctx context.Context, token string) error
}

type Email struct {
	svc emailUnsubscriber
}

func NewEmail(svc emailUnsubscriber) *Email {
	return &Email{svc: svc}
}

func (e *Email) Register(r chi.Router) {
	r.Get("/email/unsubscribe", e.Unsubscribe)
	r.Post("/email/unsubscribe", e.Unsubscribe)
}

// Unsubscribe handles both the visible link (GET) and the RFC 8058
// one-click POST that Gmail/Outlook send from their unsubscribe button.
func (e *Email) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" && r.Method == http.MethodPost {
		_ = r.ParseForm()
		token = strings.TrimSpace(r.PostFormValue("token"))
	}

	if token == "" {
		e.render(w, r, http.StatusBadRequest, "This unsubscribe link is invalid.")
		return
	}

	if err := e.svc.Unsubscribe(r.Context(), token); err != nil {
		e.render(w, r, http.StatusBadRequest, "This unsubscribe link is invalid or has expired.")
		return
	}

	e.render(w, r, http.StatusOK, "You've been unsubscribed from SnapClaw product updates. You'll still receive important account emails.")
}

func (e *Email) render(w http.ResponseWriter, r *http.Request, code int, message string) {
	if r.Method == http.MethodPost {
		w.WriteHeader(code)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(
		"<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">" +
			"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
			"<title>SnapClaw</title></head>" +
			"<body style=\"font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;" +
			"max-width:480px;margin:64px auto;padding:0 20px;color:#374151;text-align:center;\">" +
			"<h1 style=\"font-size:20px;color:#111827;\">SnapClaw</h1>" +
			"<p style=\"font-size:15px;line-height:1.6;\">" + template.HTMLEscapeString(message) + "</p>" +
			"</body></html>",
	))
}
