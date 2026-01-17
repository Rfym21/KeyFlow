package services

import (
	"context"
	"gpt-load/internal/config"
	"gpt-load/internal/models"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// LogCleanupService 负责清理过期的请求日志
type LogCleanupService struct {
	db              *gorm.DB
	settingsManager *config.SystemSettingsManager
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// NewLogCleanupService 创建新的日志清理服务
func NewLogCleanupService(db *gorm.DB, settingsManager *config.SystemSettingsManager) *LogCleanupService {
	return &LogCleanupService{
		db:              db,
		settingsManager: settingsManager,
		stopCh:          make(chan struct{}),
	}
}

// Start 启动日志清理服务
func (s *LogCleanupService) Start() {
	s.wg.Add(1)
	go s.run()
	logrus.Debug("Log cleanup service started")
}

// Stop 停止日志清理服务
func (s *LogCleanupService) Stop(ctx context.Context) {
	close(s.stopCh)

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logrus.Info("LogCleanupService stopped gracefully.")
	case <-ctx.Done():
		logrus.Warn("LogCleanupService stop timed out.")
	}
}

// run 运行日志清理的主循环
func (s *LogCleanupService) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(2 * time.Hour)
	defer ticker.Stop()

	// 启动时先执行一次清理
	s.cleanupExpiredLogs()

	for {
		select {
		case <-ticker.C:
			s.cleanupExpiredLogs()
		case <-s.stopCh:
			return
		}
	}
}

// cleanupExpiredLogs 清理过期的请求日志
func (s *LogCleanupService) cleanupExpiredLogs() {
	// 获取日志保留天数配置
	settings := s.settingsManager.GetSettings()
	retentionDays := settings.RequestLogRetentionDays

	if retentionDays <= 0 {
		logrus.Debug("Log retention is disabled (retention_days <= 0)")
		return
	}

	// 计算过期时间点
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays).UTC()

	// 执行删除操作
	result := s.db.Where("timestamp < ?", cutoffTime).Delete(&models.RequestLog{})
	if result.Error != nil {
		logrus.WithError(result.Error).Error("Failed to cleanup expired request logs")
		return
	}

	if result.RowsAffected > 0 {
		logrus.WithFields(logrus.Fields{
			"deleted_count":  result.RowsAffected,
			"cutoff_time":    cutoffTime.Format(time.RFC3339),
			"retention_days": retentionDays,
		}).Info("Successfully cleaned up expired request logs")
	} else {
		logrus.Debug("No expired request logs found to cleanup")
	}
}
