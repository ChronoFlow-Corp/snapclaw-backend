package configurer

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"containermanager/internal/entities"
	"containermanager/internal/service/commands"
)

type ClawConfigurer struct {
	basePath string
}

func NewClawConfigurer(basePath string) *ClawConfigurer {
	return &ClawConfigurer{basePath: basePath}
}

func (c *ClawConfigurer) Configure(cm commands.CreateClaw) (string, error) {
	const op = "configurer.ClawConfigurer.Configure"

	basePath, err := c.configPath(cm.UserID, cm.ClawID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	err = os.MkdirAll(basePath, 0o755)
	if err != nil {
		return "", err
	}

	err = configure(basePath, cm.Config)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return basePath, nil
}

func (c *ClawConfigurer) Update(cm commands.UpdateClaw) (string, error) {
	const op = "configurer.ClawConfigurer.Update"

	basePath, err := c.configPath(cm.UserID, cm.ClawID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	err = os.MkdirAll(basePath, 0o755)
	if err != nil {
		return "", err
	}

	err = configure(basePath, cm.Config)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return basePath, nil
}

func (c *ClawConfigurer) DeleteClawConfig(userID, clawID string) error {
	const op = "configurer.ClawConfigurer.DeleteClawConfig"

	basePath, err := c.configPath(userID, clawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.RemoveAll(basePath); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *ClawConfigurer) ArchiveClawConfig(userID, clawID string, w io.Writer) error {
	const op = "configurer.ClawConfigurer.ArchiveClawConfig"

	if w == nil {
		return fmt.Errorf("%s: writer is required", op)
	}

	basePath, err := c.configPath(userID, clawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	info, err := os.Stat(basePath)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s: user config path is not a directory", op)
	}

	archiveRoot := path.Base(basePath)
	if err := tarDir(w, basePath, archiveRoot); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (c *ClawConfigurer) RestoreClawConfig(userID, clawID string, r io.Reader) error {
	const op = "configurer.ClawConfigurer.RestoreClawConfig"

	if r == nil {
		return fmt.Errorf("%s: reader is required", op)
	}

	basePath, err := c.configPath(userID, clawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.RemoveAll(basePath); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	archiveRoot := path.Base(basePath)

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		name := filepath.Clean(hdr.Name)
		name = strings.TrimPrefix(name, "/")
		if name == "." || name == "" {
			continue
		}

		prefix := archiveRoot + string(os.PathSeparator)
		if name == archiveRoot {
			continue
		}
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
		}

		target := filepath.Join(basePath, name)
		cleanTargetPath := filepath.Clean(target)
		if cleanTargetPath == basePath || !strings.HasPrefix(cleanTargetPath+string(os.PathSeparator), basePath+string(os.PathSeparator)) {
			return fmt.Errorf("%s: invalid tar path %q", op, hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(cleanTargetPath, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(cleanTargetPath), 0o755); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}

			f, err := os.OpenFile(cleanTargetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}

			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return fmt.Errorf("%s: %w", op, err)
			}

			if err := f.Close(); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		default:
			return fmt.Errorf("%s: unsupported tar entry %q", op, hdr.Name)
		}
	}

	return nil
}

func (c *ClawConfigurer) configPath(userID, clawID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("user id is required")
	}
	if clawID == "" {
		return "", fmt.Errorf("claw id is required")
	}

	basePath := path.Join(c.basePath, userID, clawID)
	cleanBase := path.Clean(c.basePath)
	cleanTarget := path.Clean(basePath)

	if cleanTarget == cleanBase || !strings.HasPrefix(cleanTarget+"/", cleanBase+"/") {
		return "", fmt.Errorf("invalid user or claw id")
	}

	return cleanTarget, nil
}

func tarDir(w io.Writer, root string, archiveRoot string) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		archPath := archiveRoot
		if rel != "." {
			archPath = filepath.ToSlash(filepath.Join(archiveRoot, rel))
		} else {
			archPath = filepath.ToSlash(archiveRoot)
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
}

func configure(basePath string, cfg []entities.ClawConfig) error {
	for _, c := range cfg {
		switch c.FileType {
		case entities.ClawConfigTypeDir:
			err := os.MkdirAll(fmt.Sprintf("%s/%s/", basePath, c.Name), 0o755)
			if err != nil {
				return err
			}

			err = configure(fmt.Sprintf("%s/%s/", basePath, c.Name), c.ClawConfig)
			if err != nil {
				return err
			}
		case entities.ClawConfigTypeJson:
			if err := writeFile(fmt.Sprintf("%s/%s.json", basePath, c.Name), c.Data); err != nil {
				return err
			}
		case entities.ClawConfigTypeMd:
			if err := writeFile(fmt.Sprintf("%s/%s.md", basePath, c.Name), c.Data); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeFile(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}
