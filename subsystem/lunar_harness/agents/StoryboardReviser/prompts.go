package StoryboardReviser

// prompts.go 修订师提示词的加载（统一放在 LunarHarness/prompts）。

import (
	"encoding/json"
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

// userPrompt 拼装修订师的用户消息：把审查稿按 NDJSON 原样给出。
func userPrompt(req Request) string {
	var b strings.Builder
	b.WriteString("作品标题：" + strings.TrimSpace(req.Title) + "\n")
	if style := strings.TrimSpace(req.Style); style != "" {
		b.WriteString("画风：" + style + "\n")
	}
	ratio := strings.TrimSpace(req.Ratio)
	if ratio == "" {
		ratio = defaultRatio
	}
	b.WriteString("画幅：" + ratio + "\n")
	b.WriteString("\n分镜列表（NDJSON，按最终顺序）：\n")
	for _, shot := range req.Shots {
		line, err := json.Marshal(revisedLine{
			Act:        shot.Act,
			Scene:      shot.Scene,
			Narration:  shot.Narration,
			Continuity: shot.Continuity,
			Prompt:     shot.Prompt,
			Ratio:      shot.Ratio,
		})
		if err != nil {
			continue
		}
		b.Write(line)
		b.WriteString("\n")
	}
	b.WriteString("\n请按提示词要求输出修订后的 NDJSON。")
	return b.String()
}
