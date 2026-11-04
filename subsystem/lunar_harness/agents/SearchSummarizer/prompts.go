package SearchSummarizer

// prompts.go 摘要提示词的加载（统一放在 LunarHarness/prompts）
//
// 任务型提示词用 `---user---` 分隔系统段与用户模板：
// 系统段是稳定的行为约束，用户模板只保留带 {占位符} 的数据骨架。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// loadPromptPair 读取任务型提示词的系统段与用户模板
func loadPromptPair(name string) (string, string) {
	data, err := prompts.Read(name)
	if err != nil {
		logger.Error("CrystalAstral", "搜索者-摘要读取提示词 %s 失败: %v", name, err)
		return "", ""
	}
	raw := strings.TrimSpace(string(data))
	if index := strings.Index(raw, promptUserMarker); index >= 0 {
		return strings.TrimSpace(raw[:index]), strings.TrimSpace(raw[index+len(promptUserMarker):])
	}
	return raw, ""
}

// renderPrompt 单次替换提示词中的 {占位符}（pairs 为「占位符, 值」成对传入）
func renderPrompt(template string, pairs ...string) string {
	return agentbase.RenderPrompt(template, pairs...)
}
