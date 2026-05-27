package dto

type TelegramManagerCreateLinkRequest struct {
	ClawID string `json:"claw_id"`
}

type TelegramManagerCreateLinkResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	DeepLinkURL string `json:"deepLinkUrl"`
}

type TelegramManagerStatusResponse struct {
	ID                 string `json:"id"`
	Status             string `json:"status"`
	ChannelID          string `json:"channelId,omitempty"`
	ManagedBotUserID   int64  `json:"managedBotUserId,omitempty"`
	ManagedBotName     string `json:"managedBotName,omitempty"`
	ManagedBotUsername string `json:"managedBotUsername,omitempty"`
}
