package cache

// 缓存管理逻辑：按群号分组的消息缓存 + 摘要系统

import (
	"fmt"
	"strings"

	"bridge_adapter/pkg/config"
	"bridge_adapter/pkg/logger"
	"bridge_adapter/pkg/types"
)

// AddMessage 将消息添加到对应群聊的缓存中。
// 超过上限时自动淘汰最早的消息。
func AddMessage(msg types.CachedMessage) {
	CacheMutex.Lock()
	defer CacheMutex.Unlock()

	groupID := msg.GroupID
	cache, exists := GroupCaches[groupID]
	if !exists {
		cache = &types.GroupCache{GroupID: groupID}
		GroupCaches[groupID] = cache
	}

	cache.Messages = append(cache.Messages, msg)
	maxSize := config.GetMaxGroupCache()

	if len(cache.Messages) > maxSize {
		// 淘汰最早的消息，保留最新的 maxSize 条
		cache.Messages = cache.Messages[len(cache.Messages)-maxSize:]
	}

	logger.Debug("群 %d 缓存消息数: %d/%d", groupID, len(cache.Messages), maxSize)
}

// GetGroupMessages 获取指定群聊的所有缓存消息（只读副本）
func GetGroupMessages(groupID int64) []types.CachedMessage {
	CacheMutex.RLock()
	defer CacheMutex.RUnlock()

	cache, exists := GroupCaches[groupID]
	if !exists {
		return nil
	}

	result := make([]types.CachedMessage, len(cache.Messages))
	copy(result, cache.Messages)
	return result
}

// ClearGroupCache 清除指定群聊的消息缓存
func ClearGroupCache(groupID int64) {
	CacheMutex.Lock()
	defer CacheMutex.Unlock()

	delete(GroupCaches, groupID)
	logger.Info("已清除群 %d 的消息缓存", groupID)
}

// GenerateSummary 为指定群聊生成消息摘要。
// 摘要格式：将缓存中的消息按发送者+内容格式化拼接。
func GenerateSummary(groupID int64, triggerUser string, keyword string) types.SummaryEntry {
	messages := GetGroupMessages(groupID)
	if len(messages) == 0 {
		return types.SummaryEntry{
			TriggerUser: triggerUser,
			Keyword:     keyword,
			Content:     "(空缓存)",
		}
	}

	var sb strings.Builder
	for i, msg := range messages {
		senderName := config.GetUserName(msg.GroupID, msg.UserID)
		if str, ok := msg.Content.(string); ok {
			sb.WriteString(fmt.Sprintf("%s: %s", senderName, str))
		} else {
			sb.WriteString(fmt.Sprintf("%s: [多媒体消息]", senderName))
		}
		if i < len(messages)-1 {
			sb.WriteString("; ")
		}
	}

	return types.SummaryEntry{
		TriggerUser: triggerUser,
		Keyword:     keyword,
		Content:     sb.String(),
	}
}

// AddSummary 将摘要添加到指定群聊的摘要列表中。
// 超过上限时自动淘汰最早的摘要。
func AddSummary(groupID int64, summary types.SummaryEntry) {
	CacheMutex.Lock()
	defer CacheMutex.Unlock()

	summaries := GroupSummaries[groupID]
	summaries = append(summaries, summary)
	maxSize := config.GetMaxGroupSummary()

	if len(summaries) > maxSize {
		summaries = summaries[len(summaries)-maxSize:]
	}

	GroupSummaries[groupID] = summaries
	logger.Info("群 %d 摘要数: %d/%d", groupID, len(summaries), maxSize)
}

// GetAllSummaries 获取所有群聊的摘要（只读副本），用于AI路由上下文
func GetAllSummaries() map[int64][]types.SummaryEntry {
	CacheMutex.RLock()
	defer CacheMutex.RUnlock()

	result := make(map[int64][]types.SummaryEntry, len(GroupSummaries))
	for gid, summaries := range GroupSummaries {
		copied := make([]types.SummaryEntry, len(summaries))
		copy(copied, summaries)
		result[gid] = copied
	}
	return result
}

// GetGroupSummaryCount 获取指定群聊的摘要数量
func GetGroupSummaryCount(groupID int64) int {
	CacheMutex.RLock()
	defer CacheMutex.RUnlock()

	return len(GroupSummaries[groupID])
}

// GetCacheStats 获取缓存统计信息（用于日志和监控）
func GetCacheStats() (totalGroups int, totalMessages int, totalSummaries int) {
	CacheMutex.RLock()
	defer CacheMutex.RUnlock()

	totalGroups = len(GroupCaches)
	for _, cache := range GroupCaches {
		totalMessages += len(cache.Messages)
	}
	for _, summaries := range GroupSummaries {
		totalSummaries += len(summaries)
	}
	return
}
