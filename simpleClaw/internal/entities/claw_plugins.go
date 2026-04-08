package entities

type PluginsConfig struct {
	Enabled bool                   `json:"enabled,omitempty"`
	Allow   []string               `json:"allow,omitempty"`
	Deny    []string               `json:"deny,omitempty"`
	Load    *PluginLoadConfig      `json:"load,omitempty"`
	Entries map[string]PluginEntry `json:"entries,omitempty"`
}

type PluginLoadConfig struct {
	Paths []string `json:"paths,omitempty"`
}

type PluginEntry struct {
	Enabled bool           `json:"enabled,omitempty"`
	Hooks   map[string]any `json:"hooks,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
}
