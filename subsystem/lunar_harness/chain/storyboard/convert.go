package storyboard

// convert.go 智能体结构化结果与链内分镜模型之间的转换。
//
// 链只做编排：智能体返回自己的类型，这里统一映射到任务视图与绘制输入，
// 避免把智能体的内部结构泄漏到步骤机。

import (
	"strings"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/agents/StoryboardPlanner"
)

// appendThoughtBounded 追加过程文本并只保留尾部 maxThoughtBytes 字节。
// 截断点向前对齐到合法 UTF-8 起始字节，避免把中文/emoji 截成半个字符。
func appendThoughtBounded(current, delta string) string {
	combined := current + delta
	if len(combined) <= maxThoughtBytes {
		return combined
	}
	start := len(combined) - maxThoughtBytes
	for start < len(combined) && !utf8.RuneStart(combined[start]) {
		start++
	}
	return "…（前文已省略）\n" + combined[start:]
}

// shotViewFromPlanner 把分镜规划结果转成逐镜视图。
func shotViewFromPlanner(shot StoryboardPlanner.Shot, status string) ShotView {
	return ShotView{
		Index:     shot.Index,
		Act:       shot.Act,
		Scene:     shot.Scene,
		Narration: shot.Narration,
		Prompt:    shot.Prompt,
		Ratio:     shot.Ratio,
		Status:    status,
	}
}

// toStoryboardPlan 把智能体的规划结果转成链内分镜计划。
func toStoryboardPlan(plan StoryboardPlanner.Result) storyboardPlan {
	shots := make([]plannedShot, 0, len(plan.Shots))
	for _, shot := range plan.Shots {
		shots = append(shots, plannedShot{
			Index:      shot.Index,
			Act:        shot.Act,
			Scene:      shot.Scene,
			Narration:  shot.Narration,
			Continuity: shot.Continuity,
			Prompt:     shot.Prompt,
			Ratio:      shot.Ratio,
			Style:      shot.Style,
		})
	}
	return storyboardPlan{Title: plan.Title, Style: plan.Style, Shots: shots}
}

// shotsFromPlan 按分镜计划生成逐镜视图。
func shotsFromPlan(plan storyboardPlan, status string) []ShotView {
	shots := make([]ShotView, 0, len(plan.Shots))
	for _, shot := range plan.Shots {
		shots = append(shots, ShotView{
			Index:     shot.Index,
			Act:       shot.Act,
			Scene:     shot.Scene,
			Narration: shot.Narration,
			Prompt:    shot.Prompt,
			Ratio:     shot.Ratio,
			Status:    status,
		})
	}
	return shots
}

// fillMissingPrompts 为画面描述为空的镜头补上可绘制提示词。
func fillMissingPrompts(shots []plannedShot) []plannedShot {
	for index := range shots {
		if strings.TrimSpace(shots[index].Prompt) != "" {
			continue
		}
		shots[index].Prompt = StoryboardPlanner.ComposePrompt(StoryboardPlanner.Shot{
			Act:        shots[index].Act,
			Scene:      shots[index].Scene,
			Narration:  shots[index].Narration,
			Continuity: shots[index].Continuity,
		})
	}
	return shots
}
