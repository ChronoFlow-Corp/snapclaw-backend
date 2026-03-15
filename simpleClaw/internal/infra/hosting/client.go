package hosting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	headers map[string]string,
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}

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

func (c *client) updateClaw(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	payload updateClawRequest,
) error {
	const op = "infra.hosting.client.updateClaw"

	if c == nil || c.http == nil {
		return fmt.Errorf("%s: http client is not initialized", op)
	}

	url := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if url == "" {
		return fmt.Errorf("%s: base url is required", op)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		url+clawsEndpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}

		return fmt.Errorf(
			"%s: unexpected status %d: %s",
			op,
			resp.StatusCode,
			msg,
		)
	}

	return nil
}

func (c *client) deleteClaw(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	userID string,
	clawID string,
	deleteConfig bool,
) error {
	const op = "infra.hosting.client.deleteClaw"

	if c == nil || c.http == nil {
		return fmt.Errorf("%s: http client is not initialized", op)
	}

	rawURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if rawURL == "" {
		return fmt.Errorf("%s: base url is required", op)
	}

	u, err := url.Parse(rawURL + clawsEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set("userId", userID)
	q.Set("clawId", clawID)
	if deleteConfig {
		q.Set("deleteConfig", "true")
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}

		return fmt.Errorf(
			"%s: unexpected status %d: %s",
			op,
			resp.StatusCode,
			msg,
		)
	}

	return nil
}

func (c *client) configArchive(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	userID string,
	clawID string,
	deleteAfter bool,
) (io.ReadCloser, error) {
	const op = "infra.hosting.client.configArchive"

	if c == nil || c.http == nil {
		return nil, fmt.Errorf("%s: http client is not initialized", op)
	}

	rawURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if rawURL == "" {
		return nil, fmt.Errorf("%s: base url is required", op)
	}

	if userID == "" {
		return nil, fmt.Errorf("%s: user id is required", op)
	}

	if clawID == "" {
		return nil, fmt.Errorf("%s: claw id is required", op)
	}

	u, err := url.Parse(rawURL + clawsConfigEndpoint)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set("userId", userID)
	q.Set("clawId", clawID)
	if deleteAfter {
		q.Set("deleteAfter", "true")
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Accept", "application/x-tar")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}

		return nil, fmt.Errorf(
			"%s: unexpected status %d: %s",
			op,
			resp.StatusCode,
			msg,
		)
	}

	return resp.Body, nil
}

func (c *client) restoreConfigArchive(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	userID string,
	clawID string,
	body io.Reader,
) error {
	const op = "infra.hosting.client.restoreConfigArchive"

	if c == nil || c.http == nil {
		return fmt.Errorf("%s: http client is not initialized", op)
	}

	rawURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if rawURL == "" {
		return fmt.Errorf("%s: base url is required", op)
	}

	if userID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if clawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	if body == nil {
		return fmt.Errorf("%s: body is required", op)
	}

	u, err := url.Parse(rawURL + clawsConfigEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set("userId", userID)
	q.Set("clawId", clawID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), body)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Content-Type", "application/x-tar")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}

		return fmt.Errorf(
			"%s: unexpected status %d: %s",
			op,
			resp.StatusCode,
			msg,
		)
	}

	return nil
}

func (c *client) stopClaw(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	userID string,
	clawID string,
) error {
	const op = "infra.hosting.client.stopClaw"

	if c == nil || c.http == nil {
		return fmt.Errorf("%s: http client is not initialized", op)
	}

	rawURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if rawURL == "" {
		return fmt.Errorf("%s: base url is required", op)
	}

	u, err := url.Parse(rawURL + clawsStopEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set("userId", userID)
	q.Set("clawId", clawID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}

		return fmt.Errorf(
			"%s: unexpected status %d: %s",
			op,
			resp.StatusCode,
			msg,
		)
	}

	return nil
}

func (c *client) startClaw(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	userID string,
	clawID string,
) error {
	const op = "infra.hosting.client.startClaw"

	if c == nil || c.http == nil {
		return fmt.Errorf("%s: http client is not initialized", op)
	}

	rawURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if rawURL == "" {
		return fmt.Errorf("%s: base url is required", op)
	}

	u, err := url.Parse(rawURL + clawsStartEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set("userId", userID)
	q.Set("clawId", clawID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}

		return fmt.Errorf(
			"%s: unexpected status %d: %s",
			op,
			resp.StatusCode,
			msg,
		)
	}

	return nil
}

func (c *client) approvePairing(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	userID string,
	clawID string,
	code string,
) error {
	const op = "infra.hosting.client.approvePairing"

	if c == nil || c.http == nil {
		return fmt.Errorf("%s: http client is not initialized", op)
	}

	rawURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if rawURL == "" {
		return fmt.Errorf("%s: base url is required", op)
	}

	if userID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if clawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	if code == "" {
		return fmt.Errorf("%s: code is required", op)
	}

	u, err := url.Parse(rawURL + approveEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set("userId", userID)
	q.Set("clawId", clawID)
	q.Set("code", code)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}

		return fmt.Errorf(
			"%s: unexpected status %d: %s",
			op,
			resp.StatusCode,
			msg,
		)
	}

	return nil
}
