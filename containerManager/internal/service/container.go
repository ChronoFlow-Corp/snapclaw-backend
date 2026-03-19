package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"shared/consts"
	"strings"
	"time"

	"containermanager/internal/entities"
	"containermanager/internal/infrastucture/pkg/configurer"
	"containermanager/internal/infrastucture/pkg/docker"
	"containermanager/internal/infrastucture/sql/storage"
	"containermanager/internal/service/commands"

	"github.com/google/uuid"
)

type Container struct {
	clRepo  ClawRepository
	manager *docker.Manager
	p       *Porter
	cfg     *configurer.ClawConfigurer
	gog     GogConfig
}

type GogConfig struct {
	KeyringBackend  string
	KeyringPassword string
}

var errForbidden = errors.New("container belongs to another user")

var ErrInvalidCode = errors.New("invalid code")

func NewContainer(
	cfg *configurer.ClawConfigurer,
	clRepo ClawRepository,
	manager *docker.Manager,
	gog GogConfig,
) (*Container, error) {
	c := &Container{
		cfg:     cfg,
		clRepo:  clRepo,
		manager: manager,
		p:       NewPorter(),
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

func (c *Container) Start(ctx context.Context, cm commands.StartClaw) error {
	const op = "service.Container.Start"

	if cm.UserID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if cm.ClawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	cont, err := c.clRepo.GetByUserClawID(ctx, cm.UserID, cm.ClawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if cont.UserID != cm.UserID {
		return fmt.Errorf("%s: %w", op, errForbidden)
	}

	err = c.manager.Start(ctx, cont.ContainerID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	cont.Status = entities.ContainerStatusRunning

	err = c.clRepo.Update(ctx, cont)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *Container) Stop(ctx context.Context, cm commands.StopClaw) error {
	const op = "service.Container.Stop"

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

func (c *Container) Update(ctx context.Context, cm commands.UpdateClaw) error {
	const op = "service.Container.Update"

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

func (c *Container) Delete(ctx context.Context, cm commands.DeleteClaw) error {
	const op = "service.Container.Delete"

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
		ID:          containerRecordID,
		UserID:      cm.UserID,
		ClawID:      cm.ClawID,
		ContainerID: cID,
		Port:        cPort,
		Status:      entities.ContainerStatusStop,
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

	switch cm.Provider {
	case consts.GmailProvider:
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
		return fmt.Errorf("%s: %w", op, fmt.Errorf("unknown provider: %s", cm.Provider))
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
