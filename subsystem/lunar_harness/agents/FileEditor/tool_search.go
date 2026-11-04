package FileEditor

// tool_search.go 检索类工具：联网查询（资源句柄读取由文件路径定位与 task_scheduling 承担）。

import (
	"context"
	"fmt"
	"strings"
)

// SearchFunc 联网查询能力的实现，由装配层注入搜索链入口。
// 智能体层不得反向依赖链路层，因此这里只保留函数钩子，未装配时明确报错。
var SearchFunc func(string) (string, error)

// WebQuery 联网查询。
func WebQuery(ctx context.Context, query string) OpResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return failResult(ToolWeb, "", fmt.Errorf("query 不能为空"))
	}
	if SearchFunc == nil {
		return failResult(ToolWeb, "", fmt.Errorf("联网查询能力尚未装配"))
	}
	summary, err := SearchFunc(query)
	if err != nil {
		return failResult(ToolWeb, "", err)
	}
	_ = ctx
	return newResult(ToolWeb, "",
		"联网查询「"+previewRunes(query, 512)+"」完成", nil,
		map[string]any{"query": query, "summary": strings.TrimSpace(summary)})
}
