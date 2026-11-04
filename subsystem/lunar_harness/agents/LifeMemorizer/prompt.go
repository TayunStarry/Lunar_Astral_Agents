package LifeMemorizer

// prompt.go 记忆整理任务提示：固定文案放在 prompts/lifeMemoryTask.md，
// 本批条数与保留下限按次注入。

import (
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// TaskPrompt 返回条目化整理任务提示。
func TaskPrompt(itemCount, minKeep int) string {
	return agentbase.RenderPrompt(
		promptText(promptTask),
		"{item-count}", strconv.Itoa(itemCount),
		"{min-keep}", strconv.Itoa(minKeep),
	)
}

// promptText 读取内嵌提示词并去掉文件末尾换行（保持与迁移前文案逐字一致）。
func promptText(name string) string {
	return strings.TrimRight(agentbase.PromptText(name), "\r\n")
}
