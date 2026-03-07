package entities

import "simpleClaw/internal/entities/channels"

type ClawConfig struct {
	Env      Env           `json:"env,omitempty"`
	Auth     Auth          `json:"auth,omitempty"`
	Identity Identity      `json:"identity,omitempty"`
	Logging  Logging       `json:"logging,omitempty"`
	Messages Messages      `json:"messages,omitempty"`
	Routing  Routing       `json:"routing,omitempty"`
	Meta     ConfigMeta    `json:"meta,omitempty"`
	Tooling  ToolingMedia  `json:"tools,omitempty"` // см. замечание про duplicate keys ниже
	Tools    ToolsConfig   `json:"toolsRuntime,omitempty"`
	Session  SessionConfig `json:"session,omitempty"`
	Channels ClawChannels  `json:"channels,omitempty"`
	Agents   Agents        `json:"agents,omitempty"`
	Models   ModelsConfig  `json:"models,omitempty"`
	Cron     CronConfig    `json:"cron,omitempty"`
	Hooks    HooksConfig   `json:"hooks,omitempty"`
	Gateway  GatewayConfig `json:"gateway,omitempty"`
	Skills   SkillsConfig  `json:"skills,omitempty"`
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

type Identity struct {
	Name  string `json:"name,omitempty"`
	Theme string `json:"theme,omitempty"`
	Emoji string `json:"emoji,omitempty"`
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

type Routing struct {
	GroupChat GroupChatConfig `json:"groupChat,omitempty"`
	Queue     QueueConfig     `json:"queue,omitempty"`
}

type GroupChatConfig struct {
	MentionPatterns []string `json:"mentionPatterns,omitempty"`
	HistoryLimit    int      `json:"historyLimit,omitempty"`
}

type QueueConfig struct {
	Mode       string            `json:"mode,omitempty"`
	DebounceMs int               `json:"debounceMs,omitempty"`
	Cap        int               `json:"cap,omitempty"`
	Drop       string            `json:"drop,omitempty"`
	ByChannel  map[string]string `json:"byChannel,omitempty"`
}

type ToolingMedia struct {
	Media MediaTools `json:"media,omitempty"`
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

// ---------------- Tools (runtime / permissions) ----------------
// Примечание: в исходнике встречаются два объекта "tools". Я поместил второй в ToolsConfig и дал json tag "toolsRuntime" —
// при десериализации реального файла возможно нужно вручную объединить/переименовать ключи или использовать промежуточный map[string]json.RawMessage.
type ToolsConfig struct {
	Allow    []string      `json:"allow,omitempty"`
	Deny     []string      `json:"deny,omitempty"`
	Exec     ToolsExec     `json:"exec,omitempty"`
	Elevated ToolsElevated `json:"elevated,omitempty"`
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
	Defaults AgentDefaults `json:"defaults,omitempty"`
}

type AgentDefaults struct {
	Workspace             string               `json:"workspace,omitempty"`
	UserTimezone          string               `json:"userTimezone,omitempty"`
	Model                 AgentModelSelection  `json:"model,omitempty"`
	ImageModel            AgentModelSelection  `json:"imageModel,omitempty"`
	Models                map[string]ModelMeta `json:"models,omitempty"`
	ThinkingDefault       string               `json:"thinkingDefault,omitempty"`
	VerboseDefault        string               `json:"verboseDefault,omitempty"`
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

type SandboxConfig struct {
	Mode          string         `json:"mode,omitempty"`
	PerSession    bool           `json:"perSession,omitempty"`
	WorkspaceRoot string         `json:"workspaceRoot,omitempty"`
	Docker        SandboxDocker  `json:"docker,omitempty"`
	Browser       SandboxBrowser `json:"browser,omitempty"`
}

type SandboxDocker struct {
	Image        string   `json:"image,omitempty"`
	Workdir      string   `json:"workdir,omitempty"`
	ReadOnlyRoot bool     `json:"readOnlyRoot,omitempty"`
	Tmpfs        []string `json:"tmpfs,omitempty"`
	Network      string   `json:"network,omitempty"`
	User         string   `json:"user,omitempty"`
}

type SandboxBrowser struct {
	Enabled bool `json:"enabled"`
}

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

// ---------------- Cron ----------------
type CronConfig struct {
	Enabled           bool         `json:"enabled"`
	Store             string       `json:"store,omitempty"`
	MaxConcurrentRuns int          `json:"maxConcurrentRuns,omitempty"`
	SessionRetention  string       `json:"sessionRetention,omitempty"`
	RunLog            RunLogConfig `json:"runLog,omitempty"`
}

type RunLogConfig struct {
	MaxBytes  int `json:"maxBytes,omitempty"`
	KeepLines int `json:"keepLines,omitempty"`
}

// ---------------- Hooks ----------------
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

type GatewayConfig struct {
	Mode      string                 `json:"mode,omitempty"`
	Port      int                    `json:"port,omitempty"`
	Bind      string                 `json:"bind,omitempty"`
	ControlUI ControlUI              `json:"controlUi,omitempty"`
	Auth      GatewayAuth            `json:"auth,omitempty"`
	Tailscale map[string]interface{} `json:"tailscale,omitempty"`
	Remote    map[string]string      `json:"remote,omitempty"`
	Reload    ReloadConfig           `json:"reload,omitempty"`
}

type ControlUI struct {
	Enabled  bool   `json:"enabled"`
	BasePath string `json:"basePath,omitempty"`
}

type GatewayAuth struct {
	Mode           string `json:"mode,omitempty"`
	Token          string `json:"token,omitempty"`
	AllowTailscale bool   `json:"allowTailscale,omitempty"`
}

type ReloadConfig struct {
	Mode       string `json:"mode,omitempty"`
	DebounceMs int    `json:"debounceMs,omitempty"`
}

type SkillsConfig struct {
	AllowBundled []string              `json:"allowBundled,omitempty"`
	Load         SkillsLoad            `json:"load,omitempty"`
	Install      SkillsInstall         `json:"install,omitempty"`
	Entries      map[string]SkillEntry `json:"entries,omitempty"`
}

type SkillsLoad struct {
	ExtraDirs []string `json:"extraDirs,omitempty"`
}

type SkillsInstall struct {
	PreferBrew  bool   `json:"preferBrew,omitempty"`
	NodeManager string `json:"nodeManager,omitempty"`
}

type SkillEntry struct {
	Enabled bool              `json:"enabled,omitempty"`
	APIKey  string            `json:"apiKey,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}
