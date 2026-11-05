package thinking

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

// repairRecord 一次智能修复上报的快照。
type repairRecord struct {
	agent string
	err   error
}

// useDialogueModel 替换单轮模型调用入口，测试结束恢复。
func useDialogueModel(t *testing.T, fn ModelRunFunc) {
	t.Helper()
	previous := dialogueModelRun
	dialogueModelRun = fn
	t.Cleanup(func() { dialogueModelRun = previous })
}

// useTestTool 注册一次性测试工具，测试结束移除。
func useTestTool(t *testing.T, name string, handler ToolHandler) {
	t.Helper()
	toolreg.RegisterExecutor(name, func(_ context.Context, args map[string]any) (any, error) {
		return handler(args), nil
	})
	t.Cleanup(func() {
		toolreg.UnregisterExecutor(name)
	})
}

// captureReports 捕获智能修复上报（同步登记，测试内可直接读取）。
func captureReports(t *testing.T) func() []repairRecord {
	t.Helper()
	var mu sync.Mutex
	records := make([]repairRecord, 0, 4)
	agentbase.SetRepairHook(func(agent string, err error) {
		mu.Lock()
		records = append(records, repairRecord{agent: agent, err: err})
		mu.Unlock()
	})
	t.Cleanup(func() { agentbase.SetRepairHook(nil) })
	return func() []repairRecord {
		mu.Lock()
		defer mu.Unlock()
		return append([]repairRecord{}, records...)
	}
}

// resetDialogueGlobals 隔离本轮应答涉及的全局状态。
func resetDialogueGlobals(t *testing.T) {
	t.Helper()
	stateMu.Lock()
	lastFailure = ""
	finalResponse = ""
	fastAckSent = false
	stateMu.Unlock()
	unreadMu.Lock()
	unreadContext = nil
	unreadRecords = nil
	unreadMu.Unlock()
}

// newLoopTestRole 构造只用于工具循环的对话者。
func newLoopTestRole() *DialogueRole {
	role := &DialogueRole{
		ModelBuilder: &ModelBuilder{
			RoleTag:           "对话者",
			ReasoningStrength: "low",
			EnableTools:       true,
			SystemPrompt:      "系统提示",
			ContextLimit:      20,
		},
		ImageWindow: imageWindowSize,
	}
	return role
}

// stubResponse 构造一次模型响应。
func stubResponse(content string, calls ...ToolCallItem) *ModelResponse {
	return &ModelResponse{Body: ModelResponseBody{Choices: []ModelChoice{{
		Message: AssistantMessage{Role: RoleAssistant, Content: content, ToolCalls: calls},
	}}}}
}

// stubToolCall 构造一次工具调用。
func stubToolCall(id, name, arguments string) ToolCallItem {
	return ToolCallItem{
		ID:       id,
		Type:     "function",
		Function: ToolCallFunction{Name: name, Arguments: arguments},
	}
}

// finalText 读取终稿。
func finalText() string {
	stateMu.Lock()
	defer stateMu.Unlock()
	return finalResponse
}

// failureText 读取本轮失败原因。
func failureText() string {
	stateMu.Lock()
	defer stateMu.Unlock()
	return lastFailure
}

// countMessages 统计满足条件的上下文消息数量。
func countMessages(role *DialogueRole, roleName, needle string) int {
	count := 0
	for _, message := range role.Messages {
		if message.Role != roleName {
			continue
		}
		if text, ok := message.Content.(string); ok && strings.Contains(text, needle) {
			count++
		}
	}
	return count
}

// TestProcessBlocksPushedWithoutClaimingTurn 思考区/代码块 auto=true、不朗读：
// 它们只用于前端展示，不能消费 QQ 的 @ / 引用，也不能提前收尾。
func TestProcessBlocksPushedWithoutClaimingTurn(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	previous := PushMessageFunc
	records := make([]PushContextData, 0, 2)
	PushMessageFunc = func(msgType string, data any) {
		if msgType != "context" {
			return
		}
		if ctx, ok := data.(PushContextData); ok {
			records = append(records, ctx)
		}
	}
	t.Cleanup(func() {
		PushMessageFunc = previous
		config.LocalDir = oldDir
	})

	pushProcessBlocks([]string{"先想一想"}, []string{"```go\nfmt.Println(1)\n```"})

	if len(records) != 2 {
		t.Fatalf("思考区与代码块应各推送 1 次，实际 %d 次", len(records))
	}
	for i, ctx := range records {
		if ctx.Type != "text" {
			t.Fatalf("第 %d 条过程内容类型不符: %q", i+1, ctx.Type)
		}
		if ctx.Audio != "" {
			t.Fatalf("第 %d 条过程内容不应合成语音", i+1)
		}
		if !ctx.Auto {
			t.Fatalf("第 %d 条过程内容不是终稿，应标记 auto=true", i+1)
		}
	}
}

// useSpeechSynth 替换语音合成，测试结束恢复。
func useSpeechSynth(t *testing.T, fn func(string) string) {
	t.Helper()
	previous := synthesizeSpeech
	synthesizeSpeech = fn
	t.Cleanup(func() { synthesizeSpeech = previous })
}

// captureAssistantText 替换模型正文推送入口，记录每次推送，测试结束恢复。
func captureAssistantText(t *testing.T) func() []fastAckPushRecord {
	t.Helper()
	previous := assistantTextPushHook
	var mu sync.Mutex
	records := make([]fastAckPushRecord, 0, 4)
	assistantTextPushHook = func(kind, content, audio string, auto bool) bool {
		mu.Lock()
		records = append(records, fastAckPushRecord{kind: kind, content: content, audio: audio, auto: auto})
		mu.Unlock()
		return true
	}
	t.Cleanup(func() { assistantTextPushHook = previous })
	return func() []fastAckPushRecord {
		mu.Lock()
		defer mu.Unlock()
		return append([]fastAckPushRecord{}, records...)
	}
}

// TestToolRoundTextPushedAsNormalMessage 模型在工具调用轮次产出的正文按正常对话消息推送：
// 携带 TTS 音频、auto=false，且不会走快速应答入口。
func TestToolRoundTextPushedAsNormalMessage(t *testing.T) {
	resetDialogueGlobals(t)
	role := newLoopTestRole()
	pushes := captureAssistantText(t)
	fastPushes := captureFastAck(t)
	useSpeechSynth(t, func(text string) string { return "audio:" + text })

	var executions int32
	useTestTool(t, "dialogue_test_lookup", func(map[string]any) ToolResult {
		atomic.AddInt32(&executions, 1)
		return ToolResult{Text: "查询结果"}
	})

	modelCalls := 0
	useDialogueModel(t, func(_ *ModelBuilder, _ []PostMessage, _ []ToolCall) (*ModelResponse, error) {
		modelCalls++
		if modelCalls == 1 {
			return stubResponse("我先查一下", stubToolCall("lookup-1", "dialogue_test_lookup", `{"q":"星月"}`)), nil
		}
		return stubResponse("查完了"), nil
	})

	role.runToolLoop(&ChatCache{})

	records := pushes()
	if len(records) != 1 {
		t.Fatalf("模型正文应推送 1 次，实际 %d 次", len(records))
	}
	if got := records[0]; got.kind != "text" || got.content != "我先查一下" {
		t.Fatalf("模型正文推送内容不符，实得 kind=%q content=%q", got.kind, got.content)
	}
	if records[0].audio == "" {
		t.Fatal("模型正文应走 TTS，音频不应为空")
	}
	// auto 只表达「本轮尚未收尾、不占用 QQ 应答份额与 @ 回执」；
	// 它必须为 true，否则中间正文会提前释放应答份额，终稿可能被投到下一个请求的会话。
	if !records[0].auto {
		t.Fatal("模型工具轮次正文不应消费本轮应答份额（auto 应为 true）")
	}
	if len(fastPushes()) != 0 {
		t.Fatal("非复合工具不应触发快速应答")
	}
	if got := atomic.LoadInt32(&executions); got != 1 {
		t.Fatalf("工具应执行 1 次，实得 %d 次", got)
	}
}

// 轮次上限：允许 5 轮真实执行，第 6 轮拒绝并回填未执行原因，随后不带工具收尾。
func TestToolLoopStopsAtRoundLimit(t *testing.T) {
	resetDialogueGlobals(t)
	role := newLoopTestRole()

	var executions int32
	useTestTool(t, "dialogue_test_ping", func(map[string]any) ToolResult {
		atomic.AddInt32(&executions, 1)
		return ToolResult{Text: "pong"}
	})

	modelCalls := 0
	useDialogueModel(t, func(_ *ModelBuilder, _ []PostMessage, tools []ToolCall) (*ModelResponse, error) {
		modelCalls++
		if len(tools) == 0 {
			return stubResponse("收尾完成"), nil
		}
		// 参数随轮次变化，避免命中重复调用检测。
		return stubResponse("", stubToolCall(fmt.Sprintf("call-%d", modelCalls), "dialogue_test_ping", fmt.Sprintf(`{"n":%d}`, modelCalls))), nil
	})

	role.runToolLoop(&ChatCache{})

	if got := atomic.LoadInt32(&executions); got != maxToolRounds {
		t.Fatalf("工具应执行 %d 轮，实得 %d 轮", maxToolRounds, got)
	}
	if modelCalls != maxToolRounds+2 {
		t.Fatalf("模型应调用 %d 次（5 轮执行 + 1 轮拒绝 + 1 轮收尾），实得 %d 次", maxToolRounds+2, modelCalls)
	}
	if got := finalText(); got != "收尾完成" {
		t.Fatalf("轮次上限后应使用收尾终稿，实得 %q", got)
	}
	if got := countMessages(role, RoleTool, "本轮工具调用已达上限"); got != 1 {
		t.Fatalf("应回填 1 条未执行原因，实得 %d 条", got)
	}
	if got := failureText(); got != "" {
		t.Fatalf("轮次上限属于正常收尾，不应记为失败: %q", got)
	}
}

// 重复调用：同一响应签名第 3 次出现即拦截，并向上报。
func TestToolLoopBlocksRepeatedToolCall(t *testing.T) {
	resetDialogueGlobals(t)
	role := newLoopTestRole()
	reports := captureReports(t)

	var executions int32
	useTestTool(t, "dialogue_test_repeat", func(map[string]any) ToolResult {
		atomic.AddInt32(&executions, 1)
		return ToolResult{Text: "ok"}
	})
	useDialogueModel(t, func(_ *ModelBuilder, _ []PostMessage, _ []ToolCall) (*ModelResponse, error) {
		return stubResponse("", stubToolCall("same-call", "dialogue_test_repeat", `{"q":"同样的查询"}`)), nil
	})

	role.runToolLoop(&ChatCache{})

	if got := atomic.LoadInt32(&executions); got != dialogueMaxRepeats-1 {
		t.Fatalf("重复签名前应执行 %d 轮，实得 %d 轮", dialogueMaxRepeats-1, got)
	}
	failure := failureText()
	if !strings.Contains(failure, "工具循环重复") {
		t.Fatalf("重复调用应记为循环失败，实得 %q", failure)
	}
	covered := false
	for _, record := range reports() {
		if record.agent == dialogueAgentID && strings.Contains(record.err.Error(), "重复") {
			covered = true
		}
	}
	if !covered {
		t.Fatal("循环重复应上报给拯救者")
	}
}

// 工具执行失败：未注册工具不中断本轮但需上报；工具 panic 中断本轮并上报。
func TestToolExecutionFailureReportsToSavior(t *testing.T) {
	t.Run("未注册工具", func(t *testing.T) {
		resetDialogueGlobals(t)
		role := newLoopTestRole()
		reports := captureReports(t)
		useDialogueModel(t, func(_ *ModelBuilder, _ []PostMessage, _ []ToolCall) (*ModelResponse, error) {
			return stubResponse("我来查一下", stubToolCall("missing-call", "dialogue_test_missing", `{}`)), nil
		})

		role.runToolLoop(&ChatCache{})

		if got := finalText(); got != "我来查一下" {
			t.Fatalf("全部工具缺失时应原样回填本轮正文，实得 %q", got)
		}
		if got := countMessages(role, RoleTool, "未找到工具包"); got != 1 {
			t.Fatalf("应回填 1 条未找到工具包提示，实得 %d 条", got)
		}
		covered := false
		for _, record := range reports() {
			if record.agent == dialogueAgentID && strings.Contains(record.err.Error(), "dialogue_test_missing") {
				covered = true
			}
		}
		if !covered {
			t.Fatalf("缺失工具应上报给拯救者，实得 %#v", reports())
		}
	})

	t.Run("工具 panic", func(t *testing.T) {
		resetDialogueGlobals(t)
		role := newLoopTestRole()
		reports := captureReports(t)
		useTestTool(t, "dialogue_test_panic", func(map[string]any) ToolResult {
			panic("boom")
		})
		useDialogueModel(t, func(_ *ModelBuilder, _ []PostMessage, _ []ToolCall) (*ModelResponse, error) {
			return stubResponse("", stubToolCall("panic-call", "dialogue_test_panic", `{}`)), nil
		})

		role.runToolLoop(&ChatCache{})

		failure := failureText()
		if !strings.Contains(failure, "工具执行失败") || !strings.Contains(failure, "dialogue_test_panic") {
			t.Fatalf("工具 panic 应记为工具执行失败，实得 %q", failure)
		}
		covered := false
		for _, record := range reports() {
			if record.agent == dialogueAgentID && strings.Contains(record.err.Error(), "工具执行失败") {
				covered = true
			}
		}
		if !covered {
			t.Fatalf("工具执行失败应上报给拯救者，实得 %#v", reports())
		}
	})
}
