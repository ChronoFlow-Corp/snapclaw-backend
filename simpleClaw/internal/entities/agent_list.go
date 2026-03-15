package entities

type AgentListConfig struct {
	ID            string               `json:"id,omitempty"`
	Default       bool                 `json:"default,omitempty"`
	Name          string               `json:"name,omitempty"`
	Workspace     string               `json:"workspace,omitempty"`
	AgentDir      string               `json:"agentDir,omitempty"`
	SkipBootstrap bool                 `json:"skipBootstrap,omitempty"`
	Model         *AgentModelSelection `json:"model,omitempty"`
	Params        *Params              `json:"params,omitempty"`
	Identity      *Identity            `json:"identity,omitempty"`
	GroupChat     *GroupChat           `json:"groupChat,omitempty"`
	Sandbox       *Sandbox             `json:"sandbox,omitempty"`
	Runtime       *Runtime             `json:"runtime,omitempty"`
	Subagents     *Subagents           `json:"subagents,omitempty"`
	Tools         *Tools               `json:"tools,omitempty"`
}

type Params struct {
	CacheRetention string `json:"cacheRetention,omitempty"`
}

type Identity struct {
	Name   string `json:"name,omitempty"`
	Theme  string `json:"theme,omitempty"`
	Emoji  string `json:"emoji,omitempty"`
	Avatar string `json:"avatar,omitempty"`
}

type GroupChat struct {
	MentionPatterns []string `json:"mentionPatterns,omitempty"`
}

type Sandbox struct {
	Mode string `json:"mode,omitempty"`
}

type Runtime struct {
	Type string `json:"type,omitempty"`
	ACP  *ACP   `json:"acp,omitempty"`
}

type ACP struct {
	Agent   string `json:"agent,omitempty"`
	Backend string `json:"backend,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
}

type Subagents struct {
	AllowAgents []string `json:"allowAgents,omitempty"`
}

type Tools struct {
	Profile  string    `json:"profile,omitempty"`
	Allow    []string  `json:"allow,omitempty"`
	Deny     []string  `json:"deny,omitempty"`
	Elevated *Elevated `json:"elevated,omitempty"`
}

type Elevated struct {
	Enabled bool `json:"enabled,omitempty"`
}
