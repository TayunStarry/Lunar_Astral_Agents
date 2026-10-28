package lunar

// handle.go 句柄的薄适配层
//
// 句柄文法、类型码中文名、文本/二进制判定全部由 engine/resource 权威实现；
// 本文件只把 resource.Handle 转成 lunar 侧面向模型的 ResourceHandle 视图，
// 不保留第二份正则或标签表。

import (
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/engine/resource"
)

// kindLabel 类型码的中文名
func kindLabel(kind string) string {
	return resource.KindLabel(kind)
}

// isKnownKind 判断类型码是否为已定义的类型（D/I/V/A/T/U）
func isKnownKind(kind string) bool {
	return kind != "" && kindLabel(kind) != kind
}

// isTextKind 判断资源是否为文本类（链接只含地址与摘要，同样按文本直接取回）
func isTextKind(kind string) bool {
	return resource.IsTextKind(kind)
}

// parseHandle 解析句柄文本（容忍首尾空白与方括号包裹）
func parseHandle(raw string) *ResourceHandle {
	h, ok := resource.ParseHandle(raw)
	if !ok || h.Seq <= 0 {
		return nil
	}
	handle := &ResourceHandle{Kind: h.Kind, Seq: int(h.Seq), Anchor: "none"}
	switch {
	case h.HasSection:
		handle.Anchor = "section"
		handle.Section = h.SectionOrdinal
	case h.HasLine:
		handle.Anchor = "line"
		handle.LineStart = h.LineStart
		handle.LineEnd = h.LineEnd
		if handle.LineEnd == 0 {
			handle.LineEnd = handle.LineStart
		}
	}
	return handle
}

// toResourceHandle 把 lunar 侧句柄转回权威句柄类型
func toResourceHandle(handle ResourceHandle) resource.Handle {
	out := resource.Handle{Kind: handle.Kind, Seq: int64(handle.Seq)}
	switch handle.Anchor {
	case "section":
		if handle.Section > 0 {
			out.HasSection = true
			out.SectionOrdinal = handle.Section
		}
	case "line":
		if handle.LineStart > 0 {
			end := handle.LineEnd
			if end == 0 {
				end = handle.LineStart
			}
			out.HasLine = true
			out.LineStart = handle.LineStart
			out.LineEnd = end
		}
	}
	return out
}

// formatHandle 按文法格式化句柄（含锚点）
func formatHandle(handle ResourceHandle) string {
	return toResourceHandle(handle).FullText()
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
	return resource.FormatHandle(kind, int64(seq))
}

// sectionHandle 构造章节锚点句柄
func sectionHandle(kind string, seq, ordinal int) string {
	return handleOf(kind, seq) + "|§" + strconv.Itoa(ordinal)
}

// extractHandles 从文本中提取全部句柄
func extractHandles(text string) []string {
	return resource.ExtractHandles(text)
}

// describeHandle 面向用户的句柄说明
func describeHandle(raw string) string {
	handle := parseHandle(raw)
	if handle == nil {
		return strings.TrimSpace(raw)
	}
	base := kindLabel(handle.Kind) + " " + handleOf(handle.Kind, handle.Seq)
	switch handle.Anchor {
	case "section":
		return base + " 第 " + strconv.Itoa(handle.Section) + " 节"
	case "line":
		return base + " 第 " + strconv.Itoa(handle.LineStart) + "-" + strconv.Itoa(handle.LineEnd) + " 行"
	}
	return base
}

// grammarCheatsheet 工具描述拼接用的文法速查
func grammarCheatsheet() string {
	return "需要快速探查内容时，把需求填进 use_resource 的 ask 字段"
}
