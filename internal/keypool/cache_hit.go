package keypool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CacheHitEntry 缓存条目
type CacheHitEntry struct {
	KeyID   uint  `json:"key_id"`
	ExpTime int64 `json:"exp_time"`
}

// CalculatePromptHash 计算prompt哈希
// dropCount: 从末尾移除的message数量
func CalculatePromptHash(messages []json.RawMessage, dropCount int) string {
	if dropCount >= len(messages) || len(messages)-dropCount < 1 {
		return ""
	}
	truncated := messages[:len(messages)-dropCount]
	data, err := json.Marshal(truncated)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]) // 返回32位hex（16字节）
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
