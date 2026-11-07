package viewer

// memory.go 观影记忆的渲染。
//
// 记忆的存取、上限维持见 engine/viewerstore；这里只负责把记忆渲染进提示词。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

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
