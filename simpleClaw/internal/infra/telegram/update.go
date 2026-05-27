package telegram

type GetUpdatesParams struct {
	Offset         int64
	Limit          int
	TimeoutSeconds int
	AllowedUpdates []string
}

type Update struct {
	UpdateID   int64              `json:"update_id"`
	Message    *Message           `json:"message,omitempty"`
	ManagedBot *ManagedBotUpdated `json:"managed_bot,omitempty"`
}

type Message struct {
	Text string `json:"text,omitempty"`
	Chat Chat   `json:"chat"`
	From *User  `json:"from,omitempty"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type ManagedBotUpdated struct {
	User User `json:"user"`
	Bot  User `json:"bot"`
}
