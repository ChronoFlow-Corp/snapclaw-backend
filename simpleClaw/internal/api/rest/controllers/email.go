package controllers

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"strings"

	emailservice "simpleClaw/internal/service/email"

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
	r.Get("/email/unsubscribe", e.Confirm)
	r.Post("/email/unsubscribe", e.Unsubscribe)
}

// Confirm handles the visible unsubscribe link. A GET must be side-effect-free:
// mail-security scanners and link prefetchers (Outlook SafeLinks, Mimecast,
// Apple Mail Privacy Protection, …) routinely issue GET requests against links
// in email, so the opt-out write happens only in Unsubscribe (POST). This
// renders a small confirmation page whose form POSTs the token back.
func (e *Email) Confirm(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		e.renderMessage(w, http.StatusBadRequest, "Ссылка для отписки недействительна.")
		return
	}

	e.renderConfirm(w, r.URL.Path, token)
}

// Unsubscribe performs the opt-out write. It serves both the confirmation
// form's POST and the RFC 8058 one-click POST that Gmail/Outlook send from
// their unsubscribe button (which carries the token in the query string).
func (e *Email) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		_ = r.ParseForm()
		token = strings.TrimSpace(r.PostFormValue("token"))
	}

	if token == "" {
		e.renderMessage(w, http.StatusBadRequest, "Ссылка для отписки недействительна.")
		return
	}

	if err := e.svc.Unsubscribe(r.Context(), token); err != nil {
		if errors.Is(err, emailservice.ErrInvalidToken) {
			e.renderMessage(w, http.StatusBadRequest, "Ссылка для отписки недействительна.")
			return
		}

		// Transient failure (e.g. datastore unavailable): return 5xx so RFC 8058
		// one-click clients retry instead of telling the user the link is broken.
		e.renderMessage(w, http.StatusInternalServerError,
			"Не удалось обработать запрос. Попробуйте позже.")
		return
	}

	e.renderMessage(w, http.StatusOK,
		"Вы отписались от новостей о продукте SnapClaw. Важные письма по вашему аккаунту вы продолжите получать.")
}

func (e *Email) renderConfirm(w http.ResponseWriter, action, token string) {
	body := `<p style="font-size:15px;line-height:1.6;">` +
		`Отписаться от новостей о продукте SnapClaw? ` +
		`Важные письма по вашему аккаунту вы продолжите получать.</p>` +
		`<form method="post" action="` + template.HTMLEscapeString(action) + `" style="margin-top:20px;">` +
		`<input type="hidden" name="token" value="` + template.HTMLEscapeString(token) + `">` +
		`<button type="submit" style="background:#4f46e5;color:#ffffff;border:0;border-radius:8px;` +
		`padding:12px 22px;font-size:15px;font-weight:600;cursor:pointer;">Отписаться</button>` +
		`</form>`

	e.writePage(w, http.StatusOK, body)
}

func (e *Email) renderMessage(w http.ResponseWriter, code int, message string) {
	e.writePage(w, code,
		`<p style="font-size:15px;line-height:1.6;">`+template.HTMLEscapeString(message)+`</p>`)
}

func (e *Email) writePage(w http.ResponseWriter, code int, inner string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(
		"<!doctype html><html lang=\"ru\"><head><meta charset=\"utf-8\">" +
			"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
			"<title>SnapClaw</title></head>" +
			"<body style=\"font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;" +
			"max-width:480px;margin:64px auto;padding:0 20px;color:#374151;text-align:center;\">" +
			"<h1 style=\"font-size:20px;color:#111827;\">SnapClaw</h1>" +
			inner +
			"</body></html>",
	))
}
