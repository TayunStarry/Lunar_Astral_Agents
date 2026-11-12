package file

// loop.go 工具型智能体共用的工具推理循环。
// 它不是智能体本身：每个智能体包只声明自己的 AgentID、角色、提示词与工具，
// 再调用这里完成模型调用、工具回填、轮次上限与失败上报。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Run 执行一次工具推理循环。
func Run(ctx context.Context, req Request) (summary string, err error) {
	defer func() {
		if err != nil {
			agentbase.ReportRun(req.AgentID, err)
		}
	}()
	maxRounds := req.MaxRounds
	if maxRounds <= 0 {
		maxRounds = defaultMaxRounds
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	summaryTokens := req.SummaryMaxTokens
	if summaryTokens <= 0 {
		summaryTokens = defaultSummaryMaxTokens
	}
	base := agentbase.NewAgent(agentbase.AgentConfig{
		ID:                 req.AgentID,
		Label:              req.Role,
		MaxRounds:          maxRounds,
		MaxToolResultRunes: -1,
	})
	mutated := false
	afterTool := false
	buildStream := func(messages []agentbase.Message, tools []agentbase.ToolDefinition, budget int) agentbase.StreamRequest {
		if len(tools) == 0 {
			tools = nil
		}
		return agentbase.StreamRequest{
			Agent:       req.AgentID,
			Role:        req.Role,
			Timeout:     timeout,
			ReportTools: true,
			MaxTokens:   budget,
			Messages:    messages,
			Tools:       tools,
		}
	}
	result, runErr := base.RunToolLoop(ctx, agentbase.ToolLoopRequest{
		Messages: []agentbase.Message{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.Instruction},
		},
		Tools: req.Tools,
		Call: func(ctx context.Context, _ int, messages []agentbase.Message, tools []agentbase.ToolDefinition) (agentbase.Reply, error) {
			budget := maxTokens
			switch {
			case req.KeepFullBudget:
				// 故事创作需要读大纲后整章生成、写完再追加进度，不压缩输出额度。
			case mutated:
				// 修改完成后只需要一句话总结；防止模型继续复述或续写正文。
				budget = summaryTokens
			case afterTool:
				// 工具刚执行过：限制输出，避免工具报错后模型无限解释或重写正文。
				budget = defaultPostToolMaxTokens
			}
			reply, callErr := base.Stream(ctx, buildStream(messages, tools, budget))
			if callErr != nil && isToolArgsParseError(callErr) {
				// 工具参数过长被截断时，llama-server 会直接把不完整的 JSON 判为 500。
				// 换用更大的输出额度重来一次，让整段参数有机会完整生成。
				retryBudget := budget * 2
				if retryBudget > maxRetryTokens {
					retryBudget = maxRetryTokens
				}
				if retryBudget > budget {
					budget = retryBudget
					reply, callErr = base.Stream(ctx, buildStream(messages, tools, budget))
				}
			}
			if callErr != nil {
				return reply, callErr
			}
			// 输出额度不足导致工具参数被截断时，换用更大的额度重来一次：
			// - 总结阶段被截断：直接恢复完整额度，允许模型发起新的修改；
			// - 首次生成被截断：按倍数放大，避免一整章正文被截在半途。
			if strings.EqualFold(strings.TrimSpace(reply.FinishReason), "length") &&
				hasIncompleteToolCalls(reply.ToolCalls) {
				retryBudget := maxTokens
				if budget == maxTokens {
					retryBudget = budget * 2
				}
				if retryBudget > maxRetryTokens {
					retryBudget = maxRetryTokens
				}
				if retryBudget > budget {
					return base.Stream(ctx, buildStream(messages, tools, retryBudget))
				}
			}
			return reply, nil
		},
		Execute: func(_ context.Context, call agentbase.ToolCall) (string, bool, error) {
			args, parseErr := agentbase.ParseToolArgs(call.Function.Arguments)
			if parseErr != nil {
				return toolJSON(map[string]any{"error": "参数不是合法 JSON"}), false, nil
			}
			outcome, done, execErr := req.Handler(call.Function.Name, args)
			if execErr != nil {
				return toolJSON(map[string]any{"error": execErr.Error()}), false, nil
			}
			afterTool = true
			if outcome.Mutated {
				mutated = true
			}
			content := strings.TrimSpace(outcome.JSON)
			if content == "" {
				content = toolJSON(map[string]any{"summary": outcome.Summary})
			}
			return agentbase.TruncateRunes(content, maxToolResultRunes), done, nil
		},
		DisableReport: true,
	})
	if runErr != nil {
		return "", runErr
	}
	return strings.TrimSpace(result.Reply.Text), nil
}

// hasIncompleteToolCalls 判断回复是否包含被截断的工具调用。
// 输出额度不足时，工具参数往往不是合法 JSON；此时应换用完整额度重试。
func hasIncompleteToolCalls(calls []agentbase.ToolCall) bool {
	for _, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			return true
		}
		if _, err := agentbase.ParseToolArgs(call.Function.Arguments); err != nil {
			return true
		}
	}
	return false
}

// isToolArgsParseError 识别 llama-server 因工具参数 JSON 不完整而返回的错误。
func isToolArgsParseError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "Failed to parse tool call arguments") ||
		strings.Contains(text, "parse tool call arguments")
}

// toolJSON 把值序列化为 JSON 文本。
func toolJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(raw)
}
