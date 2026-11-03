//go:build windows

package desktop

// observer.go 桌面操纵的工具实况登记。
// 模型的流式实况、循环监管与计量统一由 protocol/agentbase 负责；
// 这里只保留「工具名 + 参数」的实况注入，供控制台复盘各阶段操作。

// SetToolObserver 注入工具调用观察者（实况屏展示「工具名 + 参数」）。
func SetToolObserver(fn func(role, name, args string)) {
	autoObserverMu.Lock()
	defer autoObserverMu.Unlock()
	autoObsTool = fn
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
