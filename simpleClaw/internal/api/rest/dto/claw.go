package dto

type CreateClawRequest struct {
	Name       string           `json:"name"`
	Model      string           `json:"model"`
	ChannelIDs []string         `json:"channelIds"`
	ApiLimits  *ApiLimitRequest `json:"apiLimits,omitempty"`
}

type ApiLimitRequest struct {
	MonthlyBudgetUSD float64 `json:"monthlyBudgetUsd"`
}

type UpdateClawRequest struct {
	Name       string           `json:"name"`
	Model      string           `json:"model"`
	ChannelIDs []string         `json:"channelIds"`
	ApiLimits  *ApiLimitRequest `json:"apiLimits,omitempty"`
}
