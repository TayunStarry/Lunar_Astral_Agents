//go:build windows

package execute

// operate.go 「桌面执行」的操作意图：键盘 / 鼠标 / UIA 写入。
// 每轮只启用一个意图；返回值必须携带「是否真的执行」与「是否产生可见变化」。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/desktop"
	desktopos "LunarSubsystem/LunarHarness/engine/desktop"
)

// Act 按规划决策执行一个操作，返回结构化的尝试记录（供编排层判定停滞与去重）。
// 单次模型回复里的多个 tool_calls 按顺序执行（上限 maxActionsPerReply）；
// 某个动作失败后，同回复中剩余动作不再执行，但会显式回填“未执行”结果——
// 既不静默丢弃，也不在已出错的状态上继续叠加操作。
func Act(ctx context.Context, dec desktop.PlanDecision, handoff desktop.HandoffRecord) (attempt desktop.ActionAttempt) {
	attempt.Decision = dec.Decision
	intent, ok := intentOf(dec.Decision)
	if !ok {
		attempt.Result = "未知决策：" + dec.Action
		return attempt
	}
	var executed []string
	defer func() {
		attempt.Tool = strings.Join(executed, "+")
		if attempt.Success {
			desktop.RecordCall(modelRole, nil)
			return
		}
		reason := strings.TrimSpace(attempt.Result)
		if reason == "" {
			reason = "操作未产生结果"
		}
		runErr := errors.New(reason)
		desktop.RecordCall(modelRole, runErr)
		base().ReportRun(runErr)
	}()

	// 鼠标决策才需要画面：把带坐标网格的截图直接交给鼠标操作者，
	// 不再经过一轮“视觉理解者”转述（转述既慢又容易与真实窗口状态矛盾）。
	userMsg := desktop.ChatMessage{Role: "user", Content: "任务目标：" + dec.Action}
	if intent == IntentMouse {
		parts := []desktop.ContentPart{{Type: "text", Text: "任务目标：" + dec.Action + "\n\n下方是当前前台窗口截图（带窗口内像素坐标网格），请据此确定点击/拖拽坐标。"}}
		if img, err := desktop.CallDesktop(ctx, desktop.DesktopCallTimeout, "截取前台窗口", desktopos.CaptureAnnotated); err == nil {
			parts = append(parts, desktop.ContentPart{Type: "image_url", ImageURL: &desktop.ImageURL{URL: "data:image/png;base64," + desktop.Base64Encode(img)}})
		}
		userMsg.Content = parts
	}
	messages := []desktop.ChatMessage{{Role: "system", Content: promptsByIntent[intent]}, userMsg}
	changed := false
	for i := 0; i < 3; i++ {
		if err := ctx.Err(); err != nil {
			attempt.Result = "操作已中断"
			return attempt
		}
		msg, err := chat(ctx, messages, roleTools(toolsByIntent[intent]))
		if err != nil {
			attempt.Result = "操作失败: " + err.Error()
			return attempt
		}
		if len(msg.ToolCalls) == 0 {
			attempt.Result = "操作者未执行任何工具（空操作）"
			return attempt
		}
		calls := msg.ToolCalls
		overflow := 0
		if len(calls) > maxActionsPerReply {
			overflow = len(calls) - maxActionsPerReply
			calls = calls[:maxActionsPerReply]
		}
		messages = append(messages, desktop.ChatMessage{Role: "assistant", Content: msg.Content, ToolCalls: calls})

		failed := ""
		failedUnknown := false
		roundChanged := false
		var roundTexts []string
		for idx, tc := range calls {
			if failed != "" {
				// 前一个动作已失败：不再执行，但显式回填结果，保持 assistant/tool 消息配对完整
				reportUnreported([]desktop.ToolCall{tc}, "同一回复中的前一个动作已失败")
				skip, _ := json.Marshal(desktop.ToolResult{Success: false, Error: "未执行：同一回复中的前一个动作已失败"})
				messages = append(messages, desktop.ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(skip)})
				continue
			}
			if err := ctx.Err(); err != nil {
				reportUnreported(calls[idx:], "操作已取消")
				attempt.Result = "操作已中断"
				return attempt
			}
			args := parseArgs(tc.Function.Arguments)
			executed = append(executed, tc.Function.Name)
			inputText := ""
			if isTextInputTool(tc.Function.Name) {
				inputText = strings.TrimSpace(argStr(args, "text"))
			}
			if inputText != "" && repeatedInput(handoff, inputText) {
				reportUnreported([]desktop.ToolCall{tc}, "窗口已包含「"+inputText+"」，跳过重复输入")
				reportUnreported(calls[idx+1:], "前一个动作被判定为重复输入，未执行")
				if overflow > 0 {
					reportUnreported(msg.ToolCalls[maxActionsPerReply:], "前一个动作被判定为重复输入，未执行")
				}
				attempt.InputText = inputText
				attempt.Duplicate = true
				attempt.Success = true
				attempt.Changed = true
				attempt.Result = "窗口已包含「" + inputText + "」，跳过重复输入"
				return attempt
			}
			outcome, dispatchErr := dispatchWithDiff(ctx, tc.Function.Name, args)
			reportToolResult(tc.Function.Name, outcome.state, outcome.summary)
			res := outcome.res
			if dispatchErr != nil {
				if ctx.Err() != nil || errors.Is(dispatchErr, desktop.ErrCanceled) {
					reportUnreported(calls[idx+1:], "同批前一个动作已取消")
					attempt.Result = "操作已中断"
					return attempt
				}
				// 超时属于「状态未确认」：动作可能已经落地，不能并入普通失败
				failed = fmt.Sprintf("第 %d 个动作超时，状态未确认", idx+1)
				failedUnknown = true
			} else if res.Success {
				changed = changed || outcome.effect
				roundChanged = roundChanged || outcome.effect
				roundTexts = append(roundTexts, res.Text)
			} else {
				failed = fmt.Sprintf("第 %d 个动作失败: %s", idx+1, res.Error)
			}
			if inputText != "" {
				attempt.InputText = inputText
			}
			rb, _ := json.Marshal(res)
			messages = append(messages, desktop.ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(rb)})
		}

		if overflow > 0 {
			reportUnreported(msg.ToolCalls[maxActionsPerReply:], fmt.Sprintf("超出单次动作上限 %d", maxActionsPerReply))
			notice := fmt.Sprintf("执行者一次返回 %d 个动作，超过单次上限 %d（仅执行前 %d 个）", len(msg.ToolCalls), maxActionsPerReply, maxActionsPerReply)
			if failed != "" {
				notice += "；" + failed
			}
			if failedUnknown {
				attempt.Result = "状态未确认: " + notice
			} else {
				attempt.Result = "操作失败: " + notice
			}
			attempt.Changed = changed
			return attempt
		}
		if failed != "" {
			// 超时只是「没等到结论」，动作可能已经落地，不得写成失败
			if failedUnknown {
				attempt.Result = "状态未确认: " + failed
			} else {
				attempt.Result = "操作失败: " + failed
			}
			continue
		}
		summary := strings.Join(roundTexts, "；")
		if roundChanged {
			attempt.Result = fmt.Sprintf("%s(%s)", dec.Decision, summary)
			attempt.Success = true
			attempt.Changed = true
			return attempt
		}
		// 工具报告成功但画面无变化：可能是无效操作（点错位置/焦点未切换），继续给操作者一次机会
		attempt.Result = fmt.Sprintf("%s(%s)，但画面无变化", dec.Decision, summary)
		attempt.Success = true
		attempt.Changed = changed
	}
	attempt.Changed = changed
	return attempt
}

// intentOf 把规划决策映射为执行意图。
func intentOf(decision string) (Intent, bool) {
	switch decision {
	case "keyboard":
		return IntentKeyboard, true
	case "mouse":
		return IntentMouse, true
	case "uia":
		return IntentUIA, true
	}
	return "", false
}

// repeatedInput 判断相同的文本输入是否已成功执行过，且当前前台窗口标题已包含该文本。
// 只拦截「同一段文本 + 窗口已证实包含」的组合，避免把多步输入误判为重复。
func repeatedInput(handoff desktop.HandoffRecord, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || !strings.Contains(handoff.Foreground, text) {
		return false
	}
	for i := len(handoff.Attempts) - 1; i >= 0; i-- {
		attempt := handoff.Attempts[i]
		if attempt.Success && attempt.InputText == text {
			return true
		}
	}
	return false
}
