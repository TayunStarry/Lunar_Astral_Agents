//go:build windows

package execute

// agent.go 「桌面执行」的唯一模型入口：意图 → 提示词与工具子集 → 基座调用。
//
// 本成员是所有会改变桌面状态的动作的唯一出口：启动/激活/关闭窗口、
// 键鼠输入、UIA 写入。一次调用只做一个原子动作，结果与判定交回编排层。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/desktop"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	desktopos "LunarSubsystem/LunarHarness/engine/desktop"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// base 构建本成员的智能体基座（统一超时与轮次）。
func base() *agentbase.Agent {
	return agentbase.NewAgent(agentbase.AgentConfig{
		ID: AgentID, Label: modelRole, Timeout: modelTimeout, MaxRounds: maxRounds,
	})
}

// chat 发起一次带循环监管的模型调用。
func chat(ctx context.Context, messages []desktop.ChatMessage, tools []desktop.ToolDef) (desktop.ChatMessage, error) {
	if ctx.Err() != nil {
		return desktop.ChatMessage{}, desktop.ErrCanceled
	}
	reply, err := base().StreamWithRetry(ctx, agentbase.StreamRequest{
		Agent:       AgentID,
		Role:        modelRole,
		Messages:    messages,
		Tools:       tools,
		ReportTools: true,
	}, agentbase.RetryOptions{
		MaxAttempts: autoLoopMaxRetries + 1,
		RetryLoop:   true,
		OnRetry: func(attempt int, err error) {
			logger.Warn("LunarHarness", modelRole+"检测到模型重复输出，已中断并重新生成（重试 %d/%d）", attempt, autoLoopMaxRetries)
		},
	})
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, desktop.ErrCanceled) {
			return desktop.ChatMessage{}, desktop.ErrCanceled
		}
		if errors.Is(err, agentbase.ErrLoop) {
			logger.Error("LunarHarness", modelRole+"模型连续 %d 次陷入重复输出，跳过本次调用", autoLoopMaxRetries+1)
		}
		return desktop.ChatMessage{}, err
	}
	calls := make([]desktop.ToolCall, 0, len(reply.ToolCalls))
	for _, call := range reply.ToolCalls {
		item := desktop.ToolCall{ID: call.ID, Type: call.Type}
		item.Function.Name = call.Function.Name
		item.Function.Arguments = call.Function.Arguments
		calls = append(calls, item)
	}
	return desktop.ChatMessage{Role: "assistant", Content: reply.Text, ToolCalls: calls}, nil
}

// Launch 启动/激活目标应用，最多进行 3 轮工具调用。
// 必须真正打开/激活目标窗口才算完成；模型未发起启动调用或每次都失败时如实报错。
// 单次回复里的多个 tool_calls 按顺序全部执行（上限 maxActionsPerReply），
// 超出上限直接报错，不再只取第一条、静默丢掉其余动作。
func Launch(ctx context.Context, plan desktop.EditorTask, handoff *desktop.HandoffRecord) (err error) {
	defer func() {
		desktop.RecordCall(modelRole, err)
		base().ReportRun(err)
	}()

	sysReq := desktop.ChatMessage{Role: "user", Content: desktop.BuildPlanContext(plan) + "\n\n请按流程找到/打开目标应用并输出 JSON。"}
	finalTools := roleTools(launcherTools)
	messages := []desktop.ChatMessage{{Role: "system", Content: promptLauncher}, sysReq}
	openedKey := ""
	lastErr := ""
	for range 3 {
		if err := ctx.Err(); err != nil {
			return desktop.ErrCanceled
		}
		msg, err := chat(ctx, messages, finalTools)
		if err != nil {
			return err
		}
		if len(msg.ToolCalls) == 0 {
			break
		}
		calls := msg.ToolCalls
		overflow := 0
		if len(calls) > maxActionsPerReply {
			overflow = len(calls) - maxActionsPerReply
			calls = calls[:maxActionsPerReply]
		}
		messages = append(messages, desktop.ChatMessage{Role: "assistant", Content: msg.Content, ToolCalls: calls})
		for idx, tc := range calls {
			if err := ctx.Err(); err != nil {
				reportUnreported(calls[idx:], "操作已取消")
				return desktop.ErrCanceled
			}
			args := parseArgs(tc.Function.Arguments)
			outcome, dispatchErr := dispatchWithDiff(ctx, tc.Function.Name, args)
			state, summary := outcome.state, outcome.summary
			res := outcome.res
			if dispatchErr != nil {
				if ctx.Err() != nil || errors.Is(dispatchErr, desktop.ErrCanceled) {
					reportToolResult(tc.Function.Name, state, summary)
					reportUnreported(calls[idx+1:], "同批前一个动作已取消")
					return desktop.ErrCanceled
				}
				lastErr = dispatchErr.Error()
			} else if res.Success {
				switch tc.Function.Name {
				case "launch_program":
					openedKey = argStr(args, "name")
				case "activate_window":
					openedKey = argStr(args, "title")
				case "open_folder":
					openedKey = filepath.Base(strings.TrimSpace(strings.TrimRight(argStr(args, "path"), `\`)))
				}
				// 启动/打开应用后立刻把新窗口提到前台，避免“只能看到旧窗口/开始界面”的体感。
				// 置前台成功是一次真实的窗口查询证据，可把结果从「状态未确认」升级为「已生效」。
				if tc.Function.Name == "launch_program" || tc.Function.Name == "open_folder" {
					key := argStr(args, "name")
					if key == "" {
						if p := argStr(args, "path"); p != "" {
							key = filepath.Base(strings.TrimSpace(strings.TrimRight(p, `\`)))
						}
					}
					activated, actErr := activateLaunched(ctx, key)
					if actErr != nil {
						reportToolResult(tc.Function.Name, agentbase.ToolUnknown, "启动已发出，等待置前台时被取消，状态未确认")
						reportUnreported(calls[idx+1:], "同批前一个动作已取消")
						return desktop.ErrCanceled
					}
					if activated != "" {
						state = agentbase.ToolApplied
						summary = "已查询到目标窗口并置于前台：" + activated
					}
				}
			} else {
				lastErr = res.Error
			}
			reportToolResult(tc.Function.Name, state, summary)
			rb, _ := json.Marshal(res)
			messages = append(messages, desktop.ChatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(rb)})
		}
		if overflow > 0 {
			reportUnreported(msg.ToolCalls[maxActionsPerReply:], fmt.Sprintf("超出单次动作上限 %d", maxActionsPerReply))
			return fmt.Errorf("启动者一次返回 %d 个动作，超过单次上限 %d", len(msg.ToolCalls), maxActionsPerReply)
		}
	}
	if openedKey == "" {
		if lastErr != "" {
			return errors.New("未能打开目标应用: " + lastErr)
		}
		return errors.New("启动者未发起任何打开应用的调用，任务中止")
	}
	handoff.Foreground = desktopos.ForegroundTitle()
	handoff.TargetApp = openedKey
	return nil
}

// activateLaunched 启动后立即把新窗口置前台；返回窗口查询确认到的标题。
// 返回空串表示已按要求重试仍没查询到目标窗口——调用方据此保持「状态未确认」。
func activateLaunched(ctx context.Context, key string) (string, error) {
	activated, _ := desktop.CallDesktop(ctx, 3*time.Second, "激活目标窗口", func() (string, error) {
		return desktopos.ActivateLaunchByName(key), nil
	})
	if activated != "" {
		desktop.Trace("【启动者】启动即置前台: %s", activated)
		return activated, nil
	}
	// 首次启动可能慢于 900ms：再补两次，确保执行循环开始时前台已是目标应用
	for attempt := 0; attempt < 2; attempt++ {
		select {
		case <-ctx.Done():
			return "", desktop.ErrCanceled
		case <-time.After(600 * time.Millisecond):
		}
		retried, _ := desktop.CallDesktop(ctx, 3*time.Second, "激活目标窗口", func() (string, error) {
			return desktopos.ActivateLaunchByName(key), nil
		})
		if retried != "" {
			desktop.Trace("【启动者】启动即置前台（重试 %d）: %s", attempt+1, retried)
			return retried, nil
		}
	}
	return "", nil
}
