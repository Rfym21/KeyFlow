package db

import (
	"fmt"
	"gpt-load/internal/types"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func NewDB(configManager types.ConfigManager) (*gorm.DB, error) {
	dbConfig := configManager.GetDatabaseConfig()
	dsn := dbConfig.DSN
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_DSN is not configured")
	}

	var newLogger logger.Interface
	if configManager.GetLogConfig().Level == "debug" {
		newLogger = logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
			logger.Config{
				SlowThreshold:             time.Second, // Slow SQL threshold
				LogLevel:                  logger.Info, // Log level
				IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
				Colorful:                  true,        // Disable color
			},
		)
	}

	var dialector gorm.Dialector
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		dialector = postgres.New(postgres.Config{
			DSN:                  dsn,
			PreferSimpleProtocol: true,
		})
	} else if strings.Contains(dsn, "@tcp") {
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		dialector = mysql.Open(dsn)
	} else {
		if err := os.MkdirAll(filepath.Dir(dsn), 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
		dialector = sqlite.Open(dsn + "?_busy_timeout=5000")
	}

	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger:      newLogger,
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}
	// Set connection pool parameters for all drivers
	sqlDB.SetMaxIdleConns(50)
	sqlDB.SetMaxOpenConns(500)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return DB, nil
}
