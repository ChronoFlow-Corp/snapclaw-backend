package entities

import (
	"path"

	"simpleClaw/internal/entities/channels"
)

const (
	workspacePrefix = "./workspace/"
	agentDirPrefix  = "./agent/"
)

type ClawConfig struct {
	Env      Env            `json:"env,omitempty"`
	Auth     *Auth          `json:"auth,omitempty"`
	Logging  *Logging       `json:"logging,omitempty"`
	Messages *Messages      `json:"messages,omitempty"`
	Meta     *ConfigMeta    `json:"meta,omitempty"`
	Plugins  *PluginsConfig `json:"plugins,omitempty"`
	Tools    *ToolsConfig   `json:"tools,omitempty"`
	Session  *SessionConfig `json:"session,omitempty"`
	Channels *ClawChannels  `json:"channels,omitempty"`
	Agents   Agents         `json:"agents,omitempty"`
	Models   *ModelsConfig  `json:"models,omitempty"`
	Cron     *CronConfig    `json:"cron,omitempty"`
	Hooks    *HooksConfig   `json:"hooks,omitempty"`
	Gateway  *GatewayConfig `json:"gateway,omitempty"`
	Skills   *SkillsConfig  `json:"skills,omitempty"`
}

type ConfigMeta struct {
	LastTouchedVersion string `json:"lastTouchedVersion,omitempty"`
	LastTouchedAt      string `json:"lastTouchedAt,omitempty"`
}

type Env struct {
	OpenRouterAPIKey string            `json:"OPENROUTER_API_KEY,omitempty"`
	Vars             map[string]string `json:"vars,omitempty"`
	ShellEnv         ShellEnvConfig    `json:"shellEnv,omitempty"`
}

type ShellEnvConfig struct {
	Enabled   bool `json:"enabled"`
	TimeoutMs int  `json:"timeoutMs,omitempty"`
}
type Auth struct {
	Profiles map[string]AuthProfile `json:"profiles,omitempty"`
	Order    map[string][]string    `json:"order,omitempty"`
}

type AuthProfile struct {
	Provider string `json:"provider,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Email    string `json:"email,omitempty"`
	// secrets live elsewhere (auth-profiles.json)
}

type Logging struct {
	Level           string `json:"level,omitempty"`
	File            string `json:"file,omitempty"`
	ConsoleLevel    string `json:"consoleLevel,omitempty"`
	ConsoleStyle    string `json:"consoleStyle,omitempty"`
	RedactSensitive string `json:"redactSensitive,omitempty"`
}

type Messages struct {
	MessagePrefix    string `json:"messagePrefix,omitempty"`
	ResponsePrefix   string `json:"responsePrefix,omitempty"`
	AckReaction      string `json:"ackReaction,omitempty"`
	AckReactionScope string `json:"ackReactionScope,omitempty"`
}

type QueueConfig struct {
	Mode       string            `json:"mode,omitempty"`
	DebounceMs int               `json:"debounceMs,omitempty"`
	Cap        int               `json:"cap,omitempty"`
	Drop       string            `json:"drop,omitempty"`
	ByChannel  map[string]string `json:"byChannel,omitempty"`
}

type MediaTools struct {
	Audio MediaAudio `json:"audio,omitempty"`
	Video MediaVideo `json:"video,omitempty"`
}

type MediaAudio struct {
	Enabled        bool         `json:"enabled"`
	MaxBytes       int64        `json:"maxBytes,omitempty"`
	Models         []MediaModel `json:"models,omitempty"`
	TimeoutSeconds int          `json:"timeoutSeconds,omitempty"`
}

type MediaVideo struct {
	Enabled  bool         `json:"enabled"`
	MaxBytes int64        `json:"maxBytes,omitempty"`
	Models   []MediaModel `json:"models,omitempty"`
}

type MediaModel struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// for CLI fallback one could use Type/Command/Args etc.
}

type ToolsConfig struct {
	Allow    []string        `json:"allow,omitempty"`
	Deny     []string        `json:"deny,omitempty"`
	Exec     *ToolsExec      `json:"exec,omitempty"`
	Elevated *ToolsElevated  `json:"elevated,omitempty"`
	Web      *WebToolsConfig `json:"web,omitempty"`
	Media    MediaTools      `json:"media,omitempty"`
}
type ToolsExec struct {
	BackgroundMs int `json:"backgroundMs,omitempty"`
	TimeoutSec   int `json:"timeoutSec,omitempty"`
	CleanupMs    int `json:"cleanupMs,omitempty"`
}

type ToolsElevated struct {
	Enabled   bool                `json:"enabled"`
	AllowFrom map[string][]string `json:"allowFrom,omitempty"`
}

type SessionConfig struct {
	Scope                 string                        `json:"scope,omitempty"`
	Reset                 SessionResetConfig            `json:"reset,omitempty"`
	ResetByChannel        map[string]SessionResetConfig `json:"resetByChannel,omitempty"`
	ResetTriggers         []string                      `json:"resetTriggers,omitempty"`
	Store                 string                        `json:"store,omitempty"`
	Maintenance           SessionMaintenance            `json:"maintenance,omitempty"`
	TypingIntervalSeconds int                           `json:"typingIntervalSeconds,omitempty"`
	SendPolicy            SendPolicyConfig              `json:"sendPolicy,omitempty"`
}

type SessionResetConfig struct {
	Mode        string `json:"mode,omitempty"`
	AtHour      int    `json:"atHour,omitempty"`
	IdleMinutes int    `json:"idleMinutes,omitempty"`
}

type SessionMaintenance struct {
	Mode                  string `json:"mode,omitempty"`
	PruneAfter            string `json:"pruneAfter,omitempty"` // формат "30d" — оставить string или создать custom Duration
	MaxEntries            int    `json:"maxEntries,omitempty"`
	RotateBytes           string `json:"rotateBytes,omitempty"`
	ResetArchiveRetention string `json:"resetArchiveRetention,omitempty"`
	MaxDiskBytes          string `json:"maxDiskBytes,omitempty"`
	HighWaterBytes        string `json:"highWaterBytes,omitempty"`
}

type SendPolicyConfig struct {
	Default string           `json:"default,omitempty"`
	Rules   []SendPolicyRule `json:"rules,omitempty"`
}

type SendPolicyRule struct {
	Action string                 `json:"action,omitempty"`
	Match  map[string]interface{} `json:"match,omitempty"`
}

type ClawChannels struct {
	WhatsApp *WhatsAppConfig          `json:"whatsapp,omitempty"`
	Telegram *channels.TelegramConfig `json:"telegram,omitempty"`
	Discord  *DiscordConfig           `json:"discord,omitempty"`
	Slack    *SlackConfig             `json:"slack,omitempty"`
}

type WhatsAppConfig struct {
	DmPolicy       string   `json:"dmPolicy,omitempty"`
	AllowFrom      []string `json:"allowFrom,omitempty"`
	GroupPolicy    string   `json:"groupPolicy,omitempty"`
	GroupAllowFrom []string `json:"groupAllowFrom,omitempty"`
	// TODO: add groups type
	Groups map[string]interface{} `json:"groups,omitempty"`
}

type DiscordConfig struct {
	Enabled bool                    `json:"enabled"`
	Token   string                  `json:"token,omitempty"`
	Dm      DiscordDMConfig         `json:"dm,omitempty"`
	Guilds  map[string]DiscordGuild `json:"guilds,omitempty"`
}

type DiscordDMConfig struct {
	Enabled   bool     `json:"enabled"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

type DiscordGuild struct {
	Slug           string                  `json:"slug,omitempty"`
	RequireMention bool                    `json:"requireMention,omitempty"`
	Channels       map[string]GuildChannel `json:"channels,omitempty"`
}

type GuildChannel struct {
	Allow          bool `json:"allow,omitempty"`
	RequireMention bool `json:"requireMention,omitempty"`
}

type SlackConfig struct {
	Enabled      bool                     `json:"enabled"`
	BotToken     string                   `json:"botToken,omitempty"`
	AppToken     string                   `json:"appToken,omitempty"`
	Channels     map[string]ChannelPolicy `json:"channels,omitempty"`
	Dm           SlackDMConfig            `json:"dm,omitempty"`
	SlashCommand SlackSlashCommand        `json:"slashCommand,omitempty"`
}

type ChannelPolicy struct {
	Allow          bool `json:"allow,omitempty"`
	RequireMention bool `json:"requireMention,omitempty"`
}

type SlackDMConfig struct {
	Enabled   bool     `json:"enabled"`
	AllowFrom []string `json:"allowFrom,omitempty"`
}

type SlackSlashCommand struct {
	Enabled       bool   `json:"enabled"`
	Name          string `json:"name,omitempty"`
	SessionPrefix string `json:"sessionPrefix,omitempty"`
	Ephemeral     bool   `json:"ephemeral,omitempty"`
}

type Agents struct {
	List []AgentListConfig `json:"list,omitempty"`
}

type AgentDefault struct {
	Workspace             string               `json:"workspace,omitempty"`
	UserTimezone          string               `json:"userTimezone,omitempty"`
	Model                 AgentModelSelection  `json:"model,omitempty"`
	ImageModel            AgentModelSelection  `json:"imageModel,omitempty"`
	Models                map[string]ModelMeta `json:"models,omitempty"`
	ThinkingDefault       string               `json:"thinkingDefault,omitempty"`
	VerboseDefault        string               `json:"verboseDefault,omitempty"`
	SkipBootstrap         bool                 `json:"skipBootstrap,omitempty"`
	Compaction            AgentCompaction      `json:"compaction,omitempty"`
	SubAgents             Subagents            `json:"subagents,omitempty"`
	ElevatedDefault       string               `json:"elevatedDefault,omitempty"`
	BlockStreamingDefault string               `json:"blockStreamingDefault,omitempty"`
	TimeoutSeconds        int                  `json:"timeoutSeconds,omitempty"`
	MediaMaxMb            int                  `json:"mediaMaxMb,omitempty"`
	TypingIntervalSeconds int                  `json:"typingIntervalSeconds,omitempty"`
	MaxConcurrent         int                  `json:"maxConcurrent,omitempty"`
	Heartbeat             HeartbeatConfig      `json:"heartbeat,omitempty"`
	MemorySearch          MemorySearchConfig   `json:"memorySearch,omitempty"`
	Sandbox               SandboxConfig        `json:"sandbox,omitempty"`
	// и т.д.
}

type AgentCompaction struct {
	ReserveTokensFloor int `json:"reserveTokensFloor,omitempty"`
	MemoryFlush        struct {
		Enabled              bool `json:"enabled"`
		SoftThreshHoldTokens int  `json:"softThresholdTokens,omitempty"`
	} `json:"memoryFlush,omitempty"`
}

type AgentModelSelection struct {
	Primary   string   `json:"primary,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

type ModelMeta struct {
	Alias string `json:"alias,omitempty"`
}

type HeartbeatConfig struct {
	Every        string `json:"every,omitempty"`
	Model        string `json:"model,omitempty"`
	Target       string `json:"target,omitempty"`
	DirectPolicy string `json:"directPolicy,omitempty"`
	To           string `json:"to,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
	AckMaxChars  int    `json:"ackMaxChars,omitempty"`
}

type MemorySearchConfig struct {
	Provider   string            `json:"provider,omitempty"`
	Model      string            `json:"model,omitempty"`
	Remote     map[string]string `json:"remote,omitempty"`
	ExtraPaths []string          `json:"extraPaths,omitempty"`
}

type CronConfig struct {
	Enabled           bool         `json:"enabled"`
	Store             string       `json:"store,omitempty"`
	MaxConcurrentRuns int          `json:"maxConcurrentRuns,omitempty"`
	SessionRetention  string       `json:"sessionRetention,omitempty"`
	RunLog            RunLogConfig `json:"runLog,omitempty"`
}

type RunLogConfig struct {
	MaxBytes  string `json:"maxBytes,omitempty"`
	KeepLines int    `json:"keepLines,omitempty"`
}

func NewDefaultClawConfig(model string) ClawConfig {
	return NewCreateClawConfig(CreateClawConfigInput{PrimaryModel: model})
}

func (c *ClawConfig) AddTelegramChannel(ch *channels.TelegramConfig) {
	if c.Channels == nil {
		c.Channels = &ClawChannels{}
	}

	c.Channels.Telegram = NewTelegramChannelConfig(ch)

	if c.Channels.Telegram != nil {
		c.Channels.Telegram.Enabled = true
	}
}

type CreateClawConfigInput struct {
	PrimaryModel string
}

func NewCreateClawConfig(input CreateClawConfigInput) ClawConfig {
	return ClawConfig{
		Env:     NewDefaultEnvConfig(),
		Logging: NewDefaultLoggingConfig(),
		Tools:   NewDefaultToolsConfig(),
		Session: NewDefaultSessionConfig(),
		Agents: Agents{
			List: []AgentListConfig{
				NewDefaultMainAgentConfig(input.PrimaryModel),
			},
		},
		Cron:    NewDefaultCronConfig(),
		Gateway: NewDefaultGatewayConfig(),
	}
}

func NewDefaultEnvConfig() Env {
	return Env{
		OpenRouterAPIKey: "${OPENROUTER_API_KEY}",
		Vars:             map[string]string{},
		ShellEnv: ShellEnvConfig{
			Enabled:   true,
			TimeoutMs: 30000,
		},
	}
}

func NewDefaultLoggingConfig() *Logging {
	return &Logging{
		Level:        "info",
		ConsoleLevel: "info",
		ConsoleStyle: "pretty",
	}
}

func NewDefaultToolsConfig() *ToolsConfig {
	return &ToolsConfig{
		Media: MediaTools{
			Audio: MediaAudio{
				Enabled:        false,
				MaxBytes:       10 * 1024 * 1024,
				TimeoutSeconds: 60,
			},
			Video: MediaVideo{
				Enabled:  false,
				MaxBytes: 50 * 1024 * 1024,
			},
		},
	}
}

func NewDefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		Scope: "per-sender",
		Reset: SessionResetConfig{
			Mode:        "idle",
			IdleMinutes: 60,
		},
		Store: "file",
		Maintenance: SessionMaintenance{
			Mode:       "warn",
			PruneAfter: "30d",
			MaxEntries: 1000,
		},
		TypingIntervalSeconds: 3,
		SendPolicy: SendPolicyConfig{
			Default: "allow",
		},
	}
}

func NewDefaultMainAgentConfig(model string) AgentListConfig {
	return AgentListConfig{
		ID:            "main",
		Default:       true,
		Name:          "main",
		Workspace:     path.Join(workspacePrefix, "main"),
		AgentDir:      path.Join(agentDirPrefix, "agents"),
		SkipBootstrap: false,
		Model: &AgentModelSelection{
			Primary: model,
		},
		GroupChat: &GroupChat{
			MentionPatterns: []string{"@OpenClaw"},
		},
		Sandbox: &Sandbox{
			Mode: "off",
		},
		Subagents: &Subagents{
			AllowAgents: []string{"*"},
		},
		Tools: &Tools{
			Allow: []string{
				"group:openclaw",
			},
			Deny: []string{"canvas", "browser"},
		},
	}
}

func NewDefaultGatewayConfig() *GatewayConfig {
	return &GatewayConfig{
		Mode: "local",
		Bind: "lan",
		ControlUI: ControlUI{
			Enabled: false,
		},
		Auth: GatewayAuth{
			Mode:  "token",
			Token: "${OPENCLAW_GATEWAY_TOKEN}",
		},
		Reload: ReloadConfig{
			Mode:       "restart",
			DebounceMs: 2000,
		},
	}
}

func NewDefaultCronConfig() *CronConfig {
	return &CronConfig{
		Enabled:           true,
		MaxConcurrentRuns: 2,
		SessionRetention:  "24h",
		RunLog: RunLogConfig{
			MaxBytes:  "2mb",
			KeepLines: 1000,
		},
	}
}

func NewTelegramChannelConfig(src *channels.TelegramConfig) *channels.TelegramConfig {
	if src == nil {
		return nil
	}

	return &channels.TelegramConfig{
		Enabled:               true,
		BotToken:              src.BotToken,
		DmPolicy:              src.DmPolicy,
		AllowFrom:             append([]string(nil), src.AllowFrom...),
		GroupPolicy:           src.GroupPolicy,
		GroupAllowFrom:        append([]string(nil), src.GroupAllowFrom...),
		Groups:                cloneMap(src.Groups),
		CustomCommands:        append([]channels.TelegramCustomCommand(nil), src.CustomCommands...),
		HistoryLimit:          src.HistoryLimit,
		ReplyToMode:           src.ReplyToMode,
		LinkPreview:           src.LinkPreview,
		Streaming:             src.Streaming,
		Actions:               src.Actions,
		ReactionNotifications: src.ReactionNotifications,
		MediaMaxMb:            src.MediaMaxMb,
	}
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}
