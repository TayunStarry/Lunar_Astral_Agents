package resource

// sections.go 段落结构解析 —— 资源目录的「索引单元」（不进句柄文法）
//
// 按 Markdown 标题层级解析为段落表，每个段落记录行区间与层级路径。
// 规则与 TypeScript 侧 split-markdown.ts 保持一致（标题层级路径以「 / 」连接）；
// 无标题的纯文本/源码退化为按固定行块切段，保证 outline 始终可用。

import (
	"strings"
)

// ParseMarkdownSections 解析文本的段落结构（1 基行号，含端点）
//
// 行为：
//   - 存在 Markdown 标题 → 每个标题一节，正文为标题行之后到下一标题行之前
//   - 无标题 → 按 SectionBlockLines 行分块，标题为空
func ParseMarkdownSections(text string) []SectionDraft {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}

	type heading struct {
		index int // 0 基行号
		level int
		title string
	}
	headings := []heading{}
	stack := []heading{}
	for i, line := range lines {
		level, title, ok := parseHeadingLine(line)
		if !ok {
			continue
		}
		// 维护层级栈：遇到更浅或同级标题就弹出
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, heading{index: i, level: level, title: title})
		headings = append(headings, heading{index: i, level: level, title: title})
	}
	if len(headings) == 0 {
		return fallbackSections(lines)
	}

	drafts := make([]SectionDraft, 0, len(headings))
	// 层级路径按标题顺序重算（与 TS 侧 parseMarkdownSections 的栈序一致）
	pathStack := []heading{}
	for ordinal, h := range headings {
		for len(pathStack) > 0 && pathStack[len(pathStack)-1].level >= h.level {
			pathStack = pathStack[:len(pathStack)-1]
		}
		pathStack = append(pathStack, h)
		path := make([]string, 0, len(pathStack))
		for _, item := range pathStack {
			path = append(path, item.title)
		}
		endIndex := len(lines)
		if ordinal+1 < len(headings) {
			endIndex = headings[ordinal+1].index
		}
		drafts = append(drafts, SectionDraft{
			Ordinal:   ordinal + 1,
			Level:     h.level,
			Title:     h.title,
			Path:      strings.Join(path, " / "),
			LineStart: h.index + 1,
			LineEnd:   endIndex,
			Chars:     countChars(lines[h.index:endIndex]),
		})
	}
	return drafts
}

// fallbackSections 无标题文本的行块切分
func fallbackSections(lines []string) []SectionDraft {
	drafts := []SectionDraft{}
	total := len(lines)
	for start := 0; start < total; start += SectionBlockLines {
		end := start + SectionBlockLines
		if end > total {
			end = total
		}
		drafts = append(drafts, SectionDraft{
			Ordinal:   len(drafts) + 1,
			Level:     0,
			Title:     "",
			Path:      "",
			LineStart: start + 1,
			LineEnd:   end,
			Chars:     countChars(lines[start:end]),
		})
	}
	return drafts
}

// parseHeadingLine 解析 Markdown ATX 标题行（# 到 ######，后接空格）
func parseHeadingLine(line string) (int, string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	rest := trimmed[level:]
	if rest == "" {
		// 纯 # 行按标题处理（内容为空）
		return level, "", true
	}
	if rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false
	}
	return level, strings.TrimSpace(rest), true
}

// countChars 统计行块字符数（不含换行）
func countChars(lines []string) int {
	total := 0
	for _, line := range lines {
		total += len([]rune(line))
	}
	return total
}
