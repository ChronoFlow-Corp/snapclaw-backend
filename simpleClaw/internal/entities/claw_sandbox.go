package entities

type SandboxConfig struct {
	Mode          string         `json:"mode,omitempty"`
	PerSession    bool           `json:"perSession,omitempty"`
	WorkspaceRoot string         `json:"workspaceRoot,omitempty"`
	Docker        SandboxDocker  `json:"docker,omitempty"`
	Browser       SandboxBrowser `json:"browser,omitempty"`
}

type SandboxDocker struct {
	Image        string   `json:"image,omitempty"`
	Workdir      string   `json:"workdir,omitempty"`
	ReadOnlyRoot bool     `json:"readOnlyRoot,omitempty"`
	Tmpfs        []string `json:"tmpfs,omitempty"`
	Network      string   `json:"network,omitempty"`
	User         string   `json:"user,omitempty"`
}

type SandboxBrowser struct {
	Enabled bool `json:"enabled"`
}
