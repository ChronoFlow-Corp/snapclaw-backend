package channels

type (
	ReplyToMode string
	Streaming   string
	Reaction    string
)

const (
	ReplyOff   ReplyToMode = "off"
	ReplyFirst ReplyToMode = "first"
	ReplyAll   ReplyToMode = "all"
)

const (
	StreamingOff      Streaming = "off"
	StreamingPartial  Streaming = "partial"
	StreamingBlock    Streaming = "block"
	StreamingProgress Streaming = "progress"
)

const (
	ReactionOff Reaction = "off"
	ReactionOwn Reaction = "own"
	ReactionAll Reaction = "all"
)

type TelegramConfig struct {
	DmPolicy              DmPolicy                `json:"dmPolicy,omitempty"`
	AllowFrom             []string                `json:"allowFrom,omitempty"`
	Enabled               bool                    `json:"enabled"`
	BotToken              string                  `json:"botToken,omitempty"`
	GroupPolicy           string                  `json:"groupPolicy,omitempty"`
	CustomCommands        []TelegramCustomCommand `json:"customCommands,omitempty"`
	LinkPreview           bool                    `json:"linkPreview,omitempty"`
	Streaming             Streaming               `json:"streaming,omitempty"`
	Actions               Actions                 `json:"actions,omitempty"`
	ReactionNotifications Reaction                `json:"reactionNotifications,omitempty"`
	MediaMaxMb            int                     `json:"mediaMaxMb,omitempty"`
	GroupAllowFrom        []string                `json:"groupAllowFrom,omitempty"`
	HistoryLimit          int                     `json:"historyLimit,omitempty"`
	ReplyToMode           string                  `json:"replyToMode,omitempty"`
	// TODO: add groups type
	Groups map[string]interface{} `json:"groups,omitempty"`
}

type TelegramCustomCommand struct {
	Command     string `json:"command,omitempty"`
	Description string `json:"description,omitempty"`
}

type Actions struct {
	Reactions   bool `json:"reactions,omitempty"`
	SendMessage bool `json:"sendMessage,omitempty"`
}
