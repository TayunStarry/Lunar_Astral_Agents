//go:build windows

package desktop

// ==== 启动阶段：编纂者 → 启动者 ====
// 把自然语言请求收敛为结构化任务，并确保目标应用真的被打开、置于前台。

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// editorPhase 提示词编纂者：将用户原始指令解析为结构化任务 JSON。
func editorPhase(ctx context.Context, instruction string) (editorTask, error) {
	msg := chatMessage{Role: "user", Content: "用户请求：「" + instruction + "」\n\n请输出结构化任务 JSON。"}
	text, err := autoChatText(ctx, autoRoleEditor, []chatMessage{{Role: "system", Content: promptEditor}, msg})
	if err != nil {
		return editorTask{}, err
	}
	var t editorTask
	if err := json.Unmarshal(extractJSON(text), &t); err != nil {
		// 兜底：解析失败也保留原始指令，不阻塞流程
		return editorTask{Task: instruction, Goal: instruction, Acceptance: []string{}}, nil
	}
	return t, nil
}

// launcherPhase 软件启动者：查找/打开目标应用，最多进行 3 轮工具调用。
// 必须真正打开/激活目标窗口才算完成；模型未发起启动调用或每次都失败时如实报错。
func launcherPhase(ctx context.Context, plan editorTask, handoff *HandoffRecord) error {
	sysReq := chatMessage{Role: "user", Content: buildPlanContext(plan) + "\n\n请按流程找到/打开目标应用并输出 JSON。"}
	finalTools := roleTools(launcherTools)
	messages := []chatMessage{{Role: "system", Content: promptLauncher}, sysReq}
	openedKey := ""
	lastErr := ""
	for range 3 {
		if err := ctx.Err(); err != nil {
			return ErrCanceled
		}
		msg, err := autoChat(ctx, autoRoleLauncher, messages, finalTools)
		if err != nil {
			return err
		}
		if len(msg.ToolCalls) == 0 {
			break
		}
		tc := msg.ToolCalls[0]
		messages = append(messages, chatMessage{Role: "assistant", Content: msg.Content, ToolCalls: []ltpToolCall{tc}})
		args := parseLTPArgs(tc.Function.Arguments)
		observeTool(autoRoleLauncher, tc.Function.Name, tc.Function.Arguments)
		res, _ := dispatchWithDiff(tc.Function.Name, args)
		if res.Success {
			switch tc.Function.Name {
			case "launch_program":
				openedKey = ltpArgStr(args, "name")
			case "activate_window":
				openedKey = ltpArgStr(args, "title")
			case "open_folder":
				openedKey = filepath.Base(strings.TrimSpace(strings.TrimRight(ltpArgStr(args, "path"), `\`)))
			}
			// 启动/打开应用后立刻把新窗口提到前台，避免“只能看到旧窗口/开始界面”的体感
			if tc.Function.Name == "launch_program" || tc.Function.Name == "open_folder" {
				key := ltpArgStr(args, "name")
				if key == "" {
					if p := ltpArgStr(args, "path"); p != "" {
						key = filepath.Base(strings.TrimSpace(strings.TrimRight(p, `\`)))
					}
				}
				if t := DTActivateLaunchByName(key); t != "" {
					traceAuto("【启动者】启动即置前台: %s", t)
				} else {
					// 首次启动可能慢于 900ms：再补两次，确保执行循环开始时前台已是目标应用
					for attempt := 0; attempt < 2; attempt++ {
						time.Sleep(600 * time.Millisecond)
						if t := DTActivateLaunchByName(key); t != "" {
							traceAuto("【启动者】启动即置前台（重试 %d）: %s", attempt+1, t)
							break
						}
					}
				}
			}
		} else {
			lastErr = res.Error
		}
		rb, _ := json.Marshal(res)
		messages = append(messages, chatMessage{Role: "tool", ToolCallID: tc.ID, Content: string(rb)})
	}
	if openedKey == "" {
		if lastErr != "" {
			return fmt.Errorf("未能打开目标应用: %s", lastErr)
		}
		return fmt.Errorf("启动者未发起任何打开应用的调用，任务中止")
	}
	handoff.Foreground = DTForegroundTitle()
	handoff.TargetApp = openedKey
	return nil
}
