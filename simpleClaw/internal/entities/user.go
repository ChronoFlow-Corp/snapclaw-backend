package entities

import (
	"time"

	"github.com/google/uuid"
)

const (
	UserRole  = "user"
	AdminRole = "admin"
)

type (
	UserIDCtxKey    struct{}
	SessionIDCtxKey struct{}
)

type User struct {
	ID               uuid.UUID
	Name             string
	Nickname         string
	AvatarURL        string
	Email            string
	OpenRouterKeyID  string
	OpenRouterApiKey string
	BalanceMinor     int64
	Role             string
	CreatedAt        time.Time
}

func NewUser(name string, nickname, avatarUrl, email string, role string) User {
	return User{
		ID:        uuid.New(),
		Name:      name,
		Nickname:  nickname,
		AvatarURL: avatarUrl,
		Email:     email,
		Role:      role,
		CreatedAt: time.Now(),
	}
}
