package agentdump

// variable.go 导出注册表。

import "sync"

var (
	// mu 保护注册表
	mu sync.RWMutex
	// providers 导出标识 → 提供者
	providers = map[string]Provider{}
	// order 注册顺序（控制台清单按此顺序展示）
	order []string
)
