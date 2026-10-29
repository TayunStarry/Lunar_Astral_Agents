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

func (s *session) memorySearch(ctx context.Context, args map[string]any) toolOutcome {
	query := strings.TrimSpace(argString(args, "query"))
	if query == "" {
		return failure(toolMemorySearch, fmt.Errorf("query 不能为空"))
	}
	limit := argInt(args, "limit", 8)
	entries, err := memory.Query(ctx, query, "text", limit, memory.QueryOptions{})
	if err != nil {
		terms := strings.Fields(query)
		if len(terms) == 0 {
			terms = []string{query}
		}
		entries, err = memory.KeywordSearch("text", terms, false, limit)
	}
	if err != nil {
		return failure(toolMemorySearch, err)
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
		Summary:   fmt.Sprintf("记忆检索「%s」命中 %d 条", previewRunes(query, 40), len(lines)),
		Reference: "记忆检索《" + previewRunes(query, 60) + "》：" + strings.Join(lines, "\n"),
	}
}

func handleSearch(args map[string]any) toolOutcome {
	query := strings.TrimSpace(argString(args, "query"))
	if query == "" {
		return failure(toolHandleSearch, fmt.Errorf("query 不能为空"))
	}
	hits, err := resource.Search(query, "", argInt(args, "limit", 8))
	if err != nil {
		return failure(toolHandleSearch, err)
	}
	return toolOutcome{
		JSON:      jsonText(hits),
		Summary:   fmt.Sprintf("句柄检索「%s」命中 %d 条", previewRunes(query, 40), len(hits)),
		Reference: fmt.Sprintf("句柄检索《%s》：%s", previewRunes(query, 60), previewRunes(jsonText(hits), 400)),
	}
}

func handleRead(args map[string]any) toolOutcome {
	handle := strings.TrimSpace(argString(args, "handle"))
	if handle == "" {
		return failure(toolHandleRead, fmt.Errorf("handle 不能为空"))
	}
	result, err := resource.Read(handle, argInt(args, "max_bytes", 64<<10))
	if err != nil {
		return failure(toolHandleRead, err)
	}
	return toolOutcome{
		JSON:      jsonText(result),
		Summary:   "读取句柄 " + handle,
		Reference: "句柄 " + handle + "《" + previewRunes(result.Title, 60) + "》：" + previewRunes(result.Content, 500),
	}
}
