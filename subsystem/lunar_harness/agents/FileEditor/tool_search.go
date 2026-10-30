package FileEditor

// tool_search.go 检索类工具：联网查询、记忆库查询、句柄系统检索与读取。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/WebSearch"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// WebQuery 联网查询。
func WebQuery(ctx context.Context, query string) OpResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return failResult(ToolWeb, "", fmt.Errorf("query 不能为空"))
	}
	summary, err := WebSearch.Run(query)
	if err != nil {
		return failResult(ToolWeb, "", err)
	}
	_ = ctx
	return newResult(ToolWeb, "",
		"联网查询「"+previewRunes(query, 40)+"」完成", nil,
		map[string]any{"query": query, "summary": strings.TrimSpace(summary)})
}

// MemoryQuery 记忆库查询。
func MemoryQuery(ctx context.Context, query string, limit int) OpResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return failResult(ToolMemory, "", fmt.Errorf("query 不能为空"))
	}
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	entries, err := memory.Query(ctx, query, "text", limit, memory.QueryOptions{})
	if err != nil {
		terms := strings.Fields(query)
		if len(terms) == 0 {
			terms = []string{query}
		}
		entries, err = memory.KeywordSearch("text", terms, false, limit)
	}
	if err != nil {
		return failResult(ToolMemory, "", err)
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		line := previewRunes(entry.Content, 240)
		if line == "" {
			continue
		}
		lines = append(lines, "["+previewRunes(entry.ID, 12)+"] "+line)
	}
	return newResult(ToolMemory, "",
		fmt.Sprintf("记忆检索「%s」命中 %d 条", previewRunes(query, 40), len(lines)), nil,
		map[string]any{"query": query, "results": lines})
}

// HandleSearch 句柄系统检索。
func HandleSearch(query string, limit int) OpResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return failResult(ToolHandleSr, "", fmt.Errorf("query 不能为空"))
	}
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	hits, err := resource.Search(query, "", limit)
	if err != nil {
		return failResult(ToolHandleSr, "", err)
	}
	handles := make([]string, 0, len(hits))
	for _, hit := range hits {
		if handle := resolveHandle(hit); handle != "" {
			handles = append(handles, handle)
		}
	}
	return newResult(ToolHandleSr, "",
		fmt.Sprintf("句柄检索「%s」命中 %d 条", previewRunes(query, 40), len(hits)), handles,
		map[string]any{"hits": hits})
}

// HandleRead 按句柄读取。
func HandleRead(handle string, maxBytes int) OpResult {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return failResult(ToolHandleRd, "", fmt.Errorf("handle 不能为空"))
	}
	if maxBytes <= 0 {
		maxBytes = 64 << 10
	}
	result, err := resource.Read(handle, maxBytes)
	if err != nil {
		return failResult(ToolHandleRd, "", err)
	}
	detail := map[string]any{
		"title":     result.Title,
		"content":   result.Content,
		"mime":      result.MIME,
		"blob_path": result.BlobPath,
		"lost":      result.Lost,
		"truncated": result.Truncated,
		"binary":    result.Base64 != "",
	}
	if result.Base64 != "" {
		// 不可读二进制（zip / exe / gguf…）仍以 @D 句柄登记，但内容只以 base64 回传。
		detail["base64"] = result.Base64
	}
	summary := "读取句柄 " + handle
	if result.Base64 != "" {
		summary += "（二进制，已按 base64 返回）"
	}
	return newResult(ToolHandleRd, "", summary, []string{handle}, detail)
}

// resolveHandle 从检索命中里取句柄文本（字段缺失时返回空串）。
func resolveHandle(hit resource.SearchHit) string {
	return strings.TrimSpace(hit.Handle)
}
