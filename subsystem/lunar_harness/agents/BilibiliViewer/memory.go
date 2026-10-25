package BilibiliViewer

// memory.go 观影记忆：动态正文与历史记忆都由模型重新生成。
//
// 历史记忆是独属于观影者的上下文：她回应了谁、在哪回应、有没有被 @、看完的感受，
// 以及下一轮想接着做什么。它会注入后续所有生成调用（选片关键词、流程编排、评论、
// 回复、@分享），让 B 站里的互动真正影响下一轮行为，而不是每轮都从零开始。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// memoryPromptBlock 把历史记忆附到自定义提示词后面（无记忆时返回空串）
func memoryPromptBlock() string {
	memory := strings.TrimSpace(loadDiaryMemory())
	if memory == "" {
		return ""
	}
	return "\n\n## 我的观影记忆（作为判断背景，不要照抄）\n\n" + memory
}

// generateDynamicText 生成本轮要发到 B 站动态的日记正文。
// 生成失败返回空串，调用方保留上一版动态、不误发脏内容。
func generateDynamicText(prev diarySections, activities []lifeActivity, watched []*videoRecord, instruction string) string {
	task := "写这一轮要发到 B 站动态的日记正文：2–6 行，口语化，像随手发的心情；" +
		"每行一句，可以用一个空行分段；不要标题、不要 markdown、不要列表符号、不要链接、不要 @；总字数 300 以内。" +
		"只输出正文，不要解释。"
	system, err := lifeSystemWithoutMemory(task)
	if err != nil {
		logger.Warn("BilibiliViewer", "读取人设失败，跳过动态生成: %v", err)
		return ""
	}
	material := buildDiaryMaterial(prev, activities, watched, instruction)
	if strings.TrimSpace(prev.Dynamic) != "" {
		material += "\n【上一版动态（只作风格参考，不要照抄）】\n" + prev.Dynamic + "\n"
	}
	text, err := chatText(system, material)
	if err != nil {
		logger.Warn("BilibiliViewer", "动态日记生成失败: %v", err)
		return ""
	}
	dynamic := normalizeDynamic(text)
	if dynamic == "" {
		logger.Warn("BilibiliViewer", "动态日记为空，保留上一版")
	}
	return dynamic
}

// generateMemory 生成本轮历史记忆：把上一版记忆与本轮全部互动交给模型重写。
func generateMemory(prev diarySections, activities []lifeActivity, watched []*videoRecord, instruction, dynamicPosted string) string {
	task := fmt.Sprintf("根据下面的材料更新你的观影日记【历史记忆】：第一人称、意识流，不要流水账；"+
		"记住这轮看了什么、回应了谁（在视频评论区还是动态区）、说了什么、有没有人 @ 你、"+
		"你的感受，以及下次想接着做什么；总字数不超过 %d 字，超了要自己提炼，只保留最重要的部分。"+
		"只输出记忆正文，不要解释。", memoryMaxRunes)
	system, err := lifeSystemWithoutMemory(task)
	if err != nil {
		logger.Warn("BilibiliViewer", "读取人设失败，记忆改用本轮活动摘要: %v", err)
		return fallbackMemory(prev, activities)
	}
	material := buildDiaryMaterial(prev, activities, watched, instruction)
	if strings.TrimSpace(dynamicPosted) != "" {
		material += "\n【本轮发到动态的正文】\n" + dynamicPosted + "\n"
	}
	text, err := chatText(system, material)
	if err != nil {
		logger.Warn("BilibiliViewer", "历史记忆生成失败，改用本轮活动摘要: %v", err)
		return fallbackMemory(prev, activities)
	}
	memory := clampMemory(text)
	if memory == "" {
		memory = fallbackMemory(prev, activities)
	}
	return memory
}

// buildDiaryMaterial 拼模型输入：上一版记忆/动态 + 本轮经历与互动
func buildDiaryMaterial(prev diarySections, activities []lifeActivity, watched []*videoRecord, instruction string) string {
	var material strings.Builder
	if strings.TrimSpace(prev.Memory) != "" {
		fmt.Fprintf(&material, "【上一版历史记忆】\n%s\n\n", truncate(prev.Memory, memoryMaxRunes))
	} else if strings.TrimSpace(prev.Legacy) != "" {
		// 旧模板日记没有记忆段：把旧文交还给模型，让它一次性提炼出长期记忆
		fmt.Fprintf(&material, "【旧版日记（用于提炼长期记忆）】\n%s\n\n", truncate(prev.Legacy, 3000))
	}
	if strings.TrimSpace(instruction) != "" {
		fmt.Fprintf(&material, "【本轮指令】\n%s\n\n", instruction)
	}

	if len(watched) > 0 {
		material.WriteString("【本轮看过的视频】\n")
		for i, video := range watched {
			if video == nil {
				continue
			}
			fmt.Fprintf(&material, "%d. 《%s》｜UP主：%s｜链接：%s\n观看感受：%s\n",
				i+1, video.Info.Title, video.Info.Owner, video.Info.URL, truncate(video.Summary, 400))
			if video.CommentText != "" {
				fmt.Fprintf(&material, "我在评论区说：%s\n", video.CommentText)
			}
			if video.ShareText != "" {
				fmt.Fprintf(&material, "我的 @分享评论：%s\n", video.ShareText)
			}
		}
		material.WriteString("\n")
	} else {
		material.WriteString("【本轮没有看视频】\n\n")
	}

	if len(activities) > 0 {
		material.WriteString("【本轮互动】\n")
		for _, activity := range activities {
			material.WriteString(formatActivity(activity))
			material.WriteString("\n")
		}
		material.WriteString("\n")
	} else {
		material.WriteString("【本轮没有新的互动】\n\n")
	}
	return material.String()
}

// fallbackMemory 模型不可用时的记忆兜底：旧记忆 + 本轮活动摘要
func fallbackMemory(prev diarySections, activities []lifeActivity) string {
	var builder strings.Builder
	base := strings.TrimSpace(prev.Memory)
	if base == "" {
		base = truncate(strings.TrimSpace(prev.Legacy), 2000)
	}
	if base != "" {
		builder.WriteString(base)
		builder.WriteString("\n")
	}
	for _, activity := range activities {
		builder.WriteString(formatActivity(activity))
		builder.WriteString("\n")
	}
	return clampMemory(builder.String())
}

// formatActivity 把一条互动记录排成一行人话（喂给记忆模型）
func formatActivity(activity lifeActivity) string {
	label := activityLabel(activity.Kind)
	parts := make([]string, 0, 5)
	if activity.Where != "" {
		parts = append(parts, "位置："+activity.Where)
	}
	if activity.Target != "" {
		parts = append(parts, "对象："+activity.Target)
	}
	if activity.Content != "" {
		parts = append(parts, "对方说："+truncate(activity.Content, 120))
	}
	if activity.Reply != "" {
		parts = append(parts, "我说："+truncate(activity.Reply, 120))
	}
	if activity.Note != "" {
		parts = append(parts, "备注："+activity.Note)
	}
	if len(parts) == 0 {
		return "- " + label
	}
	return "- " + label + "｜" + strings.Join(parts, "｜")
}

// activityLabel 互动类型的中文标签
func activityLabel(kind string) string {
	switch kind {
	case "inbox":
		return "回应未读消息"
	case "comment":
		return "发视频评论"
	case "share":
		return "@分享评论"
	case "dyn_reply":
		return "回复动态评论"
	default:
		return "互动"
	}
}
