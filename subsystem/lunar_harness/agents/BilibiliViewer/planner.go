package BilibiliViewer

// planner.go 哔哩观影的自决编排：根据对话者的自然语言指令决定流程与是否分享。
//
// 对话者只给需求，不再传 flow/share；模型判定失败时按关键词兜底，
// 保证不会因为一次分类失败而取消整轮观影。

import (
	"regexp"
	"strings"
)

var (
	// planFlowRe 解析模型输出中的流程
	planFlowRe = regexp.MustCompile(`(?i)flow\s*[=:：]\s*(standard|watch_comment|reply_only)`)
	// planShareRe 解析模型输出中的分享开关
	planShareRe = regexp.MustCompile(`(?i)share\s*[=:：]\s*(true|false)`)
)

// decidePlan 返回本轮流程与是否分享
func decidePlan(instruction string) (Flow, bool) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return FlowStandard, true
	}
	system := "你是哔哩观影的任务编排器。根据用户这句话决定本轮流程与是否分享。\n" +
		"flow 取值：standard（处理未读消息+看视频与评论+写日记动态+回复动态评论）；" +
		"watch_comment（只看视频与评论+写日记动态）；reply_only（只回复未读消息）。\n" +
		"share 取值：true（从看过的视频里挑一条发 @分享评论）/ false（不分享、不 @ 人）。\n" +
		"用户没有明确说明时使用 standard 与 share=true。只输出两行：flow=... 与 share=..." +
		memoryPromptBlock()
	if text, err := chatTextWithDisplay(system, instruction, formatPlanDisplay); err == nil {
		if flowMatch := planFlowRe.FindStringSubmatch(text); len(flowMatch) > 1 {
			if flow, ok := ParseFlow(flowMatch[1]); ok {
				share := true
				if shareMatch := planShareRe.FindStringSubmatch(text); len(shareMatch) > 1 {
					share = strings.EqualFold(shareMatch[1], "true")
				}
				return flow, share
			}
		}
	}
	return fallbackPlan(instruction)
}

// formatPlanDisplay 把编排模型的输出排成一行可读文本。
// 模型常把两行写成 "flow=watch_commentshare=true"（没有换行与分隔），实况里挤成一团；
// 这里按解析结果补上分隔符，例如 "flow = watch_comment · share = true"。
func formatPlanDisplay(text string) string {
	flow := planFlowRe.FindStringSubmatch(text)
	if len(flow) < 2 {
		return text
	}
	share := "true"
	if match := planShareRe.FindStringSubmatch(text); len(match) > 1 {
		share = strings.ToLower(match[1])
	}
	return "flow = " + flow[1] + " · share = " + share
}

// fallbackPlan 模型判定失败时的关键词兜底
func fallbackPlan(instruction string) (Flow, bool) {
	share := true
	for _, word := range []string{
		"不要分享", "不用分享", "别分享", "不要@", "不用@", "别@", "不@", "别艾特", "不要艾特",
	} {
		if strings.Contains(instruction, word) {
			share = false
			break
		}
	}
	switch {
	case strings.Contains(instruction, "只回"), strings.Contains(instruction, "回复消息"),
		strings.Contains(instruction, "回消息"), strings.Contains(instruction, "处理消息"):
		return FlowReplyOnly, share
	case strings.Contains(instruction, "视频"), strings.Contains(instruction, "看"), strings.Contains(instruction, "刷"):
		return FlowWatchComment, share
	}
	return FlowStandard, share
}
