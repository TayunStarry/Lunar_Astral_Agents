package Descriptor

// outline.go 文件结构大纲：优先复用资源目录的段落表，其次按语言特征扫描顶层声明

import (
	"fmt"
	"strings"

	"CorePersonality/engine/persistence"
)

// buildOutline 生成结构大纲（每项带行区间）
func buildOutline(doc *docContext) []outlineItem {
	// 资源目录已有段落表（Markdown 标题等）：直接复用，保证与 use_resource 的章节大纲同源
	if sections, _, err := persistence.ResourceOutline(doc.Handle); err == nil && len(sections) > 0 {
		items := make([]outlineItem, 0, len(sections))
		for _, section := range sections {
			label := strings.TrimSpace(section.Path)
			if label == "" {
				label = section.Title
			}
			end := section.LineEnd
			if end < section.LineStart {
				end = section.LineStart
			}
			items = append(items, outlineItem{
				Start: section.LineStart,
				End:   end,
				Level: section.Level,
				Label: label,
			})
			if len(items) >= maxOutlineItems {
				break
			}
		}
		return items
	}
	return detectOutline(doc)
}

// detectOutline 按语言特征扫描顶层声明，行区间由相邻声明推算
func detectOutline(doc *docContext) []outlineItem {
	ext := strings.ToLower(doc.Resource.Ext)
	items := make([]outlineItem, 0, 32)
	for index, raw := range doc.Lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		label, level, ok := declarationOf(trimmed, line, ext)
		if !ok {
			continue
		}
		items = append(items, outlineItem{
			Start: index + 1,
			Level: level,
			Label: truncateRunes(label, 100),
		})
		if len(items) >= maxOutlineItems {
			break
		}
	}
	for i := range items {
		if i+1 < len(items) {
			items[i].End = items[i+1].Start - 1
		} else {
			items[i].End = len(doc.Lines)
		}
		if items[i].End < items[i].Start {
			items[i].End = items[i].Start
		}
	}
	return items
}

// declarationOf 判断一行是否是顶层结构声明
func declarationOf(trimmed, raw, ext string) (string, int, bool) {
	switch ext {
	case ".md", ".markdown":
		if match := reMDHeading.FindStringSubmatch(trimmed); match != nil {
			level := len(match[1])
			return match[2], level, true
		}
		return "", 0, false
	}
	// 其余语言只认顶格声明，避免把缩进内的语句当成结构
	if len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t') {
		return "", 0, false
	}
	switch ext {
	case ".go":
		switch {
		case reGoPackage.MatchString(trimmed):
			return trimmed, 1, true
		case reGoFunc.MatchString(trimmed):
			return trimmed, 2, true
		case reGoDecl.MatchString(trimmed):
			return trimmed, 2, true
		}
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue":
		if reTSDecl.MatchString(trimmed) {
			return trimmed, 2, true
		}
	case ".py":
		if rePyDecl.MatchString(trimmed) {
			return trimmed, 2, true
		}
	case ".rs":
		if reRustDecl.MatchString(trimmed) {
			return trimmed, 2, true
		}
	case ".java", ".cs", ".c", ".cc", ".cpp", ".cxx", ".h", ".hpp":
		if reCDecl.MatchString(trimmed) {
			return trimmed, 2, true
		}
	case ".yaml", ".yml":
		if !strings.HasPrefix(trimmed, "-") && strings.Contains(trimmed, ":") {
			return trimmed, 2, true
		}
	case ".sql":
		upper := strings.ToUpper(trimmed)
		for _, prefix := range []string{"CREATE TABLE", "CREATE VIEW", "CREATE INDEX", "CREATE FUNCTION", "CREATE PROCEDURE"} {
			if strings.HasPrefix(upper, prefix) {
				return trimmed, 2, true
			}
		}
	}
	return "", 0, false
}

// formatOutline 把大纲渲染为模型可读的清单（limit<=0 表示全部）
func formatOutline(items []outlineItem, limit int) string {
	if len(items) == 0 {
		return "（未识别到可用的结构大纲）"
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	var builder strings.Builder
	for _, item := range items {
		if item.Level > 0 {
			fmt.Fprintf(&builder, "L%d-%d [%d] %s\n", item.Start, item.End, item.Level, item.Label)
			continue
		}
		fmt.Fprintf(&builder, "L%d-%d %s\n", item.Start, item.End, item.Label)
	}
	return strings.TrimRight(builder.String(), "\n")
}
