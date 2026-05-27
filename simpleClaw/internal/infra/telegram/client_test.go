package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClientGetMe(t *testing.T) {
	t.Parallel()

	cl := &Client{
		baseURL: "https://api.telegram.org",
		token:   "manager-token",
		http: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/botmanager-token/getMe" {
					t.Fatalf("path = %q", req.URL.Path)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body: io.NopCloser(strings.NewReader(`{
						"ok": true,
						"result": {
							"id": 42,
							"is_bot": true,
							"username": "simpleclaw_manager_bot",
							"can_manage_bots": true
						}
					}`)),
					Header: make(http.Header),
				}, nil
			}),
		},
	}

	me, err := cl.GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}

	if me.ID != 42 {
		t.Fatalf("ID = %d", me.ID)
	}

	if !me.CanManageBots {
		t.Fatal("expected CanManageBots to be true")
	}
}

func TestClientGetManagedBotToken(t *testing.T) {
	t.Parallel()

	cl := &Client{
		baseURL: "https://api.telegram.org",
		token:   "manager-token",
		http: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/botmanager-token/getManagedBotToken" {
					t.Fatalf("path = %q", req.URL.Path)
				}

				if got := req.URL.Query().Get("user_id"); got != "777" {
					t.Fatalf("user_id = %q", got)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":"777:managed-token"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	token, err := cl.GetManagedBotToken(context.Background(), 777)
	if err != nil {
		t.Fatalf("GetManagedBotToken() error = %v", err)
	}

	if token != "777:managed-token" {
		t.Fatalf("token = %q", token)
	}
}

func TestClientReplaceManagedBotToken(t *testing.T) {
	t.Parallel()

	cl := &Client{
		baseURL: "https://api.telegram.org",
		token:   "manager-token",
		http: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/botmanager-token/replaceManagedBotToken" {
					t.Fatalf("path = %q", req.URL.Path)
				}

				if got := req.URL.Query().Get("user_id"); got != "777" {
					t.Fatalf("user_id = %q", got)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":"777:new-token"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	token, err := cl.ReplaceManagedBotToken(context.Background(), 777)
	if err != nil {
		t.Fatalf("ReplaceManagedBotToken() error = %v", err)
	}

	if token != "777:new-token" {
		t.Fatalf("token = %q", token)
	}
}

func TestClientGetManagedBotToken_DecodesTelegramError(t *testing.T) {
	t.Parallel()

	cl := &Client{
		baseURL: "https://api.telegram.org",
		token:   "manager-token",
		http: &http.Client{
			Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Status:     "400 Bad Request",
					Body:       io.NopCloser(strings.NewReader(`{"ok":false,"error_code":400,"description":"Bad Request: managed bot not found"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	_, err := cl.GetManagedBotToken(context.Background(), 777)
	if err == nil {
		t.Fatal("expected error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}

	if apiErr.Description != "Bad Request: managed bot not found" {
		t.Fatalf("description = %q", apiErr.Description)
	}
}

func TestClientDeleteWebhook(t *testing.T) {
	t.Parallel()

	cl := &Client{
		baseURL: "https://api.telegram.org",
		token:   "manager-token",
		http: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/botmanager-token/deleteWebhook" {
					t.Fatalf("path = %q", req.URL.Path)
				}

				if got := req.URL.Query().Get("drop_pending_updates"); got != "true" {
					t.Fatalf("drop_pending_updates = %q", got)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	if err := cl.DeleteWebhook(context.Background(), true); err != nil {
		t.Fatalf("DeleteWebhook() error = %v", err)
	}
}

func TestClientGetUpdates(t *testing.T) {
	t.Parallel()

	cl := &Client{
		baseURL: "https://api.telegram.org",
		token:   "manager-token",
		http: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/botmanager-token/getUpdates" {
					t.Fatalf("path = %q", req.URL.Path)
				}

				if got := req.URL.Query().Get("offset"); got != "11" {
					t.Fatalf("offset = %q", got)
				}

				if got := req.URL.Query().Get("timeout"); got != "30" {
					t.Fatalf("timeout = %q", got)
				}

				if got := req.URL.Query().Get("allowed_updates"); !strings.Contains(got, "managed_bot") {
					t.Fatalf("allowed_updates = %q", got)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body: io.NopCloser(strings.NewReader(`{
						"ok": true,
						"result": [
							{"update_id": 11, "message": {"text": "/start code-1", "chat": {"id": 2002}, "from": {"id": 1001, "username": "snapclaw_user"}}},
							{"update_id": 12, "managed_bot": {"user": {"id": 1001, "username": "snapclaw_user"}, "bot": {"id": 3003, "is_bot": true, "username": "snapclaw_helper_bot", "first_name": "Snapclaw Helper"}}}
						]
					}`)),
					Header: make(http.Header),
				}, nil
			}),
		},
	}

	updates, err := cl.GetUpdates(context.Background(), GetUpdatesParams{
		Offset:         11,
		TimeoutSeconds: 30,
		AllowedUpdates: []string{"message", "managed_bot"},
	})
	if err != nil {
		t.Fatalf("GetUpdates() error = %v", err)
	}

	if len(updates) != 2 {
		t.Fatalf("len(updates) = %d", len(updates))
	}

	if updates[0].UpdateID != 11 {
		t.Fatalf("first update id = %d", updates[0].UpdateID)
	}

	if updates[1].ManagedBot == nil || updates[1].ManagedBot.Bot.ID != 3003 {
		t.Fatalf("second update = %#v", updates[1])
	}
}
