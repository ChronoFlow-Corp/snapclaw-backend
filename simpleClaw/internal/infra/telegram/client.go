package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"shared/pkg/observability"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.telegram.org"
	defaultTimeout = 15 * time.Second
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type User struct {
	ID            int64  `json:"id"`
	IsBot         bool   `json:"is_bot"`
	Username      string `json:"username,omitempty"`
	FirstName     string `json:"first_name,omitempty"`
	CanManageBots bool   `json:"can_manage_bots,omitempty"`
}

type WebhookInfo struct {
	URL                  string `json:"url"`
	HasCustomCertificate bool   `json:"has_custom_certificate"`
	PendingUpdateCount   int    `json:"pending_update_count"`
}

type APIError struct {
	Code        int
	Description string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}

	if strings.TrimSpace(e.Description) == "" {
		return fmt.Sprintf("telegram api error: code=%d", e.Code)
	}

	return fmt.Sprintf("telegram api error: code=%d description=%s", e.Code, e.Description)
}

type apiResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

type setWebhookRequest struct {
	URL            string   `json:"url"`
	SecretToken    string   `json:"secret_token,omitempty"`
	AllowedUpdates []string `json:"allowed_updates,omitempty"`
}

type sendMessageRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

func New(baseURL, token string, timeout time.Duration) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return &Client{
		baseURL: baseURL,
		token:   strings.TrimSpace(token),
		http:    observability.NewHTTPClient(timeout),
	}
}

func (c *Client) GetMe(ctx context.Context) (User, error) {
	return get[User](c, ctx, "getMe", nil)
}

func (c *Client) DeleteWebhook(ctx context.Context, dropPendingUpdates bool) error {
	_, err := get[bool](c, ctx, "deleteWebhook", map[string]string{
		"drop_pending_updates": strconv.FormatBool(dropPendingUpdates),
	})

	return err
}

func (c *Client) SetWebhook(
	ctx context.Context,
	webhookURL, secretToken string,
	allowedUpdates []string,
) error {
	_, err := post[bool](c, ctx, "setWebhook", setWebhookRequest{
		URL:            strings.TrimSpace(webhookURL),
		SecretToken:    strings.TrimSpace(secretToken),
		AllowedUpdates: append([]string(nil), allowedUpdates...),
	})

	return err
}

func (c *Client) GetWebhookInfo(ctx context.Context) (WebhookInfo, error) {
	return get[WebhookInfo](c, ctx, "getWebhookInfo", nil)
}

func (c *Client) GetUpdates(ctx context.Context, params GetUpdatesParams) ([]Update, error) {
	query := make(map[string]string)
	if params.Offset > 0 {
		query["offset"] = strconv.FormatInt(params.Offset, 10)
	}
	if params.Limit > 0 {
		query["limit"] = strconv.Itoa(params.Limit)
	}
	if params.TimeoutSeconds > 0 {
		query["timeout"] = strconv.Itoa(params.TimeoutSeconds)
	}
	if len(params.AllowedUpdates) > 0 {
		raw, err := json.Marshal(params.AllowedUpdates)
		if err != nil {
			return nil, fmt.Errorf("telegram client: marshal allowed updates: %w", err)
		}
		query["allowed_updates"] = string(raw)
	}

	return get[[]Update](c, ctx, "getUpdates", query)
}

func (c *Client) GetManagedBotToken(ctx context.Context, userID int64) (string, error) {
	return get[string](c, ctx, "getManagedBotToken", map[string]string{
		"user_id": strconv.FormatInt(userID, 10),
	})
}

func (c *Client) ReplaceManagedBotToken(ctx context.Context, userID int64) (string, error) {
	return get[string](c, ctx, "replaceManagedBotToken", map[string]string{
		"user_id": strconv.FormatInt(userID, 10),
	})
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	_, err := post[json.RawMessage](c, ctx, "sendMessage", sendMessageRequest{
		ChatID: chatID,
		Text:   text,
	})

	return err
}

func get[T any](c *Client, ctx context.Context, method string, query map[string]string) (T, error) {
	var zero T

	req, err := c.newRequest(ctx, http.MethodGet, method, query, nil)
	if err != nil {
		return zero, err
	}

	return do[T](c, req)
}

func post[T any](c *Client, ctx context.Context, method string, body any) (T, error) {
	var zero T

	payload, err := json.Marshal(body)
	if err != nil {
		return zero, fmt.Errorf("telegram client: marshal %s: %w", method, err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, method, nil, bytes.NewReader(payload))
	if err != nil {
		return zero, err
	}

	req.Header.Set("Content-Type", "application/json")

	return do[T](c, req)
}

func (c *Client) newRequest(
	ctx context.Context,
	method string,
	apiMethod string,
	query map[string]string,
	body io.Reader,
) (*http.Request, error) {
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("telegram client: http client is not initialized")
	}

	if strings.TrimSpace(c.token) == "" {
		return nil, fmt.Errorf("telegram client: bot token is required")
	}

	if strings.TrimSpace(apiMethod) == "" {
		return nil, fmt.Errorf("telegram client: api method is required")
	}

	u, err := url.Parse(c.baseURL + "/bot" + c.token + "/" + apiMethod)
	if err != nil {
		return nil, fmt.Errorf("telegram client: build %s request: %w", apiMethod, err)
	}

	if len(query) > 0 {
		q := u.Query()
		for key, value := range query {
			q.Set(key, value)
		}
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("telegram client: new %s request: %w", apiMethod, err)
	}

	req.Header.Set("Accept", "application/json")

	return req, nil
}

func do[T any](c *Client, req *http.Request) (T, error) {
	var zero T

	resp, err := c.http.Do(req)
	if err != nil {
		return zero, fmt.Errorf("telegram client: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, fmt.Errorf("telegram client: read %s response: %w", req.URL.Path, err)
	}

	var apiResp apiResponse[T]
	if len(body) > 0 {
		if err := json.Unmarshal(body, &apiResp); err != nil {
			return zero, fmt.Errorf("telegram client: decode %s response: %w", req.URL.Path, err)
		}
	}

	if resp.StatusCode != http.StatusOK || !apiResp.OK {
		if apiResp.ErrorCode == 0 {
			apiResp.ErrorCode = resp.StatusCode
		}

		return zero, &APIError{
			Code:        apiResp.ErrorCode,
			Description: apiResp.Description,
		}
	}

	return apiResp.Result, nil
}
