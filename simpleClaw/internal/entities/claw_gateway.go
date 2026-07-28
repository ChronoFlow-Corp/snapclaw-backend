package entities

type GatewayConfig struct {
	Mode      string                 `json:"mode,omitempty"`
	Port      int                    `json:"port,omitempty"`
	Bind      string                 `json:"bind,omitempty"`
	ControlUI ControlUI              `json:"controlUi,omitempty"`
	Auth      GatewayAuth            `json:"auth,omitempty"`
	Tailscale map[string]interface{} `json:"tailscale,omitempty"`
	Remote    map[string]string      `json:"remote,omitempty"`
	Reload    ReloadConfig           `json:"reload,omitempty"`
}

type ControlUI struct {
	Enabled  bool   `json:"enabled"`
	BasePath string `json:"basePath,omitempty"`
}

type GatewayAuth struct {
	Mode           string `json:"mode,omitempty"`
	Token          string `json:"token,omitempty"`
	AllowTailscale bool   `json:"allowTailscale,omitempty"`
}

type ReloadConfig struct {
	Mode       string `json:"mode,omitempty"`
	DebounceMs int    `json:"debounceMs,omitempty"`
}
