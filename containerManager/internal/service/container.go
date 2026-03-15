package service

import (
	"context"
	"errors"
	"fmt"
	"io"
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
}

var errForbidden = errors.New("container belongs to another user")

func NewContainer(
	cfg *configurer.ClawConfigurer,
	clRepo ClawRepository,
	manager *docker.Manager,
) *Container {
	c := &Container{
		cfg:     cfg,
		clRepo:  clRepo,
		manager: manager,
		p:       NewPorter(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := c.onStart(ctx)
	if err != nil {
		panic(err)
	}

	return c
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

	releasePort := func() {
		if cPort != "" {
			c.p.Release(cPort)
			cPort = ""
		}
	}

	cID, err := c.manager.Create(ctx, docker.CreateOptions{
		HostPort:      cPort,
		HostIP:        "127.0.0.1",
		ContainerPort: cPort,
		Volumes:       []string{fmt.Sprintf("%s:/app/:rw", cfgPath)},
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

	err := c.cfg.ApprovePair(userID, clawID, code)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
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
