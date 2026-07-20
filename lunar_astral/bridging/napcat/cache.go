package napcat

// 消息缓存管理：FIFO策略，最大容量20条

import (
	"logger"
)

// AddCachedMessage 添加消息到缓存，超出容量时自动抛弃最老消息(FIFO)
func AddCachedMessage(msg CachedMessage) {
	messageCache.mu.Lock()
	defer messageCache.mu.Unlock()

	messageCache.Messages = append(messageCache.Messages, msg)

	if len(messageCache.Messages) > maxCacheSize {
		// FIFO: 移除最老的消息
		messageCache.Messages = messageCache.Messages[len(messageCache.Messages)-maxCacheSize:]
	}

	logger.SubInfo("LunarCore", "Napcat", "群 %d 缓存消息数: %d/%d", msg.GroupID, len(messageCache.Messages), maxCacheSize)
}

// GetCachedMessages 获取所有缓存消息的只读副本
func GetCachedMessages() []CachedMessage {
	messageCache.mu.RLock()
	defer messageCache.mu.RUnlock()

	result := make([]CachedMessage, len(messageCache.Messages))
	copy(result, messageCache.Messages)
	return result
}

// ClearCachedMessages 清空所有缓存消息
func ClearCachedMessages() {
	messageCache.mu.Lock()
	defer messageCache.mu.Unlock()

	messageCache.Messages = messageCache.Messages[:0]
	logger.SubInfo("LunarCore", "Napcat", "已清空消息缓存")
}

// GetCacheSize 获取当前缓存大小
func GetCacheSize() int {
	messageCache.mu.RLock()
	defer messageCache.mu.RUnlock()

	return len(messageCache.Messages)
}
