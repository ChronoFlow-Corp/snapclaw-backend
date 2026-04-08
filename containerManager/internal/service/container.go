package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"shared/pkg/hostingapi"
	"shared/pkg/observability"
	"strings"
	"sync"
	"time"

	"containermanager/internal/entities"
	"containermanager/internal/infrastucture/pkg/configurer"
	"containermanager/internal/infrastucture/pkg/docker"
	"containermanager/internal/infrastucture/sql/storage"
	"containermanager/internal/service/commands"

	"github.com/google/uuid"
)

type Container struct {
	clRepo              ClawRepository
	manager             containerRuntime
	p                   *Porter
	cfg                 *configurer.ClawConfigurer
	gog                 GogConfig
	maxClaws            int
	startMu             sync.Mutex
	readAvailableMemory func() (uint64, error)
	coldStartMinBytes   uint64
	warmStartMinBytes   uint64
	metrics             *observability.OperationMetrics
}

type GogConfig struct {
	KeyringBackend  string
	KeyringPassword string
}

var errForbidden = errors.New("container belongs to another user")

var (
	ErrInvalidCode               = errors.New("invalid code")
	ErrApproveChannelUnsupported = errors.New("approve channel is not supported")
	ErrServerCapacityExceeded    = errors.New("server capacity exceeded")
	ErrServerMemoryUnavailable   = errors.New("server memory unavailable")
)

const (
	coldStartMinAvailableBytes = 800 * 1024 * 1024
	warmStartMinAvailableBytes = 800 * 1024 * 1024
)

func NewContainer(
	cfg *configurer.ClawConfigurer,
	clRepo ClawRepository,
	manager containerRuntime,
	maxClaws int,
	gog GogConfig,
	metrics ...*observability.OperationMetrics,
) (*Container, error) {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	c := &Container{
		cfg:                 cfg,
		clRepo:              clRepo,
		manager:             manager,
		p:                   NewPorter(),
		maxClaws:            maxClaws,
		readAvailableMemory: readAvailableMemoryBytes,
		coldStartMinBytes:   coldStartMinAvailableBytes,
		warmStartMinBytes:   warmStartMinAvailableBytes,
		metrics:             opMetrics,
		gog: GogConfig{
			KeyringBackend:  strings.TrimSpace(gog.KeyringBackend),
			KeyringPassword: strings.TrimSpace(gog.KeyringPassword),
		},
	}

	if c.gog.KeyringBackend == "" {
		c.gog.KeyringBackend = "file"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := c.onStart(ctx)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Container) MaxClaws() int {
	if c == nil {
		return 0
	}

	return c.maxClaws
}

func (c *Container) ensureMemoryAvailable(requiredBytes uint64) error {
	if requiredBytes == 0 {
		return nil
	}

	if c.readAvailableMemory == nil {
		return fmt.Errorf("%w: memory reader is not configured", ErrServerMemoryUnavailable)
	}

	availableBytes, err := c.readAvailableMemory()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrServerMemoryUnavailable, err)
	}

	if availableBytes < requiredBytes {
		return fmt.Errorf(
			"%w: available=%d required=%d",
			ErrServerMemoryUnavailable,
			availableBytes,
			requiredBytes,
		)
	}

	return nil
}

func (c *Container) Start(ctx context.Context, cm commands.StartClaw) (err error) {
	const op = "service.Container.Start"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		c.metrics,
		"service.container",
		"claw.start",
		"claw_lifecycle",
	)

	defer func() { finish(err) }()

	if cm.UserID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	c.startMu.Lock()
	defer c.startMu.Unlock()

	cont, err := c.clRepo.GetByUserClawID(ctx, cm.UserID, cm.ClawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cont.UserID != cm.UserID {
		return fmt.Errorf("%s: %w", op, errForbidden)
	}

	if cont.Status == entities.ContainerStatusRunning {
		slog.Default().Info("runtime already running",
			slog.String("user_id", cm.UserID),
			slog.String("claw_id", cm.ClawID),
			slog.String("runtime_record_id", cont.ID.String()),
			slog.String("docker_container_id", cont.ContainerID),
		)
		return nil
	}

	requiredMemory := c.coldStartMinBytes

	if cont.HasStartedOnce {
		requiredMemory = c.warmStartMinBytes
	}

	if err := c.ensureMemoryAvailable(requiredMemory); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	err = c.manager.Start(ctx, cont.ContainerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	cont.Status = entities.ContainerStatusRunning
	cont.HasStartedOnce = true

	err = c.clRepo.Update(ctx, cont)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	slog.Default().Info("runtime started",
		slog.String("user_id", cm.UserID),
		slog.String("claw_id", cm.ClawID),
		slog.String("runtime_record_id", cont.ID.String()),
		slog.String("docker_container_id", cont.ContainerID),
		slog.String("port", cont.Port),
	)

	return nil
}

func (c *Container) Stop(ctx context.Context, cm commands.StopClaw) (err error) {
	const op = "service.Container.Stop"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		c.metrics,
		"service.container",
		"claw.stop",
		"claw_lifecycle",
	)

	defer func() { finish(err) }()

	if cm.UserID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	cDb, err := c.clRepo.GetByUserClawID(ctx, cm.UserID, cm.ClawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cDb.UserID != cm.UserID {
		return fmt.Errorf("%s: %w", op, errForbidden)
	}

	if cDb.Status == entities.ContainerStatusStop {
		slog.Default().Info("runtime already stopped",
			slog.String("user_id", cm.UserID),
			slog.String("claw_id", cm.ClawID),
			slog.String("runtime_record_id", cDb.ID.String()),
			slog.String("docker_container_id", cDb.ContainerID),
		)
		return nil
	}

	err = c.manager.Stop(ctx, cDb.ContainerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	cDb.Status = entities.ContainerStatusStop

	err = c.clRepo.Update(ctx, cDb)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	slog.Default().Info("runtime stopped",
		slog.String("user_id", cm.UserID),
		slog.String("claw_id", cm.ClawID),
		slog.String("runtime_record_id", cDb.ID.String()),
		slog.String("docker_container_id", cDb.ContainerID),
	)

	return nil
}

func (c *Container) Update(ctx context.Context, cm commands.UpdateClaw) (err error) {
	const op = "service.Container.Update"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		c.metrics,
		"service.container",
		"claw.update",
		"claw_lifecycle",
	)

	defer func() { finish(err) }()

	if cm.UserID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	cDb, err := c.clRepo.GetByUserClawID(ctx, cm.UserID, cm.ClawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cDb.UserID != cm.UserID {
		return fmt.Errorf("%s: %w", op, errForbidden)
	}

	if _, err := c.cfg.Update(cm); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *Container) Restart() error {
	return nil
}

func (c *Container) Delete(ctx context.Context, cm commands.DeleteClaw) (err error) {
	const op = "service.Container.Delete"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		c.metrics,
		"service.container",
		"claw.delete",
		"claw_lifecycle",
	)

	defer func() { finish(err) }()

	if cm.UserID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	cDb, err := c.clRepo.GetByUserClawID(ctx, cm.UserID, cm.ClawID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			slog.Default().Info("runtime delete skipped because runtime is missing",
				slog.String("user_id", cm.UserID),
				slog.String("claw_id", cm.ClawID),
			)
			return nil
		}

		return fmt.Errorf("%s: %w", op, err)
	}

	if cDb.UserID != cm.UserID {
		return fmt.Errorf("%s: %w", op, errForbidden)
	}

	err = c.manager.Remove(ctx, cDb.ContainerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	err = c.clRepo.Remove(ctx, cDb)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	c.p.Release(cDb.Port)

	if cm.DeleteConfig {
		if c.cfg == nil {
			return fmt.Errorf("%s: configurer is not configured", op)
		}

		err := c.cfg.DeleteClawConfig(cm.UserID, cm.ClawID)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	slog.Default().Info("runtime deleted",
		slog.String("user_id", cm.UserID),
		slog.String("claw_id", cm.ClawID),
		slog.String("runtime_record_id", cDb.ID.String()),
		slog.String("docker_container_id", cDb.ContainerID),
		slog.Bool("delete_config", cm.DeleteConfig),
	)

	return nil
}

func (c *Container) RuntimeState(
	ctx context.Context,
	cm commands.StateClaw,
) (entities.Container, error) {
	const op = "service.Container.RuntimeState"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		c.metrics,
		"service.container",
		"claw.state",
		"claw_lifecycle",
	)

	var err error
	defer func() { finish(err) }()

	if cm.UserID == "" {
		return entities.Container{}, fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return entities.Container{}, fmt.Errorf("%s: claw id is required", op)
	}

	cl, err := c.clRepo.GetByUserClawID(ctx, cm.UserID, cm.ClawID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return entities.Container{}, fmt.Errorf("%s: %w", op, storage.ErrNotFound)
		}

		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	if cl.UserID != cm.UserID {
		return entities.Container{}, fmt.Errorf("%s: %w", op, errForbidden)
	}

	return cl, nil
}

func (c *Container) Create(ctx context.Context, cm commands.CreateClaw) (string, error) {
	created, err := c.ensureRuntime(ctx, cm, false)
	if err != nil {
		return "", err
	}

	return created.ID.String(), nil
}

func (c *Container) Ensure(
	ctx context.Context,
	cm commands.CreateClaw,
) (entities.Container, error) {
	return c.ensureRuntime(ctx, cm, true)
}

func (c *Container) ensureRuntime(
	ctx context.Context,
	cm commands.CreateClaw,
	allowExisting bool,
) (entities.Container, error) {
	const op = "service.Container.CreateClaw"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		c.metrics,
		"service.container",
		"claw.create",
		"claw_lifecycle",
	)

	var err error

	defer func() { finish(err) }()

	if cm.UserID == "" {
		return entities.Container{}, fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return entities.Container{}, fmt.Errorf("%s: claw id is required", op)
	}

	cDb, err := c.clRepo.GetByUserClawID(ctx, cm.UserID, cm.ClawID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	if uuid.Nil != cDb.ID {
		if allowExisting {
			slog.Default().Info("runtime already exists",
				slog.String("user_id", cm.UserID),
				slog.String("claw_id", cm.ClawID),
				slog.String("runtime_record_id", cDb.ID.String()),
				slog.String("docker_container_id", cDb.ContainerID),
				slog.String("port", cDb.Port),
			)
			return cDb, nil
		}

		return entities.Container{}, fmt.Errorf("%s: %w", op, errors.New("claw already exists"))
	}

	if c.maxClaws > 0 {
		containers, err := c.clRepo.GetAll(ctx)
		if err != nil {
			return entities.Container{}, fmt.Errorf("%s: %w", op, err)
		}

		if len(containers) >= c.maxClaws {
			return entities.Container{}, fmt.Errorf("%s: %w", op, ErrServerCapacityExceeded)
		}
	}

	cfgPath, err := c.cfg.Configure(cm)
	if err != nil {
		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	cPort, err := c.p.Acquire()
	if err != nil {
		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	manifest, err := c.cfg.RuntimeBindings(cm.UserID, cm.ClawID)
	if err != nil {
		c.p.Release(cPort)

		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	gogWatchPort := ""
	if binding, ok := findRuntimeBinding(manifest, "gmail-pubsub"); ok && binding.RequiresSecondaryPort {
		gogWatchPort, err = GogWatchPortFromGateway(cPort)
		if err != nil {
			c.p.Release(cPort)

			return entities.Container{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	releasePort := func() {
		if cPort != "" {
			c.p.Release(cPort)
			cPort = ""
		}
	}

	cID, err := c.manager.Create(ctx, docker.CreateOptions{
		HostPort:               cPort,
		HostIP:                 "127.0.0.1",
		ContainerPort:          cPort,
		SecondaryHostPort:      gogWatchPort,
		SecondaryContainerPort: gogWatchPort,
		Volumes:                c.containerVolumes(cfgPath),
		Env: append([]string{
			"OPENCLAW_HOME=/app/",
			"NODE_ENV=production",
			"OPENCLAW_GATEWAY_BIND=loopback",
			"OPENCLAW_SKIP_CANVAS_HOST=1",
			fmt.Sprintf("OPENCLAW_GATEWAY_PORT=%s", cPort),
			"OPENCLAW_CONFIG_PATH=/app/openclaw.json",
			"NODE_OPTIONS=--max-old-space-size=3072",
		}, cm.Vars...),
	})
	if err != nil {
		releasePort()

		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	created := entities.Container{
		ID:             uuid.New(),
		UserID:         cm.UserID,
		ClawID:         cm.ClawID,
		ContainerID:    cID,
		Port:           cPort,
		Status:         entities.ContainerStatusStop,
		HasStartedOnce: false,
	}

	err = c.clRepo.Create(ctx, created)
	if err != nil {
		_ = c.manager.Remove(ctx, cID)

		releasePort()

		return entities.Container{}, fmt.Errorf("%s: %w", op, err)
	}

	slog.Default().Info("runtime ensured",
		slog.String("user_id", cm.UserID),
		slog.String("claw_id", cm.ClawID),
		slog.String("runtime_record_id", created.ID.String()),
		slog.String("docker_container_id", created.ContainerID),
		slog.String("port", created.Port),
	)

	return created, nil
}

func (c *Container) GetInfo() error {
	return nil
}

func (c *Container) ConfigArchive(
	ctx context.Context,
	cm commands.ConfigArchive,
	w io.Writer,
) error {
	const op = "service.Container.ConfigArchive"

	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cm.UserID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	if c.cfg == nil {
		return fmt.Errorf("%s: configurer is not configured", op)
	}

	err = c.cfg.ArchiveClawConfig(cm.UserID, cm.ClawID, w)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cm.DeleteAfter {
		err := c.cfg.DeleteClawConfig(cm.UserID, cm.ClawID)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

func (c *Container) RestoreConfig(
	ctx context.Context,
	cm commands.RestoreConfig,
	r io.Reader,
) error {
	const op = "service.Container.RestoreConfig"

	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cm.UserID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	if c.cfg == nil {
		return fmt.Errorf("%s: configurer is not configured", op)
	}

	err = c.cfg.RestoreClawConfig(cm.UserID, cm.ClawID, r)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *Container) Approve(ctx context.Context, clawID, userID, channelType, code string) error {
	const op = "container.Manager.Approve"

	if c.clRepo == nil {
		return fmt.Errorf("%s: repository is not configured", op)
	}

	if c.manager == nil {
		return fmt.Errorf("%s: runtime manager is not configured", op)
	}

	cont, err := c.clRepo.GetByUserClawID(ctx, userID, clawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cont.ContainerID == "" {
		return fmt.Errorf("%s: container id is required", op)
	}

	channelType, err = hostingapi.NormalizeApproveChannelType(channelType)
	if err != nil {
		return fmt.Errorf("%s: %w", op, ErrApproveChannelUnsupported)
	}

	err = c.manager.ExecPairingApprove(ctx, cont.ContainerID, docker.ExecPairingApproveOptions{
		ChannelType: channelType,
		Code:        code,
	})
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "invalid code") {
			return fmt.Errorf("%s: %w", op, ErrInvalidCode)
		}
		if strings.Contains(msg, "not supported") || strings.Contains(msg, "unsupported") {
			return fmt.Errorf("%s: %w", op, ErrApproveChannelUnsupported)
		}

		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *Container) Connect(ctx context.Context, cm commands.ConnectCommand) error {
	const op = "service.Container.Connect"

	cont, err := c.clRepo.GetByUserClawID(ctx, cm.UserID, cm.ClawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	provider, err := hostingapi.NormalizeProvider(cm.Provider)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	switch provider {
	case hostingapi.ProviderGmail:
		importPayload, normalizeErr := normalizeGogImportPayload(cm.Token)
		if normalizeErr != nil {
			return fmt.Errorf("%s: %w", op, normalizeErr)
		}

		var payload gogImportPayload
		if err := json.Unmarshal(importPayload, &payload); err != nil {
			return fmt.Errorf("%s: parse normalized payload: %w", op, err)
		}

		if strings.EqualFold(c.gog.KeyringBackend, "file") && c.gog.KeyringPassword == "" {
			return fmt.Errorf("%s: gog keyring password is required for file backend", op)
		}

		binding, err := c.RuntimeBinding(ctx, cont.UserID, cont.ClawID, "gmail-pubsub")
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		gogWatchPort, err := GogWatchPortFromGateway(cont.Port)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		err = c.manager.ExecGmail(ctx, cont.ContainerID, importPayload, docker.ExecGmailOptions{
			KeyringBackend:  c.gog.KeyringBackend,
			KeyringPassword: c.gog.KeyringPassword,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		err = c.manager.StartGmailWatch(ctx, cont.ContainerID, docker.ExecGmailWatchStartOptions{
			Account:         payload.Email,
			Topic:           payload.Topic,
			Labels:          payload.Labels,
			KeyringBackend:  c.gog.KeyringBackend,
			KeyringPassword: c.gog.KeyringPassword,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		err = c.manager.StartGmailWatcher(ctx, cont.ContainerID, docker.ExecGmailWatcherOptions{
			Account:         payload.Email,
			WatchPort:       gogWatchPort,
			WatchPath:       firstNonEmpty(binding.InternalPath, "/gmail-pubsub"),
			HookURL:         fmt.Sprintf("http://127.0.0.1:%s/hooks/gmail", cont.Port),
			KeyringBackend:  c.gog.KeyringBackend,
			KeyringPassword: c.gog.KeyringPassword,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	default:
		return fmt.Errorf("%s: %w", op, fmt.Errorf("unknown provider: %s", provider))
	}

	return nil
}

func (c *Container) onStart(ctx context.Context) error {
	const op = "service.Container.onStart"

	containers, err := c.clRepo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	for _, cc := range containers {
		c.p.Occupy(cc.Port)
	}

	return nil
}

func (c *Container) ListRunning(ctx context.Context) ([]entities.Container, error) {
	const op = "service.Container.ListRunning"

	containers, err := c.clRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	running := make([]entities.Container, 0, len(containers))
	for _, cc := range containers {
		if cc.Status == entities.ContainerStatusRunning && strings.TrimSpace(cc.Port) != "" {
			running = append(running, cc)
		}
	}

	return running, nil
}

func (c *Container) RuntimeBinding(
	ctx context.Context,
	userID,
	clawID,
	bindingName string,
) (entities.RuntimeBinding, error) {
	const op = "service.Container.RuntimeBinding"

	manifest, err := c.cfg.RuntimeBindings(userID, clawID)
	if err != nil {
		return entities.RuntimeBinding{}, fmt.Errorf("%s: %w", op, err)
	}

	binding, ok := findRuntimeBinding(manifest, bindingName)
	if !ok {
		return entities.RuntimeBinding{}, fmt.Errorf("%s: %w", op, storage.ErrNotFound)
	}

	return binding, nil
}

func findRuntimeBinding(
	manifest entities.RuntimeBindingManifest,
	name string,
) (entities.RuntimeBinding, bool) {
	for _, binding := range manifest.Bindings {
		if binding.Name == name {
			return binding, true
		}
	}

	return entities.RuntimeBinding{}, false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}

func (c *Container) containerVolumes(cfgPath string) []string {
	volumes := []string{fmt.Sprintf("%s:/app/:rw", cfgPath)}

	credentialsPath := strings.TrimSpace(c.cfg.GetCredentialsPath())
	if credentialsPath == "" {
		return volumes
	}

	return append(
		volumes,
		fmt.Sprintf(
			"%s:%s:ro",
			credentialsPath,
			docker.GogBootstrapCredentialsPath,
		),
	)
}
