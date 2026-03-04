package dto

type AddChannelRequest struct {
	Name            string          `json:"name"`
	TelegramChannel TelegramChannel `json:"telegramChannel"`
}

type TelegramChannel struct {
	DmPolicy  string   `json:"dmPolicy"`
	BotToken  string   `json:"botToken"`
	AllowFrom []string `json:"allowFrom"`
}
