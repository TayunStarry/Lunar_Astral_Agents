package modellease

// variable.go 模型租约的全局状态（业务逻辑见 lease.go）

import "sync"

var (
	// swapMu 串行化所有「卸载主模型 → 使用轻量模型」的切换任务，
	// 避免视觉理解、网络搜索、文献解析同时争抢同一张显卡。
	swapMu sync.Mutex
	// callGate 普通模型调用与切换窗口之间的门闸：
	// 切换期间持有写锁，普通调用持有读锁，防止轻量模型被并发请求挤出显存。
	callGate sync.RWMutex
)
