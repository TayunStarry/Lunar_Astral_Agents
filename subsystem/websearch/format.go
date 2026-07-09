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

// formatResultsTruncated 格式化搜索结果，对Snippet做截断保护
// 用于深度研究等可能产生大量结果的场景，防止提示词溢出
func formatResultsTruncated(results []SearchResult, maxSnippetLen int) string {
	var builder strings.Builder
	for _, r := range results {
		snippet := r.Snippet
		runes := []rune(snippet)
		if len(runes) > maxSnippetLen {
			snippet = string(runes[:maxSnippetLen]) + "..."
		}
		builder.WriteString(fmt.Sprintf("「%s」", r.Title))
		if snippet != "" {
			builder.WriteString(fmt.Sprintf("：%s", snippet))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

// formatWebpageResultsFallback 网页搜索无 LLM 时的回退格式化（带截断保护）
func formatWebpageResultsFallback(query string, contentParts []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索 %q 的结果：\n\n", query))
	content := strings.Join(contentParts, "\n\n")
	runes := []rune(content)
	if len(runes) > webpageMaxFallbackChars {
		content = string(runes[:webpageMaxFallbackChars]) + "\n\n[内容已截断]"
	}
	sb.WriteString(content)
	return sb.String()
}
