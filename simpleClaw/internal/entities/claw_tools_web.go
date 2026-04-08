package entities

type WebToolsConfig struct {
	Search *WebSearchToolConfig `json:"search,omitempty"`
}

type WebSearchToolConfig struct {
	Enabled         bool   `json:"enabled"`
	Provider        string `json:"provider,omitempty"`
	MaxResults      int    `json:"maxResults,omitempty"`
	TimeoutSeconds  int    `json:"timeoutSeconds,omitempty"`
	CacheTTLMinutes int    `json:"cacheTtlMinutes,omitempty"`
}
