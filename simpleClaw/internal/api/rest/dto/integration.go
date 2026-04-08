package dto

type ConnectIntegrationRequest struct {
	ID                string         `json:"id,omitempty"`
	ExternalAccountID string         `json:"externalAccountId,omitempty"`
	DisplayName       string         `json:"displayName,omitempty"`
	SecretPayload     map[string]any `json:"secretPayload,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type IntegrationResponse struct {
	ID                string         `json:"id"`
	Capability        string         `json:"capability"`
	Provider          string         `json:"provider"`
	ExternalAccountID string         `json:"externalAccountId,omitempty"`
	DisplayName       string         `json:"displayName,omitempty"`
	Status            string         `json:"status"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}
