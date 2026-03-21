package sql

import (
	"gorm.io/gorm"
	"simpleClaw/internal/infra/sql/models"
)

func Migration(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.GmailToken{},
		&models.Channel{},
		&models.Server{},
		&models.Claw{},
	)
}
