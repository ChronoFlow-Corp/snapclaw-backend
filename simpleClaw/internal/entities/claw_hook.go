package entities

type HooksConfig struct {
	Enabled       bool            `json:"enabled"`
	Path          string          `json:"path,omitempty"`
	Token         string          `json:"token,omitempty"`
	Presets       []string        `json:"presets,omitempty"`
	TransformsDir string          `json:"transformsDir,omitempty"`
	Mappings      []HookMapping   `json:"mappings,omitempty"`
	Gmail         GmailHookConfig `json:"gmail,omitempty"`
}

type HookMapping struct {
	ID              string                 `json:"id,omitempty"`
	Match           map[string]interface{} `json:"match,omitempty"`
	Action          string                 `json:"action,omitempty"`
	WakeMode        string                 `json:"wakeMode,omitempty"`
	Name            string                 `json:"name,omitempty"`
	SessionKey      string                 `json:"sessionKey,omitempty"`
	MessageTemplate string                 `json:"messageTemplate,omitempty"`
	TextTemplate    string                 `json:"textTemplate,omitempty"`
	Deliver         bool                   `json:"deliver,omitempty"`
	Channel         string                 `json:"channel,omitempty"`
	To              string                 `json:"to,omitempty"`
	Thinking        string                 `json:"thinking,omitempty"`
	TimeoutSeconds  int                    `json:"timeoutSeconds,omitempty"`
	Transform       HookTransform          `json:"transform,omitempty"`
}

type HookTransform struct {
	Module string `json:"module,omitempty"`
	Export string `json:"export,omitempty"`
}

type GmailHookConfig struct {
	Account           string            `json:"account,omitempty"`
	Label             string            `json:"label,omitempty"`
	Topic             string            `json:"topic,omitempty"`
	Subscription      string            `json:"subscription,omitempty"`
	PushToken         string            `json:"pushToken,omitempty"`
	HookUrl           string            `json:"hookUrl,omitempty"`
	IncludeBody       bool              `json:"includeBody,omitempty"`
	MaxBytes          int               `json:"maxBytes,omitempty"`
	RenewEveryMinutes int               `json:"renewEveryMinutes,omitempty"`
	Serve             ServeConfig       `json:"serve,omitempty"`
	Tailscale         map[string]string `json:"tailscale,omitempty"`
}

type ServeConfig struct {
	Bind string `json:"bind,omitempty"`
	Port int    `json:"port,omitempty"`
	Path string `json:"path,omitempty"`
}
