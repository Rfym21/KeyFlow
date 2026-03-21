package keypool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
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
// 优先级：Header session_id → Header x-session-id → Body metadata(SHA-256 hash) → Body prompt_cache_key → Body previous_response_id
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
		Metadata           json.RawMessage `json:"metadata"`
		PromptCacheKey     string          `json:"prompt_cache_key"`
		PreviousResponseID string          `json:"previous_response_id"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return ""
	}

	// 3. Body: metadata（不区分格式，直接对原始内容取 SHA-256 作为 session ID）
	if id := hashRawMetadata(body.Metadata); validateSessionID(id) {
		return id
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

// hashRawMetadata 对 metadata 原始内容取 SHA-256 作为 session ID
func hashRawMetadata(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	hash := sha256.Sum256(raw)
	return "meta_" + hex.EncodeToString(hash[:16])
}

// CacheControlResult 缓存控制检测结果
type CacheControlResult struct {
	Found bool          // 请求体中是否存在 cache_control 标记
	TTL   time.Duration // 缓存 TTL
}

// DetectCacheControl 检测请求体中是否存在 cache_control 标记并返回对应 TTL
// 检测范围：system、messages.content、tools、顶层 cache_control
// ephemeral + ttl="1h" → 1小时，其他有 cache_control → 5分钟，无 cache_control → Found=false
func DetectCacheControl(bodyBytes []byte) CacheControlResult {
	// cacheControlBlock 用于解析含 cache_control 字段的 block
	type cacheControlBlock struct {
		CacheControl *struct {
			Type string `json:"type"`
			TTL  string `json:"ttl"`
		} `json:"cache_control"`
	}

	// bestResult 记录最长 TTL 的结果
	best := CacheControlResult{Found: false, TTL: defaultCacheTTL}

	// checkBlock 检查单个 block 的 cache_control
	checkBlock := func(cc *struct {
		Type string `json:"type"`
		TTL  string `json:"ttl"`
	}) {
		if cc == nil {
			return
		}
		best.Found = true
		if cc.Type == "ephemeral" && cc.TTL == "1h" {
			best.TTL = longCacheTTL
		}
	}

	// checkContentBlocks 检查 content（可能是字符串或数组）
	checkContentBlocks := func(content json.RawMessage) {
		var blocks []cacheControlBlock
		if json.Unmarshal(content, &blocks) == nil {
			for _, b := range blocks {
				checkBlock(b.CacheControl)
			}
		}
	}

	var body struct {
		CacheControl *struct {
			Type string `json:"type"`
			TTL  string `json:"ttl"`
		} `json:"cache_control"`
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Tools []cacheControlBlock `json:"tools"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return best
	}

	// 1. 顶层 cache_control（自动缓存模式）
	checkBlock(body.CacheControl)

	// 2. system（可能是字符串或 content block 数组）
	checkContentBlocks(body.System)

	// 3. messages.content
	for _, msg := range body.Messages {
		checkContentBlocks(msg.Content)
	}

	// 4. tools
	for _, tool := range body.Tools {
		checkBlock(tool.CacheControl)
	}

	return best
}

// RequiresCacheControl 判断是否需要请求体包含 cache_control 才启用缓存命中增强
// 仅 Anthropic 渠道且 model 含 "claude" 时需要
func RequiresCacheControl(channelType string, bodyBytes []byte) bool {
	if !strings.EqualFold(channelType, "anthropic") {
		return false
	}
	var body struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(body.Model), "claude")
}

// validateSessionID 校验 Session ID 格式
func validateSessionID(id string) bool {
	if len(id) < sessionIDMinLen || len(id) > sessionIDMaxLen {
		return false
	}
	return sessionIDPattern.MatchString(id)
}
