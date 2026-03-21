package hosting

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
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
