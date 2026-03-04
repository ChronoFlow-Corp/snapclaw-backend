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
	Email            string
	OpenRouterKeyID  string
	OpenRouterApiKey string
	Role             string
	CreatedAt        time.Time
}

func NewUser(name string, email string, role string) User {
	return User{
		ID:        uuid.New(),
		Name:      name,
		Email:     email,
		Role:      role,
		CreatedAt: time.Now(),
	}
}
