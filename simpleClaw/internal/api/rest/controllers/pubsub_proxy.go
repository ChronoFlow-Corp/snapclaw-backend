package controllers

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"shared/pkg/observability"
	"strings"
	"sync"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/pkg/slctx"

	"github.com/go-chi/chi/v5"
)

type proxyTargetService interface {
	GetAll(ctx context.Context) ([]entities.Server, error)
}

type PubSubProxyOptions struct {
	Token          string
	ForwardTimeout time.Duration
	RetryCount     int
	RetryBackoff   time.Duration
	Metrics        *observability.PubSubFanoutMetrics
}

type PubSubProxy struct {
	servers        proxyTargetService
	client         *http.Client
	token          string
	forwardTimeout time.Duration
	retryCount     int
	retryBackoff   time.Duration
	metrics        *observability.PubSubFanoutMetrics
}

type proxyTarget struct {
	ServerID   string
	ServerName string
	URL        string
	AuthToken  string
}

type forwardResult struct {
	target     proxyTarget
	statusCode int
	duration   time.Duration
	err        error
}

var errProxyTargetMissing = errors.New("proxy target missing")

func NewPubSubProxy(servers proxyTargetService, opts PubSubProxyOptions) *PubSubProxy {
	timeout := opts.ForwardTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	retryBackoff := opts.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = 250 * time.Millisecond
	}

	retryCount := max(opts.RetryCount, 0)

	return &PubSubProxy{
		servers:        servers,
		client:         observability.NewHTTPClient(timeout),
		token:          opts.Token,
		forwardTimeout: timeout,
		retryCount:     retryCount,
		retryBackoff:   retryBackoff,
		metrics:        opts.Metrics,
	}
}

func (p *PubSubProxy) Register(r chi.Router) {
	r.Get("/health", p.Health)
	r.Post("/pubsub", p.HandlePubSub)
}

func (p *PubSubProxy) Health(w http.ResponseWriter, _ *http.Request) {
	writePlainText(slog.Default(), w, http.StatusOK, "OK")
}

func (p *PubSubProxy) HandlePubSub(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()

	var flowErr error

	targetCount := 0
	successCount := 0
	ctxWithComponent := observability.WithComponent(r.Context(), "controller.pubsub_proxy")
	ctx, logger, finishFlow := observability.StartFlow(
		ctxWithComponent,
		slctx.Logger(ctxWithComponent),
		"gmail_pubsub_fanout",
		"pubsub.forward",
	)

	defer func() {
		p.metrics.Observe(
			"gmail_pubsub_fanout",
			targetCount,
			successCount,
			flowErr,
			time.Since(startedAt),
		)
		finishFlow(flowErr)
	}()

	r = r.WithContext(ctx)

	if !isJSONContentType(r.Header.Get("Content-Type")) {
		flowErr = observability.DecorateError(
			errors.New("invalid content type"),
			observability.ErrorAttrs{
				Result: observability.ResultValidationError,
				Kind:   observability.ErrorKindValidation,
				Source: observability.ErrorSourceHTTP,
			},
		)

		http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)

		return
	}

	if !p.isAuthorized(inboundToken(r)) {
		logger.Warn("pubsub request rejected", slog.String("reason", "invalid ingress token"))

		flowErr = observability.DecorateError(
			errors.New("invalid ingress token"),
			observability.ErrorAttrs{
				Result: observability.ResultDenied,
				Kind:   observability.ErrorKindDenied,
				Source: observability.ErrorSourceHTTP,
			},
		)

		http.Error(w, "unauthorized", http.StatusUnauthorized)

		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("failed to read pubsub request body", slog.Any("err", err))
		flowErr = observability.DecorateError(err, observability.ErrorAttrs{
			Result: observability.ResultValidationError,
			Kind:   observability.ErrorKindValidation,
			Source: observability.ErrorSourceHTTP,
		})

		http.Error(w, "failed to read request body", http.StatusBadRequest)

		return
	}

	servers, err := p.servers.GetAll(r.Context())
	if err != nil {
		logger.Error("failed to load proxy targets", slog.Any("err", err))
		flowErr = err

		http.Error(w, "failed to load targets", http.StatusServiceUnavailable)

		return
	}

	targets := make([]proxyTarget, 0, len(servers))
	for _, srv := range servers {
		target, err := buildProxyTarget(srv)
		if err != nil {
			if !errors.Is(err, errProxyTargetMissing) {
				logger.Warn(
					"skipping invalid proxy target",
					slog.String("server_id", srv.ID.String()),
					slog.String("server_name", srv.Name),
					slog.String("proxy_url", srv.ProxyURL),
					slog.Any("err", err),
				)
			}

			continue
		}

		targets = append(targets, target)
	}

	logger.Info(
		"pubsub request received",
		slog.Int("body_bytes", len(body)),
		slog.Int("targets", len(targets)),
		slog.String("content_type", r.Header.Get("Content-Type")),
		slog.String("user_agent", r.UserAgent()),
	)
	targetCount = len(targets)

	if len(targets) == 0 {
		logger.Warn("pubsub request has no configured downstream targets")

		flowErr = observability.DecorateError(
			errors.New("no downstream targets configured"),
			observability.ErrorAttrs{
				Result: observability.ResultError,
				Kind:   observability.ErrorKindUnexpected,
				Source: observability.ErrorSourceExternal,
			},
		)

		http.Error(w, "no downstream targets configured", http.StatusBadGateway)

		return
	}

	headers := forwardedHeaders(r.Header)
	results := make(chan forwardResult, len(targets))

	var wg sync.WaitGroup

	for _, target := range targets {
		wg.Add(1)

		go func(ctx context.Context, target proxyTarget) {
			defer wg.Done()

			results <- p.forward(ctx, logger, target, body, headers)
		}(r.Context(), target)
	}

	wg.Wait()
	close(results)

	successCount = 0

	for res := range results {
		if res.err == nil && res.statusCode >= http.StatusOK &&
			res.statusCode < http.StatusMultipleChoices {
			successCount++
		}
	}

	if successCount > 0 {
		if successCount < len(targets) {
			flowErr = observability.DecorateError(
				errors.New("partial downstream failure"),
				observability.ErrorAttrs{
					Result: observability.ResultPartialSuccess,
					Kind:   observability.ErrorKindUnexpected,
					Source: observability.ErrorSourceExternal,
				},
			)
		}

		writePlainText(logger, w, http.StatusOK, "OK")

		return
	}

	flowErr = observability.DecorateError(
		errors.New("all downstream requests failed"),
		observability.ErrorAttrs{
			Result: observability.ResultError,
			Kind:   observability.ErrorKindUnexpected,
			Source: observability.ErrorSourceExternal,
		},
	)

	http.Error(w, "all downstream requests failed", http.StatusBadGateway)
}

func (p *PubSubProxy) forward(
	ctx context.Context,
	logger *slog.Logger,
	target proxyTarget,
	body []byte,
	headers http.Header,
) forwardResult {
	attempts := max(p.retryCount+1, 1)

	var result forwardResult

	for attempt := 1; attempt <= attempts; attempt++ {
		result = p.forwardOnce(ctx, logger, target, body, headers, attempt)
		if result.err == nil && result.statusCode >= http.StatusOK &&
			result.statusCode < http.StatusMultipleChoices {
			return result
		}

		if attempt == attempts || !shouldRetryForward(result) {
			return result
		}

		backoff := time.Duration(attempt) * p.retryBackoff
		if backoff <= 0 {
			continue
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()

			result.err = ctx.Err()

			return result
		case <-timer.C:
		}
	}

	return result
}

func (p *PubSubProxy) forwardOnce(
	ctx context.Context,
	logger *slog.Logger,
	target proxyTarget,
	body []byte,
	headers http.Header,
	attempt int,
) forwardResult {
	start := time.Now()

	reqCtx, cancel := context.WithTimeout(ctx, p.forwardTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodPost,
		target.URL,
		bytes.NewReader(body),
	)
	if err != nil {
		logger.Error(
			"failed to build downstream request",
			slog.String("target", target.URL),
			slog.String("server_id", target.ServerID),
			slog.Int("attempt", attempt),
			slog.Any("err", err),
		)

		return forwardResult{
			target:   target,
			duration: time.Since(start),
			err:      err,
		}
	}

	req.Header = headers.Clone()

	if target.AuthToken != "" {
		req.Header.Set("Authorization", target.AuthToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		logger.Warn(
			"pubsub forward attempt failed",
			slog.String("target", target.URL),
			slog.String("server_id", target.ServerID),
			slog.String("server_name", target.ServerName),
			slog.Int("attempt", attempt),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.Any("err", err),
		)

		return forwardResult{
			target:   target,
			duration: time.Since(start),
			err:      err,
		}
	}

	if err := drainAndClose(logger, resp.Body); err != nil {
		logger.Warn(
			"failed to drain downstream response body",
			slog.String("target", target.URL),
			slog.String("server_id", target.ServerID),
			slog.Int("attempt", attempt),
			slog.Any("err", err),
		)
	}

	result := forwardResult{
		target:     target,
		statusCode: resp.StatusCode,
		duration:   time.Since(start),
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		logger.Info(
			"pubsub forward succeeded",
			slog.String("target", target.URL),
			slog.String("server_id", target.ServerID),
			slog.String("server_name", target.ServerName),
			slog.Int("attempt", attempt),
			slog.Int("status", resp.StatusCode),
			slog.Int64("duration_ms", result.duration.Milliseconds()),
		)

		return result
	}

	logger.Warn(
		"pubsub forward returned non-2xx",
		slog.String("target", target.URL),
		slog.String("server_id", target.ServerID),
		slog.String("server_name", target.ServerName),
		slog.Int("attempt", attempt),
		slog.Int("status", resp.StatusCode),
		slog.Int64("duration_ms", result.duration.Milliseconds()),
	)

	return result
}

func shouldRetryForward(result forwardResult) bool {
	if result.err != nil {
		return true
	}

	return result.statusCode == http.StatusTooManyRequests ||
		result.statusCode >= http.StatusInternalServerError
}

func buildProxyTarget(srv entities.Server) (proxyTarget, error) {
	rawURL := strings.TrimSpace(srv.ProxyURL)
	if rawURL == "" {
		return proxyTarget{}, errProxyTargetMissing
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return proxyTarget{}, errors.New("proxy url must be an absolute http/https url")
	}

	switch parsed.Scheme {
	case "http", "https":
	default:
		return proxyTarget{}, errors.New("proxy url must use http or https")
	}

	return proxyTarget{
		ServerID:   srv.ID.String(),
		ServerName: srv.Name,
		URL:        parsed.String(),
		AuthToken:  strings.TrimSpace(srv.SecretKey),
	}, nil
}

func forwardedHeaders(src http.Header) http.Header {
	dst := make(http.Header)

	copyHeaderValue(dst, src, "Content-Type")
	copyHeaderValue(dst, src, "User-Agent")

	for key, values := range src {
		if !strings.HasPrefix(strings.ToLower(key), "x-goog-") {
			continue
		}

		dst[key] = append([]string(nil), values...)
	}

	return dst
}

func copyHeaderValue(dst, src http.Header, key string) {
	if values, ok := src[key]; ok {
		dst[key] = append([]string(nil), values...)
	}
}

func isJSONContentType(raw string) bool {
	if raw == "" {
		return false
	}

	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return false
	}

	return mediaType == "application/json"
}

func (p *PubSubProxy) isAuthorized(rawToken string) bool {
	if p.token == "" {
		return true
	}

	return subtle.ConstantTimeCompare([]byte(rawToken), []byte(p.token)) == 1
}

func inboundToken(r *http.Request) string {
	rawAuth := strings.TrimSpace(r.Header.Get("Authorization"))
	if rawAuth != "" {
		parts := strings.Fields(rawAuth)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}

		if len(parts) == 1 {
			return strings.TrimSpace(parts[0])
		}

		return rawAuth
	}

	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func writePlainText(logger *slog.Logger, w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)

	if _, err := io.WriteString(w, body); err != nil {
		logger.Warn("failed to write response", slog.Any("err", err))
	}
}

func drainAndClose(logger *slog.Logger, body io.ReadCloser) error {
	if _, err := io.Copy(io.Discard, body); err != nil {
		closeErr := body.Close()
		if closeErr != nil {
			logger.Warn("failed to close downstream response body", slog.Any("err", closeErr))
		}

		return err
	}

	err := body.Close()
	if err != nil {
		return err
	}

	return nil
}
