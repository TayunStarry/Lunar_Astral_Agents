package websearch

import (
	"strings"
)

// isMemoryEmpty 判断记忆检索结果是否为空/无意义
func isMemoryEmpty(result string) bool {
	emptyMarkers := []string{
		"未找到相关", "没有找到", "无相关", "暂无",
		"记忆库为空", "记忆库中没有", "no results",
	}
	for _, m := range emptyMarkers {
		if strings.Contains(strings.ToLower(result), strings.ToLower(m)) {
			return true
		}
	}
	return len(strings.TrimSpace(result)) < 20
}

// truncateToRunes 按 rune 截断字符串
func truncateToRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// extractFirstItemFromList 从列表格式文本中提取第一条内容
func extractFirstItemFromList(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "0123456789.")
		line = strings.TrimLeft(line, ")")
		line = strings.TrimLeft(line, "-•")
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "*")
		line = strings.TrimSpace(line)
		if len(line) > 4 {
			cutIdx := strings.IndexAny(line, "（(")
			if cutIdx > 10 {
				line = line[:cutIdx]
			}
			cutIdx = strings.IndexAny(line, "，,")
			if cutIdx > 15 {
				line = line[:cutIdx]
			}
			line = cleanSearchQuery(line)
			if len([]rune(line)) > 40 {
				line = string([]rune(line)[:40])
			}
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// cleanSearchQuery 清理搜索查询中的特殊字符
func cleanSearchQuery(query string) string {
	var builder strings.Builder
	builder.Grow(len(query))
	for _, r := range query {
		switch r {
		case 0x201C, 0x201D, 0x2018, 0x2019, 0x0022, 0x0027,
			0x300A, 0x300B, 0x3010, 0x3011, 0x300C, 0x300D, 0x300E, 0x300F:
			continue
		case 0x2014, 0x2013:
			builder.WriteRune(' ')
		default:
			builder.WriteRune(r)
		}
	}
	result := builder.String()
	result = strings.ReplaceAll(result, " - ", " ")
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return strings.TrimSpace(result)
}