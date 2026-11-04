package LifeMemorizer

// parse.go 记忆条目化整理的下限计算、模型输出解析与保底整理。

import (
	"encoding/json"
	"fmt"
	"strings"
)

// minKeepFor 整理后至少保留的条数：按输入条数比例取下限，并夹在 [floor, ceiling]。
func minKeepFor(count int) int {
	if count <= 0 {
		return 0
	}
	return min(min(max(count*KeepPercent/100, KeepFloor), KeepCeiling), count)
}

// fallbackTexts 模型过度压缩时的保底：原条目逐条去重、折叠空白并截短，
// 保留全部事实，只是把长段落压成短条目。
func fallbackTexts(contents []string) []string {
	out := make([]string, 0, len(contents))
	seen := make(map[string]bool, len(contents))
	for _, content := range contents {
		text := strings.TrimSpace(strings.Join(strings.Fields(content), " "))
		if text == "" {
			continue
		}
		text = truncateAtSentence(text, MaxTextRunes)
		if seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}

// parseArray 从模型输出里取出 JSON 字符串数组并清洗。
func parseArray(raw string) ([]string, error) {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		kept := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				continue
			}
			kept = append(kept, line)
		}
		text = strings.TrimSpace(strings.Join(kept, "\n"))
	}
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("输出不是 JSON 数组")
	}
	var values []string
	if err := json.Unmarshal([]byte(text[start:end+1]), &values); err != nil {
		return nil, fmt.Errorf("解析 JSON 数组失败: %w", err)
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		// 先按句读截短再去重：超长条目截断后可能变得完全相同，按截断后的正文判重才准
		value = truncateAtSentence(strings.TrimSpace(value), MaxTextRunes)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out, nil
}

// joinContents 把一批旧记忆正文拼成逐条列表（每条一行）。
func joinContents(contents []string) string {
	var builder strings.Builder
	for _, content := range contents {
		builder.WriteString("- ")
		builder.WriteString(content)
		builder.WriteString("\n")
	}
	return builder.String()
}
