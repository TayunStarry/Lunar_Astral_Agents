package thinking

// handle.go 句柄的薄适配层
//
// 句柄文法、类型码中文名、文本/二进制判定全部由 engine/resource 权威实现；
// 本文件只把 resource.Handle 转成 lunar 侧面向模型的 ResourceHandle 视图，
// 不保留第二份正则或标签表。

import (
	"LunarSubsystem/LunarHarness/engine/resource"
)

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

// ==== 工具参数小工具 ====
