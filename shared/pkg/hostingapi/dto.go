package hostingapi

type CreateClawRequest struct {
	UserID     string           `json:"userId"`
	ClawID     string           `json:"clawId"`
	Vars       []string         `json:"vars,omitempty"`
	ClawConfig []ClawConfigFile `json:"clawConfig"`
}

type UpdateClawRequest struct {
	UserID     string           `json:"userId"`
	ClawID     string           `json:"clawId"`
	Vars       []string         `json:"vars,omitempty"`
	ClawConfig []ClawConfigFile `json:"clawConfig"`
}

type ClawConfigFile struct {
	Name       string           `json:"name"`
	Data       string           `json:"data,omitempty"`
	FileType   string           `json:"fileType"`
	ClawConfig []ClawConfigFile `json:"clawConfig,omitempty"`
}

type CreateClawResponse struct {
	ContainerID string `json:"containerId"`
	ServerID    string `json:"serverId,omitempty"`
	Status      string `json:"status,omitempty"`
}

type CapacityResponse struct {
	MaxClaws int `json:"maxClaws"`
}
