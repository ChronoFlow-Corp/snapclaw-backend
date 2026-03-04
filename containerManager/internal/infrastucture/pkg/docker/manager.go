package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/build"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const MB = 1024 * 1024

const imageName = "openclaw-gateway"

type Manager struct {
	images map[string]imageInfo
	cl     *client.Client
}

func NewManager(ctx context.Context, cl *client.Client) *Manager {
	m := &Manager{
		images: make(map[string]imageInfo),
		cl:     cl,
	}

	go m.ping(ctx)

	err := m.getImages(ctx)
	if err != nil {
		panic(fmt.Sprintf("unable to get images: %v", err))
	}

	return m
}

func (m *Manager) Build(ctx context.Context, bCtx io.Reader) error {
	const op = "container.Manager.Build"

	err := m.getImages(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	img, ok := m.images[imageName]
	if ok {
		fmt.Printf("image %s already exists: %s\n", imageName, img.id)

		return nil
	}

	rs, err := m.cl.ImageBuild(context.Background(), bCtx, build.ImageBuildOptions{
		Dockerfile: "Dockerfile",
		Context:    bCtx,
		Tags:       []string{imageName},
	})
	if err != nil {
		return err
	}

	io.Copy(os.Stdout, rs.Body)

	return nil
}

func (m *Manager) Create(ctx context.Context, opts CreateOptions) (string, error) {
	const op = "container.Manager.Create"

	p := nat.Port(fmt.Sprintf("%s/tcp", opts.ContainerPort))

	rs, err := m.cl.ContainerCreate(ctx, &dcontainer.Config{
		Cmd:          strslice.StrSlice{"node", "openclaw.mjs", "gateway"},
		Image:        m.images[imageName].id,
		Env:          opts.Env,
		ExposedPorts: nat.PortSet{p: struct{}{}},
	}, &dcontainer.HostConfig{
		PortBindings: nat.PortMap{p: []nat.PortBinding{{
			HostPort: opts.HostPort,
			HostIP:   opts.HostIP,
		}}},
		Binds:         opts.Volumes,
		RestartPolicy: dcontainer.RestartPolicy{Name: "unless-stopped"},
		NetworkMode:   "bridge",
		LogConfig: dcontainer.LogConfig{
			Type:   "json-file",
			Config: map[string]string{"max-size": "10m"},
		},
		Resources: dcontainer.Resources{Memory: MB * 1500},
	}, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return rs.ID, nil
}

func (m *Manager) Start(ctx context.Context, containerID string) error {
	const op = "container.Manager.Start"

	err := m.cl.ContainerStart(ctx, containerID, dcontainer.StartOptions{})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Attach(ctx context.Context, containerID string) (*types.HijackedResponse, error) {
	const op = "container.Manager.Attach"

	return nil, nil
}

func (m *Manager) Stop(ctx context.Context, containerID string) error {
	const op = "container.Manager.Stop"

	err := m.cl.ContainerStop(ctx, containerID, dcontainer.StopOptions{})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Remove(ctx context.Context, containerID string) error {
	const op = "container.Manager.Remove"

	err := m.cl.ContainerRemove(ctx, containerID, dcontainer.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Stat(ctx context.Context, containerID string) error {
	const op = "container.Manager.Stat"

	return nil
}

func (m *Manager) getImages(ctx context.Context) error {
	const op = "container.Manager.getImages"

	images, err := m.cl.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			spl := strings.Split(tag, ":")
			if slices.Contains(spl, imageName) {
				m.images[imageName] = imageInfo{
					name: imageName,
					id:   img.ID,
					tag:  img.RepoTags,
				}
			}
		}
	}

	return nil
}

func (m *Manager) ping(ctx context.Context) error {
	t := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-t.C:
			_, err := m.cl.Ping(ctx)
			if err != nil {
				panic(err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}
