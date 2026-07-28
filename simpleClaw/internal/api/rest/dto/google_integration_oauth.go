package dto

type GoogleIntegrationOAuthStartRequest struct {
	Capabilities []string `json:"capabilities"`
	ReturnTo     string   `json:"returnTo,omitempty"`
}

type GoogleIntegrationOAuthStartResponse struct {
	AuthURL      string   `json:"authUrl"`
	Capabilities []string `json:"capabilities"`
}
