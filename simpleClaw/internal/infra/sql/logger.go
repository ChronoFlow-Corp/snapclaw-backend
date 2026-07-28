package sql

import (
	"io"
	"log"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

func NewGormLogger(w io.Writer) gormlogger.Interface {
	return gormlogger.New(
		log.New(w, "\r\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
}
