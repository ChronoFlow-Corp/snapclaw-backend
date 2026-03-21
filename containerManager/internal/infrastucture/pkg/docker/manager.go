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

	"containermanager/internal/pkg/logctx"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/build"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"shared/pkg/observability"
)

const MB = 1024 * 1024

const imageName = "openclaw_npm"

type Manager struct {
	images  map[string]imageInfo
	cl      *client.Client
	metrics *observability.OperationMetrics
}

func NewManager(ctx context.Context, cl *client.Client, metrics ...*observability.OperationMetrics) (*Manager, error) {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	m := &Manager{
		images:  make(map[string]imageInfo),
		cl:      cl,
		metrics: opMetrics,
	}

	go m.ping(ctx)

	err := m.getImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("container.Manager.NewManager: unable to get images: %w", err)
	}

	return m, nil
}

func (m *Manager) Build(ctx context.Context, dockerfile string, buildCtxPaths []string) error {
	const op = "container.Manager.Build"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.docker", "docker.image.build", "claw_lifecycle")

	var err error

	defer func() { finish(err) }()

	err = m.getImages(ctx)
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
		return fmt.Errorf("%s: build context: %w", op, err)
	}

	rs, err := m.cl.ImageBuild(ctx, buildCtx, build.ImageBuildOptions{
		Dockerfile: dockerfile,
		Context:    buildCtx,
		Remove:     true,
		Tags:       []string{imageName},
	})
	if err != nil {
		return fmt.Errorf("%s: image build: %w", op, err)
	}
	defer rs.Body.Close()

	if _, err := io.Copy(os.Stdout, rs.Body); err != nil {
		return fmt.Errorf("%s: stream build output: %w", op, err)
	}

	return nil
}

func (m *Manager) Create(ctx context.Context, opts CreateOptions) (string, error) {
	const op = "container.Manager.Create"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.docker", "docker.container.create", "claw_lifecycle")

	var err error

	defer func() { finish(err) }()

	primaryPort := nat.Port(fmt.Sprintf("%s/tcp", opts.ContainerPort))
	exposedPorts := nat.PortSet{
		primaryPort: struct{}{},
	}
	portBindings := nat.PortMap{
		primaryPort: []nat.PortBinding{{
			HostPort: opts.HostPort,
			HostIP:   opts.HostIP,
		}},
	}

	if opts.SecondaryContainerPort != "" && opts.SecondaryHostPort != "" {
		secondaryPort := nat.Port(fmt.Sprintf("%s/tcp", opts.SecondaryContainerPort))
		exposedPorts[secondaryPort] = struct{}{}
		portBindings[secondaryPort] = []nat.PortBinding{{
			HostPort: opts.SecondaryHostPort,
			HostIP:   opts.HostIP,
		}}
	}

	rs, err := m.cl.ContainerCreate(ctx, &dcontainer.Config{
		Image:        m.images[imageName].id,
		Env:          opts.Env,
		ExposedPorts: exposedPorts,
	}, &dcontainer.HostConfig{
		PortBindings:  portBindings,
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

func (m *Manager) Start(ctx context.Context, containerID string) (err error) {
	const op = "container.Manager.Start"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.docker", "docker.container.start", "claw_lifecycle")

	defer func() { finish(err) }()

	err = m.cl.ContainerStart(ctx, containerID, dcontainer.StartOptions{})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Attach(ctx context.Context, containerID string) (*types.HijackedResponse, error) {
	const op = "container.Manager.Attach"

	return nil, nil
}

func (m *Manager) Stop(ctx context.Context, containerID string) (err error) {
	const op = "container.Manager.Stop"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.docker", "docker.container.stop", "claw_lifecycle")

	defer func() { finish(err) }()

	err = m.cl.ContainerStop(ctx, containerID, dcontainer.StopOptions{})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (m *Manager) Remove(ctx context.Context, containerID string) (err error) {
	const op = "container.Manager.Remove"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.docker", "docker.container.remove", "claw_lifecycle")

	defer func() { finish(err) }()

	err = m.cl.ContainerRemove(ctx, containerID, dcontainer.RemoveOptions{
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

func (m *Manager) ExecGmail(
	ctx context.Context,
	containerID string,
	token []byte,
	opts ExecGmailOptions,
) error {
	const op = "container.Manager.ExecGmail"

	ctx, _, finish := observability.StartOperation(ctx, slog.Default(), m.metrics, "infra.docker", "docker.exec.gmail", "claw_pairing")

	var err error

	defer func() { finish(err) }()

	log := logctx.Logger(ctx).With(slog.String("container_id", containerID))

	if containerID == "" {
		err := fmt.Errorf("%s: container id is required", op)
		log.Error("gmail exec failed", slog.Any("err", err))

		return err
	}

	if len(bytes.TrimSpace(token)) == 0 {
		err := fmt.Errorf("%s: token is required", op)
		log.Error("gmail exec failed", slog.Any("err", err))

		return err
	}

	if strings.TrimSpace(opts.KeyringBackend) == "" {
		opts.KeyringBackend = "file"
	}

	createRs, err := m.cl.ContainerExecCreate(ctx, containerID, execConnectGmail(opts))
	if err != nil {
		log.Error("gmail exec create failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	hjr, err := m.cl.ContainerExecAttach(ctx, createRs.ID, dcontainer.ExecAttachOptions{})
	if err != nil {
		log.Error("gmail exec attach failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}
	defer hjr.Close()

	if _, err := io.Copy(hjr.Conn, bytes.NewReader(token)); err != nil {
		log.Error("gmail exec write failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	if err := hjr.CloseWrite(); err != nil {
		log.Error("gmail exec close write failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, hjr.Reader); err != nil {
		log.Error("gmail exec read failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	inspect, err := m.cl.ContainerExecInspect(ctx, createRs.ID)
	if err != nil {
		log.Error("gmail exec inspect failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	if inspect.ExitCode != 0 {
		stdoutStr := strings.TrimSpace(stdoutBuf.String())
		stderrStr := strings.TrimSpace(stderrBuf.String())

		errText := stderrStr
		if errText == "" {
			errText = stdoutStr
		}

		if errText == "" {
			errText = "unknown error"
		}

		trim := func(s string) string {
			const maxLen = 2048
			if len(s) <= maxLen {
				return s
			}

			return s[:maxLen] + "...(truncated)"
		}

		log.Error(
			"gmail exec failed",
			slog.Int("exit_code", inspect.ExitCode),
			slog.String("stderr", trim(stderrStr)),
			slog.String("stdout", trim(stdoutStr)),
		)

		return fmt.Errorf("%s: exec failed with exit code %d: %s", op, inspect.ExitCode, errText)
	}

	return nil
}

func (m *Manager) StartGmailWatch(
	ctx context.Context,
	containerID string,
	opts ExecGmailWatchStartOptions,
) error {
	const op = "container.Manager.StartGmailWatch"

	log := logctx.Logger(ctx).With(slog.String("container_id", containerID))

	if containerID == "" {
		err := fmt.Errorf("%s: container id is required", op)
		log.Error("start gmail watch failed", slog.Any("err", err))

		return err
	}

	createRs, err := m.cl.ContainerExecCreate(ctx, containerID, execStartGmailWatch(opts))
	if err != nil {
		log.Error("start gmail watch exec create failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	hjr, err := m.cl.ContainerExecAttach(ctx, createRs.ID, dcontainer.ExecAttachOptions{})
	if err != nil {
		log.Error("start gmail watch exec attach failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}
	defer hjr.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, hjr.Reader); err != nil {
		log.Error("start gmail watch exec read failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	inspect, err := m.cl.ContainerExecInspect(ctx, createRs.ID)
	if err != nil {
		log.Error("start gmail watch exec inspect failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	if inspect.ExitCode != 0 {
		stdoutStr := strings.TrimSpace(stdoutBuf.String())
		stderrStr := strings.TrimSpace(stderrBuf.String())

		errText := stderrStr
		if errText == "" {
			errText = stdoutStr
		}

		if errText == "" {
			errText = "unknown error"
		}

		log.Error(
			"start gmail watch failed",
			slog.Int("exit_code", inspect.ExitCode),
			slog.String("stderr", stderrStr),
			slog.String("stdout", stdoutStr),
		)

		return fmt.Errorf("%s: exec failed with exit code %d: %s", op, inspect.ExitCode, errText)
	}

	return nil
}

func (m *Manager) StartGmailWatcher(
	ctx context.Context,
	containerID string,
	opts ExecGmailWatcherOptions,
) error {
	const op = "container.Manager.StartGmailWatcher"

	log := logctx.Logger(ctx).With(slog.String("container_id", containerID))

	if containerID == "" {
		err := fmt.Errorf("%s: container id is required", op)
		log.Error("start gmail watcher failed", slog.Any("err", err))

		return err
	}

	createRs, err := m.cl.ContainerExecCreate(ctx, containerID, execStartGmailWatcher(opts))
	if err != nil {
		log.Error("start gmail watcher exec create failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	hjr, err := m.cl.ContainerExecAttach(ctx, createRs.ID, dcontainer.ExecAttachOptions{})
	if err != nil {
		log.Error("start gmail watcher exec attach failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}
	defer hjr.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, hjr.Reader); err != nil {
		log.Error("start gmail watcher exec read failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	inspect, err := m.cl.ContainerExecInspect(ctx, createRs.ID)
	if err != nil {
		log.Error("start gmail watcher exec inspect failed", slog.Any("err", err))

		return fmt.Errorf("%s: %w", op, err)
	}

	if inspect.ExitCode != 0 {
		stdoutStr := strings.TrimSpace(stdoutBuf.String())
		stderrStr := strings.TrimSpace(stderrBuf.String())

		errText := stderrStr
		if errText == "" {
			errText = stdoutStr
		}

		if errText == "" {
			errText = "unknown error"
		}

		log.Error(
			"start gmail watcher failed",
			slog.Int("exit_code", inspect.ExitCode),
			slog.String("stderr", stderrStr),
			slog.String("stdout", stdoutStr),
		)

		return fmt.Errorf("%s: exec failed with exit code %d: %s", op, inspect.ExitCode, errText)
	}

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
	defer t.Stop()

	for {
		select {
		case <-t.C:
			_, err := m.cl.Ping(ctx)
			if err != nil {
				slog.Error("docker ping failed", slog.Any("err", err))
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
