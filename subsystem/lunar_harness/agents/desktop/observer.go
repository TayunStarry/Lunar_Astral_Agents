//go:build windows

package desktop

// ==== 模型实况与计量 ====
// 把桌面操纵各角色的模型调用同步到 TUI「核心智能 实况」，
// 并把 token 消耗计入「运行状态」，与对话者/观影者等智能体保持一致。

import (
	"strings"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/model/loopguard"
)

// SetModelObserver 注入实况观察者与模型计量回调（宿主在 TUI 启动前调用）。
func SetModelObserver(begin func(role string), delta func(role, kind, text string), end func(role string), meter func(prompt, completion int64, generation time.Duration, estimated bool)) {
	autoObserverMu.Lock()
	defer autoObserverMu.Unlock()
	autoObsBegin, autoObsDelta, autoObsEnd, autoObsMeter = begin, delta, end, meter
}

// SetToolObserver 注入工具调用观察者（实况屏展示「工具名 + 参数」）。
func SetToolObserver(fn func(role, name, args string)) {
	autoObserverMu.Lock()
	defer autoObserverMu.Unlock()
	autoObsTool = fn
}

// observeBegin 标记一次模型调用开始。
func observeBegin(role string) {
	autoObserverMu.Lock()
	fn := autoObsBegin
	autoObserverMu.Unlock()
	if fn != nil {
		fn(role)
	}
}

// observeDelta 推送一段流式增量（思考 / 正文）。
func observeDelta(role, kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	autoObserverMu.Lock()
	fn := autoObsDelta
	autoObserverMu.Unlock()
	if fn != nil {
		fn(role, kind, text)
	}
}

// observeEnd 标记一次模型调用结束。
func observeEnd(role string) {
	autoObserverMu.Lock()
	fn := autoObsEnd
	autoObserverMu.Unlock()
	if fn != nil {
		fn(role)
	}
}

// observeTool 记录一次工具调用（工具名 + 参数摘要）。
func observeTool(role, name, args string) {
	autoObserverMu.Lock()
	fn := autoObsTool
	autoObserverMu.Unlock()
	if fn != nil {
		fn(role, name, args)
	}
}

// reportModelCall 把一次调用计入「运行状态」的模型计量：
// 优先使用服务端回传的真实 usage，缺失时按请求体与输出文本长度估算。
func reportModelCall(requestBody []byte, completionText string, usage *ltpUsage, elapsed time.Duration) {
	autoObserverMu.Lock()
	fn := autoObsMeter
	autoObserverMu.Unlock()
	if fn == nil {
		return
	}
	if usage != nil && (usage.Prompt > 0 || usage.Completion > 0) {
		fn(usage.Prompt, usage.Completion, elapsed, false)
		return
	}
	prompt := int64(utf8.RuneCountInString(string(requestBody))) / 3
	completion := int64(utf8.RuneCountInString(completionText))
	fn(prompt, completion, elapsed, true)
}

// 流式增量的种类（与 loopguard 的文本分类一致）。
const (
	streamKindThink   = loopguard.KindThink
	streamKindContent = loopguard.KindContent
)
