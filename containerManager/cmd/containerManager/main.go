package main

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"containermanager/config"
	"containermanager/internal/infrastucture/pkg/configurer"
	"containermanager/internal/infrastucture/pkg/docker"
	"containermanager/internal/infrastucture/sql/pgx"
	"containermanager/internal/infrastucture/sql/storage"
	"containermanager/internal/interface/rest"
	"containermanager/internal/interface/rest/controllers"
	"containermanager/internal/service"

	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg := config.MustLoadConfig()

	ctx := context.Background()

	pool, err := pgx.New(ctx, cfg.Postgres.URL)
	if err != nil {
		panic(err)
	}

	st := storage.NewContainer(pool)

	c := configurer.NewClawConfigurer(cfg.Image.BasePath)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	m := docker.NewManager(ctx, cli)

	buildCtx := bytes.NewBuffer([]byte{})

	err = TarDir(buildCtx, cfg.Image.BuildCtx...)
	if err != nil {
		panic(err)
	}

	err = m.Build(ctx, buildCtx)
	if err != nil {
		panic(err)
	}

	s := service.NewContainer(c, st, m)

	cl := controllers.NewClaw(s)

	mux := chi.NewRouter()
	mux.Use(middleware.Logger)
	mux.Use(middleware.Recoverer)

	cl.Register(mux)

	server := rest.NewServer("localhost:8080", mux)

	server.Start()
}

func TarDir(w io.Writer, roots ...string) error {
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
