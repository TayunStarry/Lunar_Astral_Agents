package lunar

import (
	"regexp"
	"strconv"
	"strings"
)

// ==== 统一资源句柄文法 ====

var (
	handleExactRegex  = regexp.MustCompile(`^@([DIVAT])\.(\d{3,})(?:\|(?:§(\d+)|L(\d+)-(\d+)))?$`)
	handleGlobalRegex = regexp.MustCompile(`@([DIVAT])\.(\d{3,})(?:\|(?:§(\d+)|L(\d+)-(\d+)))?`)
)

// kindLabels 类型码 → 中文类型名
var kindLabels = map[string]string{
	"D": "文档",
	"I": "图片",
	"V": "视频",
	"A": "音频",
	"T": "表格",
}

// isTextKind 判断资源是否为文本类
func isTextKind(kind string) bool {
	return kind == "D" || kind == "T"
}

// parseHandle 解析句柄文本（容忍首尾空白与方括号包裹）
func parseHandle(raw string) *ResourceHandle {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil
	}
	text = strings.TrimPrefix(text, "[")
	text = strings.TrimPrefix(text, "【")
	text = strings.TrimSuffix(text, "]")
	text = strings.TrimSuffix(text, "】")
	match := handleExactRegex.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	seq, err := strconv.Atoi(match[2])
	if err != nil || seq <= 0 {
		return nil
	}
	handle := &ResourceHandle{Kind: match[1], Seq: seq, Anchor: "none"}
	if match[3] != "" {
		handle.Anchor = "section"
		handle.Section, _ = strconv.Atoi(match[3])
	} else if match[4] != "" {
		handle.Anchor = "line"
		handle.LineStart, _ = strconv.Atoi(match[4])
		if match[5] != "" {
			handle.LineEnd, _ = strconv.Atoi(match[5])
		} else {
			handle.LineEnd = handle.LineStart
		}
	}
	return handle
}

// padSeq 序号补零（三位起）
func padSeq(seq int) string {
	if seq >= 1000 {
		return strconv.Itoa(seq)
	}
	return strings.Repeat("0", 3-len(strconv.Itoa(seq))) + strconv.Itoa(seq)
}

// formatHandle 按文法格式化句柄
func formatHandle(handle ResourceHandle) string {
	text := "@" + handle.Kind + "." + padSeq(handle.Seq)
	switch handle.Anchor {
	case "section":
		if handle.Section > 0 {
			text += "|§" + strconv.Itoa(handle.Section)
		}
	case "line":
		if handle.LineStart > 0 {
			end := handle.LineEnd
			if end == 0 {
				end = handle.LineStart
			}
			text += "|L" + strconv.Itoa(handle.LineStart) + "-" + strconv.Itoa(end)
		}
	}
	return text
}

// normalizeHandle 规整句柄文本
func normalizeHandle(raw string) string {
	handle := parseHandle(raw)
	if handle == nil {
		return ""
	}
	return formatHandle(*handle)
}

// handleOf 构造不带锚点的句柄
func handleOf(kind string, seq int) string {
	return "@" + kind + "." + padSeq(seq)
}

// sectionHandle 构造章节锚点句柄
func sectionHandle(kind string, seq, ordinal int) string {
	return handleOf(kind, seq) + "|§" + strconv.Itoa(ordinal)
}

// extractHandles 从文本中提取全部句柄
func extractHandles(text string) []string {
	if text == "" {
		return nil
	}
	found := make([]string, 0)
	for _, match := range handleGlobalRegex.FindAllString(text, -1) {
		if handle := parseHandle(match); handle != nil {
			found = append(found, formatHandle(*handle))
		}
	}
	return found
}

// describeHandle 面向用户的句柄说明
func describeHandle(raw string) string {
	handle := parseHandle(raw)
	if handle == nil {
		return strings.TrimSpace(raw)
	}
	label := kindLabels[handle.Kind]
	if label == "" {
		label = handle.Kind
	}
	base := label + " " + handleOf(handle.Kind, handle.Seq)
	if handle.Anchor == "section" {
		return base + " 第 " + strconv.Itoa(handle.Section) + " 节"
	}
	if handle.Anchor == "line" {
		return base + " 第 " + strconv.Itoa(handle.LineStart) + "-" + strconv.Itoa(handle.LineEnd) + " 行"
	}
	return base
}

// grammarCheatsheet 工具描述拼接用的文法速查
func grammarCheatsheet() string {
	return "句柄形如 @D.003（文档）、@I.007（图片）、@V.001（视频）、@A.002（音频）；" +
		"段落锚点可写 @D.003|§4（第 4 节）或 @D.003|L120-168（第 120-168 行）；" +
		"需要描述者协助时，把需求填进 use_resource 的 ask 字段（不要拼进句柄）"
}
