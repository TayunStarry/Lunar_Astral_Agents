package storyboard

// convert.go 智能体结构化结果与链内分镜模型之间的转换。
//
// 链只做编排：智能体返回自己的类型，这里统一映射到任务视图与绘制输入，
// 避免把智能体的内部结构泄漏到步骤机。

import (
	"strings"

	"LunarSubsystem/LunarHarness/agents/StoryboardPlanner"
	"LunarSubsystem/LunarHarness/agents/StoryboardReviser"
)

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

// reviserShots 把链内分镜计划转成修订智能体的输入。
func reviserShots(shots []plannedShot) []StoryboardReviser.Shot {
	out := make([]StoryboardReviser.Shot, 0, len(shots))
	for _, shot := range shots {
		out = append(out, StoryboardReviser.Shot{
			Act:        shot.Act,
			Scene:      shot.Scene,
			Narration:  shot.Narration,
			Continuity: shot.Continuity,
			Prompt:     shot.Prompt,
			Ratio:      shot.Ratio,
		})
	}
	return out
}

// plannedShotsFromReviser 把修订结果转成链内分镜计划。
func plannedShotsFromReviser(shots []StoryboardReviser.Shot, style string) []plannedShot {
	out := make([]plannedShot, 0, len(shots))
	for index, shot := range shots {
		out = append(out, plannedShot{
			Index:      index + 1,
			Act:        shot.Act,
			Scene:      shot.Scene,
			Narration:  shot.Narration,
			Continuity: shot.Continuity,
			Prompt:     shot.Prompt,
			Ratio:      shot.Ratio,
			Style:      style,
		})
	}
	return out
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

// clipRunes 按字符数截断。
func clipRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
