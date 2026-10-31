//go:build windows

package desktop

// ==== 流式响应解析 ====
// 边收边把思考/正文送入思考监管（loopguard）与 TUI「推理实况」，
// 并在流末聚合工具调用分片。

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

// readAutoStream 解析 OpenAI 兼容 SSE，返回聚合后的助手消息与 token 用量。
// 命中重复输出时 guard 会取消请求，读取随连接断开而终止。
func readAutoStream(body io.Reader, guard *loopguard.Guard, role string) (chatMessage, *ltpUsage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var content, reasoning strings.Builder
	var usage *ltpUsage
	calls := map[int]*ltpToolCallAccum{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk ltpChatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return chatMessage{}, nil, fmt.Errorf("模型错误: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.ReasoningContent != "" {
				reasoning.WriteString(choice.Delta.ReasoningContent)
				guard.Feed(loopguard.KindThink, choice.Delta.ReasoningContent)
				observeDelta(role, streamKindThink, choice.Delta.ReasoningContent)
			}
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				guard.Feed(loopguard.KindContent, choice.Delta.Content)
				observeDelta(role, streamKindContent, choice.Delta.Content)
			}
			for _, tc := range choice.Delta.ToolCalls {
				accum := calls[tc.Index]
				if accum == nil {
					accum = &ltpToolCallAccum{}
					calls[tc.Index] = accum
				}
				if tc.ID != "" {
					accum.ID = tc.ID
				}
				if tc.Type != "" {
					accum.Type = tc.Type
				}
				if tc.Function.Name != "" {
					accum.Name = tc.Function.Name
				}
				accum.Arguments.WriteString(tc.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return chatMessage{}, nil, fmt.Errorf("读取模型流失败: %w", err)
	}

	text := strings.TrimSpace(content.String())
	toolCalls := assembleToolCalls(calls)
	if text == "" && len(toolCalls) == 0 {
		return chatMessage{}, nil, fmt.Errorf("模型返回空响应")
	}
	return chatMessage{Role: "assistant", Content: text, ToolCalls: toolCalls}, usage, nil
}

// assembleToolCalls 按分片索引顺序拼装完整工具调用。
func assembleToolCalls(calls map[int]*ltpToolCallAccum) []ltpToolCall {
	if len(calls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	out := make([]ltpToolCall, 0, len(indexes))
	for _, index := range indexes {
		accum := calls[index]
		if accum.Name == "" {
			continue
		}
		out = append(out, ltpToolCall{
			ID:   accum.ID,
			Type: stringOr(accum.Type, "function"),
			Function: ltpToolFunc{
				Name:      accum.Name,
				Arguments: accum.Arguments.String(),
			},
		})
	}
	return out
}
