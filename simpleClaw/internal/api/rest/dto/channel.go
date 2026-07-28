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

type AddChannelResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	BotToken  string   `json:"botToken"`
	DmPolicy  string   `json:"dmPolicy"`
	AllowFrom []string `json:"allowFrom"`
}
