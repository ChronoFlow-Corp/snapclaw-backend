package service

import (
	"context"
	"errors"
	"fmt"
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

	cont, err := c.clRepo.GetByID(ctx, cm.ContainerID)
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

	cDb, err := c.clRepo.GetByID(ctx, cm.ContainerID)
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

	cDb.Status = entities.ConstainerStatusStop

	err = c.clRepo.Update(ctx, cDb)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *Container) Update(ctx context.Context, cm commands.UpdateClaw) error {
	const op = "service.Container.Update"

	cDb, err := c.clRepo.GetByID(ctx, cm.ContainerID)
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

	cDb, err := c.clRepo.GetByID(ctx, cm.ContainerID)
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

	return nil
}

func (c *Container) Create(ctx context.Context, cm commands.CreateClaw) (string, error) {
	const op = "service.Container.CreateClaw"

	cDb, err := c.clRepo.GetByUserID(ctx, cm.UserID)
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
		Volumes:       []string{fmt.Sprintf("%s:/app/.openclaw:rw", cfgPath)},
		Env: []string{
			"OPENCLAW_HOME=/app/.openclaw",
			"NODE_ENV=production",
			"OPENCLAW_GATEWAY_BIND=lan",
			fmt.Sprintf("OPENCLAW_GATEWAY_PORT=%s", cPort),
			"OPENCLAW_CONFIG_PATH=/app/.openclaw/openclaw.json",
		},
	})
	if err != nil {
		releasePort()
		return "", fmt.Errorf("%s: %w", op, err)
	}

	containerRecordID := uuid.New()
	err = c.clRepo.Create(ctx, entities.Container{
		ID:          containerRecordID,
		UserID:      cm.UserID,
		ContainerID: cID,
		Port:        cPort,
		Status:      entities.ConstainerStatusStop,
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
