package database

import (
	"fmt"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/models"
)

// DB is the shared GORM handle.
var DB *gorm.DB

// Connect opens the database using the configured driver and runs migrations.
func Connect(cfg *config.Config) error {
	var dialector gorm.Dialector
	switch cfg.DBDriver {
	case "postgres":
		dialector = postgres.Open(cfg.DBDSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DBDSN)
	default:
		return fmt.Errorf("unsupported DB_DRIVER: %s", cfg.DBDriver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return err
	}
	DB = db

	if err := Migrate(); err != nil {
		return err
	}
	log.Printf("database connected (%s)", cfg.DBDriver)
	return nil
}

// Migrate creates or updates tables for every model. Exported so tests can
// migrate their own database.
func Migrate() error {
	return DB.AutoMigrate(
		&models.User{},
		&models.Server{},
		&models.ServerMember{},
		&models.ServerBan{},
		&models.Channel{},
		&models.ChannelMember{},
		&models.Message{},
		&models.Attachment{},
		&models.Friend{},
		&models.Notification{},
	)
}
