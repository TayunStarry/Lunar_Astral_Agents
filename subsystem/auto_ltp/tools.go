//go:build windows

package AutoLTP

import (
	"encoding/json"
	"fmt"
	"image"
	"strings"
)

// dispatchTool 按工具名分发到对应桌面操作原语，返回统一结果。
func dispatchTool(name string, args map[string]any) ltpToolResult {
	switch name {
	case "list_windows":
		wins := DTListWindows()
		if len(wins) == 0 {
			return ok("未发现可见窗口")
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "当前前台窗口：%s\n共 %d 个窗口：\n", DTForegroundTitle(), len(wins))
		for _, w := range wins {
			fmt.Fprintf(&sb, "- %s（进程:%s, 类:%s, PID:%d）\n", w.Title, w.Process, w.Class, w.PID)
		}
		return ok(sb.String())

	case "activate_window":
		if err := DTActivateWindow(ltpArgStr(args, "title")); err != nil {
			return fail(err.Error())
		}
		return ok("已激活窗口「" + ltpArgStr(args, "title") + "」")

	case "launch_program":
		s, err := DTLaunchProgram(ltpArgStr(args, "name"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(s)

	case "open_folder":
		s, err := DTOpenFolder(ltpArgStr(args, "path"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(s)

	case "close_window":
		s, err := DTCloseWindow(ltpArgStr(args, "title"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(s)

	case "type_text":
		if err := DTTypeText(ltpArgStr(args, "text")); err != nil {
			return fail(err.Error())
		}
		return ok("已键入文本")

	case "type_and_send":
		s, err := DTTypeAndSend(ltpArgStr(args, "text"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(s)

	case "press_key":
		if err := DTPressKey(ltpArgStr(args, "key")); err != nil {
			return fail(err.Error())
		}
		return ok("已按下「" + ltpArgStr(args, "key") + "」")

	case "click":
		sx, sy, err := DTWindowToScreen(ltpArgInt(args, "x"), ltpArgInt(args, "y"))
		if err != nil {
			return fail(err.Error())
		}
		if err := DTMouseClick(sx, sy, ltpArgStr(args, "button"), ltpArgBool(args, "double")); err != nil {
			return fail(err.Error())
		}
		btn := ltpArgStr(args, "button")
		if btn == "" {
			btn = "左键"
		}
		return ok(fmt.Sprintf("已在窗口(%d,%d)%s点击", ltpArgInt(args, "x"), ltpArgInt(args, "y"), btn))

	case "mouse_hold":
		sx, sy, err := DTWindowToScreen(ltpArgInt(args, "x"), ltpArgInt(args, "y"))
		if err != nil {
			return fail(err.Error())
		}
		if err := DTMouseButton(sx, sy, ltpArgStr(args, "button"), ltpArgInt(args, "hold_ms")); err != nil {
			return fail(err.Error())
		}
		return ok(fmt.Sprintf("已在窗口(%d,%d)按住", ltpArgInt(args, "x"), ltpArgInt(args, "y")))

	case "mouse_drag":
		if err := DTMouseDrag(ltpArgInt(args, "x1"), ltpArgInt(args, "y1"), ltpArgInt(args, "x2"), ltpArgInt(args, "y2"), 10); err != nil {
			return fail(err.Error())
		}
		return ok(fmt.Sprintf("已拖拽从(%d,%d)到(%d,%d)", ltpArgInt(args, "x1"), ltpArgInt(args, "y1"), ltpArgInt(args, "x2"), ltpArgInt(args, "y2")))

	case "move_mouse":
		sx, sy, err := DTWindowToScreen(ltpArgInt(args, "x"), ltpArgInt(args, "y"))
		if err != nil {
			return fail(err.Error())
		}
		if err := DTMoveMouse(sx, sy); err != nil {
			return fail(err.Error())
		}
		return ok(fmt.Sprintf("已移动光标到窗口(%d,%d)", ltpArgInt(args, "x"), ltpArgInt(args, "y")))

	case "scroll_wheel":
		sx, sy, err := DTWindowToScreen(ltpArgInt(args, "x"), ltpArgInt(args, "y"))
		if err != nil {
			return fail(err.Error())
		}
		if err := DTScrollWheel(sx, sy, ltpArgStr(args, "direction"), ltpArgInt(args, "amount")); err != nil {
			return fail(err.Error())
		}
		return ok("已滚动滚轮：" + ltpArgStr(args, "direction"))

	case "uia_click":
		brief, ok2, err := DTUIAClick(ltpArgStr(args, "target"), ltpArgStr(args, "control_type"))
		if err != nil {
			return fail(err.Error())
		}
		if !ok2 {
			return fail("UIA 点击失败: " + brief)
		}
		return ok("已点击 UIA 元素 " + brief)

	case "uia_input":
		brief, ok2, err := DTUIAInput(ltpArgStr(args, "target"), ltpArgStr(args, "text"))
		if err != nil {
			return fail(err.Error())
		}
		if !ok2 {
			return fail("UIA 输入失败: " + brief)
		}
		return ok("已向 UIA 元素输入文本 " + brief)

	case "capture_screenshot":
		if _, err := DTCaptureAnnotated(); err != nil {
			return fail(err.Error())
		}
		return ok("已截取当前窗口（含坐标网格），截图已归档供视觉定位与记录")

	default:
		return fail("未知工具: " + name)
	}
}

// isOpTool 判断工具名是否为会改变画面内容的操作类工具（用于差异检测）。
func isOpTool(name string) bool {
	switch name {
	case "click", "mouse_hold", "mouse_drag", "move_mouse", "scroll_wheel",
		"type_text", "type_and_send", "press_key",
		"uia_click", "uia_input", "activate_window":
		return true
	}
	return false
}

// dispatchWithDiff 执行工具前先拍摄窗口缩略图，操作后对比画面差异并追加提示。
func dispatchWithDiff(name string, args map[string]any) ltpToolResult {
	var before any
	if isOpTool(name) {
		img, err := dtCaptureWindowThumb()
		if err == nil {
			before = img
		}
	}
	res := dispatchTool(name, args)
	if before != nil {
		if after, err := dtCaptureWindowThumb(); err == nil {
			changed, ratio, detail := DTDiffWindowThumb(before.(*image.RGBA), after)
			if changed {
				res.Text += fmt.Sprintf("\n【差异检测】操作后画面有变化：%s（变化占比约 %.1f%%）——这从画面侧佐证了操作可能已生效，请再结合截图确认。", detail, ratio*100)
			} else {
				res.Text += fmt.Sprintf("\n【差异检测】操作后画面几乎无变化（%.1f%%）——从画面侧看不出效果，请务必结合截图判断操作是否真正生效。", ratio*100)
			}
		}
	}
	return res
}

// ok 构造成功结果。
func ok(text string) ltpToolResult { return ltpToolResult{Success: true, Text: text} }

// fail 构造失败结果（携带错误描述）。
func fail(err string) ltpToolResult { return ltpToolResult{Success: false, Error: err} }

// ==== 工具参数解析 ====

// parseLTPArgs 解析工具调用的 JSON 参数串为 map；解析失败返回空 map。
func parseLTPArgs(s string) map[string]any {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return map[string]any{}
	}
	return m
}

// ltpArgStr 从参数 map 中读取指定 key 的字符串值，缺失或类型不符时返回空串。
func ltpArgStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// ltpArgInt 从参数 map 中读取指定 key 的整数值，缺失或类型不符时返回 0。
func ltpArgInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// ltpArgBool 从参数 map 中读取指定 key 的布尔值，缺失时返回 false。
func ltpArgBool(args map[string]any, key string) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}
