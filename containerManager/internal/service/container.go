package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"shared/pkg/hostingapi"
	"shared/pkg/observability"
	"strconv"
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

var ErrInvalidCode = errors.New("invalid code")
var ErrServerCapacityExceeded = errors.New("server capacity exceeded")
var ErrServerMemoryUnavailable = errors.New("server memory unavailable")

const (
	coldStartMinAvailableBytes = 2 * 1024 * 1024 * 1024
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
		readAvailableMemory: readLinuxMemAvailableBytes,
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
		return fmt.Errorf("%w: %v", ErrServerMemoryUnavailable, err)
	}

	if availableBytes < requiredBytes {
		return fmt.Errorf("%w: available=%d required=%d", ErrServerMemoryUnavailable, availableBytes, requiredBytes)
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

	err = c.manager.Stop(ctx, cDb.ContainerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	cDb.Status = entities.ContainerStatusStop

	err = c.clRepo.Update(ctx, cDb)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

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

		if err := c.cfg.DeleteClawConfig(cm.UserID, cm.ClawID); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

func (c *Container) Create(ctx context.Context, cm commands.CreateClaw) (string, error) {
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
		return "", fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return "", fmt.Errorf("%s: claw id is required", op)
	}

	cDb, err := c.clRepo.GetByUserClawID(ctx, cm.UserID, cm.ClawID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	if uuid.Nil != cDb.ID {
		return "", fmt.Errorf("%s: %w", op, errors.New("claw already exists"))
	}

	if c.maxClaws > 0 {
		containers, err := c.clRepo.GetAll(ctx)
		if err != nil {
			return "", fmt.Errorf("%s: %w", op, err)
		}

		if len(containers) >= c.maxClaws {
			return "", fmt.Errorf("%s: %w", op, ErrServerCapacityExceeded)
		}
	}

	cfgPath, err := c.cfg.Configure(cm)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	cPort, err := c.p.Acquire()
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	gogWatchPort, err := GogWatchPortFromGateway(cPort)
	if err != nil {
		c.p.Release(cPort)
		return "", fmt.Errorf("%s: %w", op, err)
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
			fmt.Sprintf("OPENCLAW_GATEWAY_PORT=%s", cPort),
			"OPENCLAW_CONFIG_PATH=/app/openclaw.json",
			"NODE_OPTIONS=--max-old-space-size=3072",
		}, cm.Vars...),
	})
	if err != nil {
		releasePort()
		return "", fmt.Errorf("%s: %w", op, err)
	}

	containerRecordID := uuid.New()
	err = c.clRepo.Create(ctx, entities.Container{
		ID:             containerRecordID,
		UserID:         cm.UserID,
		ClawID:         cm.ClawID,
		ContainerID:    cID,
		Port:           cPort,
		Status:         entities.ContainerStatusStop,
		HasStartedOnce: false,
	})
	if err != nil {
		_ = c.manager.Remove(ctx, cID)
		releasePort()
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return containerRecordID.String(), nil
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

	if err := ctx.Err(); err != nil {
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

	if err := c.cfg.ArchiveClawConfig(cm.UserID, cm.ClawID, w); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cm.DeleteAfter {
		if err := c.cfg.DeleteClawConfig(cm.UserID, cm.ClawID); err != nil {
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

	if err := ctx.Err(); err != nil {
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

	if err := c.cfg.RestoreClawConfig(cm.UserID, cm.ClawID, r); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *Container) Approve(clawID, userID, code string) error {
	const op = "container.Manager.Approve"

	if c.cfg == nil {
		return fmt.Errorf("%s: configurer is not configured", op)
	}

	err := c.cfg.ApprovePair(userID, clawID, code)
	if err != nil {
		if errors.Is(err, configurer.ErrInvalidCode) {
			return fmt.Errorf("%s: %w", op, ErrInvalidCode)
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
			WatchPath:       "/gmail-pubsub",
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

func readLinuxMemAvailableBytes() (uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}

		fields := strings.Fields(strings.TrimPrefix(line, "MemAvailable:"))
		if len(fields) == 0 {
			return 0, errors.New("MemAvailable value is missing")
		}

		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse MemAvailable: %w", err)
		}

		return value * 1024, nil
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan /proc/meminfo: %w", err)
	}

	return 0, errors.New("MemAvailable is not present in /proc/meminfo")
}
