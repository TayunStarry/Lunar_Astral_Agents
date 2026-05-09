package main

import (
	"sync"
)

// PointerManager 指针管理器
type PointerManager struct {
	mu       sync.RWMutex
	pointers map[string]*Pointer
}

// NewPointerManager 创建指针管理器
func NewPointerManager() *PointerManager {
	return &PointerManager{
		pointers: make(map[string]*Pointer),
	}
}

// Get 获取指针
func (pm *PointerManager) Get(name string) (*Pointer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	p, ok := pm.pointers[name]
	return p, ok
}

// Set 设置 class 指针
func (pm *PointerManager) SetClass(name string, lines []string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.pointers[name] = &Pointer{
		Type:  PointerClass,
		Lines: lines,
	}
}

// SetSnapshot 设置 snapshot 指针
func (pm *PointerManager) SetSnapshot(name string, snapshot SnapshotState) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.pointers[name] = &Pointer{
		Type:     PointerSnapshot,
		Snapshot: snapshot,
	}
}

// Exists 检查指针是否存在
func (pm *PointerManager) Exists(name string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	_, ok := pm.pointers[name]
	return ok
}

// Delete 删除指针
func (pm *PointerManager) Delete(name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	delete(pm.pointers, name)
}
