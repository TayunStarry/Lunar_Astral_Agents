package crystal

// ==== LTP9 查询事件（Go 侧触发） ====
// 月华调用内置 Web-LTP 网络搜索时，在搜索开始前与报告返回前把负载推送到 LTP9 引擎域：
//   - before_query_starts：改写搜索指令或取消本次搜索；
//   - before_query_ends：改写/追加报告，或取消本次报告（月华的工具调用立即收到取消回执）。
// 无插件订阅、未回传业务结果或回传结构非法时，一律沿用原始数据。

import (
	"strings"

	StarLTP "LunarSubsystem/LunarHarness/engine/plugin"
)

// applyBeforeQueryStarts 查询开始前事件：返回改写后的指令与是否继续执行。
func applyBeforeQueryStarts(instruction string) (string, bool) {
	res := StarLTP.Emit(StarLTP.EventBeforeQueryStarts, map[string]any{"instruction": instruction}, "")
	feedback, ok := res.Return.(map[string]any)
	if !res.Returned || !ok {
		return instruction, true
	}
	if cancel, _ := feedback["cancel"].(bool); cancel {
		return "", false
	}
	if rewritten, ok := feedback["instruction"].(string); ok && strings.TrimSpace(rewritten) != "" {
		return rewritten, true
	}
	return instruction, true
}

// applyBeforeQueryEnds 查询结束前事件：返回改写后的报告与是否继续交付。
func applyBeforeQueryEnds(instruction, report string) (string, bool) {
	res := StarLTP.Emit(StarLTP.EventBeforeQueryEnds, map[string]any{
		"instruction": instruction,
		"report":      report,
	}, "")
	feedback, ok := res.Return.(map[string]any)
	if !res.Returned || !ok {
		return report, true
	}
	if cancel, _ := feedback["cancel"].(bool); cancel {
		return "", false
	}
	if rewritten, ok := feedback["report"].(string); ok {
		return rewritten, true
	}
	return report, true
}
