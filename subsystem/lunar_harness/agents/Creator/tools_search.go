package Creator

// tools_search.go 检索类工具：联网搜索、记忆库查询、句柄检索与读取。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/WebSearch"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/resource"
)

func (s *session) webSearch(args map[string]any) toolOutcome {
	query := strings.TrimSpace(argString(args, "query"))
	if query == "" {
		return failure(toolWebSearch, fmt.Errorf("query 不能为空"))
	}
	summary, err := WebSearch.Run(query)
	if err != nil {
		return failure(toolWebSearch, err)
	}
	summary = strings.TrimSpace(summary)
	return toolOutcome{
		JSON:      jsonText(map[string]any{"query": query, "summary": summary}),
		Summary:   "联网搜索「" + previewRunes(query, 40) + "」",
		Reference: "联网搜索《" + previewRunes(query, 60) + "》：" + summary,
	}
}

// fetchResources 统一资源检索：memory / handle / list。
func (s *session) fetchResources(ctx context.Context, args map[string]any) toolOutcome {
	mode := strings.ToLower(strings.TrimSpace(argString(args, "type")))
	query := argStrings(args, "query")
	limit := argInt(args, "limit", 5)
	if limit <= 0 {
		limit = 5
	}
	if limit > 50 {
		limit = 50
	}
	switch mode {
	case "memory":
		return fetchMemory(ctx, query, limit)
	case "handle":
		return fetchHandles(query, limit)
	case "list":
		return fetchHandleList(query, limit)
	default:
		return failure(toolFetchResources, fmt.Errorf("type 只能取 memory / handle / list"))
	}
}

// fetchMemory 记忆模式。
func fetchMemory(ctx context.Context, query []string, limit int) toolOutcome {
	if len(query) == 0 {
		return failure(toolFetchResources, fmt.Errorf("memory 模式需要 query 关键词"))
	}
	entries, err := memory.Query(ctx, strings.Join(query, " "), "text", limit, memory.QueryOptions{})
	if err != nil || len(entries) == 0 {
		entries, err = memory.KeywordSearch("text", query, false, limit)
	}
	if err != nil {
		return failure(toolFetchResources, err)
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		line := previewRunes(entry.Content, 240)
		if line == "" {
			continue
		}
		lines = append(lines, "["+previewRunes(entry.ID, 12)+"] "+line)
	}
	return toolOutcome{
		JSON:      jsonText(map[string]any{"query": query, "results": lines}),
		Summary:   fmt.Sprintf("记忆检索「%s」命中 %d 条", previewRunes(strings.Join(query, "、"), 40), len(lines)),
		Reference: "记忆检索《" + previewRunes(strings.Join(query, "、"), 60) + "》：" + strings.Join(lines, "\n"),
	}
}

// fetchHandles 句柄模式：读取完整信息与内容。
func fetchHandles(query []string, limit int) toolOutcome {
	if len(query) == 0 {
		return failure(toolFetchResources, fmt.Errorf("handle 模式需要 query 句柄数组"))
	}
	if len(query) > limit {
		query = query[:limit]
	}
	results := make([]map[string]any, 0, len(query))
	missing := make([]string, 0)
	for _, raw := range query {
		handle := strings.TrimSpace(raw)
		if handle == "" {
			continue
		}
		result, err := resource.Read(handle, 64<<10)
		if err != nil {
			missing = append(missing, handle)
			continue
		}
		results = append(results, map[string]any{
			"handle": handle, "title": result.Title, "content": result.Content,
			"mime": result.MIME, "blob_path": result.BlobPath,
			"lost": result.Lost, "truncated": result.Truncated, "binary": result.Base64 != "",
		})
	}
	if len(results) == 0 {
		return failure(toolFetchResources, fmt.Errorf("句柄都不可用：%s", strings.Join(missing, "、")))
	}
	return toolOutcome{
		JSON:      jsonText(map[string]any{"results": results, "missing": missing}),
		Summary:   fmt.Sprintf("读取 %d 个句柄", len(results)),
		Reference: "句柄读取：" + previewRunes(jsonText(results), 500),
	}
}

// fetchHandleList 列表模式：query 为空列全部，填类型码只列该类型。
func fetchHandleList(query []string, limit int) toolOutcome {
	kind := ""
	if len(query) > 0 {
		kind = strings.ToUpper(strings.TrimSpace(query[0]))
		switch kind {
		case "D", "I", "G", "A", "V", "U":
		default:
			return failure(toolFetchResources, fmt.Errorf("类型码无效：%s（可用 D/I/G/A/V/U）", kind))
		}
	}
	entries, err := resource.List(kind, limit)
	if err != nil {
		return failure(toolFetchResources, err)
	}
	return toolOutcome{
		JSON:      jsonText(entries),
		Summary:   fmt.Sprintf("资源目录列出 %d 条", len(entries)),
		Reference: fmt.Sprintf("资源目录《%s》：%s", kind, previewRunes(jsonText(entries), 400)),
	}
}
