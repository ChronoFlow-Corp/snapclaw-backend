package hosting

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	"shared/pkg/hostingapi"
	"shared/pkg/observability"
	"simpleClaw/internal/entities"

	"github.com/google/uuid"
)

const (
	openRouterAPIKeyVar     = "OPENROUTER_API_KEY"
	openClawGatewayTokenVar = "OPENCLAW_GATEWAY_TOKEN"
)

// Manager orchestrates container lifecycle interactions with containerManager over HTTP.
type Manager struct {
	client  *client
	metrics *observability.OperationMetrics
	secrets RuntimeSecrets
}

func NewManager(metrics ...*observability.OperationMetrics) *Manager {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Manager{
		client:  newClient(defaultHTTPTimeout),
		metrics: opMetrics,
	}
}

func (m *Manager) WithRuntimeSecrets(secrets RuntimeSecrets) *Manager {
	m.secrets = secrets

	return m
}

func (m *Manager) Create(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) (Container, error) {
	const op = "infra.hosting.Manager.Create"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.hosting", "hosting.create", "claw_lifecycle")

	var err error

	defer func() { finish(err) }()

	if m == nil || m.client == nil {
		return Container{}, fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return Container{}, fmt.Errorf("%s: user id is required", op)
	}

	if server.URL == "" {
		return Container{}, fmt.Errorf("%s: server url is required", op)
	}

	configFiles, err := buildConfigFiles(cl.Config)
	if err != nil {
		return Container{}, fmt.Errorf("%s: %w", op, err)
	}

	vars := buildVars(cl.Config, m.secrets)

	resp, err := m.client.ensureRuntime(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
		hostingapi.EnsureRuntimeRequest{
			UserID:     cl.UserID.String(),
			ClawID:     cl.ID.String(),
			Vars:       vars,
			ClawConfig: configFiles,
		},
	)
	if err != nil {
		return Container{}, fmt.Errorf("%s: %w", op, err)
	}

	if resp.RuntimeRecordID == "" {
		return Container{}, fmt.Errorf("%s: empty runtime record id received", op)
	}

	container := Container{
		ID: resp.RuntimeRecordID,
	}

	if server.ID != uuid.Nil {
		container.ServerID = server.ID
	} else if cl.ServerID != uuid.Nil {
		container.ServerID = cl.ServerID
	}

	return container, nil
}

func (m *Manager) Capacity(ctx context.Context, server entities.Server) (int, error) {
	const op = "infra.hosting.Manager.Capacity"

	if m == nil || m.client == nil {
		return 0, fmt.Errorf("%s: http client is not configured", op)
	}

	if server.URL == "" {
		return 0, fmt.Errorf("%s: server url is required", op)
	}

	resp, err := m.client.capacity(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
	)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	if resp.MaxClaws <= 0 {
		return 0, fmt.Errorf("%s: invalid max claws %d", op, resp.MaxClaws)
	}

	return resp.MaxClaws, nil
}

func (m *Manager) Delete(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
	deleteConfig bool,
) (err error) {
	const op = "infra.hosting.Manager.Delete"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.hosting", "hosting.delete", "claw_lifecycle")

	defer func() { finish(err) }()

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cl.ContainerID == "" {
		return nil
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	_ = m.Stop(ctx, cl, server)

	if err := m.client.lifecycleCommand(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
		hostingapi.ClawsDeleteEndpoint,
		newLifecycleCommandRequest(cl.UserID.String(), cl.ID.String(), "delete"),
	); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Start(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) (err error) {
	const op = "infra.hosting.Manager.Start"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.hosting", "hosting.start", "claw_lifecycle")

	defer func() { finish(err) }()

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cl.ContainerID == "" {
		return nil
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	if err := m.client.startClaw(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
		cl.UserID.String(),
		cl.ID.String(),
	); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Stop(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) (err error) {
	const op = "infra.hosting.Manager.Stop"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.hosting", "hosting.stop", "claw_lifecycle")

	defer func() { finish(err) }()

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cl.ContainerID == "" {
		return nil
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	if err := m.client.stopClaw(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
		cl.UserID.String(),
		cl.ID.String(),
	); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) State(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
) (RuntimeState, error) {
	const op = "infra.hosting.Manager.State"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.hosting", "hosting.state", "claw_lifecycle")

	var err error
	defer func() { finish(err) }()

	if m == nil || m.client == nil {
		return RuntimeState{}, fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return RuntimeState{}, fmt.Errorf("%s: user id is required", op)
	}

	if server.URL == "" {
		return RuntimeState{}, fmt.Errorf("%s: server url is required", op)
	}

	resp, err := m.client.runtimeState(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
		cl.UserID.String(),
		cl.ID.String(),
	)
	if err != nil {
		return RuntimeState{}, fmt.Errorf("%s: %w", op, err)
	}

	return RuntimeState{
		RuntimeRecordID:   resp.RuntimeRecordID,
		DockerContainerID: resp.DockerContainerID,
		ObservedState:     string(resp.ObservedState),
		RuntimeStatus:     string(resp.RuntimeStatus),
		Port:              string(resp.Port),
		LastError:         resp.LastError,
	}, nil
}

func (m *Manager) ConfigArchive(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
	deleteAfter bool,
) (io.ReadCloser, error) {
	const op = "infra.hosting.Manager.ConfigArchive"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.hosting", "hosting.config_archive", "claw_lifecycle")

	var err error

	defer func() { finish(err) }()

	if m == nil || m.client == nil {
		return nil, fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return nil, fmt.Errorf("%s: user id is required", op)
	}

	if server.URL == "" {
		return nil, fmt.Errorf("%s: server url is required", op)
	}

	body, err := m.client.configArchive(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
		cl.UserID.String(),
		cl.ID.String(),
		deleteAfter,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return body, nil
}

func (m *Manager) RestoreConfigArchive(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
	body io.Reader,
) (err error) {
	const op = "infra.hosting.Manager.RestoreConfigArchive"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.hosting", "hosting.config_restore", "claw_lifecycle")

	defer func() { finish(err) }()

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	if err := m.client.restoreConfigArchive(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
		cl.UserID.String(),
		cl.ID.String(),
		body,
	); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) ApprovePairing(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
	channelType string,
	code string,
) error {
	const op = "infra.hosting.Manager.ApprovePairing"

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cl.ID == uuid.Nil {
		return fmt.Errorf("%s: claw id is required", op)
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	if code == "" {
		return fmt.Errorf("%s: code is required", op)
	}

	if channelType == "" {
		return fmt.Errorf("%s: channel type is required", op)
	}

	err := m.client.approvePairing(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
		cl.UserID.String(),
		cl.ID.String(),
		channelType,
		code,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Connect(
	ctx context.Context,
	cl entities.Claw,
	server entities.Server,
	provider string,
	token []byte,
) error {
	const op = "infra.hosting.Manager.Connect"

	if m == nil || m.client == nil {
		return fmt.Errorf("%s: http client is not configured", op)
	}

	if cl.UserID == uuid.Nil {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cl.ID == uuid.Nil {
		return fmt.Errorf("%s: claw id is required", op)
	}

	if cl.ContainerID == "" {
		return nil
	}

	if server.URL == "" {
		return fmt.Errorf("%s: server url is required", op)
	}

	if provider == "" {
		return fmt.Errorf("%s: provider is required", op)
	}

	if len(token) == 0 {
		return fmt.Errorf("%s: token is required", op)
	}

	err := m.client.connect(
		ctx,
		server.URL,
		map[string]string{"Authorization": server.SecretKey},
		cl.UserID.String(),
		cl.ID.String(),
		provider,
		token,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func buildConfigFiles(cfg entities.ClawConfig) ([]hostingapi.ClawConfigFile, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal openclaw config: %w", err)
	}

	files := []hostingapi.ClawConfigFile{
		{
			Name:     openClawConfigName,
			FileType: fileTypeJSON,
			Data:     string(data),
		},
	}

	manifest := buildRuntimeBindingManifest(cfg)
	if len(manifest.Bindings) > 0 {
		bindingData, err := json.Marshal(manifest)
		if err != nil {
			return nil, fmt.Errorf("marshal runtime bindings: %w", err)
		}

		files = append(files, hostingapi.ClawConfigFile{
			Name:     runtimeBindingsName,
			FileType: fileTypeJSON,
			Data:     string(bindingData),
		})
	}

	return files, nil
}

func buildVars(cfg entities.ClawConfig, secrets RuntimeSecrets) []string {
	vars := make(map[string]string, len(cfg.Env.Vars))
	for key, value := range cfg.Env.Vars {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		if key == "BRAVE_API_KEY" && isVarRef(value) {
			if secret := strings.TrimSpace(secrets.BraveAPIKey); secret != "" {
				value = secret
			}
		}

		vars[key] = value
	}

	openRouterKey := strings.TrimSpace(cfg.Env.OpenRouterAPIKey)
	if openRouterKey != "" && !isVarRef(openRouterKey) {
		if _, ok := vars[openRouterAPIKeyVar]; !ok {
			vars[openRouterAPIKeyVar] = openRouterKey
		}
	}

	gatewayToken := strings.TrimSpace(cfg.Gateway.Auth.Token)
	if gatewayToken != "" && !isVarRef(gatewayToken) {
		if _, ok := vars[openClawGatewayTokenVar]; !ok {
			vars[openClawGatewayTokenVar] = gatewayToken
		}
	}

	if len(vars) == 0 {
		return nil
	}

	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, fmt.Sprintf("%s=%s", key, vars[key]))
	}

	return result
}

func isVarRef(value string) bool {
	value = strings.TrimSpace(value)

	return strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}")
}
