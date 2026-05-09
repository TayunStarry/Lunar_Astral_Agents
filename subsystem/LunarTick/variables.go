package main

import (
	"sync"
)

// VariableManager 变量管理器
type VariableManager struct {
	mu        sync.RWMutex
	variables map[string]Variable
}

// NewVariableManager 创建变量管理器
func NewVariableManager() *VariableManager {
	return &VariableManager{
		variables: make(map[string]Variable),
	}
}

// Get 获取变量值
func (vm *VariableManager) Get(name string) string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	if v, ok := vm.variables[name]; ok {
		if v.Mode == VarModeWriteOnly {
			return ""
		}
		return v.Value
	}
	return ""
}

// Set 设置变量值（保持原模式）
func (vm *VariableManager) Set(name string, value string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	if v, ok := vm.variables[name]; ok {
		if v.Mode != VarModeReadOnly {
			vm.variables[name] = Variable{
				Value: value,
				Mode:  v.Mode,
			}
		}
	} else {
		vm.variables[name] = Variable{
			Value: value,
			Mode:  VarModeNormal,
		}
	}
}

// SetMode 设置变量值和模式
func (vm *VariableManager) SetMode(name string, value string, mode VarMode) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	vm.variables[name] = Variable{
		Value: value,
		Mode:  mode,
	}
}

// Add 追加值到变量
func (vm *VariableManager) Add(name string, value string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	if v, ok := vm.variables[name]; ok {
		if v.Mode != VarModeReadOnly {
			vm.variables[name] = Variable{
				Value: v.Value + value,
				Mode:  v.Mode,
			}
		}
	} else {
		vm.variables[name] = Variable{
			Value: value,
			Mode:  VarModeNormal,
		}
	}
}

// Unlock 解锁变量（设置为普通模式）
func (vm *VariableManager) Unlock(name string, appendValue string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	if v, ok := vm.variables[name]; ok {
		vm.variables[name] = Variable{
			Value: v.Value + appendValue,
			Mode:  VarModeNormal,
		}
	} else {
		vm.variables[name] = Variable{
			Value: appendValue,
			Mode:  VarModeNormal,
		}
	}
}

// GetAll 获取所有变量（用于快照）
func (vm *VariableManager) GetAll() map[string]Variable {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	result := make(map[string]Variable)
	for k, v := range vm.variables {
		result[k] = v
	}
	return result
}

// RestoreAll 恢复所有变量（用于快照）
func (vm *VariableManager) RestoreAll(vars map[string]Variable) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	vm.variables = make(map[string]Variable)
	for k, v := range vars {
		vm.variables[k] = v
	}
}
