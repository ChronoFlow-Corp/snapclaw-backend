package sql

import (
	"simpleClaw/internal/infra/sql/models"

	"gorm.io/gorm"
)

func Migration(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.AccountIntegration{},
		&models.Channel{},
		&models.TelegramAccountLink{},
		&models.TelegramWebhookUpdate{},
		&models.TelegramManagedBot{},
		&models.Server{},
		&models.Claw{},
		&models.ClawCapabilityAttachment{},
		&models.ClawLifecycleOperation{},
		&models.Payment{},
		&models.PaymentMethod{},
		&models.Plan{},
		&models.UserSubscription{},
		&models.UserBalanceEntry{},
		&models.OpenRouterUsageEvent{},
	)
}
