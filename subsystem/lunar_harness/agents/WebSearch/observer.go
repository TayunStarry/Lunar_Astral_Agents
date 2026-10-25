package WebSearch

// observer.go 模型实况与计量：把网络搜索的模型思考/输出同步到 TUI，并计入运行状态。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"strings"
	"time"
	"unicode/utf8"
)

// loggerWarn 统一日志入口
func loggerWarn(format string, args ...any) {
	logger.Warn("CrystalAstral", format, args...)
}

// SetModelObserver 注入实况观察者与计量回调（app 层启动时调用）
func SetModelObserver(begin func(role string), delta func(role, kind, text string), end func(role string), meter func(prompt, completion int64, generation time.Duration, estimated bool)) {
	observerMu.Lock()
	defer observerMu.Unlock()
	observerBegin, observerDelta, observerEnd, observerMeter = begin, delta, end, meter
}

// observeBegin 通知实况开始
func observeBegin() {
	observerMu.Lock()
	fn := observerBegin
	observerMu.Unlock()
	if fn != nil {
		fn(webSearchModelRole)
	}
}

// observeDelta 通知实况增量（kind: think/content）
func observeDelta(kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	observerMu.Lock()
	fn := observerDelta
	observerMu.Unlock()
	if fn != nil {
		fn(webSearchModelRole, kind, text)
	}
}

// observeEnd 通知实况结束
func observeEnd() {
	observerMu.Lock()
	fn := observerEnd
	observerMu.Unlock()
	if fn != nil {
		fn(webSearchModelRole)
	}
}

// reportModelCall 把一次调用计入「运行状态」的模型计量：
// 优先用服务端回传的真实 usage，缺失时按请求体与输出文本长度估算（标记 estimated）。
func reportModelCall(requestBody []byte, completionText string, usage *modelUsage, elapsed time.Duration) {
	observerMu.Lock()
	fn := observerMeter
	observerMu.Unlock()
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
