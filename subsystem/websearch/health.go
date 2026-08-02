package websearch

import (
	"sync"
	"time"
)

// engineHealth 单个引擎的健康状态
type engineHealth struct {
	window        []bool
	windowSize    int
	consecFails   int
	degraded      bool
	degradedAt    time.Time
	probeInterval time.Duration
	lastProbe     time.Time
	mu            sync.Mutex
}

// EngineHealth 搜索引擎健康检查器
type EngineHealth struct {
	engines map[string]*engineHealth
	mu      sync.RWMutex
}

// NewEngineHealth 创建引擎健康检查器
func NewEngineHealth() *EngineHealth {
	return &EngineHealth{
		engines: make(map[string]*engineHealth),
	}
}

// getOrCreate 获取或创建引擎健康记录
func (eh *EngineHealth) getOrCreate(name string) *engineHealth {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	if eng, ok := eh.engines[name]; ok {
		return eng
	}
	eng := &engineHealth{
		window:        make([]bool, 0, 10),
		windowSize:    10,
		probeInterval: 2 * time.Minute,
	}
	eh.engines[name] = eng
	return eng
}

// RecordResult 记录一次搜索请求的结果
func (eh *EngineHealth) RecordResult(name string, success bool) {
	eng := eh.getOrCreate(name)
	eng.mu.Lock()
	defer eng.mu.Unlock()

	if len(eng.window) >= eng.windowSize {
		eng.window = eng.window[1:]
	}
	eng.window = append(eng.window, success)

	if success {
		eng.consecFails = 0
		eng.degraded = false
	} else {
		eng.consecFails++
		if eng.consecFails >= 3 && !eng.degraded {
			eng.degraded = true
			eng.degradedAt = time.Now()
		}
	}
}

// IsDegraded 判断引擎是否已降级
func (eh *EngineHealth) IsDegraded(name string) bool {
	eng := eh.getOrCreate(name)
	eng.mu.Lock()
	defer eng.mu.Unlock()

	if !eng.degraded {
		return false
	}

	if time.Since(eng.degradedAt) > 2*time.Minute {
		if time.Since(eng.lastProbe) > eng.probeInterval {
			eng.lastProbe = time.Now()
			return false
		}
	}

	return true
}

// MarkRecovered 标记引擎恢复
func (eh *EngineHealth) MarkRecovered(name string) {
	eng := eh.getOrCreate(name)
	eng.mu.Lock()
	defer eng.mu.Unlock()
	eng.degraded = false
	eng.consecFails = 0
}
