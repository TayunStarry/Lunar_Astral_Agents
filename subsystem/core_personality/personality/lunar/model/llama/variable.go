package llama

import (
	"os/exec"
	"sync"
)

var (
	// serverProcess 是 llama.cpp 服务器的进程 它负责处理模型推理和生成文本
	serverProcess *exec.Cmd
	// serverReady 是一个通道 用于通知调用方 llama.cpp 服务器已启动并准备就绪
	serverReady = make(chan struct{}, 1)
	// readyOnce 是一个 sync.Once 实例 用于确保 llama.cpp 服务器只启动一次
	readyOnce sync.Once
)
