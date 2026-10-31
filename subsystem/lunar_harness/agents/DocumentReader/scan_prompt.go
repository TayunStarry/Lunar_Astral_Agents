package DocumentReader

// scan_prompt.go 轻量扫描通道的提示词组装与计划解析。
//
// 主模型只负责「规划」与「归并」，轻量模型只负责「单片摘要」；
// 原文是否回读由主模型在归并阶段通过查询工具决定。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/agentchat"
)

// planScan 让主模型根据文件画像与需求产出扫描计划
func planScan(doc *docContext, handle, request string) scanPlan {
	system := agentchat.PromptText("documentScanPlan.md")
	user := agentchat.RenderPrompt(agentchat.PromptText("documentScanPlanBriefing.md"),
		"{request}", request,
		"{handle}", handle,
		"{name}", doc.Resource.Name,
		"{profile}", formatProfileText(doc.Profile),
		"{chars}", fmt.Sprintf("%d", doc.Profile.Runes),
		"{lines}", fmt.Sprintf("%d", len(doc.Lines)),
		"{chunks}", fmt.Sprintf("%d", len(doc.Chunks)),
		"{summary}", firstNonEmpty(strings.TrimSpace(doc.Resource.Summary), "（暂无现成摘要）"),
		"{outline}", formatOutline(doc.Outline, 20),
	)
	text, err := agentchat.ChatWithRetry(roleLabel,
		[]agentchat.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		}, nil, "扫描计划")
	if err != nil {
		logger.Warn("CrystalAstral", "文献解析：扫描计划生成失败，回退原始需求: %v", err)
		return scanPlan{Mode: scanModeTargeted, Goal: request}
	}
	return parseScanPlan(text, request)
}

// parseScanPlan 解析主模型输出的 JSON 计划；非 JSON 时退化为定向扫描
func parseScanPlan(raw, request string) scanPlan {
	cleaned := trimCodeFence(raw)
	payload := extractJSONObject(cleaned)
	var parsed struct {
		Mode  string   `json:"mode"`
		Goal  string   `json:"goal"`
		Focus []string `json:"focus"`
	}
	if payload != "" && json.Unmarshal([]byte(payload), &parsed) == nil {
		mode := strings.ToLower(strings.TrimSpace(parsed.Mode))
		switch mode {
		case scanModeTargeted, scanModeGeneric, scanModeDirect:
		default:
			mode = scanModeTargeted
		}
		goal := strings.TrimSpace(parsed.Goal)
		if goal == "" {
			goal = request
		}
		return scanPlan{
			Mode:  mode,
			Goal:  truncateRunes(goal, scanPlanMaxChars),
			Focus: normalizeFocus(parsed.Focus),
		}
	}
	goal := truncateRunes(strings.TrimSpace(cleaned), scanPlanMaxChars)
	if goal == "" {
		goal = request
	}
	return scanPlan{Mode: scanModeTargeted, Goal: goal}
}

// trimCodeFence 去掉模型常见的 ```json 包裹
func trimCodeFence(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```JSON")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text)
}

// extractJSONObject 取出首个 { 到最后一个 } 之间的内容
func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

// normalizeFocus 规整关注词：去空、去重、限量
func normalizeFocus(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
		if len(out) >= scanFocusMaxItems {
			break
		}
	}
	return out
}

// buildMergeBriefing 组装归并阶段的输入：只给计划、结构与笔记，不给整份原文
func buildMergeBriefing(doc *docContext, handle, request string, plan scanPlan, chunks, spans []chunkSpan, notes []scanNote) string {
	coverage := fmt.Sprintf("已扫描 %d / %d 片", len(spans), len(chunks))
	if len(spans) < len(chunks) {
		coverage += "（超出单次上限，按相关度抽取了片段）"
	}
	return agentchat.RenderPrompt(agentchat.PromptText("documentScanMergeBriefing.md"),
		"{request}", request,
		"{goal}", plan.Goal,
		"{mode}", plan.Mode,
		"{coverage}", coverage,
		"{handle}", handle,
		"{name}", doc.Resource.Name,
		"{chars}", fmt.Sprintf("%d", doc.Profile.Runes),
		"{lines}", fmt.Sprintf("%d", len(doc.Lines)),
		"{profile}", formatProfileText(doc.Profile),
		"{outline}", formatOutline(doc.Outline, 20),
		"{summary}", firstNonEmpty(strings.TrimSpace(doc.Resource.Summary), "（暂无现成摘要）"),
		"{notes}", renderScanNotes(notes),
	)
}

// renderScanNotes 渲染笔记清单，单片与总量都做截断保护
func renderScanNotes(notes []scanNote) string {
	var builder strings.Builder
	chars := 0
	for _, note := range notes {
		block := fmt.Sprintf("◆ 片 #%d L%d-%d\n%s\n\n",
			note.Span.Index, note.Span.StartLine, note.Span.EndLine,
			truncateRunes(note.Text, scanNoteMaxChars))
		blockChars := len([]rune(block))
		if chars+blockChars > scanMergeNotesChars {
			break
		}
		builder.WriteString(block)
		chars += blockChars
	}
	if builder.Len() == 0 {
		return "（无扫描笔记）"
	}
	return strings.TrimSpace(builder.String())
}

// firstNonEmpty 返回第一个非空字符串
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
