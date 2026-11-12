package StoryboardPlanner

// prompts.go 分镜师提示词的加载（统一放在 LunarHarness/prompts）。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

// loadPrompt 读取提示词文本（读取失败返回空串）。
func loadPrompt(name string) string {
	data, err := prompts.Read(name)
	if err != nil {
		logger.Error(AgentID, modelRole+"读取提示词 %s 失败: %v", name, err)
		return ""
	}
	return strings.TrimSpace(string(data))
}

// userPrompt 拼装分镜师的用户消息。
func userPrompt(story, ratio, research string) string {
	text := "故事正文：\n" + story
	if research = strings.TrimSpace(research); research != "" {
		text += "\n\n已补全的参考资料（用于校正设定与细节，不要直接复述资料腔）：\n" + research
	}
	if ratio != "" {
		text += "\n\n画幅约束：" + ratio
	}
	text += "\n\n请先输出叙事推演，然后输出分隔行 `" + planDelimiter +
		"`，最后逐行输出 NDJSON 分镜。每镜的 original 必须是故事正文中的原文片段（逐字摘录），" +
		"它会被用于旁白朗读。"
	return text
}
