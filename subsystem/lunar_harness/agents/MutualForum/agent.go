package MutualForum

// agent.go Mutual Forum 智能体的对外入口：自然语言需求 → 多轮工具调用 → 中文答复。
//
// 设计要点：
//   - 独立上下文：最近 20 轮「需求 + 答复」随每次调用一起带入；
//   - 多轮工具循环：模型通过 function calling 逐步取证/操作，上限 8 轮；
//   - 条件可用：桥接层 BridgeReady() 为 false 时工具不暴露，Run 也会拒绝执行。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Ready 返回 Mutual Forum 是否应暴露给月华。
func Ready() bool {
	return BridgeReady()
}

// Run 接收对话者的自然语言需求并执行论坛操作。
func Run(instruction string) (string, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", errors.New("请用自然语言说明要在论坛里做什么")
	}

	cfg, err := LoadConfig()
	if err != nil {
		return "", fmt.Errorf("互助论坛不可用：%w（配置文件：%s）", err, ConfigPath())
	}
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("互助论坛不可用：%w（请检查 %s 的 enabled 与 base_url）", err, ConfigPath())
	}

	if !BridgeReady() {
		return "", errors.New("互助论坛当前不可用：论坛健康检查未通过，或尚未配置可用的服务地址")
	}

	runMu.Lock()
	defer runMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	status, statusErr := InspectStatus(ctx)
	if statusErr != nil {
		return "", fmt.Errorf("互助论坛状态检查失败: %w", statusErr)
	}
	if status == nil || !status.Reachable {
		reason := "论坛健康检查未通过"
		if status != nil && status.Error != "" {
			reason = status.Error
		}
		return "", fmt.Errorf("互助论坛当前不可用：%s", reason)
	}

	systemPrompt, err := buildSystemPrompt(status)
	if err != nil {
		return "", err
	}

	messages := make([]chatMessage, 0, historyLimitMessages+8)
	messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	messages = append(messages, historySnapshot()...)
	messages = append(messages, chatMessage{Role: "user", Content: instruction})

	tools := forumTools()
	lastSummary := ""
	// 整轮只在 Run 开头/结尾各标记一次实况，多轮工具调用合并成同一轮；
	// 否则每个工具轮次都会留下一条只有标题、没有内容的空实况。
	observeBegin()
	defer observeEnd()
	for round := 0; round < maxToolRounds; round++ {
		response, callErr := chatWithTools(ctx, messages, tools, round)
		if callErr != nil {
			logger.Warn(roleLabel, "模型调用失败（第 %d 轮）: %v", round+1, callErr)
			return "", fmt.Errorf("互助论坛模型调用失败: %w", callErr)
		}

		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})

		if len(response.ToolCalls) == 0 {
			final := strings.TrimSpace(response.Content)
			if final == "" {
				if lastSummary != "" {
					final = "互助论坛已执行：" + strings.TrimSuffix(lastSummary, "；")
				} else {
					final = "互助论坛没有生成可用的答复，请换一种说法再试一次。"
				}
			}
			remember(instruction, final)
			return final, nil
		}

		for _, call := range response.ToolCalls {
			logger.Info(roleLabel, "[实况] 工具 %s（%s）", call.Function.Name, previewText(call.Function.Arguments, 200))
			args, parseErr := parseToolArgs(call.Function.Arguments)
			if parseErr != nil {
				outcome := toolError(fmt.Errorf("工具 %s 的参数不是合法 JSON: %w", call.Function.Name, parseErr))
				messages = append(messages, chatMessage{Role: "tool", ToolCallID: call.ID, Content: outcome.JSON})
				lastSummary += outcome.Summary + "；"
				continue
			}
			outcome := executeTool(ctx, call.Function.Name, args)
			messages = append(messages, chatMessage{Role: "tool", ToolCallID: call.ID, Content: outcome.JSON})
			lastSummary += outcome.Summary + "；"
		}
	}

	final := "互助论坛在限定轮次内没有完成这次请求。"
	if lastSummary != "" {
		final += "已执行的步骤：" + strings.TrimSuffix(lastSummary, "；") + "。请换一种更具体的说法再试。"
	}
	remember(instruction, final)
	logger.Warn(roleLabel, "工具轮次达到上限（%d），已停止继续调用", maxToolRounds)
	return final, nil
}

// parseToolArgs 解析模型给出的 JSON 参数；空串视为空对象。
func parseToolArgs(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

/* ---------------- 独立上下文 ---------------- */

// remember 记录一轮问答；超出上限时裁掉最早的消息。
func remember(instruction, answer string) {
	historyMu.Lock()
	defer historyMu.Unlock()
	history = append(history,
		chatMessage{Role: "user", Content: previewText(instruction, 3000)},
		chatMessage{Role: "assistant", Content: previewText(answer, 4000)},
	)
	if len(history) > historyLimitMessages {
		history = history[len(history)-historyLimitMessages:]
	}
}

// historySnapshot 返回历史上下文的副本。
func historySnapshot() []chatMessage {
	historyMu.Lock()
	defer historyMu.Unlock()
	if len(history) == 0 {
		return nil
	}
	out := make([]chatMessage, len(history))
	copy(out, history)
	return out
}

// ResetContext 清空独立上下文（测试或需要重新开始时调用）。
func ResetContext() {
	historyMu.Lock()
	history = nil
	historyMu.Unlock()
}
