package utils

import (
	"gpt-load/internal/types"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// SetupLogger configures the logging system based on the provided configuration.
func SetupLogger(configManager types.ConfigManager) {
	logConfig := configManager.GetLogConfig()

	// Set log level
	level, err := logrus.ParseLevel(logConfig.Level)
	if err != nil {
		logrus.Warn("Invalid log level, using info")
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)

	// Set log format
	if logConfig.Format == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00", // ISO 8601 format
		})
	} else {
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	// Setup file logging if enabled
	if logConfig.EnableFile {
		logDir := filepath.Dir(logConfig.FilePath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			logrus.Warnf("Failed to create log directory: %v", err)
		} else {
			logFile, err := os.OpenFile(logConfig.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				logrus.Warnf("Failed to open log file: %v", err)
			} else {
				logrus.SetOutput(io.MultiWriter(os.Stdout, logFile))
			}
		}
	}
}
