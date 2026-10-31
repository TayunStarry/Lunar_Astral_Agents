package agentchat

// util.go 消息内容与文本截断的公共工具。

import "strings"

// StringOf 把消息 content（字符串 / 片段数组）还原为纯文本
func StringOf(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if part, ok := item.(map[string]any); ok {
				if text, ok := part["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// TruncateRunes 按字符截断（超长时以省略号收尾）
func TruncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// HardCutRunes 按字符硬切断（超长时以省略号收尾）
func HardCutRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit < 1 {
		limit = 1
	}
	return string(runes[:limit-1]) + "…"
}
