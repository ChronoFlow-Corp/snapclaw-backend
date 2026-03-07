package dto

type CreateClaw struct {
	UserID     string       `json:"userId"`
	ClawID     string       `json:"clawId"`
	ClawConfig []ClawConfig `json:"clawConfig"`
}

type ClawConfig struct {
	ClawConfig []ClawConfig `json:"clawConfig"`
	Name       string       `json:"name"`
	Data       string       `json:"data"`
	FileType   string       `json:"fileType"`
}

type CreateClawResponse struct {
	ContainerID string `json:"containerId"`
}
