package RuntimeChat

// agent.go 插件运行时通用推理智能体：承接脚本 agent.chat 的模型调用。
// 模型调用与推理实况统一走 protocol/agentbase。

import (
	"context"
	"encoding/json"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// AgentID 故障归属 ID
const AgentID = "runtime_chat"

// callTimeout 单次模型调用超时
const callTimeout = 60 * time.Second

// Chat 执行一次通用推理，返回正文与工具调用。
// messages 为 OpenAI 兼容消息数组；opts 支持 system / temperature / max_tokens / tools。
func Chat(messages []any, opts map[string]any) (text string, toolCalls []any, err error) {
	defer func() { agentbase.ReportRun(AgentID, err) }()

	payload := messages
	if sys, ok := opts["system"].(string); ok && sys != "" {
		prepended := make([]any, 0, len(messages)+1)
		prepended = append(prepended, map[string]any{"role": "system", "content": sys})
		prepended = append(prepended, messages...)
		payload = prepended
	}

	req := agentbase.StreamRequest{
		Agent:       AgentID,
		Role:        "插件对话",
		Messages:    payload,
		Timeout:     callTimeout,
		ReportTools: true,
	}
	if temp, ok := opts["temperature"].(float64); ok {
		req.Temperature = &temp
	}
	if maxTokens, ok := opts["max_tokens"].(int64); ok {
		req.MaxTokens = int(maxTokens)
	}
	if tools, ok := opts["tools"].([]any); ok && len(tools) > 0 {
		req.Tools = tools
	}

	reply, err := agentbase.Stream(context.Background(), req)
	if err != nil {
		return "", nil, err
	}
	if len(reply.ToolCalls) == 0 {
		return reply.Text, nil, nil
	}
	raw, merr := json.Marshal(reply.ToolCalls)
	if merr != nil {
		return reply.Text, nil, merr
	}
	if uerr := json.Unmarshal(raw, &toolCalls); uerr != nil {
		return reply.Text, nil, uerr
	}
	return reply.Text, toolCalls, nil
}
