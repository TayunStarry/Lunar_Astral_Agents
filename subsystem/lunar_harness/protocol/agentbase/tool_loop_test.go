package agentbase

// tool_loop_test.go 通用工具循环的纯编排测试（不依赖模型服务）。

import (
	"context"
	"errors"
	"testing"
)

// fakeCall 按轮次返回预设回复；超出预设时复用最后一条。
func fakeCall(replies ...Reply) func(context.Context, int, []Message, []ToolDefinition) (Reply, error) {
	return func(_ context.Context, round int, _ []Message, _ []ToolDefinition) (Reply, error) {
		index := round - 1
		if index >= len(replies) {
			index = len(replies) - 1
		}
		return replies[index], nil
	}
}

// toolReply 构造一条带工具调用的助手回复。
func toolReply(text, name string) Reply {
	call := ToolCall{ID: "call_1", Type: "function", Function: ToolFunction{Name: name, Arguments: `{}`}}
	return Reply{
		Message:   Message{Role: "assistant", Content: text, ToolCalls: []ToolCall{call}},
		Text:      text,
		ToolCalls: []ToolCall{call},
	}
}

// TestRunToolLoopExecutesAndReturns 工具结果必须回填，下一轮无工具调用时结束。
func TestRunToolLoopExecutesAndReturns(t *testing.T) {
	agent := NewAgent(AgentConfig{Label: "测试", MaxRounds: 3, MaxToolResultRunes: -1})
	var calls []string
	result, err := agent.RunToolLoop(context.Background(), ToolLoopRequest{
		Messages: []Message{{Role: "user", Content: "开始"}},
		Call:     fakeCall(toolReply("调用工具", "ping"), Reply{Text: "完成"}),
		Execute: func(_ context.Context, call ToolCall) (string, bool, error) {
			calls = append(calls, call.Function.Name)
			return "工具结果", false, nil
		},
	})
	if err != nil {
		t.Fatalf("工具循环失败: %v", err)
	}
	if result.Rounds != 2 || result.ToolCalls != 1 {
		t.Fatalf("轮次/工具次数错误: %#v", result)
	}
	if result.Reply.Text != "完成" {
		t.Fatalf("最终答复错误: %q", result.Reply.Text)
	}
	if len(calls) != 1 || calls[0] != "ping" {
		t.Fatalf("工具未执行: %#v", calls)
	}
	toolMessage := result.Messages[len(result.Messages)-2]
	if toolMessage.Role != "tool" || toolMessage.ToolCallID != "call_1" || toolMessage.Content != "工具结果" {
		t.Fatalf("工具回填错误: %#v", toolMessage)
	}
}

// TestRunToolLoopRepeatedCallsNotInterrupted 相同工具调用不再按签名提前拦截，
// 只在轮次额度耗尽时结束。
func TestRunToolLoopRepeatedCallsNotInterrupted(t *testing.T) {
	agent := NewAgent(AgentConfig{Label: "测试", MaxRounds: 3, MaxToolResultRunes: -1})
	executions := 0
	_, err := agent.RunToolLoop(context.Background(), ToolLoopRequest{
		Call:    fakeCall(toolReply("重复", "ping")),
		Execute: func(context.Context, ToolCall) (string, bool, error) { executions++; return "结果", false, nil },
	})
	if !errors.Is(err, ErrMaxRounds) {
		t.Fatalf("应只在轮次额度耗尽时结束，实际: %v", err)
	}
	if executions != 3 {
		t.Fatalf("重复调用应执行满 3 轮，实得 %d 轮", executions)
	}
}

// TestRunToolLoopMaxRounds 轮次耗尽必须返回 ErrMaxRounds。
func TestRunToolLoopMaxRounds(t *testing.T) {
	agent := NewAgent(AgentConfig{MaxRounds: 2, MaxToolResultRunes: -1})
	_, err := agent.RunToolLoop(context.Background(), ToolLoopRequest{
		Call:      fakeCall(toolReply("继续", "ping")),
		Execute:   func(context.Context, ToolCall) (string, bool, error) { return "结果", false, nil },
		MaxRounds: 2,
	})
	if !errors.Is(err, ErrMaxRounds) {
		t.Fatalf("应返回轮次上限，实际: %v", err)
	}
}

// TestRunToolLoopTruncatesToolResult 工具结果按配置截断。
func TestRunToolLoopTruncatesToolResult(t *testing.T) {
	agent := NewAgent(AgentConfig{MaxRounds: 2, MaxToolResultRunes: 3})
	result, err := agent.RunToolLoop(context.Background(), ToolLoopRequest{
		Call:    fakeCall(toolReply("调用", "ping"), Reply{Text: "完成"}),
		Execute: func(context.Context, ToolCall) (string, bool, error) { return "一二三四五", false, nil },
	})
	if err != nil {
		t.Fatalf("工具循环失败: %v", err)
	}
	toolMessage := result.Messages[len(result.Messages)-2]
	if got := toolMessage.Content; got != "一二三…" {
		t.Fatalf("工具结果截断错误: %q", got)
	}
}

// TestRunToolLoopDoneStops 工具返回 done 时立即结束并保留本轮助手正文。
func TestRunToolLoopDoneStops(t *testing.T) {
	agent := NewAgent(AgentConfig{MaxRounds: 5, MaxToolResultRunes: -1})
	result, err := agent.RunToolLoop(context.Background(), ToolLoopRequest{
		Call:      fakeCall(toolReply("准备提交", "submit")),
		Execute:   func(context.Context, ToolCall) (string, bool, error) { return "已提交", true, nil },
		MaxRounds: 5,
	})
	if err != nil {
		t.Fatalf("工具循环失败: %v", err)
	}
	if result.Rounds != 1 || result.Reply.Text != "准备提交" {
		t.Fatalf("done 语义错误: %#v", result)
	}
}

// TestRunToolLoopContextCanceled 取消上下文必须中止循环。
func TestRunToolLoopContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	agent := NewAgent(AgentConfig{})
	_, err := agent.RunToolLoop(ctx, ToolLoopRequest{
		Call:    fakeCall(Reply{Text: "完成"}),
		Execute: func(context.Context, ToolCall) (string, bool, error) { return "", false, nil },
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回取消错误，实际: %v", err)
	}
}
