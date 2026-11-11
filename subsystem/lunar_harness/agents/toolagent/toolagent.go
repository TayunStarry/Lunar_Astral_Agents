package toolagent

// toolagent 为工具型智能体提供统一的工具推理循环。
// 它不是智能体本身：每个智能体包只声明自己的 AgentID、角色、提示词与工具，
// 再调用这里完成模型调用、工具回填、轮次上限与失败上报。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Outcome 工具执行结果：JSON 回填 + 给最终总结用的短句 + 本次涉及的资源句柄 + 审核证据。
type Outcome struct {
	JSON      string
	Summary   string
	Reference string
	Handles   []string
	// Evidence 供审核智能体核对的有界变更证据（不包含整份文件）
	Evidence string
	// Mutated 本次工具调用是否成功写入
	Mutated bool
}

// Handler 工具分发；done 为 true 时结束当前推理循环。
type Handler func(name string, args map[string]any) (Outcome, bool, error)

// Request 一次子链路智能体推理循环的输入。
type Request struct {
	// AgentID 推理实况、数据导出与失败上报使用的智能体标识。
	AgentID string
	// Role 推理实况中显示的角色名。
	Role string
	// SystemPrompt 本轮系统提示。
	SystemPrompt string
	// Instruction 用户指令或任务描述。
	Instruction string
	// Tools 本轮可用工具。
	Tools []agentbase.ToolDefinition
	// Handler 工具执行器，由所属链路注入。
	Handler Handler
	// MaxRounds 工具循环轮次上限；<=0 时使用默认值。
	MaxRounds int
	// Timeout 单次模型调用超时；<=0 时使用默认值。
	Timeout time.Duration
	// MaxTokens 单次模型调用的输出上限；<=0 时使用默认值。
	// 用于阻止模型在任务完成后继续长篇生成，避免长时间卡住。
	MaxTokens int
	// SummaryMaxTokens 已发生修改后的下一轮输出上限；<=0 时使用默认值。
	// 修改完成后模型只需一句话总结，不应再复述或续写正文。
	SummaryMaxTokens int
}

const (
	// defaultMaxRounds 默认工具循环轮次上限
	defaultMaxRounds = 24
	// defaultTimeout 默认单次模型调用超时
	defaultTimeout = 90 * time.Second
	// defaultMaxTokens 默认单次输出上限（足以生成一整章正文）
	defaultMaxTokens = 8192
	// defaultSummaryMaxTokens 修改完成后的总结输出上限
	defaultSummaryMaxTokens = 320
	// defaultPostToolMaxTokens 工具执行后、尚未发生修改时的输出上限。
	// 足够发起一次带正文的工具调用，又能阻止模型在工具报错后无限生成。
	defaultPostToolMaxTokens = 2048
	// maxRetryTokens 工具参数被截断后允许放大到的输出上限
	maxRetryTokens = 16384
	// maxToolResultRunes 单条工具结果回填模型的字符上限
	maxToolResultRunes = 6000
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
			case mutated:
				// 修改完成后只需要一句话总结；防止模型继续复述或续写正文。
				budget = summaryTokens
			case afterTool:
				// 工具刚执行过：限制输出，避免工具报错后模型无限解释或重写正文。
				budget = defaultPostToolMaxTokens
			}
			reply, callErr := base.Stream(ctx, buildStream(messages, tools, budget))
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

// toolJSON 把值序列化为 JSON 文本。
func toolJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(raw)
}
