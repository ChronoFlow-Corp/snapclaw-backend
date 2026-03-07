package dto

type UpdateClaw struct {
	UserID     string       `json:"userId"`
	ClawID     string       `json:"clawId"`
	ClawConfig []ClawConfig `json:"clawConfig"`
}
