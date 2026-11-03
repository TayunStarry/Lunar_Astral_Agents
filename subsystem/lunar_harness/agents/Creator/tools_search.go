package Creator

// tools_search.go 检索类工具：联网搜索与统一资源检索。

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/WebSearch"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
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

// fetchResources 统一资源检索：memory / handle / path / list。
// 与对话者共用 engine/resourcehub，保证句柄优先、路径自动解析为句柄。
func (s *session) fetchResources(ctx context.Context, args map[string]any) toolOutcome {
	result := resourcehub.RunArgs(ctx, args)
	if result.Error != "" {
		return failure(toolFetchResources, errors.New(result.Error))
	}
	return toolOutcome{
		JSON:      jsonText(map[string]any{"result": result.Text, "handle": result.Handle, "error": result.Error}),
		Summary:   previewRunes(result.Text, 200),
		Reference: result.Text,
	}
}
