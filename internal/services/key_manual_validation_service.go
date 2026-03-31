package services

import (
	"fmt"
	"key-flow/internal/config"
	"key-flow/internal/encryption"
	"key-flow/internal/keypool"
	"key-flow/internal/models"
	"key-flow/internal/types"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ManualValidationResult holds the result of a manual validation task.
type ManualValidationResult struct {
	TotalKeys     int                     `json:"total_keys"`
	ValidKeys     int                     `json:"valid_keys"`
	InvalidKeys   int                     `json:"invalid_keys"`
	TotalDuration int64                   `json:"total_duration"`
	Results       []keypool.KeyTestResult `json:"results"`
}

// KeyManualValidationService handles user-initiated key validation for a group.
type KeyManualValidationService struct {
	DB              *gorm.DB
	Validator       *keypool.KeyValidator
	TaskService     *TaskService
	SettingsManager *config.SystemSettingsManager
	ConfigManager   types.ConfigManager
	EncryptionSvc   encryption.Service
}

// NewKeyManualValidationService creates a new KeyManualValidationService.
func NewKeyManualValidationService(db *gorm.DB, validator *keypool.KeyValidator, taskService *TaskService, settingsManager *config.SystemSettingsManager, configManager types.ConfigManager, encryptionSvc encryption.Service) *KeyManualValidationService {
	return &KeyManualValidationService{
		DB:              db,
		Validator:       validator,
		TaskService:     taskService,
		SettingsManager: settingsManager,
		ConfigManager:   configManager,
		EncryptionSvc:   encryptionSvc,
	}
}

// StartValidationTask starts a new manual validation task for a given group.
func (s *KeyManualValidationService) StartValidationTask(group *models.Group, status string) (*TaskStatus, error) {
	var keys []models.APIKey
	query := s.DB.Where("group_id = ?", group.ID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to get keys for group %s with status '%s': %w", group.Name, status, err)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("no keys to validate in group %s", group.Name)
	}

	taskStatus, err := s.TaskService.StartTask(TaskTypeKeyValidation, group.Name, len(keys))
	if err != nil {
		return nil, err
	}

	// Run the validation in a separate goroutine
	go s.runValidation(group, keys, status)

	return taskStatus, nil
}

func (s *KeyManualValidationService) runValidation(group *models.Group, keys []models.APIKey, status string) {
	logFields := logrus.Fields{
		"group":  group.Name,
		"status": status,
	}
	if status == "" {
		logFields["status"] = "all"
	}
	logrus.WithFields(logFields).Info("Starting manual validation")

	jobs := make(chan models.APIKey, len(keys))
	results := make(chan keypool.KeyTestResult, len(keys))
	startedAt := time.Now()

	concurrency := group.EffectiveConfig.KeyValidationConcurrency

	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go s.validationWorker(&wg, group, jobs, results)
	}

	for _, key := range keys {
		jobs <- key
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	validCount := 0
	processedCount := 0
	lastUpdateTime := time.Now()
	validationResults := make([]keypool.KeyTestResult, 0, len(keys))

	for result := range results {
		processedCount++
		validationResults = append(validationResults, result)
		if result.IsValid {
			validCount++
		}

		// Throttle progress updates to once per second
		if time.Since(lastUpdateTime) > time.Second {
			if err := s.TaskService.UpdateProgress(processedCount); err != nil {
				logrus.Warnf("Failed to update task progress: %v", err)
			}
			lastUpdateTime = time.Now()
		}
	}

	// Ensure the final progress is always updated
	if err := s.TaskService.UpdateProgress(processedCount); err != nil {
		logrus.Warnf("Failed to update final task progress: %v", err)
	}

	result := ManualValidationResult{
		TotalKeys:     len(keys),
		ValidKeys:     validCount,
		InvalidKeys:   len(keys) - validCount,
		TotalDuration: time.Since(startedAt).Milliseconds(),
		Results:       validationResults,
	}

	// End the task and store the final result
	if err := s.TaskService.EndTask(result, nil); err != nil {
		logrus.Errorf("Failed to end task for group %s: %v", group.Name, err)
	}
	logrus.Infof("Manual validation finished for group %s: %+v", group.Name, result)
}

// validationResult 包含验证结果信息
func (s *KeyManualValidationService) validationWorker(wg *sync.WaitGroup, group *models.Group, jobs <-chan models.APIKey, results chan<- keypool.KeyTestResult) {
	defer wg.Done()
	for key := range jobs {
		// Decrypt the key before validation
		decryptedKey, err := s.EncryptionSvc.Decrypt(key.KeyValue)
		if err != nil {
			logrus.WithError(err).WithField("key_id", key.ID).Error("Manual validation: Failed to decrypt key for validation, marking as invalid")
			results <- keypool.KeyTestResult{
				KeyValue:   "",
				IsValid:    false,
				Error:      "Failed to decrypt key for validation.",
				StatusCode: 0,
			}
			continue
		}

		// Create a copy with decrypted value for validation
		keyForValidation := key
		keyForValidation.KeyValue = decryptedKey

		isValid, statusCode, validationErr := s.Validator.ValidateSingleKey(&keyForValidation, group, true)
		result := keypool.KeyTestResult{
			KeyValue:   keyForValidation.KeyValue,
			IsValid:    isValid,
			StatusCode: statusCode,
		}
		if validationErr != nil {
			result.Error = validationErr.Error()
		}
		results <- result
	}
}
