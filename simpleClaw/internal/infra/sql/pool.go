package sql

import (
	"simpleClaw/internal/infra/sql/models"

	"gorm.io/gorm"
)

func Migration(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.Channel{},
		&models.Server{},
		&models.Claw{},
	)
}
