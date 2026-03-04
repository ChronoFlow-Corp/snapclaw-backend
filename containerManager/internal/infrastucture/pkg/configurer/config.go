package configurer

import (
	"fmt"
	"os"
	"path"

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

	fmt.Println(c.basePath)

	basePath := path.Join(c.basePath, cm.UserID)

	err := os.MkdirAll(basePath, 0o755)
	if err != nil {
		return "", err
	}

	err = configure(basePath, cm.Config)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	fmt.Println(basePath)

	return basePath, nil
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
			f, err := os.Create(fmt.Sprintf("%s/%s.json", basePath, c.Name))
			if err != nil {
				return err
			}

			_, err = f.Write(c.Data)
			if err != nil {
				return err
			}
		case entities.ClawConfigTypeMd:
			f, err := os.Create(fmt.Sprintf("%s/%s.md", basePath, c.Name))
			if err != nil {
				return err
			}

			_, err = f.Write(c.Data)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
