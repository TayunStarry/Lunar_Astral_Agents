package lunar

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

// loadKnowledge 读取整表 [key,text] 条目
func loadKnowledge(table string) map[string]string {
	entries, err := persistence.KnowledgeLoadEntries(table)
	result := make(map[string]string, len(entries))
	if err != nil {
		logger.Warn(lunarCoreModule, "[知识库] 读取表 %s 失败: %v", table, err)
		return result
	}
	for _, pair := range entries {
		if len(pair) >= 1 && pair[0] != "" {
			value := ""
			if len(pair) >= 2 {
				value = pair[1]
			}
			result[pair[0]] = value
		}
	}
	return result
}

// loadKnowledgeEntries 读取整表原始二元组
func loadKnowledgeEntries(table string) ([][]string, error) {
	return persistence.KnowledgeLoadEntries(table)
}

// saveKnowledgeEntries 全表替换写入
func saveKnowledgeEntries(table string, entries [][]string) bool {
	if err := persistence.KnowledgeReplaceEntries(table, entries); err != nil {
		logger.Warn(lunarCoreModule, "[知识库] 写入表 %s 失败: %v", table, err)
		return false
	}
	return true
}

// saveKnowledge 将键值表写回对应逻辑表
func saveKnowledge(table string, values map[string]string) bool {
	entries := make([][]string, 0, len(values))
	for key, value := range values {
		entries = append(entries, []string{key, value})
	}
	if err := persistence.KnowledgeReplaceEntries(table, entries); err != nil {
		logger.Warn(lunarCoreModule, "[知识库] 写入表 %s 失败: %v", table, err)
		return false
	}
	return true
}

// getPromptFromKnowledge 从知识库获取媒体理解缓存
func getPromptFromKnowledge(key string) string {
	return loadKnowledge("video_summary")[key]
}

// getPromptFromKnowledgeAny 按多个候选键查找媒体理解缓存
func getPromptFromKnowledgeAny(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	values := loadKnowledge("video_summary")
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}

// savePromptToKnowledge 保存媒体理解缓存
func savePromptToKnowledge(key, prompt string) bool {
	values := loadKnowledge("video_summary")
	values[key] = prompt
	return saveKnowledge("video_summary", values)
}
