package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"shared/pkg/observability"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.resend.com"
	defaultTimeout = 15 * time.Second
)

// Message is a single email ready to be sent through Resend.
type Message struct {
	From    string
	To      string
	Subject string
	HTML    string
	Text    string
	ReplyTo string
	Headers map[string]string
	// IdempotencyKey lets Resend collapse retries of the same message into a
	// single send. Keyed on the outbox row ID, it prevents duplicates both when
	// MarkSent fails after a successful send and when more than one dispatcher
	// replica claims the same row.
	IdempotencyKey string
}

// Client is a thin HTTP client for the Resend REST API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type sendRequest struct {
	From    string            `json:"from"`
	To      []string          `json:"to"`
	Subject string            `json:"subject"`
	HTML    string            `json:"html,omitempty"`
	Text    string            `json:"text,omitempty"`
	ReplyTo string            `json:"reply_to,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type sendResponse struct {
	ID string `json:"id"`
}

type errorResponse struct {
	StatusCode int    `json:"statusCode"`
	Name       string `json:"name"`
	Message    string `json:"message"`
}

func New(baseURL, apiKey string, timeout time.Duration) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return &Client{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(apiKey),
		http:    observability.NewHTTPClient(timeout),
	}
}

// Send delivers a single message and returns the provider message id.
func (c *Client) Send(ctx context.Context, msg Message) (string, error) {
	if c == nil || c.http == nil {
		return "", fmt.Errorf("resend client: http client is not initialized")
	}

	if strings.TrimSpace(c.apiKey) == "" {
		return "", fmt.Errorf("resend client: api key is required")
	}

	if strings.TrimSpace(msg.To) == "" {
		return "", fmt.Errorf("resend client: recipient is required")
	}

	payload, err := json.Marshal(sendRequest{
		From:    msg.From,
		To:      []string{msg.To},
		Subject: msg.Subject,
		HTML:    msg.HTML,
		Text:    msg.Text,
		ReplyTo: strings.TrimSpace(msg.ReplyTo),
		Headers: msg.Headers,
	})
	if err != nil {
		return "", fmt.Errorf("resend client: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/emails", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("resend client: new request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if key := strings.TrimSpace(msg.IdempotencyKey); key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("resend client: do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("resend client: read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var apiErr errorResponse
		_ = json.Unmarshal(body, &apiErr)

		message := strings.TrimSpace(apiErr.Message)
		if message == "" {
			message = strings.TrimSpace(string(body))
		}

		return "", fmt.Errorf(
			"resend client: send failed: status=%d name=%s message=%s",
			resp.StatusCode,
			apiErr.Name,
			message,
		)
	}

	var ok sendResponse
	if len(body) > 0 {
		_ = json.Unmarshal(body, &ok)
	}

	return ok.ID, nil
}
