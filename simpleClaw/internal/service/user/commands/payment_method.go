package commands

import "github.com/google/uuid"

type AddPaymentMethod struct {
	UserID    uuid.UUID
	Title     string
	IsDefault bool
}

type SetDefaultPaymentMethod struct {
	UserID          uuid.UUID
	PaymentMethodID uuid.UUID
}
