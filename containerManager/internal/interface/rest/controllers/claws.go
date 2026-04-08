package controllers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"containermanager/internal/entities"
	"containermanager/internal/infrastucture/sql/storage"
	"containermanager/internal/interface/rest/middleware"
	"containermanager/internal/pkg/logctx"
	"containermanager/internal/service"
	"containermanager/internal/service/commands"
	"shared/pkg/hostingapi"
	"shared/pkg/observability"

	"github.com/go-chi/chi/v5"
)

type Claw struct {
	s                    clawService
	key                  string
	capacity             hostingapi.CapacityResponse
	pubSubClient         *http.Client
	pubSubForwardTimeout time.Duration
	pubSubWorkers        int
	pubSubDedup          *pubSubDeduper
	pubSubMetrics        *observability.PubSubFanoutMetrics
}

type ClawOptions struct {
	PubSubForwardTimeout time.Duration
	PubSubWorkers        int
	PubSubDedupTTL       time.Duration
	MaxClaws             int
	Metrics              *observability.PubSubFanoutMetrics
}

type clawService interface {
	Ensure(ctx context.Context, cm commands.CreateClaw) (entities.Container, error)
	Start(ctx context.Context, cm commands.StartClaw) error
	Approve(ctx context.Context, clawID, userID, channelType, code string) error
	Stop(ctx context.Context, cm commands.StopClaw) error
	Update(ctx context.Context, cm commands.UpdateClaw) error
	Delete(ctx context.Context, cm commands.DeleteClaw) error
	ConfigArchive(ctx context.Context, cm commands.ConfigArchive, w io.Writer) error
	RestoreConfig(ctx context.Context, cm commands.RestoreConfig, r io.Reader) error
	Connect(ctx context.Context, cm commands.ConnectCommand) error
	ListRunning(ctx context.Context) ([]entities.Container, error)
	RuntimeState(ctx context.Context, cm commands.StateClaw) (entities.Container, error)
	RuntimeBinding(ctx context.Context, userID, clawID, bindingName string) (entities.RuntimeBinding, error)
}

func NewClaw(s clawService, key string, opts ClawOptions) *Claw {
	forwardTimeout := opts.PubSubForwardTimeout
	if forwardTimeout <= 0 {
		forwardTimeout = 5 * time.Second
	}

	workers := opts.PubSubWorkers
	if workers <= 0 {
		workers = 32
	}

	dedupTTL := opts.PubSubDedupTTL
	if dedupTTL <= 0 {
		dedupTTL = 10 * time.Minute
	}

	return &Claw{
		s:                    s,
		key:                  key,
		capacity:             hostingapi.CapacityResponse{MaxClaws: opts.MaxClaws},
		pubSubClient:         observability.NewHTTPClient(forwardTimeout),
		pubSubForwardTimeout: forwardTimeout,
		pubSubWorkers:        workers,
		pubSubDedup:          newPubSubDeduper(dedupTTL),
		pubSubMetrics:        opts.Metrics,
	}
}

func (c *Claw) Register(mux chi.Router) {
	mux.Group(func(r chi.Router) {
		r.Use(middleware.Auth(c.key))
		r.Post(hostingapi.ClawsEnsureEndpoint, c.Ensure)
		r.Post(hostingapi.ClawsStartEndpoint, c.Start)
		r.Post(hostingapi.ClawsStopEndpoint, c.Stop)
		r.Post(hostingapi.ClawsDeleteEndpoint, c.Delete)
		r.Get(hostingapi.ClawsStateEndpoint, c.State)
		r.Get(hostingapi.CapacityEndpoint, c.Capacity)
		r.Get("/claws/config", c.ConfigArchive)
		r.Post("/claws/config", c.RestoreConfig)
		r.Get(hostingapi.ApproveEndpoint, c.Approve)
		r.Post(hostingapi.ConnectEndpoint, c.Connect)
		r.Post(hostingapi.GmailPubSubEndpoint, c.GmailPubSub)
	})
}

func (c *Claw) Ensure(w http.ResponseWriter, r *http.Request) {
	var req hostingapi.EnsureRuntimeRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeValidationError(w, err.Error())
		return
	}

	if strings.TrimSpace(req.UserID) == "" {
		writeValidationError(w, "userId is required")
		return
	}

	if strings.TrimSpace(req.ClawID) == "" {
		writeValidationError(w, "clawId is required")
		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", req.UserID),
		slog.String("claw_id", req.ClawID),
	)

	container, err := c.s.Ensure(r.Context(), mapEnsureRuntimeToCommand(req))
	if err != nil {
		log.Error("ensure runtime failed", slog.Any("err", err))
		writeServiceError(w, err)
		return
	}

	resp := hostingapi.EnsureRuntimeResponse{
		RuntimeRecordID:   container.ID.String(),
		DockerContainerID: container.ContainerID,
		Port:              hostingapi.BoundTCPPort(container.Port),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
	log.Info("runtime ensured", slog.String("runtime_record_id", container.ID.String()))
}

func (c *Claw) Capacity(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(c.capacity)
}

func (c *Claw) Start(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeLifecycleCommand(w, r)
	if !ok {
		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("operation_id", req.OperationID),
		slog.String("idempotency_key", req.IdempotencyKey),
		slog.String("user_id", req.UserID),
		slog.String("claw_id", req.ClawID),
	)

	err := c.s.Start(r.Context(), commands.StartClaw{ClawID: req.ClawID, UserID: req.UserID})
	if err != nil {
		log.Error("start claw failed", slog.Any("err", err))
		writeServiceError(w, err)

		return
	}

	w.WriteHeader(http.StatusAccepted)
	log.Info("claw start accepted")
}

func (c *Claw) Approve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get(hostingapi.QueryUserID)
	if q == "" {
		writeValidationError(w, "userId is required")

		return
	}

	clawID := r.URL.Query().Get(hostingapi.QueryClawID)
	if clawID == "" {
		writeValidationError(w, "clawId is required")

		return
	}

	code := r.URL.Query().Get(hostingapi.QueryCode)
	if code == "" {
		writeValidationError(w, "code is required")

		return
	}

	channelType := r.URL.Query().Get(hostingapi.QueryChannelType)
	if channelType == "" {
		writeValidationError(w, "channelType is required")

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
	)

	err := c.s.Approve(r.Context(), clawID, q, channelType, code)
	if err != nil {
		log.Error("approve failed", slog.Any("err", err))
		statusCode, payload := mapApproveError(err)
		hostingapi.WriteError(w, statusCode, payload.Code, payload.Message)

		return
	}

	w.WriteHeader(http.StatusOK)
	log.Info("approve succeeded")
}

func (c *Claw) Stop(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeLifecycleCommand(w, r)
	if !ok {
		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("operation_id", req.OperationID),
		slog.String("idempotency_key", req.IdempotencyKey),
		slog.String("user_id", req.UserID),
		slog.String("claw_id", req.ClawID),
	)

	err := c.s.Stop(r.Context(), commands.StopClaw{ClawID: req.ClawID, UserID: req.UserID})
	if err != nil {
		log.Error("stop claw failed", slog.Any("err", err))
		writeServiceError(w, err)

		return
	}

	w.WriteHeader(http.StatusAccepted)
	log.Info("claw stop accepted")
}

func (c *Claw) Delete(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeLifecycleCommand(w, r)
	if !ok {
		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("operation_id", req.OperationID),
		slog.String("idempotency_key", req.IdempotencyKey),
		slog.String("user_id", req.UserID),
		slog.String("claw_id", req.ClawID),
	)

	err := c.s.Delete(r.Context(), commands.DeleteClaw{
		ClawID:       req.ClawID,
		UserID:       req.UserID,
		DeleteConfig: true,
	})
	if err != nil {
		log.Error("delete claw failed", slog.Any("err", err))
		writeServiceError(w, err)

		return
	}

	w.WriteHeader(http.StatusAccepted)
	log.Info("claw delete accepted")
}

func (c *Claw) State(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get(hostingapi.QueryUserID))
	if userID == "" {
		writeValidationError(w, "userId is required")

		return
	}

	clawID := strings.TrimSpace(r.URL.Query().Get(hostingapi.QueryClawID))
	if clawID == "" {
		writeValidationError(w, "clawId is required")

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", userID),
		slog.String("claw_id", clawID),
	)

	container, err := c.s.RuntimeState(r.Context(), commands.StateClaw{UserID: userID, ClawID: clawID})
	if err != nil {
		log.Error("runtime state lookup failed", slog.Any("err", err))

		if errors.Is(err, storage.ErrNotFound) {
			hostingapi.WriteError(w, http.StatusNotFound, hostingapi.ErrCodeUnknownState, "runtime state is unknown")

			return
		}

		writeInternalError(w, err)

		return
	}

	response := hostingapi.RuntimeStateResponse{
		RuntimeRecordID:   container.ID.String(),
		DockerContainerID: container.ContainerID,
		ObservedState:     hostingapi.RuntimeObservedState(container.Status),
		RuntimeStatus:     hostingapi.RuntimeExecutionStatus(container.Status),
		Port:              hostingapi.BoundTCPPort(container.Port),
	}

	if container.Status == "" {
		response.ObservedState = hostingapi.RuntimeObservedState("unknown")
		response.RuntimeStatus = hostingapi.RuntimeExecutionStatus("unknown")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
	log.Info("runtime state returned")
}

func (c *Claw) ConfigArchive(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get(hostingapi.QueryUserID)
	if q == "" {
		writeValidationError(w, "userId is required")

		return
	}

	clawID := r.URL.Query().Get(hostingapi.QueryClawID)
	if clawID == "" {
		writeValidationError(w, "clawId is required")

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
	)

	deleteAfter := false

	if raw := r.URL.Query().Get("deleteAfter"); raw != "" {
		val, err := strconv.ParseBool(raw)
		if err != nil {
			writeValidationError(w, "deleteAfter must be a boolean")

			return
		}

		deleteAfter = val
	}

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().
		Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"claw-config-%s-%s.tar\"", q, clawID))

	err := c.s.ConfigArchive(
		r.Context(),
		commands.ConfigArchive{UserID: q, ClawID: clawID, DeleteAfter: deleteAfter},
		w,
	)
	if err != nil {
		log.Error(
			"config archive failed",
			slog.Any("err", err),
			slog.Bool("delete_after", deleteAfter),
		)

		if errors.Is(err, os.ErrNotExist) {
			hostingapi.WriteError(w, http.StatusNotFound, hostingapi.ErrCodeNotFound, err.Error())

			return
		}

		writeInternalError(w, err)

		return
	}

	log.Info("config archived", slog.Bool("delete_after", deleteAfter))
}

func (c *Claw) RestoreConfig(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get(hostingapi.QueryUserID)
	if q == "" {
		writeValidationError(w, "userId is required")

		return
	}

	clawID := r.URL.Query().Get(hostingapi.QueryClawID)
	if clawID == "" {
		writeValidationError(w, "clawId is required")

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
	)

	err := c.s.RestoreConfig(r.Context(), commands.RestoreConfig{UserID: q, ClawID: clawID}, r.Body)
	if err != nil {
		log.Error("restore config failed", slog.Any("err", err))
		writeInternalError(w, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.Info("config restored")
}

func (c *Claw) Connect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get(hostingapi.QueryUserID)
	if q == "" {
		writeValidationError(w, "userId is required")

		return
	}

	clawID := r.URL.Query().Get(hostingapi.QueryClawID)
	if clawID == "" {
		writeValidationError(w, "clawId is required")

		return
	}

	provider := r.URL.Query().Get(hostingapi.QueryProvider)
	if provider == "" {
		writeValidationError(w, "provider is required")

		return
	}

	normalizedProvider, err := hostingapi.NormalizeProvider(provider)
	if err != nil {
		writeValidationError(w, err.Error())

		return
	}

	token, err := io.ReadAll(r.Body)
	if err != nil {
		writeValidationError(w, "failed to read token")

		return
	}

	if len(bytes.TrimSpace(token)) == 0 {
		writeValidationError(w, "token is required")

		return
	}

	log := logctx.Logger(r.Context()).With(
		slog.String("user_id", q),
		slog.String("claw_id", clawID),
	)

	err = c.s.Connect(
		r.Context(),
		commands.ConnectCommand{
			ClawID:   clawID,
			UserID:   q,
			Token:    token,
			Provider: normalizedProvider,
		},
	)
	if err != nil {
		log.Error("connect failed", slog.Any("err", err))
		writeInternalError(w, err)

		return
	}

	log.Info("claw connected", slog.String("provider", normalizedProvider))
	w.WriteHeader(http.StatusNoContent)
}

func (c *Claw) GmailPubSub(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	targetCount := 0
	successCount := 0

	var flowErr error

	ctxWithComponent := observability.WithComponent(r.Context(), "controller.gmail_pubsub")
	ctx, log, finishFlow := observability.StartFlow(
		ctxWithComponent,
		logctx.Logger(ctxWithComponent),
		"gmail_pubsub_fanout",
		"pubsub.forward",
	)

	defer func() {
		c.pubSubMetrics.Observe(
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

		hostingapi.WriteError(
			w,
			http.StatusUnsupportedMediaType,
			hostingapi.ErrCodeValidation,
			"content type must be application/json",
		)

		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		flowErr = observability.DecorateError(
			fmt.Errorf("read request body: %w", err),
			observability.ErrorAttrs{
				Result: observability.ResultValidationError,
				Kind:   observability.ErrorKindValidation,
				Source: observability.ErrorSourceHTTP,
			},
		)

		writeValidationError(w, "failed to read request body")

		return
	}

	if len(bytes.TrimSpace(body)) == 0 {
		flowErr = observability.DecorateError(
			errors.New("empty request body"),
			observability.ErrorAttrs{
				Result: observability.ResultValidationError,
				Kind:   observability.ErrorKindValidation,
				Source: observability.ErrorSourceHTTP,
			},
		)

		writeValidationError(w, "request body is required")

		return
	}

	msgKey := pubSubMessageKey(body)
	if !c.pubSubDedup.Add(msgKey) {
		log.Info("gmail pubsub message skipped as duplicate", slog.String("message_key", msgKey))
		w.WriteHeader(http.StatusNoContent)

		return
	}

	containers, err := c.s.ListRunning(r.Context())
	if err != nil {
		log.Error("failed to list running containers", slog.Any("err", err))
		flowErr = fmt.Errorf("list running containers: %w", err)

		hostingapi.WriteError(
			w,
			http.StatusServiceUnavailable,
			hostingapi.ErrCodeInternal,
			"failed to list running containers",
		)

		return
	}

	if len(containers) == 0 {
		log.Info("gmail pubsub received with no running containers")
		w.WriteHeader(http.StatusNoContent)

		return
	}

	targetCount = len(containers)

	headers := forwardPubSubHeaders(r.Header)
	results := c.forwardPubSubToContainers(r.Context(), containers, body, headers)

	var okCount int

	for _, res := range results {
		if res.err == nil && res.statusCode >= http.StatusOK &&
			res.statusCode < http.StatusMultipleChoices {
			okCount++

			continue
		}

		if res.err != nil {
			log.Warn(
				"gmail pubsub forward failed",
				slog.String("container_id", res.containerID),
				slog.String("container_port", res.port),
				slog.Int64("duration_ms", res.duration.Milliseconds()),
				slog.Any("err", res.err),
			)

			continue
		}

		log.Warn(
			"gmail pubsub forward returned non-2xx",
			slog.String("container_id", res.containerID),
			slog.String("container_port", res.port),
			slog.Int("status", res.statusCode),
			slog.Int64("duration_ms", res.duration.Milliseconds()),
		)
	}

	log.Info(
		"gmail pubsub fanout completed",
		slog.String("message_key", msgKey),
		slog.Int("targets", len(containers)),
		slog.Int("success", okCount),
	)
	successCount = okCount

	if okCount == 0 {
		flowErr = observability.DecorateError(
			errors.New("all downstream requests failed"),
			observability.ErrorAttrs{
				Result: observability.ResultError,
				Kind:   observability.ErrorKindUnexpected,
				Source: observability.ErrorSourceExternal,
			},
		)

		hostingapi.WriteError(
			w,
			http.StatusBadGateway,
			hostingapi.ErrCodeInternal,
			"all downstream requests failed",
		)

		return
	}

	if okCount < len(containers) {
		flowErr = observability.DecorateError(
			errors.New("partial downstream failure"),
			observability.ErrorAttrs{
				Result: observability.ResultPartialSuccess,
				Kind:   observability.ErrorKindUnexpected,
				Source: observability.ErrorSourceExternal,
			},
		)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *Claw) forwardPubSubToContainers(
	ctx context.Context,
	containers []entities.Container,
	body []byte,
	headers http.Header,
) []pubSubForwardResult {
	results := make(chan pubSubForwardResult, len(containers))
	sem := make(chan struct{}, c.pubSubWorkers)

	var wg sync.WaitGroup

	for _, container := range containers {

		wg.Add(1)

		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			results <- c.forwardPubSubToContainer(ctx, container, body, headers)
		}()
	}

	wg.Wait()
	close(results)

	out := make([]pubSubForwardResult, 0, len(containers))

	for result := range results {
		out = append(out, result)
	}

	return out
}

func (c *Claw) forwardPubSubToContainer(
	ctx context.Context,
	container entities.Container,
	body []byte,
	headers http.Header,
) pubSubForwardResult {
	start := time.Now()

	binding, err := c.s.RuntimeBinding(ctx, container.UserID, container.ClawID, "gmail-pubsub")
	if err != nil {
		return pubSubForwardResult{
			containerID: container.ID.String(),
			port:        container.Port,
			duration:    time.Since(start),
			err:         err,
		}
	}

	watchPort, err := service.GogWatchPortFromGateway(container.Port)
	if err != nil {
		return pubSubForwardResult{
			containerID: container.ID.String(),
			port:        container.Port,
			duration:    time.Since(start),
			err:         err,
		}
	}

	path := strings.TrimSpace(binding.InternalPath)
	if path == "" {
		path = "/gmail-pubsub"
	}

	targetURL := fmt.Sprintf("http://127.0.0.1:%s%s", watchPort, path)

	reqCtx, cancel := context.WithTimeout(ctx, c.pubSubForwardTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodPost,
		targetURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return pubSubForwardResult{
			containerID: container.ID.String(),
			port:        watchPort,
			duration:    time.Since(start),
			err:         err,
		}
	}

	req.Header = headers.Clone()

	resp, err := c.pubSubClient.Do(req)
	if err != nil {
		return pubSubForwardResult{
			containerID: container.ID.String(),
			port:        watchPort,
			duration:    time.Since(start),
			err:         err,
		}
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	return pubSubForwardResult{
		containerID: container.ID.String(),
		port:        watchPort,
		duration:    time.Since(start),
		statusCode:  resp.StatusCode,
	}
}

type pubSubForwardResult struct {
	containerID string
	port        string
	statusCode  int
	duration    time.Duration
	err         error
}

type pubSubDeduper struct {
	ttl   time.Duration
	mu    sync.Mutex
	items map[string]time.Time
}

func newPubSubDeduper(ttl time.Duration) *pubSubDeduper {
	return &pubSubDeduper{
		ttl:   ttl,
		items: make(map[string]time.Time),
	}
}

func (d *pubSubDeduper) Add(key string) bool {
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()

	for k, expiresAt := range d.items {
		if !expiresAt.After(now) {
			delete(d.items, k)
		}
	}

	if expiresAt, ok := d.items[key]; ok && expiresAt.After(now) {
		return false
	}

	d.items[key] = now.Add(d.ttl)

	return true
}

type pubSubEnvelope struct {
	Message struct {
		MessageID    string `json:"messageId"`
		MessageIDAlt string `json:"message_id"`
	} `json:"message"`
}

func pubSubMessageKey(body []byte) string {
	var envelope pubSubEnvelope
	err := json.Unmarshal(body, &envelope)
	if err == nil {
		key := strings.TrimSpace(envelope.Message.MessageID)
		if key == "" {
			key = strings.TrimSpace(envelope.Message.MessageIDAlt)
		}

		if key != "" {
			return "msg:" + key
		}
	}

	sum := sha256.Sum256(body)

	return "sha256:" + fmt.Sprintf("%x", sum[:])
}

func forwardPubSubHeaders(src http.Header) http.Header {
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
	if strings.TrimSpace(raw) == "" {
		return false
	}

	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return false
	}

	return mediaType == "application/json"
}

func decodeLifecycleCommand(w http.ResponseWriter, r *http.Request) (hostingapi.LifecycleCommandRequest, bool) {
	var req hostingapi.LifecycleCommandRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeValidationError(w, "invalid request body")

		return hostingapi.LifecycleCommandRequest{}, false
	}

	req.OperationID = strings.TrimSpace(req.OperationID)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.UserID = strings.TrimSpace(req.UserID)
	req.ClawID = strings.TrimSpace(req.ClawID)

	if req.UserID == "" {
		writeValidationError(w, "userId is required")

		return hostingapi.LifecycleCommandRequest{}, false
	}

	if req.ClawID == "" {
		writeValidationError(w, "clawId is required")

		return hostingapi.LifecycleCommandRequest{}, false
	}

	return req, true
}

func writeValidationError(w http.ResponseWriter, message string) {
	hostingapi.WriteError(w, http.StatusBadRequest, hostingapi.ErrCodeValidation, message)
}

func writeInternalError(w http.ResponseWriter, err error) {
	hostingapi.WriteError(
		w,
		http.StatusInternalServerError,
		hostingapi.ErrCodeInternal,
		err.Error(),
	)
}

func mapEnsureRuntimeToCommand(req hostingapi.EnsureRuntimeRequest) commands.CreateClaw {
	cfg := make([]entities.ClawConfig, 0, len(req.ClawConfig))
	for _, file := range req.ClawConfig {
		cfg = append(cfg, entities.ClawConfig{
			Name:     file.Name,
			FileType: file.FileType,
			Data:     []byte(file.Data),
		})
	}

	return commands.CreateClaw{
		UserID: req.UserID,
		ClawID: req.ClawID,
		Vars:   append([]string(nil), req.Vars...),
		Config: cfg,
	}
}

func writeServiceError(w http.ResponseWriter, err error) {
	statusCode, payload := mapServiceError(err)
	hostingapi.WriteError(w, statusCode, payload.Code, payload.Message)
}

func mapServiceError(err error) (int, hostingapi.ErrorResponse) {
	if errors.Is(err, service.ErrServerCapacityExceeded) {
		return http.StatusConflict, hostingapi.ErrorResponse{
			Code:    hostingapi.ErrCodeServerCapacityExceeded,
			Message: service.ErrServerCapacityExceeded.Error(),
		}
	}

	if errors.Is(err, service.ErrServerMemoryUnavailable) {
		return http.StatusConflict, hostingapi.ErrorResponse{
			Code:    hostingapi.ErrCodeServerMemoryUnavailable,
			Message: service.ErrServerMemoryUnavailable.Error(),
		}
	}

	return http.StatusInternalServerError, hostingapi.ErrorResponse{
		Code:    hostingapi.ErrCodeInternal,
		Message: err.Error(),
	}
}
