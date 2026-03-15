package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/build"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const MB = 1024 * 1024

const imageName = "openclaw_npm"

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

func (m *Manager) Build(ctx context.Context, buildCtxPaths []string) error {
	const op = "container.Manager.Build"

	err := m.getImages(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	img, ok := m.images[imageName]
	if ok {
		slog.Info(
			"image already exists",
			slog.String("image", imageName),
			slog.String("id", img.id),
		)

		return nil
	}

	buildCtx := bytes.NewBuffer([]byte{})

	defer buildCtx.Reset()

	err = tarDir(buildCtx, buildCtxPaths...)
	if err != nil {
		panic(err)
	}

	rs, err := m.cl.ImageBuild(context.Background(), buildCtx, build.ImageBuildOptions{
		Dockerfile: "images/openclaw/Dockerfile",
		Context:    buildCtx,
		Remove:     true,
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
		Resources: dcontainer.Resources{
			Memory:     MB * 3000,
			MemorySwap: MB * 3000,
		},
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

func tarDir(w io.Writer, roots ...string) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	wd, _ := os.Getwd()

	for _, root := range roots {
		root = filepath.Clean(root)

		var archiveRoot string

		if strings.HasPrefix(root, "."+string(os.PathSeparator)) {
			archiveRoot = root[2:]
		} else if !filepath.IsAbs(root) {
			archiveRoot = root
		} else {
			if rel, err := filepath.Rel(wd, root); err == nil && !strings.HasPrefix(rel, "..") {
				archiveRoot = rel
			} else {
				archiveRoot = filepath.Base(root)
			}
		}

		archiveRoot = filepath.ToSlash(archiveRoot)

		info, err := os.Lstat(root)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			var link string

			if info.Mode()&os.ModeSymlink != 0 {
				link, err = os.Readlink(root)
				if err != nil {
					return err
				}
			}

			hdr, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}

			hdr.Name = archiveRoot
			if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
				hdr.Name += "/"
			}

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			if info.Mode().IsRegular() {
				f, err := os.Open(root)
				if err != nil {
					return err
				}

				_, err = io.Copy(tw, f)
				_ = f.Close()

				if err != nil {
					return err
				}
			}

			continue
		}

		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}

			var archPath string

			if rel == "." {
				archPath = archiveRoot
			} else {
				archPath = filepath.ToSlash(filepath.Join(archiveRoot, rel))
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			var link string

			if info.Mode()&os.ModeSymlink != 0 {
				link, err = os.Readlink(path)
				if err != nil {
					return err
				}
			}

			hdr, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}

			hdr.Name = archPath
			if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
				hdr.Name += "/"
			}

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			if info.Mode().IsRegular() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}

				_, err = io.Copy(tw, f)
				_ = f.Close()

				if err != nil {
					return err
				}
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}
