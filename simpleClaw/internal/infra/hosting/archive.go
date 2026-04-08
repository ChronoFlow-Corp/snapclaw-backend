package hosting

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"simpleClaw/internal/entities"
)

// WriteConfigArchive writes a tar archive with the claw config folder.
// Archive layout: <clawID>/openclaw.json.
func WriteConfigArchive(cfg entities.ClawConfig, clawID string, w io.Writer) error {
	const op = "infra.hosting.WriteConfigArchive"

	if w == nil {
		return fmt.Errorf("%s: writer is required", op)
	}

	clawID = strings.TrimSpace(clawID)
	if clawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tw := tar.NewWriter(w)
	defer tw.Close()

	dirName := clawID + "/"
	if err := tw.WriteHeader(&tar.Header{
		Name:     dirName,
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	fileName := clawID + "/" + openClawConfigName + ".json"

	hdr := &tar.Header{
		Name:     fileName,
		Mode:     0o644,
		Typeflag: tar.TypeReg,
		Size:     int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func RewriteConfigArchive(src io.Reader, clawID string, cfg entities.ClawConfig, dst io.Writer) error {
	const op = "infra.hosting.RewriteConfigArchive"

	if src == nil {
		return fmt.Errorf("%s: reader is required", op)
	}

	if dst == nil {
		return fmt.Errorf("%s: writer is required", op)
	}

	clawID = strings.TrimSpace(clawID)
	if clawID == "" {
		return fmt.Errorf("%s: claw id is required", op)
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tr := tar.NewReader(src)
	tw := tar.NewWriter(dst)
	defer tw.Close()

	rootName := filepath.ToSlash(clawID)
	rootWritten := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		cleanName := cleanArchivePath(hdr.Name)
		if cleanName == "" || cleanName == "." {
			continue
		}

		if cleanName == rootName || cleanName == rootName+"/" {
			rootWritten = true
		}

		baseName := path.Base(cleanName)
		switch {
		case baseName == openClawConfigName+".json":
			continue
		case isOpenClawBackup(cleanName):
			continue
		}

		if err := copyTarEntry(tw, hdr, tr, cleanName); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	if !rootWritten {
		if err := tw.WriteHeader(&tar.Header{
			Name:     rootName + "/",
			Mode:     0o755,
			Typeflag: tar.TypeDir,
		}); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	fileName := rootName + "/" + openClawConfigName + ".json"
	if err := tw.WriteHeader(&tar.Header{
		Name:     fileName,
		Mode:     0o644,
		Typeflag: tar.TypeReg,
		Size:     int64(len(data)),
	}); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func isOpenClawBackup(archivePath string) bool {
	baseName := path.Base(cleanArchivePath(archivePath))
	return strings.HasPrefix(baseName, openClawConfigName+".json.bak")
}

func cleanArchivePath(name string) string {
	cleaned := path.Clean(strings.TrimSpace(filepath.ToSlash(name)))
	return strings.TrimPrefix(cleaned, "/")
}

func copyTarEntry(tw *tar.Writer, hdr *tar.Header, tr *tar.Reader, name string) error {
	copyHdr := *hdr
	copyHdr.Name = name

	if err := tw.WriteHeader(&copyHdr); err != nil {
		return err
	}

	if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
		if _, err := io.Copy(tw, tr); err != nil {
			return err
		}
	}

	return nil
}
