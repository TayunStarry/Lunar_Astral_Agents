package viewer

// memory.go 观影记忆的使用与动态文案。
//
// 记忆的存取、上限维持见 engine/viewerstore；这里只负责把记忆渲染进提示词、
// 生成动态正文，以及把一轮观看结果写成新的观感。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/ViewerCritic"
	"LunarSubsystem/LunarHarness/agents/ViewerShare"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
	"LunarSubsystem/LunarHarness/prompts"
)

// recentMemoryText 把近期观感与两个单槽渲染成提示词材料（最多 limit 条）。
func recentMemoryText(limit int) string {
	lines := make([]string, 0, viewerstore.ObservationMax+2)
	observations, err := viewerstore.LoadObservations()
	if err != nil {
		logger.Warn("BilibiliViewer", "读取观影记忆失败: %v", err)
	}
	for _, obs := range observations {
		lines = append(lines, "[印象]"+observationText(obs))
	}
	if value, _, ok, _ := viewerstore.LoadProfile(); ok && strings.TrimSpace(value) != "" {
		lines = append(lines, "[习惯]"+viewerstore.SelectionMarker+value)
	}
	if value, _, ok, _ := viewerstore.LoadStance(); ok && strings.TrimSpace(value) != "" {
		lines = append(lines, "[评价]"+viewerstore.StanceMarker+value)
	}
	if limit <= 0 || limit > len(lines) {
		limit = len(lines)
	}
	return strings.Join(lines[len(lines)-limit:], "\n")
}

// memoryMaterial 记忆注入的统一格式（无记忆时返回空串）。
func memoryMaterial(limit int) string {
	text := recentMemoryText(limit)
	if text == "" {
		return ""
	}
	return "## 我的观影记忆\n\n" + text + "\n\n（以上是我之前的观影记忆，可作为写作背景；不要照抄。）"
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
		if viewerstore.LooksMechanical(line) {
			continue
		}
		kept = append(kept, line)
	}
	out := truncate(strings.Join(kept, " "), viewerstore.TextMaxRunes)
	if viewerstore.LooksMechanical(out) {
		return ""
	}
	return out
}

// upsertWatchMemory 写入单个视频的高层结论（一个视频只保留一行，按 BV 号覆盖更新）。
//
// 完整的观看解读留在 SQL（URL 主表 parse_text）与单轮轨迹里；观感表只收
// 「标题 + 链接 + 主题/氛围 + 首句结论」，避免同一段内容双写。
func upsertWatchMemory(video *videoRecord) {
	obs := buildObservation(video)
	if strings.TrimSpace(obs.BV) == "" {
		return
	}
	if viewerstore.LooksMechanical(obs.Conclusion) {
		logger.Warn("BilibiliViewer", "观影记忆含机械化信息，已拒绝写入")
		return
	}
	if err := viewerstore.SaveObservation(obs); err != nil {
		logger.Warn("BilibiliViewer", "写入观看记忆失败: %v", err)
		return
	}
}

// buildObservation 从本轮视频结果组装一条观感（只保留首句结论）。
func buildObservation(video *videoRecord) viewerstore.Observation {
	if video == nil {
		return viewerstore.Observation{}
	}
	summary := cleanMemoryText(video.WatchSummary)
	if summary == "" {
		return viewerstore.Observation{}
	}
	bv := strings.TrimSpace(video.Info.Bvid)
	if bv == "" {
		bv = viewerstore.ExtractBVID(video.Info.URL)
	}
	if bv == "" {
		logger.Warn("BilibiliViewer", "《%s》没有 BV 号，跳过观感写入", video.Info.Title)
		return viewerstore.Observation{}
	}
	return viewerstore.Observation{
		BV:         bv,
		Title:      strings.TrimSpace(video.Info.Title),
		URL:        strings.TrimSpace(video.Info.URL),
		Topics:     memoryTopics(video.Impression.Topics, viewerstore.TopicMax),
		Tone:       strings.TrimSpace(video.Impression.Tone),
		Conclusion: viewerstore.Conclusion(summary),
		UpdatedAt:  time.Now().Unix(),
	}
}

// observationText 把一条观感渲染成提示词中的一行。
func observationText(obs viewerstore.Observation) string {
	parts := make([]string, 0, 6)
	if obs.Title != "" {
		parts = append(parts, "《"+obs.Title+"》")
	}
	if obs.URL != "" {
		parts = append(parts, obs.URL)
	}
	if obs.Topics != "" {
		parts = append(parts, "主题："+obs.Topics)
	}
	if obs.Tone != "" {
		parts = append(parts, "氛围："+obs.Tone)
	}
	if obs.Conclusion != "" {
		parts = append(parts, obs.Conclusion)
	}
	return truncate(strings.Join(parts, "｜"), viewerstore.TextMaxRunes)
}

// memoryTopics 把主题词压成短标签（最多 limit 个）。
func memoryTopics(topics []string, limit int) string {
	if limit <= 0 {
		limit = viewerstore.TopicMax
	}
	parts := make([]string, 0, limit)
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		parts = append(parts, topic)
		if len(parts) >= limit {
			break
		}
	}
	return strings.Join(parts, "、")
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
	persona := ""
	if block, err := prompts.Read("lifeRole.md"); err == nil {
		persona = string(block)
	}
	text, err := ViewerCritic.RoundMemory(context.Background(), ViewerCritic.MemoryInput{
		Persona: persona, Material: material, Model: modelPrimary,
	})
	if err != nil {
		logger.Warn("BilibiliViewer", "生成本轮观影记忆失败: %v", err)
		return
	}
	memory := cleanMemoryText(text)
	if memory == "" {
		logger.Warn("BilibiliViewer", "本轮观影记忆为空或含机械化信息，跳过写入")
		return
	}
	// 本轮体验直接覆盖「评论倾向」单行，维持当前有效倾向。
	updateCommentStance(memory)
	// 选片口味由当前观感重算，观感轮转后偏好会自然渐进修正。
	refreshSelectionProfile()
	saveRoundMemory(r, memory)
}

// saveRoundMemory 记录本轮观影轮次（按轮追溯用，超出上限按最旧归档）。
func saveRoundMemory(r *runner, conclusion string) {
	if r == nil {
		return
	}
	bvids := make([]string, 0, len(r.approved))
	for _, video := range r.approved {
		if video == nil {
			continue
		}
		bv := strings.TrimSpace(video.Info.Bvid)
		if bv == "" {
			bv = viewerstore.ExtractBVID(video.Info.URL)
		}
		if bv != "" {
			bvids = append(bvids, bv)
		}
	}
	if len(bvids) == 0 {
		return
	}
	round := viewerstore.Round{
		ID:         fmt.Sprintf("%d", time.Now().UnixNano()),
		BVIDs:      strings.Join(bvids, "、"),
		Conclusion: viewerstore.Conclusion(conclusion),
		CreatedAt:  time.Now().Unix(),
	}
	if err := viewerstore.SaveRound(round); err != nil {
		logger.Warn("BilibiliViewer", "写入观影轮次失败: %v", err)
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
	text, err := ViewerShare.Dynamic(context.Background(), ViewerShare.DynamicInput{
		Persona: string(persona), Memory: memoryMaterial(12), Material: buildDynamicMaterial(r), Model: modelPrimary,
	})
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
				index+1, video.Info.Title, video.Info.Owner, truncate(summary, 512))
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
		builder.WriteString(truncate(activity.Content, 512))
	}
	if activity.Reply != "" {
		builder.WriteString("｜我说：")
		builder.WriteString(truncate(activity.Reply, 512))
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
