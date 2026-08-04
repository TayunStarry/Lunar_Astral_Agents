package websearch

import (
	"fmt"
	"strings"
)

// formatResults 格式化搜索结果为文本
func formatResults(results []SearchResult) string {
	var builder strings.Builder
	for i, r := range results {
		authLabel := ""
		if r.AuthorityScore > 0 {
			authLabel = " " + AuthorityLabel(r.AuthorityScore)
		}
		builder.WriteString(fmt.Sprintf("[%d]%s %s\n", i+1, authLabel, r.Title))
		if r.URL != "" {
			builder.WriteString(fmt.Sprintf("    %s\n", r.URL))
		}
		if r.Snippet != "" {
			builder.WriteString(fmt.Sprintf("    %s\n", r.Snippet))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

// formatResultsTruncated 格式化搜索结果，对Snippet做截断保护
func formatResultsTruncated(results []SearchResult, maxSnippetLen int) string {
	var builder strings.Builder
	for i, r := range results {
		snippet := r.Snippet
		runes := []rune(snippet)
		if len(runes) > maxSnippetLen {
			snippet = string(runes[:maxSnippetLen]) + "..."
		}
		authLabel := ""
		if r.AuthorityScore > 0 {
			authLabel = " " + AuthorityLabel(r.AuthorityScore)
		}
		builder.WriteString(fmt.Sprintf("[%d]%s %s\n", i+1, authLabel, r.Title))
		if r.URL != "" {
			builder.WriteString(fmt.Sprintf("    %s\n", r.URL))
		}
		if snippet != "" {
			builder.WriteString(fmt.Sprintf("    %s\n", snippet))
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
