package BilibiliViewer

// memory.go 观影记忆的使用与动态文案。
//
// 记忆的枚举与增删改查见 memory_entries.go（统一记忆库适配，无独立存储）；
// 这里只负责把记忆渲染进提示词，以及生成动态正文。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

// recentMemoryText 把最近的记忆条目渲染成提示词材料（最多 limit 条）。
func recentMemoryText(limit int) string {
	entries := MemoryEntries()
	if len(entries) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	lines := make([]string, 0, limit)
	for _, entry := range entries[len(entries)-limit:] {
		lines = append(lines, fmt.Sprintf("[%s]%s", entry.Kind, entry.Text))
	}
	return strings.Join(lines, "\n")
}

// memoryMaterial 记忆注入的统一格式（无记忆时返回空串）。
func memoryMaterial(limit int) string {
	text := recentMemoryText(limit)
	if text == "" {
		return ""
	}
	return "## 我的观影记忆\n\n" + text + "\n\n（以上是我之前的观影记忆，可作为写作背景；不要照抄。）"
}

// memoryLooksMechanical 判断一段文本是不是机械过程/运行统计，而不是人格化记忆。
func memoryLooksMechanical(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range mechanicalMemoryMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// cleanMemoryText 去掉机械过程行，只保留人格化的正文。
func cleanMemoryText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimLeft(line, "-*• "))
		if line == "" || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "【") {
			continue
		}
		if memoryLooksMechanical(line) {
			continue
		}
		kept = append(kept, line)
	}
	out := truncate(strings.Join(kept, " "), memoryTextMaxRunes)
	if memoryLooksMechanical(out) {
		return ""
	}
	return out
}

// recordWatchMemory 把单个视频的观影感受写入记忆库（只存人格化印象）。
func recordWatchMemory(video *videoRecord) {
	if video == nil {
		return
	}
	summary := cleanMemoryText(video.WatchSummary)
	if summary == "" {
		return
	}
	text := fmt.Sprintf("看了《%s》。%s", video.Info.Title, summary)
	if memoryLooksMechanical(text) {
		logger.Warn("BilibiliViewer", "观影记忆含机械化信息，已拒绝写入")
		return
	}
	if _, err := MemoryAddEntry("印象", text); err != nil {
		logger.Warn("BilibiliViewer", "写入观看记忆失败: %v", err)
	}
}

// recordRoundMemory 把一整轮的体验写成人格化记忆（不写数量、条目等运行信息）。
func recordRoundMemory(r *runner) {
	if r == nil {
		return
	}
	material := buildRoundMemoryMaterial(r)
	if material == "" {
		return
	}
	task := "用第一人称写一段今天刷 B 站的记忆，像日记里的一小段：" +
		"只写你对看过的视频的印象、情绪和想法，1-4 句，口语化。" +
		"不要列清单、不要写数量统计，不要出现「这一轮/切片/发了 N 条/回应了 N 条」这类运行信息；" +
		"不要编造没看过的内容。只输出正文。"
	system := task
	if persona, err := prompts.Read("lifeRole.md"); err == nil {
		system = string(persona) + "\n\n## 本次任务\n\n" + task
	}
	text, err := chatTextModel(modelPrimary, system, material)
	if err != nil {
		logger.Warn("BilibiliViewer", "生成本轮观影记忆失败: %v", err)
		return
	}
	memory := cleanMemoryText(text)
	if memory == "" {
		logger.Warn("BilibiliViewer", "本轮观影记忆为空或含机械化信息，跳过写入")
		return
	}
	if _, err := MemoryAddEntry("感受", memory); err != nil {
		logger.Warn("BilibiliViewer", "写入本轮记忆失败: %v", err)
	}
}

// buildRoundMemoryMaterial 拼本轮记忆素材：只给观影感受与互动内容。
func buildRoundMemoryMaterial(r *runner) string {
	var material strings.Builder
	for _, video := range r.approved {
		if video == nil {
			continue
		}
		summary := cleanMemoryText(video.WatchSummary)
		if summary == "" {
			summary = strings.TrimSpace(video.Impression.Summary)
		}
		if summary == "" {
			continue
		}
		fmt.Fprintf(&material, "《%s》：%s\n", video.Info.Title, summary)
	}
	for _, activity := range r.activities {
		if activity.Kind == "watch" || strings.TrimSpace(activity.Reply) == "" {
			continue
		}
		fmt.Fprintf(&material, "%s：%s\n", activityLabel(activity.Kind), activity.Reply)
	}
	return strings.TrimSpace(material.String())
}

// ==== 动态文案 ====

// generateDynamicText 生成本轮观影动态正文。
//
// 发布在看切片之前：只写挑中这些视频时的第一印象、期待与想说的一点。
func generateDynamicText(r *runner) string {
	persona, err := prompts.Read("lifeRole.md")
	if err != nil {
		logger.Warn("BilibiliViewer", "读取人设失败，跳过动态生成: %v", err)
		return ""
	}
	task := "写这一轮要发到 B 站动态的观影感受：根据你对这些视频的初步印象和评价，" +
		"写你挑中它们时的真实感觉（期待、直觉、想吐槽或想分享的一点），2–6 行，口语化。" +
		"禁止逐条复述视频内容、时间线或按视频罗列；不要标题、不要 markdown、不要列表符号、不要链接、不要 @；" +
		"总字数 300 以内。只输出正文，不要解释。"
	system := string(persona) + "\n\n## 本次任务\n\n" + task
	if block := memoryMaterial(12); block != "" {
		system += "\n\n" + block
	}
	text, err := chatTextModel(modelPrimary, system, buildDynamicMaterial(r))
	if err != nil {
		logger.Warn("BilibiliViewer", "观影动态生成失败: %v", err)
		return ""
	}
	dynamic := normalizeDynamic(text)
	if dynamic == "" {
		logger.Warn("BilibiliViewer", "观影动态为空，本轮不发布动态")
	}
	return dynamic
}

// buildDynamicMaterial 拼动态输入：视频印象 + 观看感受 + 本轮互动。
func buildDynamicMaterial(r *runner) string {
	var material strings.Builder
	if strings.TrimSpace(r.instruction) != "" {
		fmt.Fprintf(&material, "【本轮想法】\n%s\n\n", r.instruction)
	}
	if len(r.videos) > 0 {
		material.WriteString("【本轮看的视频】\n")
		for index, video := range r.videos {
			if video == nil {
				continue
			}
			summary := strings.TrimSpace(video.WatchSummary)
			if summary == "" {
				summary = video.Impression.Summary
			}
			fmt.Fprintf(&material, "%d. 《%s》｜UP主：%s｜印象：%s\n",
				index+1, video.Info.Title, video.Info.Owner, truncate(summary, 260))
		}
		material.WriteString("\n")
	}
	if len(r.activities) > 0 {
		material.WriteString("【本轮互动】\n")
		for _, activity := range r.activities {
			material.WriteString(formatActivity(activity))
			material.WriteString("\n")
		}
	}
	return material.String()
}

// formatActivity 把一条互动记录写成一行材料。
func formatActivity(activity lifeActivity) string {
	var builder strings.Builder
	label := activityLabel(activity.Kind)
	builder.WriteString(label)
	if activity.Where != "" {
		builder.WriteString("｜")
		builder.WriteString(activity.Where)
	}
	if activity.Target != "" {
		builder.WriteString("｜对方：")
		builder.WriteString(activity.Target)
	}
	if activity.Content != "" {
		builder.WriteString("｜对方说：")
		builder.WriteString(truncate(activity.Content, 80))
	}
	if activity.Reply != "" {
		builder.WriteString("｜我说：")
		builder.WriteString(truncate(activity.Reply, 120))
	}
	return builder.String()
}

// activityLabel 互动类型的中文名。
func activityLabel(kind string) string {
	switch kind {
	case "inbox":
		return "回应消息"
	case "watch":
		return "观看视频"
	case "share":
		return "分享视频"
	case "dynamic":
		return "发动态"
	default:
		return "互动"
	}
}

// normalizeDynamic 清理动态正文：去掉代码块/列表符号与多余空行，控制长度。
func normalizeDynamic(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		out = append(out, line)
	}
	joined := strings.TrimSpace(strings.Join(out, "\n"))
	return truncate(joined, dynamicMaxRunes)
}
