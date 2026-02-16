package keypool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"time"
)

const (
	sessionIDMinLen = 8
	sessionIDMaxLen = 256
	defaultCacheTTL = 5 * time.Minute
	longCacheTTL    = 1 * time.Hour
)

var sessionIDPattern = regexp.MustCompile(`^[\w\-.:]+$`)

// CacheHitEntry 缓存条目
type CacheHitEntry struct {
	KeyID   uint  `json:"key_id"`
	ExpTime int64 `json:"exp_time"`
}

// CalculatePromptHash 计算prompt哈希（自动剔除 cache_control 字段避免影响命中率）
// dropCount: 从末尾移除的message数量
func CalculatePromptHash(messages []json.RawMessage, dropCount int) string {
	if dropCount >= len(messages) || len(messages)-dropCount < 1 {
		return ""
	}
	truncated := messages[:len(messages)-dropCount]
	cleaned := stripCacheControl(truncated)
	data, err := json.Marshal(cleaned)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]) // 返回32位hex（16字节）
}

// stripCacheControl 从 messages 副本中移除所有 cache_control 字段，不修改原始数据
func stripCacheControl(messages []json.RawMessage) []json.RawMessage {
	result := make([]json.RawMessage, len(messages))
	for i, raw := range messages {
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			result[i] = raw
			continue
		}

		// 移除消息顶层的 cache_control
		modified := false
		if _, ok := msg["cache_control"]; ok {
			delete(msg, "cache_control")
			modified = true
		}

		// 处理 content 数组中每个 block 的 cache_control
		if contentRaw, ok := msg["content"]; ok {
			var contentArr []map[string]json.RawMessage
			if err := json.Unmarshal(contentRaw, &contentArr); err == nil {
				cleaned := false
				newContent := make([]map[string]json.RawMessage, len(contentArr))
				for j, block := range contentArr {
					if _, has := block["cache_control"]; has {
						// 复制 block 并移除 cache_control
						cp := make(map[string]json.RawMessage, len(block)-1)
						for k, v := range block {
							if k != "cache_control" {
								cp[k] = v
							}
						}
						newContent[j] = cp
						cleaned = true
					} else {
						newContent[j] = block
					}
				}
				if cleaned {
					if b, err := json.Marshal(newContent); err == nil {
						msg["content"] = b
						modified = true
					}
				}
			}
		}

		if modified {
			if b, err := json.Marshal(msg); err == nil {
				result[i] = b
			} else {
				result[i] = raw
			}
		} else {
			result[i] = raw
		}
	}
	return result
}

// ExtractMessages 从请求体提取messages并返回字节大小
func ExtractMessages(bodyBytes []byte) ([]json.RawMessage, int) {
	var body struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return nil, 0
	}
	messagesBytes, _ := json.Marshal(body.Messages)
	return body.Messages, len(messagesBytes)
}

// ExtractSessionID 从请求头和请求体中提取 Session ID
// 优先级：Header session_id → Header x-session-id → Body metadata.session_id → Body prompt_cache_key → Body previous_response_id
func ExtractSessionID(bodyBytes []byte, headers http.Header) string {
	// 1. Header: session_id
	if id := headers.Get("session_id"); validateSessionID(id) {
		return id
	}
	// 2. Header: x-session-id
	if id := headers.Get("x-session-id"); validateSessionID(id) {
		return id
	}

	// 解析 body 提取候选值
	var body struct {
		Metadata struct {
			SessionID string `json:"session_id"`
		} `json:"metadata"`
		PromptCacheKey     string `json:"prompt_cache_key"`
		PreviousResponseID string `json:"previous_response_id"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return ""
	}

	// 3. Body: metadata.session_id
	if validateSessionID(body.Metadata.SessionID) {
		return body.Metadata.SessionID
	}
	// 4. Body: prompt_cache_key
	if validateSessionID(body.PromptCacheKey) {
		return body.PromptCacheKey
	}
	// 5. Body: previous_response_id（加前缀区分）
	if validateSessionID(body.PreviousResponseID) {
		return "prev_" + body.PreviousResponseID
	}

	return ""
}

// DetectCacheTTL 根据 messages 中 cache_control 标记检测缓存 TTL
// ephemeral + ttl="1h" → 1小时，其他有 cache_control → 5分钟，无 cache_control → 5分钟（默认）
func DetectCacheTTL(bodyBytes []byte) time.Duration {
	var body struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return defaultCacheTTL
	}

	for _, msg := range body.Messages {
		// content 可能是字符串或数组
		var blocks []struct {
			CacheControl *struct {
				Type string `json:"type"`
				TTL  string `json:"ttl"`
			} `json:"cache_control"`
		}
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}
		for _, block := range blocks {
			if block.CacheControl != nil {
				if block.CacheControl.Type == "ephemeral" && block.CacheControl.TTL == "1h" {
					return longCacheTTL
				}
			}
		}
	}

	return defaultCacheTTL
}

// validateSessionID 校验 Session ID 格式
func validateSessionID(id string) bool {
	if len(id) < sessionIDMinLen || len(id) > sessionIDMaxLen {
		return false
	}
	return sessionIDPattern.MatchString(id)
}
