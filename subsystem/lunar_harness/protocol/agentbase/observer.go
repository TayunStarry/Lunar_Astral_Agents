package agentbase

// observer.go 模型调用的实况推送与计量回调。
//
// 各智能体传入自己的角色标签（如「视觉理解」「文献解析」），控制台据此在同一块
// 实况屏里区分来源；计量回调让 Go 侧智能体的模型调用一并进入「运行状态」统计。

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/protocol/loopguard"
)

// Observer 实况观察者与模型计量回调
type Observer struct {
	// Begin 实况开始
	Begin func(role string)
	// Delta 实况增量（kind: think/content）
	Delta func(role, kind, text string)
	// End 实况结束
	End func(role string)
	// Tool 工具调用登记（名称 + 原始参数串），由装配层桥接到控制台实况。
	Tool func(role, name, args string)
	// Meter 模型调用计量
	Meter func(prompt, completion, cached int64, generation time.Duration, estimated bool)
	// Report 重试耗尽后的失败上报（交给智能修复；role 为实况角色标签）
	Report func(role string, err error)
}

// SetObserver 注入实况观察者与计量回调（app 层启动时调用）
func SetObserver(o Observer) {
	observerMu.Lock()
	defer observerMu.Unlock()
	observer = o
}

// observeBegin 通知实况开始
func observeBegin(role string) {
	observerMu.Lock()
	fn := observer.Begin
	observerMu.Unlock()
	if fn != nil {
		fn(role)
	}
}

// observeDelta 通知实况增量（kind: think/content）
func observeDelta(role, kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	observerMu.Lock()
	fn := observer.Delta
	observerMu.Unlock()
	if fn != nil {
		fn(role, kind, text)
	}
}

// observeTool 通知一次工具调用（未注入 Tool 回调时静默）。
func observeTool(role, name, args string) {
	observerMu.Lock()
	fn := observer.Tool
	observerMu.Unlock()
	if fn != nil {
		fn(role, name, args)
	}
}

// observeEnd 通知实况结束
func observeEnd(role string) {
	observerMu.Lock()
	fn := observer.End
	observerMu.Unlock()
	if fn != nil {
		fn(role)
	}
}

// SetRepairHook 注入智能修复上报回调（由装配层接到 Savior.ReportRun）。
func SetRepairHook(fn func(agent string, err error)) {
	repairMu.Lock()
	defer repairMu.Unlock()
	repairHook = fn
}

// SetRepairDetailHook 注入结构化故障上报回调（由装配层接到 Savior.ReportDetail）。
// 注入后优先于 SetRepairHook 注入的旧回调。
func SetRepairDetailHook(fn func(detail FailureDetail)) {
	repairMu.Lock()
	defer repairMu.Unlock()
	repairDetailHook = fn
}

// Report 上报一次故障；同一智能体的故障由修复层去重。
func Report(agent string, err error) {
	ReportDetail(FailureDetail{Agent: agent, Err: err})
}

// ReportRun 上报一次运行失败，忽略主动取消与循环输出截断。
func ReportRun(agent string, err error) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, loopguard.ErrLoop) {
		return
	}
	ReportDetail(FailureDetail{Agent: agent, Err: err})
}

// ReportDetail 上报一次结构化故障。
//
// 主动取消不进入修复队列；循环输出默认视为已降级，只有显式标记为
// tool_loop / context 时才作为故障证据保留。detail.Err 与 detail.ErrorText
// 都没有时按「未知错误」降级。
func ReportDetail(detail FailureDetail) {
	if detail.Err != nil {
		if errors.Is(detail.Err, context.Canceled) {
			return
		}
		// 循环输出本身可能是「工具循环」故障的证据；显式标记为 tool_loop / context 时保留，
		// 其余旧上报路径仍按原语义忽略。
		if errors.Is(detail.Err, loopguard.ErrLoop) &&
			detail.Kind != FailureKindToolLoop && detail.Kind != FailureKindContext {
			return
		}
		if strings.TrimSpace(detail.ErrorText) == "" {
			detail.ErrorText = detail.Err.Error()
		}
	}
	detail.Agent = strings.TrimSpace(detail.Agent)
	if detail.Agent == "" {
		detail.Agent = "unknown"
	}
	detail.ErrorText = strings.TrimSpace(detail.ErrorText)
	if detail.ErrorText == "" {
		detail.ErrorText = "未知错误"
	}
	if strings.TrimSpace(detail.Kind) == "" {
		detail.Kind = FailureKindUnknown
	}
	repairMu.Lock()
	detailHook := repairDetailHook
	hook := repairHook
	repairMu.Unlock()
	if detailHook != nil {
		detailHook(detail)
		return
	}
	if hook != nil {
		hook(detail.Agent, errors.New(detail.ErrorText))
		return
	}
	// 兼容更早的注入方式：没有修复回调时退回观察者的 Report
	observerMu.Lock()
	fn := observer.Report
	observerMu.Unlock()
	if fn != nil {
		fn(detail.Agent, errors.New(detail.ErrorText))
	}
}

// reportFailure 上报一次重试耗尽后的失败（由 app 层接到智能修复）。
func reportFailure(role string, err error) {
	if err == nil {
		return
	}
	Report(role, err)
}

// reportModelCall 把一次调用计入「运行状态」的模型计量：
// 优先用服务端回传的真实 usage，缺失时按请求体与输出文本长度估算（标记 estimated）。
func reportModelCall(requestBody []byte, reply Reply, elapsed time.Duration) {
	observerMu.Lock()
	fn := observer.Meter
	observerMu.Unlock()
	if fn == nil {
		return
	}
	if reply.Usage != nil && (reply.Usage.Prompt > 0 || reply.Usage.Completion > 0) {
		cached := reply.Usage.PromptCacheHitTokens
		if cached <= 0 {
			cached = reply.Usage.CachedTokens
		}
		if reply.Usage.PromptTokensDetails != nil {
			cached = reply.Usage.PromptTokensDetails.CachedTokens
		}
		fn(reply.Usage.Prompt, reply.Usage.Completion, cached, elapsed, false)
		return
	}
	completion := int64(utf8.RuneCountInString(StringOf(reply.Message.Content)) +
		utf8.RuneCountInString(reply.Reasoning))
	for _, call := range reply.Message.ToolCalls {
		completion += int64(utf8.RuneCountInString(call.Function.Arguments))
	}
	prompt := int64(utf8.RuneCountInString(string(requestBody))) / 3
	fn(prompt, completion, 0, elapsed, true)
}
