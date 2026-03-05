package dto

type UpdateClaw struct {
	UserID      string       `json:"userId"`
	ContainerID string       `json:"containerId"`
	ClawConfig  []ClawConfig `json:"clawConfig"`
}
