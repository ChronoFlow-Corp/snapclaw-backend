package entities

type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Expiry       string `json:"expiry"`
}

type GmailToken struct {
	Email  string `json:"email"`
	Client string `json:"client"`
	Token  Token  `json:"token"`
}

type GmailConnect struct{}
