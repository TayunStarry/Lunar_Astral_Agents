package ViewerCritic

// prompts.go 评论提示词的加载（统一放在 LunarHarness/prompts）。

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

// loadPersona 读取人设提示词（复用基座进程内缓存，同一包内多次调用只读一次文件）。
//
// 读取失败时基座返回空串并记日志；调用方必须把空串当失败处理，不能静默降级成无人设。
func loadPersona() string {
	return agentbase.PromptText(personaFile)
}

// renderPrompt 单次替换提示词中的 {占位符}（pairs 为「占位符, 值」成对传入）
func renderPrompt(template string, pairs ...string) string {
	return agentbase.RenderPrompt(template, pairs...)
}

// withPersona 拼人设与任务段落。
func withPersona(persona, task string) string {
	persona = strings.TrimSpace(persona)
	if persona == "" {
		return task
	}
	return persona + taskBlock + task
}
