package creation

// tools_search.go 检索类工具：联网搜索与统一资源检索。

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/resourcehub"
)

func (s *session) webSearch(args map[string]any) toolOutcome {
	query := strings.TrimSpace(argString(args, "query"))
	if query == "" {
		return failure(toolWebSearch, fmt.Errorf("query 不能为空"))
	}
	if WebSearchHook == nil {
		return failure(toolWebSearch, fmt.Errorf("联网搜索未接入"))
	}
	summary, err := WebSearchHook(query)
	if err != nil {
		return failure(toolWebSearch, err)
	}
	summary = strings.TrimSpace(summary)
	return toolOutcome{
		JSON:      jsonText(map[string]any{"query": query, "summary": summary}),
		Summary:   "联网搜索「" + previewRunes(query, 512) + "」",
		Reference: "联网搜索《" + previewRunes(query, 512) + "》：" + summary,
	}
}

// fetchResources 统一资源检索：memory / handle / path / list。
// 与对话者共用 engine/resourcehub，保证句柄优先、路径自动解析为句柄。
func (s *session) fetchResources(ctx context.Context, args map[string]any) toolOutcome {
	result := resourcehub.RunArgs(ctx, args)
	if result.Error != "" {
		return failure(toolFetchResources, errors.New(result.Error))
	}
	return toolOutcome{
		// 直接回填资源正文：JSON 包装会把换行转义并可能在单条上限处被截成半截 JSON，
		// 模型会误判为「没取到内容」而反复重取。正文头部已带句柄、行区间与总数。
		JSON:      result.Text,
		Summary:   previewRunes(result.Text, 512),
		Reference: result.Text,
	}
}
