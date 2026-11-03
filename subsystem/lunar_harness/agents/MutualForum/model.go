package MutualForum

// model.go 互助论坛的模型调用：统一走 protocol/agentbase 的流式调用、
// 推理实况、循环监管与智能修复，不再维护本地的 SSE / 观察者实现。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// previewText 按字符截断。
func previewText(text string, limit int) string {
	return agentbase.TruncateRunes(text, limit)
}

// logTool 把工具调用写入日志，便于复盘。
func logTool(name, args, summary string, elapsed time.Duration) {
	logger.Info(roleLabel, "工具 %s（%s）→ %s，耗时 %s", name, previewText(args, 120), previewText(summary, 160), elapsed.Truncate(time.Millisecond))
}
