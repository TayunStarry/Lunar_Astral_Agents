package websearch

import (
	"fmt"
	"strings"
)

// formatResults 格式化搜索结果为自然语言文本
func formatResults(results []SearchResult) string {
	var builder strings.Builder
	for _, r := range results {
		builder.WriteString(fmt.Sprintf("「%s」", r.Title))
		if r.Snippet != "" {
			builder.WriteString(fmt.Sprintf("：%s", r.Snippet))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

// formatResultsForLLM 格式化结果供 LLM 使用（含来源 URL）
func formatResultsForLLM(results []SearchResult) string {
	var builder strings.Builder
	for i, r := range results {
		builder.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, r.Title))
		if r.Snippet != "" {
			builder.WriteString(fmt.Sprintf("   %s\n", r.Snippet))
		}
		builder.WriteString(fmt.Sprintf("   来源: %s\n\n", r.URL))
	}
	return builder.String()
}

// formatDeepResultsFallback 深层搜索无 LLM 时的回退格式化
func formatDeepResultsFallback(query string, contentParts []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索 %q 的结果：\n\n", query))
	sb.WriteString(strings.Join(contentParts, "\n\n"))
	return sb.String()
}
