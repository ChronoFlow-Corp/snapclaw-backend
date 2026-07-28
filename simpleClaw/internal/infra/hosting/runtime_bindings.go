package hosting

import "simpleClaw/internal/entities"

type runtimeBindingManifest struct {
	Bindings []runtimeBinding `json:"bindings,omitempty"`
}

type runtimeBinding struct {
	Name                  string `json:"name,omitempty"`
	Kind                  string `json:"kind,omitempty"`
	Provider              string `json:"provider,omitempty"`
	InternalPath          string `json:"internalPath,omitempty"`
	RequiresSecondaryPort bool   `json:"requiresSecondaryPort,omitempty"`
}

func buildRuntimeBindingManifest(cfg entities.ClawConfig) runtimeBindingManifest {
	manifest := runtimeBindingManifest{}

	if cfg.Hooks != nil && cfg.Hooks.Gmail.Serve.Path != "" {
		manifest.Bindings = append(manifest.Bindings, runtimeBinding{
			Name:                  "gmail-pubsub",
			Kind:                  "http-sidecar",
			Provider:              "gmail",
			InternalPath:          cfg.Hooks.Gmail.Serve.Path,
			RequiresSecondaryPort: true,
		})
	}

	return manifest
}
