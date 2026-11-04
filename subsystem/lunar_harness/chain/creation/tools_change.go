package creation

// tools_change.go change_query 工具：按路径查看内容级变更窗口（最近 10 次 + 更早合并成的原始基准）。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/fileflow"
)

// changeQueryTool 查询任意文件（或全局）的内容级变更。
func changeQueryTool(args map[string]any) toolOutcome {
	path := strings.TrimSpace(argString(args, "path"))
	limit := argInt(args, "limit", maxChangeQuery)
	if limit <= 0 || limit > maxChangeQuery {
		limit = maxChangeQuery
	}
	if path == "" {
		records, err := fileflow.RecentChanges(limit)
		if err != nil {
			return failure(toolChangeQuery, err)
		}
		return toolOutcome{
			JSON:    jsonText(map[string]any{"changes": records}),
			Summary: fmt.Sprintf("查看最近 %d 条文件变更", len(records)),
		}
	}
	_, rel, err := fileflow.Resolve(path)
	if err != nil {
		return failure(toolChangeQuery, err)
	}
	records, baseline, err := fileflow.ChangesOfPath(rel, limit)
	if err != nil {
		return failure(toolChangeQuery, err)
	}
	payload := map[string]any{"path": rel, "changes": records}
	if baseline != nil {
		payload["baseline"] = baseline
	}
	summary := fmt.Sprintf("查看 %s 的 %d 条内容级变更", rel, len(records))
	if baseline != nil && baseline.MergedCount > 0 {
		summary += fmt.Sprintf("（%d 次更早变更已合并为原始基准）", baseline.MergedCount)
	}
	return toolOutcome{JSON: jsonText(payload), Summary: previewRunes(summary, 512)}
}
