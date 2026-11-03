package agentrepair

// variable.go 注册表。

import "sync"

var (
	// mu 保护注册表
	mu sync.RWMutex
	// handlers 目标标识 → 处理器
	handlers = map[string]Handler{}
	// order 注册顺序
	order []string
)
