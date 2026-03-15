package configurer

import "time"

const (
	pendingPairingFileName  = "telegram-pairing.json"
	allowedFileNameTelegram = "telegram-allowFrom.json"
)

type PairingTelegramConfig struct {
	Version  int       `json:"version"`
	Requests []Request `json:"requests"`
}

type Request struct {
	ID        string       `json:"id"`
	Code      string       `json:"code"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
	Meta      MetaTelegram `json:"meta"`
}

type MetaTelegram struct {
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	AccountID string `json:"accountId"`
}

type TelegramPairedAllowFrom struct {
	Version   int      `json:"version"`
	AllowFrom []string `json:"allowFrom"`
}
