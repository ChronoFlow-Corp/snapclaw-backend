package models

import (
	"time"

	"github.com/google/uuid"
)

type Server struct {
	ID        uuid.UUID `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Name      string    `gorm:"type:varchar(255)"`
	Ip        string    `gorm:"type:varchar(255)"`
	Url       string    `gorm:"type:varchar(255)"`
	ProxyUrl  string    `gorm:"type:varchar(255)"`
	Status    string    `gorm:"type:varchar(255)"`
	SecretKey string    `gorm:"type:varchar(255)"`
	Claws     []Claw
	CreatedAt time.Time `gorm:"autoCreateTime"`
}
