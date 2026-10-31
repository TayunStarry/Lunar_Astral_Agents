package agentchat

// observer.go 模型调用的实况推送与计量回调。
//
// 各智能体传入自己的角色标签（如「视觉理解」「文献解析」），控制台据此在同一块
// 实况屏里区分来源；计量回调让 Go 侧智能体的模型调用一并进入「运行状态」统计。

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Observer 实况观察者与模型计量回调
type Observer struct {
	// Begin 实况开始
	Begin func(role string)
	// Delta 实况增量（kind: think/content）
	Delta func(role, kind, text string)
	// End 实况结束
	End func(role string)
	// Meter 模型调用计量
	Meter func(prompt, completion, cached int64, generation time.Duration, estimated bool)
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

// observeEnd 通知实况结束
func observeEnd(role string) {
	observerMu.Lock()
	fn := observer.End
	observerMu.Unlock()
	if fn != nil {
		fn(role)
	}
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
		cached := int64(0)
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
