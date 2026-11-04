package ViewerShare

// prompts.go 分享提示词的加载（统一放在 LunarHarness/prompts）。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

// loadPrompt 读取提示词文本（读取失败返回空串）
func loadPrompt(name string) string {
	data, err := prompts.Read(name)
	if err != nil {
		logger.Error("CrystalAstral", "观影者-分享读取提示词 %s 失败: %v", name, err)
		return ""
	}
	return strings.TrimSpace(string(data))
}

// withPersona 拼人设与任务段落。
func withPersona(persona, task string) string {
	persona = strings.TrimSpace(persona)
	if persona == "" {
		return task
	}
	return persona + taskBlock + task
}
