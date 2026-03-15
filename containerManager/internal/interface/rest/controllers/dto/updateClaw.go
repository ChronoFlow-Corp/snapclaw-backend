package dto

type UpdateClaw struct {
	UserID     string       `json:"userId"`
	ClawID     string       `json:"clawId"`
	Vars       []string     `json:"vars,omitempty"`
	ClawConfig []ClawConfig `json:"clawConfig"`
}
