package DocumentReader

// loop.go 文献解析的工具循环：直读流程与扫描归并共用同一套回填逻辑。
//
// 首次调用失败时退化为无工具回答（部分服务不支持 function calling），
// 其余轮次失败直接返回错误。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/agentchat"
)

// runToolLoop 执行最多 maxRounds 轮「模型 → 工具 → 模型」循环
func runToolLoop(doc *docContext, messages []agentchat.Message, tools []agentchat.ToolDefinition, maxRounds int) (string, error) {
	last := ""
	for round := 0; round < maxRounds; round++ {
		message, chatErr := agentchat.ChatStream(roleLabel, messages, tools)
		if chatErr != nil && round == 0 {
			logger.Warn("CrystalAstral", "文献解析：首轮失败，退化为无工具回答: %v", chatErr)
			fallback, fallbackErr := agentchat.ChatStream(roleLabel, firstTwoMessages(messages), nil)
			if fallbackErr != nil {
				return "", fallbackErr
			}
			if text := strings.TrimSpace(agentchat.StringOf(fallback.Content)); text != "" {
				return text, nil
			}
			return "", chatErr
		}
		if chatErr != nil {
			return "", chatErr
		}

		text := strings.TrimSpace(agentchat.StringOf(message.Content))
		if text != "" {
			last = text
		}
		// 回填 assistant 消息时统一成字符串 content，避免部分服务拒绝 null
		message.Content = text
		if len(message.ToolCalls) == 0 {
			if last == "" {
				return "", fmt.Errorf("文献解析没有返回内容")
			}
			return last, nil
		}

		// assistant 的 tool_calls 必须原样回填，再逐条给出配对的结果
		messages = append(messages, message)
		for _, call := range message.ToolCalls {
			messages = append(messages, agentchat.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    executeFileTool(doc, call),
			})
		}
	}
	if last != "" {
		return last, nil
	}
	return "", fmt.Errorf("文献解析探查轮次超限")
}

// firstTwoMessages 取系统消息与首条用户消息，用于无工具兜底
func firstTwoMessages(messages []agentchat.Message) []agentchat.Message {
	out := make([]agentchat.Message, 0, 2)
	for _, message := range messages {
		if len(out) >= 2 {
			break
		}
		if message.Role == "tool" || len(message.ToolCalls) > 0 {
			continue
		}
		out = append(out, message)
	}
	return out
}
