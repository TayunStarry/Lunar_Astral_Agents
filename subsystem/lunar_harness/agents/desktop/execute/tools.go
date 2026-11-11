//go:build windows

package execute

// tools.go 「桌面执行」的工具分发与画面差异检测。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strings"

	"LunarSubsystem/LunarHarness/agents/desktop"
	desktopos "LunarSubsystem/LunarHarness/engine/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// roleTools 依据意图工具白名单从 allToolDefs 中抽取对应的工具定义子集。
func roleTools(toolNames []string) []desktop.ToolDef {
	out := []desktop.ToolDef{}
	for _, n := range toolNames {
		if t, ok := allToolDefs[n]; ok {
			out = append(out, t)
		}
	}
	return out
}

// dispatchTool 按工具名分发到对应桌面操作原语，返回统一结果。
func dispatchTool(name string, args map[string]any) desktop.ToolResult {
	// 自己人窗口保护：当前前台若是本程序（TUI 控制台 / 月华 / 琉璃），
	// 键鼠与 UIA 写入会打进自己的界面（实测会把“notepad”敲进控制台而无法启动任何程序）。
	if isSelfOpTool(name) && desktopos.ForegroundIsSelf() {
		return fail("当前前台是本程序自己的窗口（控制台/桌面），已拒绝执行；请先把目标应用窗口切到前台再重试")
	}
	switch name {
	case "list_windows":
		wins := desktopos.ListWindows()
		if len(wins) == 0 {
			return ok("未发现可见窗口")
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "当前前台窗口：%s\n共 %d 个窗口：\n", desktopos.ForegroundTitle(), len(wins))
		for _, w := range wins {
			fmt.Fprintf(&sb, "- %s（进程:%s, 类:%s, PID:%d）\n", w.Title, w.Process, w.Class, w.PID)
		}
		return ok(sb.String())

	case "activate_window":
		if err := desktopos.ActivateWindow(argStr(args, "title")); err != nil {
			return fail(err.Error())
		}
		return ok("已激活窗口「" + argStr(args, "title") + "」")

	case "launch_program":
		s, err := desktopos.LaunchProgram(argStr(args, "name"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(s)

	case "open_folder":
		s, err := desktopos.OpenFolder(argStr(args, "path"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(s)

	case "close_window":
		s, err := desktopos.CloseWindow(argStr(args, "title"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(s)

	case "type_text":
		if err := desktopos.TypeText(argStr(args, "text")); err != nil {
			return fail(err.Error())
		}
		return ok("已键入文本")

	case "type_and_send":
		s, err := desktopos.TypeAndSend(argStr(args, "text"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(s)

	case "press_key":
		if err := desktopos.PressKey(argStr(args, "key")); err != nil {
			return fail(err.Error())
		}
		return ok("已按下「" + argStr(args, "key") + "」")

	case "click":
		sx, sy, err := desktopos.WindowToScreen(argInt(args, "x"), argInt(args, "y"))
		if err != nil {
			return fail(err.Error())
		}
		if err := desktopos.MouseClick(sx, sy, argStr(args, "button"), argBool(args, "double")); err != nil {
			return fail(err.Error())
		}
		btn := argStr(args, "button")
		if btn == "" {
			btn = "左键"
		}
		return ok(fmt.Sprintf("已在窗口(%d,%d)%s点击", argInt(args, "x"), argInt(args, "y"), btn))

	case "mouse_hold":
		sx, sy, err := desktopos.WindowToScreen(argInt(args, "x"), argInt(args, "y"))
		if err != nil {
			return fail(err.Error())
		}
		if err := desktopos.MouseButton(sx, sy, argStr(args, "button"), argInt(args, "hold_ms")); err != nil {
			return fail(err.Error())
		}
		return ok(fmt.Sprintf("已在窗口(%d,%d)按住", argInt(args, "x"), argInt(args, "y")))

	case "mouse_drag":
		if err := desktopos.MouseDrag(argInt(args, "x1"), argInt(args, "y1"), argInt(args, "x2"), argInt(args, "y2"), 10); err != nil {
			return fail(err.Error())
		}
		return ok(fmt.Sprintf("已拖拽从(%d,%d)到(%d,%d)", argInt(args, "x1"), argInt(args, "y1"), argInt(args, "x2"), argInt(args, "y2")))

	case "move_mouse":
		sx, sy, err := desktopos.WindowToScreen(argInt(args, "x"), argInt(args, "y"))
		if err != nil {
			return fail(err.Error())
		}
		if err := desktopos.MoveMouse(sx, sy); err != nil {
			return fail(err.Error())
		}
		return ok(fmt.Sprintf("已移动光标到窗口(%d,%d)", argInt(args, "x"), argInt(args, "y")))

	case "scroll_wheel":
		sx, sy, err := desktopos.WindowToScreen(argInt(args, "x"), argInt(args, "y"))
		if err != nil {
			return fail(err.Error())
		}
		if err := desktopos.ScrollWheel(sx, sy, argStr(args, "direction"), argInt(args, "amount")); err != nil {
			return fail(err.Error())
		}
		return ok("已滚动滚轮：" + argStr(args, "direction"))

	case "uia_click":
		brief, ok2, err := desktopos.UIAClick(argStr(args, "target"), argStr(args, "control_type"))
		if err != nil {
			return fail(err.Error())
		}
		if !ok2 {
			return fail("UIA 点击失败: " + brief)
		}
		return ok("已点击 UIA 元素 " + brief)

	case "uia_input":
		brief, ok2, err := desktopos.UIAInput(argStr(args, "target"), argStr(args, "text"))
		if err != nil {
			return fail(err.Error())
		}
		if !ok2 {
			return fail("UIA 输入失败: " + brief)
		}
		return ok("已向 UIA 元素输入文本 " + brief)

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

// isSelfOpTool 会向当前前台窗口输入/点击的工具集合（不包含 activate_window：
// 它是从自家窗口切走目标应用的手段，必须允许）。
func isSelfOpTool(name string) bool {
	switch name {
	case "click", "mouse_hold", "mouse_drag", "move_mouse", "scroll_wheel",
		"type_text", "type_and_send", "press_key",
		"uia_click", "uia_input":
		return true
	}
	return false
}

// dispatchWithDiff 执行工具前先拍摄窗口缩略图，操作后对比画面差异并追加提示。
// 返回的操作结果里带三态与一句人话：三态只依据真实证据判定，
// 无证据（超时、取消、画面无变化、无前后指纹）一律 Unknown，不猜成功。
func dispatchWithDiff(ctx context.Context, name string, args map[string]any) (diffCallOutcome, error) {
	if err := ctx.Err(); err != nil {
		return diffCallOutcome{
			res:     fail("操作已中断"),
			state:   agentbase.ToolUnknown,
			summary: "操作已取消，状态未确认",
		}, desktop.ErrCanceled
	}
	outcome, err := desktop.CallDesktop(ctx, desktop.DesktopCallTimeout, "桌面操作", func() (diffCallOutcome, error) {
		var before *image.RGBA
		if isOpTool(name) {
			img, captureErr := desktopos.CaptureWindowThumb()
			if captureErr == nil {
				before = img
			}
		}
		res := dispatchTool(name, args)
		effect := res.Success
		captured := false
		changed := false
		// 键盘/文本类工具的实际效果往往只改变少量像素（甚至不改画面，如回车），
		// 不能仅凭截图差异判失败；点击类工具则必须看到画面变化才算有效。
		inputTool := isTextInputTool(name)
		if before != nil {
			if after, captureErr := desktopos.CaptureWindowThumb(); captureErr == nil {
				captured = true
				var ratio float64
				var detail string
				changed, ratio, detail = desktopos.DiffWindowThumb(before, after)
				if changed {
					res.Text += fmt.Sprintf("\n【差异检测】操作后画面有变化：%s（变化占比约 %.1f%%）——这从画面侧佐证了操作可能已生效，请再结合截图确认。", detail, ratio*100)
				} else if inputTool {
					// 文本/按键类输入经常只改变极少像素（标题栏或单个字符），
					// 不能把「画面无变化」写成失败信号，否则规划者会重复输入。
					res.Text += fmt.Sprintf("\n【差异检测】画面几乎无变化（%.1f%%）——文本/按键类操作以工具返回为准，可能已生效；请结合窗口标题/UIA 判断，不要重复输入相同内容。", ratio*100)
				} else {
					res.Text += fmt.Sprintf("\n【差异检测】操作后画面几乎无变化（%.1f%%）——从画面侧看不出效果，请务必结合截图判断操作是否真正生效。", ratio*100)
				}
				if inputTool {
					effect = res.Success
				} else {
					effect = res.Success && changed
				}
			}
		}
		state, summary := judgeToolOutcome(name, res, captured, changed)
		return diffCallOutcome{res: res, effect: effect, state: state, summary: summary}, nil
	})
	if err != nil {
		state, summary := agentbase.ToolUnknown, "执行超时，状态未确认"
		if errors.Is(err, desktop.ErrCanceled) {
			summary = "操作已取消，状态未确认"
		}
		return diffCallOutcome{res: fail(err.Error()), state: state, summary: summary}, err
	}
	return outcome, nil
}

// judgeToolOutcome 依据真实证据判定一次工具结果的三态：
//   - NotApplied：工具明确失败（唯一的「确定未生效」证据来源）；
//   - Applied：前后画面指纹有变化，或 UIA / 窗口查询返回了确认结果；
//   - Unknown：其余全部情况（超时、取消、画面无变化、没有可核对的证据），
//     未知既不当成功也不当失败。
func judgeToolOutcome(name string, res desktop.ToolResult, captured, changed bool) (agentbase.ToolResultState, string) {
	if !res.Success {
		return agentbase.ToolNotApplied, "工具明确失败：" + res.Error
	}
	switch name {
	case "activate_window":
		return agentbase.ToolApplied, "已查询确认目标窗口在前台"
	case "uia_click":
		return agentbase.ToolApplied, "UIA 元素激活调用已确认"
	case "uia_input":
		return agentbase.ToolApplied, "UIA 元素写入调用已确认"
	case "list_windows":
		return agentbase.ToolApplied, "窗口查询已返回结果"
	case "launch_program", "open_folder":
		return agentbase.ToolUnknown, "已发出启动/打开请求，尚未查询到目标窗口"
	case "close_window":
		return agentbase.ToolUnknown, "已发送关闭请求，未确认窗口是否真的关闭"
	}
	if changed {
		return agentbase.ToolApplied, "前后画面指纹有变化，动作已生效"
	}
	if captured {
		if isTextInputTool(name) {
			return agentbase.ToolUnknown, "已提交输入，画面无变化且无查询核对，无法确认是否生效"
		}
		return agentbase.ToolUnknown, "画面无变化，无法确认动作是否生效"
	}
	return agentbase.ToolUnknown, "工具已返回，但没有可核对的前后证据"
}

// reportToolResult 上报一次真正派发过的工具结果。
func reportToolResult(name string, state agentbase.ToolResultState, summary string) {
	agentbase.ReportToolResult(modelRole, name, state, summary)
}

// reportUnreported 为没有真正派发的工具调用补齐 Unknown 结果：
// 基座已上报调用行时，结果行必须成对出现，不能留「上报了却没执行」的悬空行。
func reportUnreported(calls []desktop.ToolCall, reason string) {
	for _, tc := range calls {
		agentbase.ReportToolResult(modelRole, tc.Function.Name, agentbase.ToolUnknown, "未执行："+reason)
	}
}

// isTextInputTool 文本/按键类工具：成功即视为产生效果（截图差异可能极小）。
func isTextInputTool(name string) bool {
	switch name {
	case "type_text", "type_and_send", "press_key", "uia_input":
		return true
	}
	return false
}

// ok 构造成功结果。
func ok(text string) desktop.ToolResult { return desktop.ToolResult{Success: true, Text: text} }

// fail 构造失败结果（携带错误描述）。
func fail(err string) desktop.ToolResult { return desktop.ToolResult{Success: false, Error: err} }

// ==== 工具参数解析 ====

// parseArgs 解析工具调用的 JSON 参数串为 map；解析失败返回空 map。
func parseArgs(s string) map[string]any {
	m := map[string]any{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return map[string]any{}
	}
	return m
}

// argStr 从参数 map 中读取指定 key 的字符串值，缺失或类型不符时返回空串。
func argStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// argInt 从参数 map 中读取指定 key 的整数值，缺失或类型不符时返回 0。
func argInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// argBool 从参数 map 中读取指定 key 的布尔值，缺失时返回 false。
func argBool(args map[string]any, key string) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}
