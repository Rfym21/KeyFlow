package services

import (
	"encoding/json"
	"fmt"
	"gpt-load/internal/encryption"
	"gpt-load/internal/keypool"
	"gpt-load/internal/models"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	maxRequestKeys = 5000
	chunkSize      = 500
)

// AddKeysResult holds the result of adding multiple keys.
type AddKeysResult struct {
	AddedCount   int   `json:"added_count"`
	IgnoredCount int   `json:"ignored_count"`
	TotalInGroup int64 `json:"total_in_group"`
}

// DeleteKeysResult holds the result of deleting multiple keys.
type DeleteKeysResult struct {
	DeletedCount int   `json:"deleted_count"`
	IgnoredCount int   `json:"ignored_count"`
	TotalInGroup int64 `json:"total_in_group"`
}

// RestoreKeysResult holds the result of restoring multiple keys.
type RestoreKeysResult struct {
	RestoredCount int   `json:"restored_count"`
	IgnoredCount  int   `json:"ignored_count"`
	TotalInGroup  int64 `json:"total_in_group"`
}

// UpdateWeightResult holds the result of updating key weights.
type UpdateWeightResult struct {
	UpdatedCount int   `json:"updated_count"`
	IgnoredCount int   `json:"ignored_count"`
	TotalInGroup int64 `json:"total_in_group"`
}

// KeyWithWeight represents a key with its weight
type KeyWithWeight struct {
	Key    string
	Weight int
}

// KeyService provides services related to API keys.
type KeyService struct {
	DB            *gorm.DB
	KeyProvider   *keypool.KeyProvider
	KeyValidator  *keypool.KeyValidator
	EncryptionSvc encryption.Service
}

// NewKeyService creates a new KeyService.
func NewKeyService(db *gorm.DB, keyProvider *keypool.KeyProvider, keyValidator *keypool.KeyValidator, encryptionSvc encryption.Service) *KeyService {
	return &KeyService{
		DB:            db,
		KeyProvider:   keyProvider,
		KeyValidator:  keyValidator,
		EncryptionSvc: encryptionSvc,
	}
}

// AddMultipleKeys handles the business logic of creating new keys from a text block.
// Supports format: key:weight (e.g., "sk-xxx:10") or just key (default weight 500)
// deprecated: use KeyImportService for large imports
func (s *KeyService) AddMultipleKeys(groupID uint, keysText string) (*AddKeysResult, error) {
	keysWithWeight := s.ParseKeysWithWeightFromText(keysText)
	if len(keysWithWeight) > maxRequestKeys {
		return nil, fmt.Errorf("batch size exceeds the limit of %d keys, got %d", maxRequestKeys, len(keysWithWeight))
	}
	if len(keysWithWeight) == 0 {
		return nil, fmt.Errorf("no valid keys found in the input text")
	}

	addedCount, ignoredCount, err := s.processAndCreateKeysWithWeight(groupID, keysWithWeight, nil)
	if err != nil {
		return nil, err
	}

	var totalInGroup int64
	if err := s.DB.Model(&models.APIKey{}).Where("group_id = ?", groupID).Count(&totalInGroup).Error; err != nil {
		return nil, err
	}

	return &AddKeysResult{
		AddedCount:   addedCount,
		IgnoredCount: ignoredCount,
		TotalInGroup: totalInGroup,
	}, nil
}

// processAndCreateKeys is the lowest-level reusable function for adding keys (without weight).
func (s *KeyService) processAndCreateKeys(
	groupID uint,
	keys []string,
	progressCallback func(processed int),
) (addedCount int, ignoredCount int, err error) {
	keysWithWeight := make([]KeyWithWeight, len(keys))
	for i, k := range keys {
		keysWithWeight[i] = KeyWithWeight{Key: k, Weight: 500}
	}
	return s.processAndCreateKeysWithWeight(groupID, keysWithWeight, progressCallback)
}

// processAndCreateKeysWithWeight is the lowest-level reusable function for adding keys with weight.
func (s *KeyService) processAndCreateKeysWithWeight(
	groupID uint,
	keys []KeyWithWeight,
	progressCallback func(processed int),
) (addedCount int, ignoredCount int, err error) {
	// 1. Get existing key hashes in the group for deduplication
	var existingHashes []string
	if err := s.DB.Model(&models.APIKey{}).Where("group_id = ?", groupID).Pluck("key_hash", &existingHashes).Error; err != nil {
		return 0, 0, err
	}
	existingHashMap := make(map[string]bool)
	for _, h := range existingHashes {
		existingHashMap[h] = true
	}

	// 2. Prepare new keys for creation
	var newKeysToCreate []models.APIKey
	uniqueNewKeys := make(map[string]bool)

	for _, kw := range keys {
		trimmedKey := strings.TrimSpace(kw.Key)
		if trimmedKey == "" || uniqueNewKeys[trimmedKey] || !s.isValidKeyFormat(trimmedKey) {
			continue
		}

		// Generate hash for deduplication check
		keyHash := s.EncryptionSvc.Hash(trimmedKey)
		if existingHashMap[keyHash] {
			continue
		}

		encryptedKey, err := s.EncryptionSvc.Encrypt(trimmedKey)
		if err != nil {
			logrus.WithError(err).WithField("key", trimmedKey).Error("Failed to encrypt key, skipping")
			continue
		}

		weight := kw.Weight
		if weight < 1 {
		weight = 500
		} else if weight > 1000 {
		weight = 5000
		}

		uniqueNewKeys[trimmedKey] = true
		newKeysToCreate = append(newKeysToCreate, models.APIKey{
			GroupID:  groupID,
			KeyValue: encryptedKey,
			KeyHash:  keyHash,
			Status:   models.KeyStatusActive,
			Weight:   weight,
		})
	}

	if len(newKeysToCreate) == 0 {
		return 0, len(keys), nil
	}

	// 3. Use KeyProvider to add keys in chunks
	for i := 0; i < len(newKeysToCreate); i += chunkSize {
		end := i + chunkSize
		if end > len(newKeysToCreate) {
			end = len(newKeysToCreate)
		}
		chunk := newKeysToCreate[i:end]
		if err := s.KeyProvider.AddKeys(groupID, chunk); err != nil {
			return addedCount, len(keys) - addedCount, err
		}
		addedCount += len(chunk)

		if progressCallback != nil {
			progressCallback(i + len(chunk))
		}
	}

	return addedCount, len(keys) - addedCount, nil
}

// ParseKeysFromText parses a string of keys from various formats into a string slice.
// This function is exported to be shared with the handler layer.
func (s *KeyService) ParseKeysFromText(text string) []string {
	keysWithWeight := s.ParseKeysWithWeightFromText(text)
	keys := make([]string, len(keysWithWeight))
	for i, kw := range keysWithWeight {
		keys[i] = kw.Key
	}
	return keys
}

// ParseKeysWithWeightFromText parses a string of keys with optional weights.
// Supports format: key:weight (e.g., "sk-xxx:10") or just key (default weight 500)
func (s *KeyService) ParseKeysWithWeightFromText(text string) []KeyWithWeight {
	var result []KeyWithWeight

	// First, try to parse as a JSON array of strings
	var keys []string
	if json.Unmarshal([]byte(text), &keys) == nil && len(keys) > 0 {
		for _, key := range keys {
			if kw := s.parseKeyWithWeight(key); kw != nil {
				result = append(result, *kw)
			}
		}
		return result
	}

	// 通用解析：通过分隔符分割文本
	delimiters := regexp.MustCompile(`[\s,;\n\r\t]+`)
	splitKeys := delimiters.Split(strings.TrimSpace(text), -1)

	for _, key := range splitKeys {
		if kw := s.parseKeyWithWeight(key); kw != nil {
			result = append(result, *kw)
		}
	}

	return result
}

// parseKeyWithWeight parses a single key string with optional weight suffix
// Format: key:weight or just key
func (s *KeyService) parseKeyWithWeight(input string) *KeyWithWeight {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// 检查是否包含权重后缀 (:数字)
	// 从最后一个冒号开始检查，因为密钥本身可能包含冒号
	lastColonIdx := strings.LastIndex(input, ":")
	if lastColonIdx > 0 && lastColonIdx < len(input)-1 {
		potentialWeight := input[lastColonIdx+1:]
		weight, err := strconv.Atoi(potentialWeight)
		if err == nil && weight >= 1 && weight <= 1000 {
			key := strings.TrimSpace(input[:lastColonIdx])
			if s.isValidKeyFormat(key) {
				return &KeyWithWeight{Key: key, Weight: weight}
			}
		}
	}

	// 没有权重后缀，使用默认权重 500
	if s.isValidKeyFormat(input) {
		return &KeyWithWeight{Key: input, Weight: 500}
	}

	return nil
}

// filterValidKeys validates and filters potential API keys
func (s *KeyService) filterValidKeys(keys []string) []string {
	var validKeys []string
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if s.isValidKeyFormat(key) {
			validKeys = append(validKeys, key)
		}
	}
	return validKeys
}

// isValidKeyFormat performs basic validation on key format
func (s *KeyService) isValidKeyFormat(key string) bool {
	return strings.TrimSpace(key) != ""
}

// RestoreMultipleKeys handles the business logic of restoring keys from a text block.
func (s *KeyService) RestoreMultipleKeys(groupID uint, keysText string) (*RestoreKeysResult, error) {
	keysToRestore := s.ParseKeysFromText(keysText)
	if len(keysToRestore) > maxRequestKeys {
		return nil, fmt.Errorf("batch size exceeds the limit of %d keys, got %d", maxRequestKeys, len(keysToRestore))
	}
	if len(keysToRestore) == 0 {
		return nil, fmt.Errorf("no valid keys found in the input text")
	}

	var totalRestoredCount int64
	for i := 0; i < len(keysToRestore); i += chunkSize {
		end := i + chunkSize
		if end > len(keysToRestore) {
			end = len(keysToRestore)
		}
		chunk := keysToRestore[i:end]
		restoredCount, err := s.KeyProvider.RestoreMultipleKeys(groupID, chunk)
		if err != nil {
			return nil, err
		}
		totalRestoredCount += restoredCount
	}

	ignoredCount := len(keysToRestore) - int(totalRestoredCount)

	var totalInGroup int64
	if err := s.DB.Model(&models.APIKey{}).Where("group_id = ?", groupID).Count(&totalInGroup).Error; err != nil {
		return nil, err
	}

	return &RestoreKeysResult{
		RestoredCount: int(totalRestoredCount),
		IgnoredCount:  ignoredCount,
		TotalInGroup:  totalInGroup,
	}, nil
}

// RestoreAllInvalidKeys sets the status of all 'inactive' keys in a group to 'active'.
func (s *KeyService) RestoreAllInvalidKeys(groupID uint) (int64, error) {
	return s.KeyProvider.RestoreKeys(groupID)
}

// ClearAllInvalidKeys deletes all 'inactive' keys from a group.
func (s *KeyService) ClearAllInvalidKeys(groupID uint) (int64, error) {
	return s.KeyProvider.RemoveInvalidKeys(groupID)
}

// ClearAllKeys deletes all keys from a group.
func (s *KeyService) ClearAllKeys(groupID uint) (int64, error) {
	return s.KeyProvider.RemoveAllKeys(groupID)
}

// DeleteMultipleKeys handles the business logic of deleting keys from a text block.
func (s *KeyService) DeleteMultipleKeys(groupID uint, keysText string) (*DeleteKeysResult, error) {
	keysToDelete := s.ParseKeysFromText(keysText)
	if len(keysToDelete) > maxRequestKeys {
		return nil, fmt.Errorf("batch size exceeds the limit of %d keys, got %d", maxRequestKeys, len(keysToDelete))
	}
	if len(keysToDelete) == 0 {
		return nil, fmt.Errorf("no valid keys found in the input text")
	}

	var totalDeletedCount int64
	for i := 0; i < len(keysToDelete); i += chunkSize {
		end := i + chunkSize
		if end > len(keysToDelete) {
			end = len(keysToDelete)
		}
		chunk := keysToDelete[i:end]
		deletedCount, err := s.KeyProvider.RemoveKeys(groupID, chunk)
		if err != nil {
			return nil, err
		}
		totalDeletedCount += deletedCount
	}

	ignoredCount := len(keysToDelete) - int(totalDeletedCount)

	var totalInGroup int64
	if err := s.DB.Model(&models.APIKey{}).Where("group_id = ?", groupID).Count(&totalInGroup).Error; err != nil {
		return nil, err
	}

	return &DeleteKeysResult{
		DeletedCount: int(totalDeletedCount),
		IgnoredCount: ignoredCount,
		TotalInGroup: totalInGroup,
	}, nil
}

// ListKeysInGroupQuery builds a query to list all keys within a specific group, filtered by status.
func (s *KeyService) ListKeysInGroupQuery(groupID uint, statusFilter string, searchHash string, sortBy string, sortOrder string) *gorm.DB {
	query := s.DB.Model(&models.APIKey{}).Where("group_id = ?", groupID)

	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	if searchHash != "" {
		query = query.Where("key_hash = ?", searchHash)
	}

	// 根据排序字段排序
	switch sortBy {
	case "weight":
		query = query.Order("weight " + sortOrder)
	case "request_count":
		query = query.Order("request_count " + sortOrder)
	case "failure_count":
		query = query.Order("failure_count " + sortOrder)
	case "last_used_at":
		query = query.Order("last_used_at " + sortOrder + " NULLS LAST")
	default:
		// 默认按最后使用时间排序
		query = query.Order("last_used_at desc, updated_at desc")
	}

	return query
}

// EnrichKeysWithRealTimeWeight 用store中的实时权重更新keys
func (s *KeyService) EnrichKeysWithRealTimeWeight(keys []models.APIKey) {
	for i := range keys {
		if weight := s.KeyProvider.GetRealTimeWeight(keys[i].ID); weight > 0 {
			keys[i].Weight = weight
		}
	}
}

// TestMultipleKeys handles a one-off validation test for multiple keys.
func (s *KeyService) TestMultipleKeys(group *models.Group, keysText string) ([]keypool.KeyTestResult, error) {
	keysToTest := s.ParseKeysFromText(keysText)
	if len(keysToTest) > maxRequestKeys {
		return nil, fmt.Errorf("batch size exceeds the limit of %d keys, got %d", maxRequestKeys, len(keysToTest))
	}
	if len(keysToTest) == 0 {
		return nil, fmt.Errorf("no valid keys found in the input text")
	}

	var allResults []keypool.KeyTestResult
	for i := 0; i < len(keysToTest); i += chunkSize {
		end := i + chunkSize
		if end > len(keysToTest) {
			end = len(keysToTest)
		}
		chunk := keysToTest[i:end]
		results, err := s.KeyValidator.TestMultipleKeys(group, chunk)
		if err != nil {
			return nil, err
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

// StreamKeysToWriter fetches keys from the database in batches and writes them to the provided writer.
func (s *KeyService) StreamKeysToWriter(groupID uint, statusFilter string, writer io.Writer) error {
	query := s.DB.Model(&models.APIKey{}).Where("group_id = ?", groupID).Select("id, key_value")

	switch statusFilter {
	case models.KeyStatusActive, models.KeyStatusInvalid:
		query = query.Where("status = ?", statusFilter)
	case "all":
	default:
		return fmt.Errorf("invalid status filter: %s", statusFilter)
	}

	var keys []models.APIKey
	err := query.FindInBatches(&keys, chunkSize, func(tx *gorm.DB, batch int) error {
		for _, key := range keys {
			decryptedKey, err := s.EncryptionSvc.Decrypt(key.KeyValue)
			if err != nil {
				logrus.WithError(err).WithField("key_id", key.ID).Error("Failed to decrypt key for streaming, skipping")
				continue
			}
			if _, err := writer.Write([]byte(decryptedKey + "\n")); err != nil {
				return err
			}
		}
		return nil
	}).Error

	return err
}

// UpdateKeyWeight updates the weight of a single key by ID
func (s *KeyService) UpdateKeyWeight(keyID uint, weight int) error {
	return s.KeyProvider.UpdateKeyWeight(keyID, weight)
}

// UpdateKeysWeight updates the weight of multiple keys from a text block
func (s *KeyService) UpdateKeysWeight(groupID uint, keysText string, weight int) (*UpdateWeightResult, error) {
	if weight < 1 || weight > 1000 {
		return nil, fmt.Errorf("weight must be between 1 and 1000")
	}

	keys := s.ParseKeysFromText(keysText)
	if len(keys) > maxRequestKeys {
		return nil, fmt.Errorf("batch size exceeds the limit of %d keys, got %d", maxRequestKeys, len(keys))
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no valid keys found in the input text")
	}

	// 生成 key hashes
	var keyHashes []string
	for _, key := range keys {
		keyHash := s.EncryptionSvc.Hash(strings.TrimSpace(key))
		if keyHash != "" {
			keyHashes = append(keyHashes, keyHash)
		}
	}

	var totalUpdatedCount int64
	for i := 0; i < len(keyHashes); i += chunkSize {
		end := i + chunkSize
		if end > len(keyHashes) {
			end = len(keyHashes)
		}
		chunk := keyHashes[i:end]
		updatedCount, err := s.KeyProvider.UpdateKeysWeight(groupID, chunk, weight)
		if err != nil {
			return nil, err
		}
		totalUpdatedCount += updatedCount
	}

	ignoredCount := len(keys) - int(totalUpdatedCount)

	var totalInGroup int64
	if err := s.DB.Model(&models.APIKey{}).Where("group_id = ?", groupID).Count(&totalInGroup).Error; err != nil {
		return nil, err
	}

	return &UpdateWeightResult{
		UpdatedCount: int(totalUpdatedCount),
		IgnoredCount: ignoredCount,
		TotalInGroup: totalInGroup,
	}, nil
}

// ResetKeysWeight resets all keys' weights in a group to the default value (500)
func (s *KeyService) ResetKeysWeight(groupID uint) (int64, error) {
	return s.KeyProvider.ResetKeysWeight(groupID)
}

// ClearRequestCount clears request_count and failure_count for all keys in a group
func (s *KeyService) ClearRequestCount(groupID uint) (int64, error) {
	result := s.DB.Model(&models.APIKey{}).
		Where("group_id = ?", groupID).
		Updates(map[string]any{
			"request_count": 0,
			"failure_count": 0,
		})
	return result.RowsAffected, result.Error
}
