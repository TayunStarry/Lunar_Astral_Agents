//go:build windows

package desktop

// ==== 操作阶段：键盘 / 鼠标 / UIA 操作者 ====
// 每轮只启用一个操作者；返回值必须携带「是否真的执行」与「是否产生可见变化」。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// operationPhase 执行操作者（键盘/鼠标/UIA）：按决策调用对应操作工具。
// 返回值携带「是否真的执行了工具」与「是否产生可见变化」，供编排层判定停滞。
func operationPhase(ctx context.Context, dec planDecision) ActionAttempt {
	attempt := ActionAttempt{Decision: dec.Decision}
	var tools []string
	var role string
	switch dec.Decision {
	case "keyboard":
		tools = keyboardTools
		role = autoRoleKeyboard
	case "mouse":
		tools = mouseTools
		role = autoRoleMouse
	case "uia":
		tools = uiaOpTools
		role = autoRoleUIAOp
	default:
		attempt.Result = "未知决策：" + dec.Action
		return attempt
	}
	var prompt string
	switch dec.Decision {
	case "keyboard":
		prompt = promptKeyboard
	case "mouse":
		prompt = promptMouse
	case "uia":
		prompt = promptUIAOp
	}
	// 鼠标决策才需要画面：把带坐标网格的截图直接交给鼠标操作者，
	// 不再经过一轮“视觉理解者”转述（转述既慢又容易与真实窗口状态矛盾）。
	userMsg := chatMessage{Role: "user", Content: "任务目标：" + dec.Action}
	if dec.Decision == "mouse" {
		parts := []contentPart{{Type: "text", Text: "任务目标：" + dec.Action + "\n\n下方是当前前台窗口截图（带窗口内像素坐标网格），请据此确定点击/拖拽坐标。"}}
		if img, err := DTCaptureAnnotated(); err == nil {
			parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: "data:image/png;base64," + base64Encode(img)}})
		}
		userMsg.Content = parts
	}
	messages := []chatMessage{{Role: "system", Content: prompt}, userMsg}
	changed := false
	for i := 0; i < 3; i++ {
		if err := ctx.Err(); err != nil {
			attempt.Result = "操作已中断"
			return attempt
		}
		msg, err := autoChat(ctx, role, messages, roleTools(tools))
		if err != nil {
			attempt.Result = "操作失败: " + err.Error()
			return attempt
		}
		if len(msg.ToolCalls) == 0 {
			attempt.Result = "操作者未执行任何工具（空操作）"
			return attempt
		}
		tc := msg.ToolCalls[0]
		messages = append(messages, chatMessage{Role: "assistant", Content: msg.Content, ToolCalls: []ltpToolCall{tc}})
		args := parseLTPArgs(tc.Function.Arguments)
		attempt.Tool = tc.Function.Name
		observeTool(role, tc.Function.Name, tc.Function.Arguments)
		res, effect := dispatchWithDiff(tc.Function.Name, args)
		changed = changed || effect
		rb, _ := json.Marshal(res)
		messages = append(messages, chatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(rb)})
		if res.Success && effect {
			attempt.Result = fmt.Sprintf("%s(%s)", dec.Decision, res.Text)
			attempt.Success = true
			attempt.Changed = true
			return attempt
		}
		if res.Success {
			// 工具报告成功但画面无变化：可能是无效操作（点错位置/焦点未切换），继续给操作者一次机会
			attempt.Result = fmt.Sprintf("%s(%s)，但画面无变化", dec.Decision, res.Text)
			attempt.Success = true
			attempt.Changed = changed
			continue
		}
		attempt.Result = "操作失败: " + res.Error
	}
	attempt.Changed = changed
	return attempt
}

// ensureTargetForeground 确保目标应用窗口在前台；无法激活时如实失败，
// 避免后续阶段对着错误窗口（控制台/资源管理器）继续操作。
// 首次启动的应用可能较慢，这里最多等待约 8 秒。
func ensureTargetForeground(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		if dtForegroundMatches(target) {
			return nil
		}
		if t := DTActivateLaunchByName(target); t != "" && dtForegroundMatches(target) {
			traceAuto("【编排】目标窗口已置前: %s", t)
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("目标窗口「%s」不在前台且无法激活", target)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
