package viewer

// memory.go 观影记忆的渲染。
//
// 记忆的存取、上限维持见 engine/viewerstore；这里只负责把记忆渲染进提示词。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
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
