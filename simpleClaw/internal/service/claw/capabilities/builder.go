package capabilities

import (
	"context"
	"fmt"
	"strings"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/service/claw/commands"
)

const braveAPIKeyVar = "BRAVE_API_KEY"

type CreateValidationError string

func (e CreateValidationError) Error() string {
	return string(e)
}

const (
	ErrWebSearchRequired            CreateValidationError = "web search capability is required"
	ErrWebSearchProviderUnsupported CreateValidationError = "web search provider is not supported"
)

func BuildBaseClawConfig(primaryModel string) entities.ClawConfig {
	return entities.NewCreateClawConfig(entities.CreateClawConfigInput{
		PrimaryModel: primaryModel,
	})
}

func ApplyCreateCapabilities(
	ctx context.Context,
	cfg *entities.ClawConfig,
	input commands.CreateCapabilitySet,
) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	if input.WebSearch == nil || !input.WebSearch.Enabled {
		return ErrWebSearchRequired
	}

	if err := applyWebSearch(ctx, cfg, input.WebSearch); err != nil {
		return err
	}

	if input.FilesImages != nil && input.FilesImages.Enabled {
		applyFilesImages(cfg)
	}

	if input.Memory != nil && input.Memory.Enabled {
		applyMemory(cfg)
	}

	if input.Gmail != nil && input.Gmail.Enabled {
		applyGmail(cfg)
	}

	return nil
}

func applyWebSearch(
	_ context.Context,
	cfg *entities.ClawConfig,
	input *commands.WebSearchCapabilityInput,
) error {
	provider := strings.TrimSpace(strings.ToLower(input.Provider))
	if provider != "brave" {
		return ErrWebSearchProviderUnsupported
	}

	if cfg.Env.Vars == nil {
		cfg.Env.Vars = make(map[string]string)
	}

	if _, ok := cfg.Env.Vars[braveAPIKeyVar]; !ok {
		cfg.Env.Vars[braveAPIKeyVar] = varRef(braveAPIKeyVar)
	}

	if cfg.Plugins == nil {
		cfg.Plugins = &entities.PluginsConfig{}
	}

	if cfg.Plugins.Entries == nil {
		cfg.Plugins.Entries = make(map[string]entities.PluginEntry)
	}

	cfg.Plugins.Entries["brave"] = entities.PluginEntry{
		Config: map[string]any{
			"webSearch": map[string]any{
				"apiKey": varRef(braveAPIKeyVar),
				"mode":   "web",
			},
		},
	}

	if cfg.Tools == nil {
		cfg.Tools = entities.NewDefaultToolsConfig()
	}

	cfg.Tools.Web = &entities.WebToolsConfig{
		Search: &entities.WebSearchToolConfig{
			Enabled:        true,
			Provider:       "brave",
			MaxResults:     5,
			TimeoutSeconds: 30,
		},
	}

	return nil
}

func applyFilesImages(_ *entities.ClawConfig) {}

func applyMemory(_ *entities.ClawConfig) {}

func applyGmail(cfg *entities.ClawConfig) {
	if cfg.Hooks == nil {
		cfg.Hooks = &entities.HooksConfig{}
	}

	if cfg.Hooks.Gmail.Serve.Path == "" {
		cfg.Hooks.Gmail.Serve.Path = "/gmail-pubsub"
	}
}

func varRef(name string) string {
	return "${" + name + "}"
}
