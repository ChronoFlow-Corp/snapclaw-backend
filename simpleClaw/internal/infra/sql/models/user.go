package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID               uuid.UUID          `gorm:"primary_key;type:uuid"`
	Name             string             `gorm:"type:varchar(255)"`
	NickName         string             `gorm:"type:varchar(255)"`
	AvatarURL        string             `gorm:"type:varchar(255)"`
	Email            string             `gorm:"type:varchar(255);unique"`
	Role             string             `gorm:"type:varchar(16)"`
	OpenRouterKeyID  string             `gorm:"type:varchar(128)"`
	OpenRouterApiKey string             `gorm:"type:varchar(355)"`
	BalanceMinor     int64              `gorm:"not null;default:0"`
	MarketingOptOut  bool               `gorm:"not null;default:false"`
	CreatedAt        time.Time          `gorm:"autoCreateTime"`
	UpdatedAt        time.Time          `gorm:"autoUpdateTime"`
	Sessions         []Session          `gorm:"constraint:OnDelete:CASCADE"`
	Channels         []Channel          `gorm:"constraint:OnDelete:CASCADE"`
	Claws            []Claw             `gorm:"constraint:OnDelete:CASCADE"`
	Payments         []Payment          `gorm:"constraint:OnDelete:CASCADE"`
	PaymentMethods   []PaymentMethod    `gorm:"constraint:OnDelete:CASCADE"`
	Subscriptions    []UserSubscription `gorm:"constraint:OnDelete:CASCADE"`
	BalanceEntries   []UserBalanceEntry `gorm:"constraint:OnDelete:CASCADE"`
}

type Session struct {
	ID           uuid.UUID `gorm:"primary_key;type:uuid"`
	UserID       uuid.UUID
	RefreshToken string    `gorm:"type:text"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}
