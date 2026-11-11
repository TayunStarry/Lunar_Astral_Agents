package viewer

// model.go 观影链的模型生命周期：按阶段主动卸载模型释放显存。
// 模型调用本身由四个观影者智能体（agents/Viewer*）承担，链侧只负责编排与切换。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/modellease"
)

// unloadModel 主动卸载指定模型释放显存；失败只告警（下次调用会按需重载）。
func unloadModel(model string) {
	if strings.TrimSpace(model) == "" {
		return
	}
	if _, err := modellease.Unload(model); err != nil {
		logger.Warn("BilibiliViewer", "卸载模型[%s]失败（不影响流程）: %v", model, err)
		return
	}
}

func truncate(text string, limit int) string {
	return agentbase.TruncateRunes(text, limit)
}
