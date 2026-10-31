package LunarPersonality

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/knowledge"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// loadKnowledge 读取整表 [key,text] 条目
func loadKnowledge(table string) map[string]string {
	entries, err := knowledge.LoadEntries(table)
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
	return knowledge.LoadEntries(table)
}

// saveKnowledgeEntries 全表替换写入
func saveKnowledgeEntries(table string, entries [][]string) bool {
	if err := knowledge.ReplaceEntries(table, entries); err != nil {
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
	if err := knowledge.ReplaceEntries(table, entries); err != nil {
		logger.Warn(lunarCoreModule, "[知识库] 写入表 %s 失败: %v", table, err)
		return false
	}
	return true
}

// getPromptFromKnowledge 读取某来源 URL 的唯一解析文本（媒体理解缓存）
func getPromptFromKnowledge(key string) string {
	raw := strings.TrimSpace(key)
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "data:") {
		return ""
	}
	return resource.ParseTextOf(raw)
}

// getPromptFromKnowledgeAny 按多个候选键查找媒体理解缓存
func getPromptFromKnowledgeAny(keys []string) string {
	for _, key := range keys {
		if value := getPromptFromKnowledge(key); value != "" {
			return value
		}
	}
	return ""
}

// savePromptToKnowledge 写入某来源 URL 的唯一解析文本
func savePromptToKnowledge(key, prompt string) bool {
	raw := strings.TrimSpace(key)
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "data:") {
		return false
	}
	if err := resource.SetParseText(raw, prompt); err != nil {
		logger.Warn(lunarCoreModule, "[知识库] 写入解析文本失败: %v", err)
		return false
	}
	return true
}
