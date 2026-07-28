package configurer

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"containermanager/internal/entities"
	"containermanager/internal/service/commands"
)

var chownPath = os.Chown

const runtimeBindingsFileName = "runtime-bindings.json"

type ClawConfigurer struct {
	basePath        string
	credentialsPath string
	ownerUID        int
	ownerGID        int
}

func NewClawConfigurer(basePath string, credentialsPath string, ownerUID int, ownerGID int) *ClawConfigurer {
	return &ClawConfigurer{
		basePath:        basePath,
		credentialsPath: strings.TrimSpace(credentialsPath),
		ownerUID:        ownerUID,
		ownerGID:        ownerGID,
	}
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

	if err := c.ensureOwnership(basePath); err != nil {
		return "", fmt.Errorf("%s: ensure ownership: %w", op, err)
	}

	return basePath, nil
}

func (c *ClawConfigurer) GetCredentialsPath() string {
	return c.credentialsPath
}

func (c *ClawConfigurer) RuntimeBindings(userID, clawID string) (entities.RuntimeBindingManifest, error) {
	const op = "configurer.ClawConfigurer.RuntimeBindings"

	basePath, err := c.configPath(userID, clawID)
	if err != nil {
		return entities.RuntimeBindingManifest{}, fmt.Errorf("%s: %w", op, err)
	}

	raw, err := os.ReadFile(filepath.Join(basePath, runtimeBindingsFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return entities.RuntimeBindingManifest{}, nil
		}

		return entities.RuntimeBindingManifest{}, fmt.Errorf("%s: %w", op, err)
	}

	var manifest entities.RuntimeBindingManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return entities.RuntimeBindingManifest{}, fmt.Errorf("%s: %w", op, err)
	}

	return manifest, nil
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

	if err := c.ensureOwnership(basePath); err != nil {
		return "", fmt.Errorf("%s: ensure ownership: %w", op, err)
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

		if after, ok := strings.CutPrefix(name, prefix); ok {
			name = after
		}

		target := filepath.Join(basePath, name)

		cleanTargetPath := filepath.Clean(target)
		if cleanTargetPath == basePath ||
			!strings.HasPrefix(
				cleanTargetPath+string(os.PathSeparator),
				basePath+string(os.PathSeparator),
			) {
			return fmt.Errorf("%s: invalid tar path %q", op, hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			err := os.MkdirAll(cleanTargetPath, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(cleanTargetPath), 0o755); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}

			f, err := os.OpenFile(
				cleanTargetPath,
				os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
				os.FileMode(hdr.Mode),
			)
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

	if err := c.ensureOwnership(basePath); err != nil {
		return fmt.Errorf("%s: ensure ownership: %w", op, err)
	}

	return nil
}

func (c *ClawConfigurer) ApprovePair(userID, clawID, code string) error {
	const op = "configurer.ClawConfigurer.ApprovePair"

	if userID == "" {
		return fmt.Errorf("%s: user id is required", op)
	}

	if clawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	basePath, err := c.configPath(userID, clawID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	basePath = path.Join(basePath, "/.openclaw/credentials")

	fd, err := os.OpenFile(
		path.Join(basePath, pendingPairingFileName),
		os.O_RDWR,
		os.FileMode(0o644),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer fd.Close()

	var p PairingTelegramConfig

	err = json.NewDecoder(fd).Decode(&p)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	var approvedID string

	for i := range p.Requests {
		if p.Requests[i].Code == code {
			approvedID = p.Requests[i].ID
			p.Requests = slices.Delete(p.Requests, i, i+1)

			break
		}
	}

	if approvedID == "" {
		return fmt.Errorf("%s: %w", op, ErrInvalidCode)
	}

	approved := TelegramPairedAllowFrom{
		Version:   p.Version,
		AllowFrom: []string{approvedID},
	}

	fStat, err := fd.Stat()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	fdAllowed, err := os.OpenFile(
		path.Join(basePath, allowedFileNameTelegram),
		os.O_CREATE|os.O_WRONLY,
		fStat.Mode(),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer fdAllowed.Close()

	err = json.NewEncoder(fdAllowed).Encode(approved)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	_, err = fd.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	err = fd.Truncate(0)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	err = json.NewEncoder(fd).Encode(p)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
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

func (c *ClawConfigurer) ensureOwnership(root string) error {
	if c.ownerUID < 0 || c.ownerGID < 0 {
		return nil
	}

	return filepath.WalkDir(root, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if err := chownPath(current, c.ownerUID, c.ownerGID); err != nil {
			return fmt.Errorf("chown %s: %w", current, err)
		}

		return nil
	})
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
			err := writeFile(fmt.Sprintf("%s/%s.json", basePath, c.Name), c.Data)
			if err != nil {
				return err
			}
		case entities.ClawConfigTypeMd:
			err := writeFile(fmt.Sprintf("%s/%s.md", basePath, c.Name), c.Data)
			if err != nil {
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
