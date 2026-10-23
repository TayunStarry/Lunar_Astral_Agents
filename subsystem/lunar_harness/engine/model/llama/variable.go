package llama

import (
	"os/exec"
	"sync"
)

// ==== 模型服务进程状态 ====
//
// 为什么要用可重置的 readyOnce/serverReady（而不是原来的 sync.Once + 固定 channel）：
// llama-server 是**外部进程**，会因模型内部断言（实测：cce 崩溃于
// `GGML_ASSERT((cparams.causal_attn || cparams.n_ubatch >= n_tokens_all))`）直接退出。
// 退出之后：
//   - 原来的 waitForProcessExit 只打一行日志，没有任何重启路径；
//   - 就绪信号是一次性的，即便再调 Init 也永远等不到 `serverReady`（5 分钟后超时）。
// 于是"模型服务死了"成为不可恢复状态。把就绪信号做成可重置，重启才成为可能。

var (
	// processMu 串行化启动/重启，并保护 serverProcess
	processMu sync.Mutex
	// serverProcess 是 llama.cpp 服务器的进程 它负责处理模型推理和生成文本
	serverProcess *exec.Cmd

	// readyMu 保护就绪信号（serverReady + readyOnce）
	readyMu sync.Mutex
	// serverReady 用于通知调用方 llama.cpp 服务器已启动并准备就绪（每次启动重建一个）
	serverReady = make(chan struct{}, 1)
	// readyOnce 确保同一次启动只发一次就绪信号（每次启动重建）
	readyOnce = &sync.Once{}
)

// resetReadySignal 重建就绪信号（重启前调用：否则新进程起来了也等不到通知）
func resetReadySignal() {
	readyMu.Lock()
	defer readyMu.Unlock()
	serverReady = make(chan struct{}, 1)
	readyOnce = &sync.Once{}
}

// currentReadySignal 取当前这次启动对应的就绪通道（启动时先抓住，避免重启换了 channel）
func currentReadySignal() chan struct{} {
	readyMu.Lock()
	defer readyMu.Unlock()
	return serverReady
}

// signalReady 标记服务器已就绪（同一次启动只发一次）
func signalReady() {
	readyMu.Lock()
	once := readyOnce
	ch := serverReady
	readyMu.Unlock()
	once.Do(func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	})
}
