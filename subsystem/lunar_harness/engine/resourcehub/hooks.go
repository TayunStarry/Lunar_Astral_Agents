package resourcehub

// hooks.go 能力钩子的注入与读取。
//
// engine 不依赖 agents：视觉 / 文献 / 网页 / B 站 / 归档的解读能力由装配层
// （app）在启动时通过 SetHooks 注入。未注入的钩子返回零值，
// 调用方按「暂无描述」降级，不会 panic。

// SetHooks 注入资源解读能力。传入零值可清除注入。
func SetHooks(hooks Hooks) {
	hookMu.Lock()
	hookSet = hooks
	hookMu.Unlock()
}

// currentHooks 读取当前注入的能力钩子（并发安全）。
func currentHooks() Hooks {
	hookMu.RLock()
	defer hookMu.RUnlock()
	return hookSet
}
