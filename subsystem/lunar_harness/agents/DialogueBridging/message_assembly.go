package DialogueBridging

// 消息解析与单线程收发流程（按职责拆分）

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/mediaobject"
)

func snapshotGroupPool(groupID int64) []GroupPoolEntry {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	pool := groupPools[groupID]
	if pool == nil {
		return nil
	}
	out := make([]GroupPoolEntry, len(pool.Entries))
	copy(out, pool.Entries)
	return out
}

// addToGroupPool 添加消息到群聊缓存池，超出容量时抛弃最老消息
func addToGroupPool(groupID int64, groupName string, entry GroupPoolEntry) {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	pool := groupPools[groupID]
	if pool == nil {
		pool = &GroupPool{GroupID: groupID, GroupName: groupName}
		groupPools[groupID] = pool
	}
	pool.Entries = append(pool.Entries, entry)
	if len(pool.Entries) > maxGroupPoolSize {
		pool.Entries = pool.Entries[len(pool.Entries)-maxGroupPoolSize:]
	}
}

// clearGroupPool 清空指定群聊的缓存池
func clearGroupPool(groupID int64) {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	delete(groupPools, groupID)
}

// buildGroupMessages 将群聊缓存池条目构建为 OpenAI 格式消息（带群聊前缀）
func buildGroupMessages(groupName string, entries []GroupPoolEntry) []map[string]any {
	messages := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		prefix := fmt.Sprintf("[群聊: %s][用户: %s]: ", groupName, e.Nickname)
		messages = append(messages, buildUserMessage(prefix, e.Content, e.HasMedia))
	}
	return messages
}

// collectGroupAudioObjects 汇总群聊缓存池条目中的音频多模态对象
func collectGroupAudioObjects(entries []GroupPoolEntry) []mediaobject.Object {
	var objects []mediaobject.Object
	for _, e := range entries {
		objects = append(objects, e.AudioObjects...)
	}
	return objects
}

// contentToText 从解析结果（string 或 多模态数组）中提取纯文本
func contentToText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]map[string]any); ok {
		var sb strings.Builder
		for _, item := range arr {
			if item["type"] == "text" {
				if t, ok := item["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	}
	return ""
}

// buildUserMessage 将内容与发送者前缀组装为单条 OpenAI 消息
func buildUserMessage(prefix string, content any, hasImages bool) map[string]any {
	if hasImages {
		arr, _ := content.([]map[string]any)
		withPrefix := append([]map[string]any{{"type": "text", "text": prefix}}, arr...)
		return map[string]any{"role": "user", "content": withPrefix}
	}
	text, _ := content.(string)
	return map[string]any{"role": "user", "content": prefix + text}
}

// enqueueRequest 将请求加入队列并尝试推进
