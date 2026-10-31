package modellease

// lease.go 模型租约：把「卸载主模型、改用轻量模型」的窗口与普通模型调用隔离开。
//
// 使用约定：
//   - 切换方调用 AcquireSwap，拿到释放函数后在整个轻量扫描期间持有；
//     扫描内部对轻量模型的调用必须以「已持有租约」方式发起（agentchat.ChatOptions.HoldLease）。
//   - 普通模型调用在 agentchat 内自动调用 AcquireCall，切换期间会等待，避免主模型被反复重载。
//   - 本租约只保护经由 agentchat / 显式使用本包的调用方；直接走 HTTP 的模型调用需自行接入。

// AcquireSwap 获取一次模型切换租约。
// 返回的释放函数必须调用（通常 defer），否则其余模型调用会一直阻塞。
func AcquireSwap() func() {
	swapMu.Lock()
	callGate.Lock()
	return func() {
		callGate.Unlock()
		swapMu.Unlock()
	}
}

// AcquireCall 获取普通模型调用的共享租约。
// 返回的释放函数必须调用（通常 defer）。
func AcquireCall() func() {
	callGate.RLock()
	return callGate.RUnlock
}
