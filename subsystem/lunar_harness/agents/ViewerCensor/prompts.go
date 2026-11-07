package ViewerCensor

// prompts.go 审查提示词的加载（统一放在 LunarHarness/prompts）。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// loadPrompt 读取提示词文本（读取失败返回空串）
func loadPrompt(name string) string {
	data, err := prompts.Read(name)
	if err != nil {
		logger.Error("CrystalAstral", Role+"读取提示词 %s 失败: %v", name, err)
		return ""
	}
	return strings.TrimSpace(string(data))
}

// renderPrompt 单次替换提示词中的 {占位符}（pairs 为「占位符, 值」成对传入）
func renderPrompt(template string, pairs ...string) string {
	return agentbase.RenderPrompt(template, pairs...)
}
