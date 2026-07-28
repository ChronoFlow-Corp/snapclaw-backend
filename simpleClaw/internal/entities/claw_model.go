package entities

type ModelsConfig struct {
	Mode      string                         `json:"mode,omitempty"`
	Providers map[string]ModelProviderConfig `json:"providers,omitempty"`
}

type ModelProviderConfig struct {
	BaseURL    string            `json:"baseUrl,omitempty"`
	APIKey     string            `json:"apiKey,omitempty"`
	API        string            `json:"api,omitempty"`
	AuthHeader bool              `json:"authHeader,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Models     []ProviderModel   `json:"models,omitempty"`
}

type ProviderModel struct {
	ID            string             `json:"id,omitempty"`
	Name          string             `json:"name,omitempty"`
	API           string             `json:"api,omitempty"`
	Reasoning     bool               `json:"reasoning,omitempty"`
	Input         []string           `json:"input,omitempty"`
	Cost          map[string]float64 `json:"cost,omitempty"`
	ContextWindow int                `json:"contextWindow,omitempty"`
	MaxTokens     int                `json:"maxTokens,omitempty"`
}
