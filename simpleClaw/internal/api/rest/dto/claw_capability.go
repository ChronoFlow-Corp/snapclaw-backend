package dto

type AttachClawCapabilityRequest struct {
	Enabled              bool           `json:"enabled"`
	Provider             string         `json:"provider,omitempty"`
	AccountIntegrationID string         `json:"accountIntegrationId,omitempty"`
	Settings             map[string]any `json:"settings,omitempty"`
}
