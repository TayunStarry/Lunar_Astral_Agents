package FileEditor

// tool_search.go 检索类工具：联网查询（资源句柄读取由文件路径定位与 task_scheduling 承担）。

import (
	"context"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/WebSearch"
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
