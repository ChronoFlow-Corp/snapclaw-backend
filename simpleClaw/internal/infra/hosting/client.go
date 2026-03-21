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

	"shared/pkg/hostingapi"
	"shared/pkg/observability"
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
		http: observability.NewHTTPClient(timeout),
	}
}

func (c *client) createClaw(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	payload hostingapi.CreateClawRequest,
) (hostingapi.CreateClawResponse, error) {
	const op = "infra.hosting.client.createClaw"

	if c == nil || c.http == nil {
		return hostingapi.CreateClawResponse{}, fmt.Errorf("%s: http client is not initialized", op)
	}

	url := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if url == "" {
		return hostingapi.CreateClawResponse{}, fmt.Errorf("%s: base url is required", op)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return hostingapi.CreateClawResponse{}, fmt.Errorf("%s: %w", op, err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url+hostingapi.ClawsEndpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return hostingapi.CreateClawResponse{}, fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return hostingapi.CreateClawResponse{}, fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return hostingapi.CreateClawResponse{}, fmt.Errorf("%s: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK {
		return hostingapi.CreateClawResponse{}, unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
	}

	var out hostingapi.CreateClawResponse

	if len(respBody) > 0 {
		err := json.Unmarshal(respBody, &out)
		if err != nil {
			return hostingapi.CreateClawResponse{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	return out, nil
}

func (c *client) capacity(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
) (hostingapi.CapacityResponse, error) {
	const op = "infra.hosting.client.capacity"

	if c == nil || c.http == nil {
		return hostingapi.CapacityResponse{}, fmt.Errorf("%s: http client is not initialized", op)
	}

	url := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if url == "" {
		return hostingapi.CapacityResponse{}, fmt.Errorf("%s: base url is required", op)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+hostingapi.CapacityEndpoint, http.NoBody)
	if err != nil {
		return hostingapi.CapacityResponse{}, fmt.Errorf("%s: %w", op, err)
	}

	req.Header.Set("Accept", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return hostingapi.CapacityResponse{}, fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return hostingapi.CapacityResponse{}, fmt.Errorf("%s: %w", op, err)
	}

	if resp.StatusCode != http.StatusOK {
		return hostingapi.CapacityResponse{}, unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
	}

	var out hostingapi.CapacityResponse

	if len(respBody) > 0 {
		err := json.Unmarshal(respBody, &out)
		if err != nil {
			return hostingapi.CapacityResponse{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	return out, nil
}

func (c *client) updateClaw(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	payload hostingapi.UpdateClawRequest,
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
		url+hostingapi.ClawsEndpoint,
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
		return unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
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

	u, err := url.Parse(rawURL + hostingapi.ClawsEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set(hostingapi.QueryUserID, userID)
	q.Set(hostingapi.QueryClawID, clawID)

	if deleteConfig {
		q.Set(hostingapi.QueryDeleteConfig, "true")
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), http.NoBody)
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
		return unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
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

	u, err := url.Parse(rawURL + hostingapi.ClawsConfigEndpoint)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set(hostingapi.QueryUserID, userID)
	q.Set(hostingapi.QueryClawID, clawID)

	if deleteAfter {
		q.Set(hostingapi.QueryDeleteAfter, "true")
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
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

		return nil, unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
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

	u, err := url.Parse(rawURL + hostingapi.ClawsConfigEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set(hostingapi.QueryUserID, userID)
	q.Set(hostingapi.QueryClawID, clawID)
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

		return unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
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

	u, err := url.Parse(rawURL + hostingapi.ClawsStopEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set(hostingapi.QueryUserID, userID)
	q.Set(hostingapi.QueryClawID, clawID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
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
		return unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
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

	u, err := url.Parse(rawURL + hostingapi.ClawsStartEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set(hostingapi.QueryUserID, userID)
	q.Set(hostingapi.QueryClawID, clawID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
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
		return unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
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

	u, err := url.Parse(rawURL + hostingapi.ApproveEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set(hostingapi.QueryUserID, userID)
	q.Set(hostingapi.QueryClawID, clawID)
	q.Set(hostingapi.QueryCode, code)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
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
		return unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
	}

	return nil
}

func (c *client) connect(
	ctx context.Context,
	baseURL string,
	headers map[string]string,
	userID string,
	clawID string,
	provider string,
	token []byte,
) error {
	const op = "infra.hosting.client.connect"

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

	if provider == "" {
		return fmt.Errorf("%s: provider is required", op)
	}

	if len(token) == 0 {
		return fmt.Errorf("%s: token is required", op)
	}

	u, err := url.Parse(rawURL + hostingapi.ConnectEndpoint)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	q := u.Query()
	q.Set(hostingapi.QueryUserID, userID)
	q.Set(hostingapi.QueryClawID, clawID)
	q.Set(hostingapi.QueryProvider, provider)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(token))
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
		return unexpectedStatusError(op, resp.StatusCode, resp.Status, respBody)
	}

	return nil
}

func unexpectedStatusError(op string, statusCode int, status string, body []byte) error {
	if payload, ok := hostingapi.ParseErrorResponse(body); ok {
		if statusCode == http.StatusBadRequest && payload.Code == hostingapi.ErrCodeInvalidCode {
			return fmt.Errorf("%s: %w", op, ErrInvalidCode)
		}

		if statusCode == http.StatusConflict && payload.Code == hostingapi.ErrCodeServerCapacityExceeded {
			return fmt.Errorf("%s: %w", op, ErrServerCapacityExceeded)
		}

		if statusCode == http.StatusConflict && payload.Code == hostingapi.ErrCodeServerMemoryUnavailable {
			return fmt.Errorf("%s: %w", op, ErrServerMemoryUnavailable)
		}

		return fmt.Errorf("%s: unexpected status %d: %s", op, statusCode, payload.Message)
	}

	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = status
	}

	return fmt.Errorf("%s: unexpected status %d: %s", op, statusCode, msg)
}
