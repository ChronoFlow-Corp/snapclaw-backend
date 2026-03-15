package dto

import "time"

type UserInfoResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	NickName  string    `json:"nick_name"`
	AvatarURL string    `json:"avatar_url"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}
