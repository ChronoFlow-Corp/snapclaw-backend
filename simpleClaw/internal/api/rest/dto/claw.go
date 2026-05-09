package dto

type CreateClawRequest struct {
	Name         string                         `json:"name"`
	Model        string                         `json:"model"`
	ChannelIDs   []string                       `json:"channelIds"`
	ApiLimits    *ApiLimitRequest               `json:"apiLimits,omitempty"`
	Capabilities *CreateClawCapabilitiesRequest `json:"capabilities,omitempty"`
}

type CreateClawCapabilitiesRequest struct {
	WebSearch      *WebSearchCapabilityRequest        `json:"webSearch,omitempty"`
	FilesImages    *ToggleCapabilityRequest           `json:"filesImages,omitempty"`
	Memory         *ToggleCapabilityRequest           `json:"memory,omitempty"`
	Gmail          *IntegrationBoundCapabilityRequest `json:"gmail,omitempty"`
	GoogleCalendar *IntegrationBoundCapabilityRequest `json:"googleCalendar,omitempty"`
	Sheets         *IntegrationBoundCapabilityRequest `json:"sheets,omitempty"`
}

type WebSearchCapabilityRequest struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider,omitempty"`
}

type ToggleCapabilityRequest struct {
	Enabled bool `json:"enabled"`
}

type IntegrationBoundCapabilityRequest struct {
	Enabled              bool   `json:"enabled"`
	Provider             string `json:"provider,omitempty"`
	AccountIntegrationID string `json:"accountIntegrationId,omitempty"`
}

type ApiLimitRequest struct {
	MonthlyBudgetUSD float64 `json:"monthlyBudgetUsd"`
}

type UpdateClawRequest struct {
	Name       *string          `json:"name,omitempty"`
	Model      *string          `json:"model,omitempty"`
	ChannelIDs []string         `json:"channelIds,omitempty"`
	ApiLimits  *ApiLimitRequest `json:"apiLimits,omitempty"`
}

type ApprovePairingRequest struct {
	Code        string `json:"code"`
	ChannelType string `json:"channelType,omitempty"`
}

type ClawResponse struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	ModelKey           string `json:"modelKey,omitempty"`
	ResolvedModel      string `json:"resolvedModel,omitempty"`
	DesiredState       string `json:"desiredState"`
	ObservedState      string `json:"observedState"`
	LifecycleStatus    string `json:"lifecycleStatus"`
	CurrentOperationID string `json:"currentOperationId,omitempty"`
	LastError          string `json:"lastError,omitempty"`
}

type CreateClawResponse = ClawResponse
