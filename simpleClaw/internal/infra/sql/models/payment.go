package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Payment struct {
	ID                   string     `gorm:"primaryKey;type:varchar(255)"`
	UserID               uuid.UUID  `gorm:"type:uuid;not null;index"`
	User                 User       `gorm:"constraint:OnDelete:CASCADE;foreignKey:UserID;references:ID"`
	SubscriptionID       *uuid.UUID `gorm:"type:uuid;index"`
	Purpose              string     `gorm:"type:varchar(32);not null;default:'top_up';index"`
	Status               string     `gorm:"type:varchar(64);not null"`
	Paid                 bool       `gorm:"type:boolean;not null"`
	AmountValue          string     `gorm:"column:amount_value;type:varchar(64);not null"`
	AmountCurrency       string     `gorm:"column:amount_currency;type:varchar(16);not null"`
	Description          string     `gorm:"type:text"`
	ExpiresAt            *time.Time
	Refundable           bool           `gorm:"type:boolean;not null"`
	Test                 bool           `gorm:"type:boolean;not null"`
	AuthorizationDetails datatypes.JSON `gorm:"column:authorization_details;type:jsonb"`
	Metadata             datatypes.JSON `gorm:"type:jsonb"`
	PaymentMethod        datatypes.JSON `gorm:"column:payment_method;type:jsonb"`
	Recipient            datatypes.JSON `gorm:"type:jsonb"`
	IncomeAmountValue    *string        `gorm:"column:income_amount_value;type:varchar(64)"`
	IncomeAmountCurrency *string        `gorm:"column:income_amount_currency;type:varchar(16)"`
	CreatedAt            time.Time      `gorm:"not null"`
}
