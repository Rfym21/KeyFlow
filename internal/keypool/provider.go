package keypool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gpt-load/internal/config"
	"gpt-load/internal/encryption"
	app_errors "gpt-load/internal/errors"
	"gpt-load/internal/models"
	"gpt-load/internal/store"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// cacheHitRecord 用于跟踪cache_hit条目以便定期清理
type cacheHitRecord struct {
	GroupID uint
	Hash    string
	KeyID   uint
	ExpTime int64
}

type KeyProvider struct {
	db              *gorm.DB
	store           store.Store
	settingsManager *config.SystemSettingsManager
	encryptionSvc   encryption.Service

	// 用于跟踪cache_hit条目
	cacheHitRecords map[string]*cacheHitRecord
	cacheHitMu      sync.RWMutex
	cleanupCancel   context.CancelFunc
}

// NewProvider 创建一个新的 KeyProvider 实例。
func NewProvider(db *gorm.DB, store store.Store, settingsManager *config.SystemSettingsManager, encryptionSvc encryption.Service) *KeyProvider {
	ctx, cancel := context.WithCancel(context.Background())
	p := &KeyProvider{
		db:              db,
		store:           store,
		settingsManager: settingsManager,
		encryptionSvc:   encryptionSvc,
		cacheHitRecords: make(map[string]*cacheHitRecord),
		cleanupCancel:   cancel,
	}
	// 启动定期清理goroutine
	go p.startCacheHitCleanup(ctx)
	return p
}

// GetStore returns the underlying store
func (p *KeyProvider) GetStore() store.Store {
	return p.store
}

// SelectKey 为指定的分组使用加权随机算法选择一个可用的 APIKey。
func (p *KeyProvider) SelectKey(groupID uint) (*models.APIKey, error) {
	activeKeysListKey := fmt.Sprintf("group:%d:active_keys", groupID)

	// 1. 获取列表长度
	listLen, err := p.store.LLen(activeKeysListKey)
	if err != nil || listLen == 0 {
		if err == nil || errors.Is(err, store.ErrNotFound) {
			return nil, app_errors.ErrNoActiveKeys
		}
		return nil, fmt.Errorf("failed to get active keys list length: %w", err)
	}

	// 2. 如果只有一个 key，直接使用简单轮询
	if listLen == 1 {
		return p.selectKeyByRotate(groupID, activeKeysListKey)
	}

	// 3. 收集所有 key 的权重信息
	keyIDStr, err := p.store.Rotate(activeKeysListKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, app_errors.ErrNoActiveKeys
		}
		return nil, fmt.Errorf("failed to rotate key from store: %w", err)
	}

	// 构建 key 列表用于加权选择
	type keyWeight struct {
		id     uint64
		weight int
	}

	keys := make([]keyWeight, 0, listLen)
	totalWeight := 0

	// 获取第一个 key 的权重
	firstKeyID, _ := strconv.ParseUint(keyIDStr, 10, 64)
	firstKeyHash := fmt.Sprintf("key:%d", firstKeyID)
	firstDetails, err := p.store.HGetAll(firstKeyHash)
	if err == nil {
		w, _ := strconv.Atoi(firstDetails["weight"])
		if w <= 0 {
			w = 500
		}
		keys = append(keys, keyWeight{id: firstKeyID, weight: w})
		totalWeight += w
	}

	// 遍历获取其余 keys 的权重（通过连续 rotate）
	for i := int64(1); i < listLen; i++ {
		nextKeyIDStr, err := p.store.Rotate(activeKeysListKey)
		if err != nil {
			break
		}
		nextKeyID, _ := strconv.ParseUint(nextKeyIDStr, 10, 64)
		if nextKeyID == firstKeyID {
			break // 已经轮转回来了
		}
		keyHash := fmt.Sprintf("key:%d", nextKeyID)
		details, err := p.store.HGetAll(keyHash)
		if err == nil {
			w, _ := strconv.Atoi(details["weight"])
			if w <= 0 {
				w = 500
			}
			keys = append(keys, keyWeight{id: nextKeyID, weight: w})
			totalWeight += w
		}
	}

	if len(keys) == 0 || totalWeight == 0 {
		return nil, app_errors.ErrNoActiveKeys
	}

	// 4. 加权随机选择
	r := rand.Intn(totalWeight)
	cumulative := 0
	var selectedKeyID uint64 = keys[0].id

	for _, k := range keys {
		cumulative += k.weight
		if r < cumulative {
			selectedKeyID = k.id
			break
		}
	}

	// 5. 获取选中 key 的完整信息
	return p.getKeyDetails(groupID, selectedKeyID)
}

// selectKeyByRotate 使用简单轮询选择 key（单 key 场景优化）
func (p *KeyProvider) selectKeyByRotate(groupID uint, activeKeysListKey string) (*models.APIKey, error) {
	keyIDStr, err := p.store.Rotate(activeKeysListKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, app_errors.ErrNoActiveKeys
		}
		return nil, fmt.Errorf("failed to rotate key from store: %w", err)
	}

	keyID, err := strconv.ParseUint(keyIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse key ID '%s': %w", keyIDStr, err)
	}

	return p.getKeyDetails(groupID, keyID)
}

// getKeyDetails 获取 key 的完整信息
func (p *KeyProvider) getKeyDetails(groupID uint, keyID uint64) (*models.APIKey, error) {
	keyHashKey := fmt.Sprintf("key:%d", keyID)
	keyDetails, err := p.store.HGetAll(keyHashKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get key details for key ID %d: %w", keyID, err)
	}

	failureCount, _ := strconv.ParseInt(keyDetails["failure_count"], 10, 64)
	createdAt, _ := strconv.ParseInt(keyDetails["created_at"], 10, 64)
	baseWeight, _ := strconv.Atoi(keyDetails["base_weight"])
	if baseWeight <= 0 {
		baseWeight = 500
	}
	weight, _ := strconv.Atoi(keyDetails["weight"])
	if weight <= 0 {
		weight = baseWeight
	}

	// Decrypt the key value for use by channels
	encryptedKeyValue := keyDetails["key_string"]
	decryptedKeyValue, err := p.encryptionSvc.Decrypt(encryptedKeyValue)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"keyID": keyID,
			"error": err,
		}).Debug("Failed to decrypt key value, using as-is for backward compatibility")
		decryptedKeyValue = encryptedKeyValue
	}

	apiKey := &models.APIKey{
		ID:           uint(keyID),
		KeyValue:     decryptedKeyValue,
		Status:       keyDetails["status"],
		BaseWeight:   baseWeight,
		Weight:       weight,
		FailureCount: failureCount,
		GroupID:      groupID,
		CreatedAt:    time.Unix(createdAt, 0),
	}

	return apiKey, nil
}

// UpdateStatus 异步地提交一个 Key 状态更新任务。
// forceDisableOnFailure: 如果为true，失败时直接禁用key，不检查黑名单阈值（用于手动测试）
func (p *KeyProvider) UpdateStatus(apiKey *models.APIKey, group *models.Group, isSuccess bool, errorMessage string, forceDisableOnFailure bool) {
	go func() {
		keyHashKey := fmt.Sprintf("key:%d", apiKey.ID)
		activeKeysListKey := fmt.Sprintf("group:%d:active_keys", group.ID)

		if isSuccess {
			if err := p.handleSuccess(apiKey.ID, keyHashKey, activeKeysListKey); err != nil {
				logrus.WithFields(logrus.Fields{"keyID": apiKey.ID, "error": err}).Error("Failed to handle key success")
			}
		} else {
			if app_errors.IsUnCounted(errorMessage) {
				logrus.WithFields(logrus.Fields{
					"keyID": apiKey.ID,
					"error": errorMessage,
				}).Debug("Uncounted error, skipping failure handling")
			} else {
				if err := p.handleFailure(apiKey, group, keyHashKey, activeKeysListKey, forceDisableOnFailure); err != nil {
					logrus.WithFields(logrus.Fields{"keyID": apiKey.ID, "error": err}).Error("Failed to handle key failure")
				}
			}
		}
	}()
}

// executeTransactionWithRetry wraps a database transaction with a retry mechanism.
func (p *KeyProvider) executeTransactionWithRetry(operation func(tx *gorm.DB) error) error {
	const maxRetries = 3
	const baseDelay = 50 * time.Millisecond
	const maxJitter = 150 * time.Millisecond
	var err error

	for i := range maxRetries {
		err = p.db.Transaction(operation)
		if err == nil {
			return nil
		}

		if strings.Contains(err.Error(), "database is locked") {
			jitter := time.Duration(rand.Intn(int(maxJitter)))
			totalDelay := baseDelay + jitter
			logrus.Debugf("Database is locked, retrying in %v... (attempt %d/%d)", totalDelay, i+1, maxRetries)
			time.Sleep(totalDelay)
			continue
		}

		break
	}

	return err
}

func (p *KeyProvider) handleSuccess(keyID uint, keyHashKey, activeKeysListKey string) error {
	keyDetails, err := p.store.HGetAll(keyHashKey)
	if err != nil {
		return fmt.Errorf("failed to get key details from store: %w", err)
	}

	failureCount, _ := strconv.ParseInt(keyDetails["failure_count"], 10, 64)
	isActive := keyDetails["status"] == models.KeyStatusActive

	if failureCount == 0 && isActive {
		return nil
	}

	return p.executeTransactionWithRetry(func(tx *gorm.DB) error {
		var key models.APIKey
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&key, keyID).Error; err != nil {
			return fmt.Errorf("failed to lock key %d for update: %w", keyID, err)
		}

		updates := map[string]any{"failure_count": 0}
		if !isActive {
			updates["status"] = models.KeyStatusActive
		}

		if err := tx.Model(&key).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update key in DB: %w", err)
		}

		if err := p.store.HSet(keyHashKey, updates); err != nil {
			return fmt.Errorf("failed to update key details in store: %w", err)
		}

		if !isActive {
			logrus.WithField("keyID", keyID).Debug("Key has recovered and is being restored to active pool.")
			if err := p.store.LRem(activeKeysListKey, 0, keyID); err != nil {
				return fmt.Errorf("failed to LRem key before LPush on recovery: %w", err)
			}
			if err := p.store.LPush(activeKeysListKey, keyID); err != nil {
				return fmt.Errorf("failed to LPush key back to active list: %w", err)
			}
		}

		return nil
	})
}

func (p *KeyProvider) handleFailure(apiKey *models.APIKey, group *models.Group, keyHashKey, activeKeysListKey string, forceDisableOnFailure bool) error {
	keyDetails, err := p.store.HGetAll(keyHashKey)
	if err != nil {
		return fmt.Errorf("failed to get key details from store: %w", err)
	}

	if keyDetails["status"] == models.KeyStatusInvalid {
		return nil
	}

	failureCount, _ := strconv.ParseInt(keyDetails["failure_count"], 10, 64)

	// 获取该分组的有效配置
	blacklistThreshold := group.EffectiveConfig.BlacklistThreshold

	return p.executeTransactionWithRetry(func(tx *gorm.DB) error {
		var key models.APIKey
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&key, apiKey.ID).Error; err != nil {
			return fmt.Errorf("failed to lock key %d for update: %w", apiKey.ID, err)
		}

		newFailureCount := failureCount + 1

		updates := map[string]any{"failure_count": newFailureCount}
		// 手动测试失败直接禁用，或者达到黑名单阈值时禁用
		shouldBlacklist := forceDisableOnFailure || (blacklistThreshold > 0 && newFailureCount >= int64(blacklistThreshold))
		if shouldBlacklist {
			updates["status"] = models.KeyStatusInvalid
		}

		if err := tx.Model(&key).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update key stats in DB: %w", err)
		}

		if _, err := p.store.HIncrBy(keyHashKey, "failure_count", 1); err != nil {
			return fmt.Errorf("failed to increment failure count in store: %w", err)
		}

		if shouldBlacklist {
			if forceDisableOnFailure {
				logrus.WithFields(logrus.Fields{"keyID": apiKey.ID}).Warn("Manual test failed, key disabled immediately.")
			} else {
				logrus.WithFields(logrus.Fields{"keyID": apiKey.ID, "threshold": blacklistThreshold}).Warn("Key has reached blacklist threshold, disabling.")
			}
			if err := p.store.LRem(activeKeysListKey, 0, apiKey.ID); err != nil {
				return fmt.Errorf("failed to LRem key from active list: %w", err)
			}
			if err := p.store.HSet(keyHashKey, map[string]any{"status": models.KeyStatusInvalid}); err != nil {
				return fmt.Errorf("failed to update key status to invalid in store: %w", err)
			}
		}

		return nil
	})
}

// LoadKeysFromDB 从数据库加载所有分组和密钥，并填充到 Store 中。
func (p *KeyProvider) LoadKeysFromDB() error {
	logrus.Debug("First time startup, loading keys from DB...")

	// 1. 分批从数据库加载并使用 Pipeline 写入 Redis
	allActiveKeyIDs := make(map[uint][]any)
	batchSize := 1000
	var batchKeys []*models.APIKey

	err := p.db.Model(&models.APIKey{}).FindInBatches(&batchKeys, batchSize, func(tx *gorm.DB, batch int) error {
		logrus.Debugf("Processing batch %d with %d keys...", batch, len(batchKeys))

		var pipeline store.Pipeliner
		if redisStore, ok := p.store.(store.RedisPipeliner); ok {
			pipeline = redisStore.Pipeline()
		}

		for _, key := range batchKeys {
			keyHashKey := fmt.Sprintf("key:%d", key.ID)
			keyDetails := p.apiKeyToMap(key)

			if pipeline != nil {
				pipeline.HSet(keyHashKey, keyDetails)
			} else {
				if err := p.store.HSet(keyHashKey, keyDetails); err != nil {
					logrus.WithFields(logrus.Fields{"keyID": key.ID, "error": err}).Error("Failed to HSet key details")
				}
			}

			if key.Status == models.KeyStatusActive {
				allActiveKeyIDs[key.GroupID] = append(allActiveKeyIDs[key.GroupID], key.ID)
			}
		}

		if pipeline != nil {
			if err := pipeline.Exec(); err != nil {
				return fmt.Errorf("failed to execute pipeline for batch %d: %w", batch, err)
			}
		}
		return nil
	}).Error

	if err != nil {
		return fmt.Errorf("failed during batch processing of keys: %w", err)
	}

	// 2. 更新所有分组的 active_keys 列表
	logrus.Info("Updating active key lists for all groups...")
	for groupID, activeIDs := range allActiveKeyIDs {
		if len(activeIDs) > 0 {
			activeKeysListKey := fmt.Sprintf("group:%d:active_keys", groupID)
			p.store.Delete(activeKeysListKey)
			if err := p.store.LPush(activeKeysListKey, activeIDs...); err != nil {
				logrus.WithFields(logrus.Fields{"groupID": groupID, "error": err}).Error("Failed to LPush active keys for group")
			}
		}
	}

	return nil
}

// AddKeys 批量添加新的 Key 到池和数据库中。
func (p *KeyProvider) AddKeys(groupID uint, keys []models.APIKey) error {
	if len(keys) == 0 {
		return nil
	}

	err := p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&keys).Error; err != nil {
			return err
		}

		for _, key := range keys {
			if err := p.addKeyToStore(&key); err != nil {
				logrus.WithFields(logrus.Fields{"keyID": key.ID, "error": err}).Error("Failed to add key to store after DB creation, rolling back transaction")
				return err
			}
		}
		return nil
	})

	return err
}

// RemoveKeys 批量从池和数据库中移除 Key。
func (p *KeyProvider) RemoveKeys(groupID uint, keyValues []string) (int64, error) {
	if len(keyValues) == 0 {
		return 0, nil
	}

	var keysToDelete []models.APIKey
	var deletedCount int64

	err := p.db.Transaction(func(tx *gorm.DB) error {
		var keyHashes []string
		for _, keyValue := range keyValues {
			keyHash := p.encryptionSvc.Hash(keyValue)
			if keyHash != "" {
				keyHashes = append(keyHashes, keyHash)
			}
		}

		if len(keyHashes) == 0 {
			return nil
		}

		if err := tx.Where("group_id = ? AND key_hash IN ?", groupID, keyHashes).Find(&keysToDelete).Error; err != nil {
			return err
		}

		if len(keysToDelete) == 0 {
			return nil
		}

		keyIDsToDelete := pluckIDs(keysToDelete)

		result := tx.Where("id IN ?", keyIDsToDelete).Delete(&models.APIKey{})
		if result.Error != nil {
			return result.Error
		}
		deletedCount = result.RowsAffected

		for _, key := range keysToDelete {
			if err := p.removeKeyFromStore(key.ID, key.GroupID); err != nil {
				logrus.WithFields(logrus.Fields{"keyID": key.ID, "error": err}).Error("Failed to remove key from store after DB deletion, rolling back transaction")
				return err
			}
		}

		return nil
	})

	return deletedCount, err
}

// RestoreKeys 恢复组内所有无效的 Key。
func (p *KeyProvider) RestoreKeys(groupID uint) (int64, error) {
	var invalidKeys []models.APIKey
	var restoredCount int64

	err := p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ? AND status = ?", groupID, models.KeyStatusInvalid).Find(&invalidKeys).Error; err != nil {
			return err
		}

		if len(invalidKeys) == 0 {
			return nil
		}

		updates := map[string]any{
			"status":        models.KeyStatusActive,
			"failure_count": 0,
		}
		result := tx.Model(&models.APIKey{}).Where("group_id = ? AND status = ?", groupID, models.KeyStatusInvalid).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		restoredCount = result.RowsAffected

		for _, key := range invalidKeys {
			key.Status = models.KeyStatusActive
			key.FailureCount = 0
			if err := p.addKeyToStore(&key); err != nil {
				logrus.WithFields(logrus.Fields{"keyID": key.ID, "error": err}).Error("Failed to restore key in store after DB update, rolling back transaction")
				return err
			}
		}
		return nil
	})

	return restoredCount, err
}

// RestoreMultipleKeys 恢复指定的 Key。
func (p *KeyProvider) RestoreMultipleKeys(groupID uint, keyValues []string) (int64, error) {
	if len(keyValues) == 0 {
		return 0, nil
	}

	var keysToRestore []models.APIKey
	var restoredCount int64

	err := p.db.Transaction(func(tx *gorm.DB) error {
		var keyHashes []string
		for _, keyValue := range keyValues {
			keyHash := p.encryptionSvc.Hash(keyValue)
			if keyHash != "" {
				keyHashes = append(keyHashes, keyHash)
			}
		}

		if len(keyHashes) == 0 {
			return nil
		}

		if err := tx.Where("group_id = ? AND key_hash IN ? AND status = ?", groupID, keyHashes, models.KeyStatusInvalid).Find(&keysToRestore).Error; err != nil {
			return err
		}

		if len(keysToRestore) == 0 {
			return nil
		}

		keyIDsToRestore := pluckIDs(keysToRestore)

		updates := map[string]any{
			"status":        models.KeyStatusActive,
			"failure_count": 0,
		}
		result := tx.Model(&models.APIKey{}).Where("id IN ?", keyIDsToRestore).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		restoredCount = result.RowsAffected

		for _, key := range keysToRestore {
			key.Status = models.KeyStatusActive
			key.FailureCount = 0
			if err := p.addKeyToStore(&key); err != nil {
				logrus.WithFields(logrus.Fields{"keyID": key.ID, "error": err}).Error("Failed to restore key in store after DB update")
				return err
			}
		}

		return nil
	})

	return restoredCount, err
}

// RemoveInvalidKeys 移除组内所有无效的 Key。
func (p *KeyProvider) RemoveInvalidKeys(groupID uint) (int64, error) {
	return p.removeKeysByStatus(groupID, models.KeyStatusInvalid)
}

// RemoveAllKeys 移除组内所有的 Key。
func (p *KeyProvider) RemoveAllKeys(groupID uint) (int64, error) {
	return p.removeKeysByStatus(groupID)
}

// removeKeysByStatus is a generic function to remove keys by status.
// If no status is provided, it removes all keys in the group.
func (p *KeyProvider) removeKeysByStatus(groupID uint, status ...string) (int64, error) {
	var keysToRemove []models.APIKey
	var removedCount int64

	err := p.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("group_id = ?", groupID)
		if len(status) > 0 {
			query = query.Where("status IN ?", status)
		}

		if err := query.Find(&keysToRemove).Error; err != nil {
			return err
		}

		if len(keysToRemove) == 0 {
			return nil
		}

		deleteQuery := tx.Where("group_id = ?", groupID)
		if len(status) > 0 {
			deleteQuery = deleteQuery.Where("status IN ?", status)
		}
		result := deleteQuery.Delete(&models.APIKey{})
		if result.Error != nil {
			return result.Error
		}
		removedCount = result.RowsAffected

		for _, key := range keysToRemove {
			if err := p.removeKeyFromStore(key.ID, key.GroupID); err != nil {
				logrus.WithFields(logrus.Fields{"keyID": key.ID, "error": err}).Error("Failed to remove key from store after DB deletion, rolling back transaction")
				return err
			}
		}
		return nil
	})

	return removedCount, err
}

// RemoveKeysFromStore 直接从内存存储中移除指定的键，不涉及数据库操作
// 这个方法适用于数据库已经删除但需要清理内存存储的场景
func (p *KeyProvider) RemoveKeysFromStore(groupID uint, keyIDs []uint) error {
	if len(keyIDs) == 0 {
		return nil
	}

	activeKeysListKey := fmt.Sprintf("group:%d:active_keys", groupID)

	// 第一步：直接删除整个 active_keys 列表
	if err := p.store.Delete(activeKeysListKey); err != nil {
		logrus.WithFields(logrus.Fields{
			"groupID": groupID,
			"error":   err,
		}).Error("Failed to delete active keys list")
		return err
	}

	// 第二步：批量删除所有相关的key hash
	for _, keyID := range keyIDs {
		keyHashKey := fmt.Sprintf("key:%d", keyID)
		if err := p.store.Delete(keyHashKey); err != nil {
			logrus.WithFields(logrus.Fields{
				"keyID": keyID,
				"error": err,
			}).Error("Failed to delete key hash")
		}
	}

	logrus.WithFields(logrus.Fields{
		"groupID":  groupID,
		"keyCount": len(keyIDs),
	}).Info("Successfully cleaned up group keys from store")

	return nil
}

// addKeyToStore is a helper to add a single key to the cache.
func (p *KeyProvider) addKeyToStore(key *models.APIKey) error {
	// 1. Store key details in HASH
	keyHashKey := fmt.Sprintf("key:%d", key.ID)
	keyDetails := p.apiKeyToMap(key)
	if err := p.store.HSet(keyHashKey, keyDetails); err != nil {
		return fmt.Errorf("failed to HSet key details for key %d: %w", key.ID, err)
	}

	// 2. If active, add to the active LIST
	if key.Status == models.KeyStatusActive {
		activeKeysListKey := fmt.Sprintf("group:%d:active_keys", key.GroupID)
		if err := p.store.LRem(activeKeysListKey, 0, key.ID); err != nil {
			return fmt.Errorf("failed to LRem key %d before LPush for group %d: %w", key.ID, key.GroupID, err)
		}
		if err := p.store.LPush(activeKeysListKey, key.ID); err != nil {
			return fmt.Errorf("failed to LPush key %d to group %d: %w", key.ID, key.GroupID, err)
		}
	}
	return nil
}

// removeKeyFromStore is a helper to remove a single key from the cache.
func (p *KeyProvider) removeKeyFromStore(keyID, groupID uint) error {
	activeKeysListKey := fmt.Sprintf("group:%d:active_keys", groupID)
	if err := p.store.LRem(activeKeysListKey, 0, keyID); err != nil {
		logrus.WithFields(logrus.Fields{"keyID": keyID, "groupID": groupID, "error": err}).Error("Failed to LRem key from active list")
	}

	keyHashKey := fmt.Sprintf("key:%d", keyID)
	if err := p.store.Delete(keyHashKey); err != nil {
		return fmt.Errorf("failed to delete key HASH for key %d: %w", keyID, err)
	}
	return nil
}

// apiKeyToMap converts an APIKey model to a map for HSET.
func (p *KeyProvider) apiKeyToMap(key *models.APIKey) map[string]any {
	baseWeight := key.BaseWeight
	if baseWeight <= 0 {
		baseWeight = 500
	}
	weight := key.Weight
	if weight <= 0 {
		weight = baseWeight
	}
	return map[string]any{
		"id":            fmt.Sprint(key.ID),
		"key_string":    key.KeyValue,
		"status":        key.Status,
		"base_weight":   baseWeight,
		"weight":        weight,
		"failure_count": key.FailureCount,
		"group_id":      key.GroupID,
		"created_at":    key.CreatedAt.Unix(),
	}
}

// pluckIDs extracts IDs from a slice of APIKey.
func pluckIDs(keys []models.APIKey) []uint {
	ids := make([]uint, len(keys))
	for i, key := range keys {
		ids[i] = key.ID
	}
	return ids
}

// UpdateKeyWeight 更新单个密钥的权重（同时更新base_weight和weight，并清除缓存命中记录）
func (p *KeyProvider) UpdateKeyWeight(keyID uint, weight int) error {
	if weight < 1 || weight > 1000 {
		return fmt.Errorf("weight must be between 1 and 1000")
	}

	return p.executeTransactionWithRetry(func(tx *gorm.DB) error {
		var key models.APIKey
		if err := tx.First(&key, keyID).Error; err != nil {
			return fmt.Errorf("failed to find key %d: %w", keyID, err)
		}

		// 同时更新 base_weight 和 weight
		if err := tx.Model(&key).Updates(map[string]any{
			"base_weight": weight,
			"weight":      weight,
		}).Error; err != nil {
			return fmt.Errorf("failed to update key weight in DB: %w", err)
		}

		keyHashKey := fmt.Sprintf("key:%d", keyID)
		if err := p.store.HSet(keyHashKey, map[string]any{
			"base_weight": weight,
			"weight":      weight,
		}); err != nil {
			return fmt.Errorf("failed to update key weight in store: %w", err)
		}

		// 清除该key的缓存命中记录
		p.clearCacheHitRecordsForKey(keyID)

		return nil
	})
}

// UpdateKeysWeight 批量更新密钥的权重（同时更新base_weight和weight，并清除缓存命中记录）
func (p *KeyProvider) UpdateKeysWeight(groupID uint, keyHashes []string, weight int) (int64, error) {
	if weight < 1 || weight > 1000 {
		return 0, fmt.Errorf("weight must be between 1 and 1000")
	}

	if len(keyHashes) == 0 {
		return 0, nil
	}

	var updatedCount int64

	err := p.executeTransactionWithRetry(func(tx *gorm.DB) error {
		var keys []models.APIKey
		if err := tx.Where("group_id = ? AND key_hash IN ?", groupID, keyHashes).Find(&keys).Error; err != nil {
			return fmt.Errorf("failed to find keys: %w", err)
		}

		if len(keys) == 0 {
			return nil
		}

		// 同时更新 base_weight 和 weight
		result := tx.Model(&models.APIKey{}).
			Where("group_id = ? AND key_hash IN ?", groupID, keyHashes).
			Updates(map[string]any{
				"base_weight": weight,
				"weight":      weight,
			})

		if result.Error != nil {
			return fmt.Errorf("failed to update keys weight in DB: %w", result.Error)
		}

		updatedCount = result.RowsAffected

		// 更新缓存并清除缓存命中记录
		for _, key := range keys {
			keyHashKey := fmt.Sprintf("key:%d", key.ID)
			if err := p.store.HSet(keyHashKey, map[string]any{
				"base_weight": weight,
				"weight":      weight,
			}); err != nil {
				logrus.WithFields(logrus.Fields{
					"keyID": key.ID,
					"error": err,
				}).Error("Failed to update key weight in store")
			}
			// 清除该key的缓存命中记录
			p.clearCacheHitRecordsForKey(key.ID)
		}

		return nil
	})

	return updatedCount, err
}

// ResetKeysWeight resets all keys' weights in a group to the default value (500)
// This also resets base_weight and clears cache hit records
func (p *KeyProvider) ResetKeysWeight(groupID uint) (int64, error) {
	const defaultWeight = 500
	var updatedCount int64

	err := p.executeTransactionWithRetry(func(tx *gorm.DB) error {
		// 同时重置 base_weight 和 weight
		result := tx.Model(&models.APIKey{}).
			Where("group_id = ?", groupID).
			Updates(map[string]any{
				"base_weight": defaultWeight,
				"weight":      defaultWeight,
			})

		if result.Error != nil {
			return fmt.Errorf("failed to reset keys weight in DB: %w", result.Error)
		}

		updatedCount = result.RowsAffected

		// 更新store中的权重并清除缓存命中记录
		var keys []models.APIKey
		if err := tx.Select("id").Where("group_id = ?", groupID).Find(&keys).Error; err != nil {
			return fmt.Errorf("failed to fetch keys for store update: %w", err)
		}

		for _, key := range keys {
			keyHashKey := fmt.Sprintf("key:%d", key.ID)
			if err := p.store.HSet(keyHashKey, map[string]any{
				"base_weight": defaultWeight,
				"weight":      defaultWeight,
			}); err != nil {
				logrus.WithFields(logrus.Fields{
					"keyID": key.ID,
					"error": err,
				}).Error("Failed to reset key weight in store")
			}
			// 清除该key的缓存命中记录
			p.clearCacheHitRecordsForKey(key.ID)
		}

		return nil
	})

	return updatedCount, err
}

// ResetSingleKeyWeight resets a single key's weight to its base_weight
func (p *KeyProvider) ResetSingleKeyWeight(keyID uint) error {
	return p.executeTransactionWithRetry(func(tx *gorm.DB) error {
		var key models.APIKey
		if err := tx.First(&key, keyID).Error; err != nil {
			return fmt.Errorf("failed to find key %d: %w", keyID, err)
		}

		baseWeight := key.BaseWeight
		if baseWeight <= 0 {
			baseWeight = 500
		}

		// 更新数据库中的weight为base_weight
		if err := tx.Model(&key).Update("weight", baseWeight).Error; err != nil {
			return fmt.Errorf("failed to reset key weight in DB: %w", err)
		}

		// 更新store中的weight
		keyHashKey := fmt.Sprintf("key:%d", keyID)
		if err := p.store.HSet(keyHashKey, map[string]any{"weight": baseWeight}); err != nil {
			return fmt.Errorf("failed to reset key weight in store: %w", err)
		}

		// 清除该key的缓存命中记录
		p.clearCacheHitRecordsForKey(keyID)

		return nil
	})
}

// SelectKeyWithCacheHit 支持缓存命中的key选择
func (p *KeyProvider) SelectKeyWithCacheHit(groupID uint, bodyBytes []byte, enableCacheHit bool) (*models.APIKey, error) {
	if !enableCacheHit {
		return p.SelectKey(groupID)
	}

	messages, size := ExtractMessages(bodyBytes)
	if size <= 4096 || len(messages) < 3 {
		return p.SelectKey(groupID)
	}

	// 尝试匹配：dropCount = 2, 4, 6
	for _, dropCount := range []int{2, 4, 6} {
		hash := CalculatePromptHash(messages, dropCount)
		if hash == "" {
			continue
		}
		entry, err := p.getCacheHitEntry(groupID, hash)
		if err == nil && entry != nil {
			// 命中：检查key是否仍然有效
			apiKey, err := p.getKeyDetails(groupID, uint64(entry.KeyID))
			if err != nil || apiKey.Status != models.KeyStatusActive {
				// key已失效，删除缓存条目并恢复权重
				cacheKey := fmt.Sprintf("cache_hit:group:%d:hash:%s", groupID, hash)
				p.store.Delete(cacheKey)
				p.removeCacheHitRecord(cacheKey)
				p.AdjustKeyWeightAsync(entry.KeyID, 1) // 删除hash，权重+1
				continue
			}

			// 记录新hash（如果与命中的不同）
			newHash := CalculatePromptHash(messages, 2)
			if newHash != "" && newHash != hash {
				p.setCacheHitEntry(groupID, newHash, entry.KeyID)
				p.AdjustKeyWeightAsync(entry.KeyID, -1) // 新hash创建，权重-1
			}

			// 延迟删除旧hash（如果dropCount > 2表示是旧hash命中）
			if dropCount > 2 {
				p.scheduleHashDeletion(groupID, hash, entry.KeyID)
			}

			logrus.WithFields(logrus.Fields{
				"groupID":   groupID,
				"keyID":     entry.KeyID,
				"dropCount": dropCount,
			}).Debug("Cache hit enhancement: matched existing hash")

			return apiKey, nil
		}
	}

	// 未命中：随机选key，记录hash，权重-1
	key, err := p.SelectKey(groupID)
	if err != nil {
		return nil, err
	}

	newHash := CalculatePromptHash(messages, 2)
	if newHash != "" {
		p.setCacheHitEntry(groupID, newHash, key.ID)
		p.AdjustKeyWeightAsync(key.ID, -1)
		logrus.WithFields(logrus.Fields{
			"groupID": groupID,
			"keyID":   key.ID,
			"hash":    newHash[:8] + "...",
		}).Debug("Cache hit enhancement: created new hash entry")
	}

	return key, nil
}

// getCacheHitEntry 获取缓存条目
func (p *KeyProvider) getCacheHitEntry(groupID uint, hash string) (*CacheHitEntry, error) {
	cacheKey := fmt.Sprintf("cache_hit:group:%d:hash:%s", groupID, hash)
	data, err := p.store.Get(cacheKey)
	if err != nil {
		return nil, err
	}
	var entry CacheHitEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// setCacheHitEntry 设置缓存条目（10分钟过期）
func (p *KeyProvider) setCacheHitEntry(groupID uint, hash string, keyID uint) {
	cacheKey := fmt.Sprintf("cache_hit:group:%d:hash:%s", groupID, hash)
	expTime := time.Now().Add(10 * time.Minute).Unix()
	entry := CacheHitEntry{KeyID: keyID, ExpTime: expTime}
	data, _ := json.Marshal(entry)
	p.store.Set(cacheKey, data, 10*time.Minute)

	// 跟踪条目以便定期清理
	p.cacheHitMu.Lock()
	p.cacheHitRecords[cacheKey] = &cacheHitRecord{
		GroupID: groupID,
		Hash:    hash,
		KeyID:   keyID,
		ExpTime: expTime,
	}
	p.cacheHitMu.Unlock()
}

// removeCacheHitRecord 从跟踪map中移除记录
func (p *KeyProvider) removeCacheHitRecord(cacheKey string) {
	p.cacheHitMu.Lock()
	delete(p.cacheHitRecords, cacheKey)
	p.cacheHitMu.Unlock()
}

// clearCacheHitRecordsForKey 清除指定key的所有缓存命中记录
func (p *KeyProvider) clearCacheHitRecordsForKey(keyID uint) {
	p.cacheHitMu.Lock()
	var keysToDelete []string
	for cacheKey, record := range p.cacheHitRecords {
		if record.KeyID == keyID {
			keysToDelete = append(keysToDelete, cacheKey)
		}
	}
	for _, cacheKey := range keysToDelete {
		delete(p.cacheHitRecords, cacheKey)
		// 从store中删除
		p.store.Delete(cacheKey)
	}
	p.cacheHitMu.Unlock()

	if len(keysToDelete) > 0 {
		logrus.WithFields(logrus.Fields{
			"keyID": keyID,
			"count": len(keysToDelete),
		}).Debug("Cache hit enhancement: cleared cache hit records for key")
	}
}

// scheduleHashDeletion 延迟5分钟删除hash并恢复权重
func (p *KeyProvider) scheduleHashDeletion(groupID uint, hash string, keyID uint) {
	go func() {
		time.Sleep(5 * time.Minute)
		cacheKey := fmt.Sprintf("cache_hit:group:%d:hash:%s", groupID, hash)
		if err := p.store.Delete(cacheKey); err == nil {
			p.AdjustKeyWeightAsync(keyID, 1)
			logrus.WithFields(logrus.Fields{
				"groupID": groupID,
				"keyID":   keyID,
			}).Debug("Cache hit enhancement: deleted old hash, restored weight")
		}
		// 从跟踪map中删除
		p.removeCacheHitRecord(cacheKey)
	}()
}

// AdjustKeyWeightAsync 异步调整权重，上限为 base_weight
func (p *KeyProvider) AdjustKeyWeightAsync(keyID uint, delta int) {
	go func() {
		keyHashKey := fmt.Sprintf("key:%d", keyID)
		details, err := p.store.HGetAll(keyHashKey)
		if err != nil {
			return
		}
		currentWeight, _ := strconv.Atoi(details["weight"])
		baseWeight, _ := strconv.Atoi(details["base_weight"])
		if baseWeight <= 0 {
			baseWeight = 500
		}
		newWeight := currentWeight + delta
		if newWeight < 1 {
			newWeight = 1
		}
		if newWeight > baseWeight {
			newWeight = baseWeight
		}
		p.store.HSet(keyHashKey, map[string]any{"weight": newWeight})
	}()
}

// startCacheHitCleanup 启动定期清理过期hash的goroutine
func (p *KeyProvider) startCacheHitCleanup(ctx context.Context) {
	cleanupTicker := time.NewTicker(1 * time.Minute)
	syncTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()
	defer syncTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanupTicker.C:
			p.cleanupExpiredCacheHitEntries()
		case <-syncTicker.C:
			p.syncWeightsToDatabase()
		}
	}
}

// cleanupExpiredCacheHitEntries 清理过期的cache_hit条目并恢复权重
func (p *KeyProvider) cleanupExpiredCacheHitEntries() {
	now := time.Now().Unix()
	var expiredRecords []*cacheHitRecord

	// 收集过期的条目
	p.cacheHitMu.RLock()
	for _, record := range p.cacheHitRecords {
		if record.ExpTime <= now {
			expiredRecords = append(expiredRecords, record)
		}
	}
	p.cacheHitMu.RUnlock()

	if len(expiredRecords) == 0 {
		return
	}

	// 清理过期条目并恢复权重
	for _, record := range expiredRecords {
		cacheKey := fmt.Sprintf("cache_hit:group:%d:hash:%s", record.GroupID, record.Hash)

		// 从store中删除（可能已经被TTL自动删除）
		p.store.Delete(cacheKey)

		// 恢复权重
		p.AdjustKeyWeightAsync(record.KeyID, 1)

		// 从跟踪map中删除
		p.removeCacheHitRecord(cacheKey)

		logrus.WithFields(logrus.Fields{
			"groupID": record.GroupID,
			"keyID":   record.KeyID,
			"hash":    record.Hash[:8] + "...",
		}).Debug("Cache hit enhancement: cleaned up expired hash, restored weight")
	}

	if len(expiredRecords) > 0 {
		logrus.WithField("count", len(expiredRecords)).Debug("Cache hit enhancement: cleanup completed")
	}
}

// StopCacheHitCleanup 停止定期清理goroutine
func (p *KeyProvider) StopCacheHitCleanup() {
	if p.cleanupCancel != nil {
		p.cleanupCancel()
	}
}

// syncWeightsToDatabase 将store中的权重同步到数据库
func (p *KeyProvider) syncWeightsToDatabase() {
	// 获取所有活跃的key
	var keys []models.APIKey
	if err := p.db.Select("id", "weight").Find(&keys).Error; err != nil {
		logrus.WithError(err).Error("Failed to fetch keys for weight sync")
		return
	}

	if len(keys) == 0 {
		return
	}

	updatedCount := 0
	for _, key := range keys {
		keyHashKey := fmt.Sprintf("key:%d", key.ID)
		details, err := p.store.HGetAll(keyHashKey)
		if err != nil {
			continue
		}

		storeWeight, _ := strconv.Atoi(details["weight"])
		if storeWeight <= 0 {
			storeWeight = 500
		}

		// 只有权重不同时才更新数据库
		if storeWeight != key.Weight {
			if err := p.db.Model(&models.APIKey{}).Where("id = ?", key.ID).Update("weight", storeWeight).Error; err != nil {
				logrus.WithFields(logrus.Fields{
					"keyID": key.ID,
					"error": err,
				}).Error("Failed to sync weight to database")
			} else {
				updatedCount++
			}
		}
	}

	if updatedCount > 0 {
		logrus.WithField("count", updatedCount).Debug("Weight sync: updated keys in database")
	}
}

// GetRealTimeWeight 从store获取key的实时权重
func (p *KeyProvider) GetRealTimeWeight(keyID uint) int {
	keyHashKey := fmt.Sprintf("key:%d", keyID)
	details, err := p.store.HGetAll(keyHashKey)
	if err != nil {
		return 0 // 返回0表示未找到，调用方可使用数据库值
	}
	weight, _ := strconv.Atoi(details["weight"])
	return weight
}
