package hosting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// client executes HTTP requests against containerManager.
type client struct {
	http *http.Client
}

func newClient(timeout time.Duration) *client {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}

	return &client{
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *client) createClaw(
	ctx context.Context,
	baseURL string,
	payload createClawRequest,
) (createClawResponse, error) {
	const op = "infra.hosting.client.createClaw"

	if c == nil || c.http == nil {
		return createClawResponse{}, fmt.Errorf("%s: http client is not initialized", op)
	}

	url := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if url == "" {
		return createClawResponse{}, fmt.Errorf("%s: base url is required", op)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return createClawResponse{}, fmt.Errorf("%s: %w", op, err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url+clawsEndpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return createClawResponse{}, fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return createClawResponse{}, fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return createClawResponse{}, fmt.Errorf("%s: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}

		return createClawResponse{}, fmt.Errorf(
			"%s: unexpected status %d: %s",
			op,
			resp.StatusCode,
			msg,
		)
	}

	var out createClawResponse
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &out); err != nil {
			return createClawResponse{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	return out, nil
}
