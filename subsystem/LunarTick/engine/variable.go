package engine

import (
	"fmt"
	"sync"
)

type VarEntry struct {
	Value string
	Mode  VarMode
}

type VarStore struct {
	mu   sync.RWMutex
	vars map[string]VarEntry
}

func NewVarStore() *VarStore {
	return &VarStore{
		vars: make(map[string]VarEntry),
	}
}

func (vs *VarStore) Get(name string) string {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	entry, exists := vs.vars[name]
	if !exists {
		return ""
	}
	if entry.Mode == ModeWriteOnly {
		return ""
	}
	return entry.Value
}

func (vs *VarStore) Set(name, value string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	entry := vs.vars[name]
	if entry.Mode == ModeReadOnly {
		return
	}
	entry.Value = value
	vs.vars[name] = entry
}

func (vs *VarStore) Add(name, value string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	entry := vs.vars[name]
	if entry.Mode == ModeReadOnly {
		return
	}
	entry.Value += value
	vs.vars[name] = entry
}

func (vs *VarStore) SetMode(name string, mode VarMode) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	entry := vs.vars[name]
	entry.Mode = mode
	vs.vars[name] = entry
}

func (vs *VarStore) GetEntry(name string) (VarEntry, bool) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	entry, exists := vs.vars[name]
	if exists && entry.Mode == ModeWriteOnly {
		return VarEntry{Value: "", Mode: ModeWriteOnly}, true
	}
	return entry, exists
}

func (vs *VarStore) Snapshot() map[string]VarEntry {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	snap := make(map[string]VarEntry, len(vs.vars))
	for k, v := range vs.vars {
		snap[k] = v
	}
	return snap
}

func (vs *VarStore) Restore(snap map[string]VarEntry) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.vars = make(map[string]VarEntry, len(snap))
	for k, v := range snap {
		vs.vars[k] = v
	}
}

func (vs *VarStore) GetAll() map[string]string {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	result := make(map[string]string, len(vs.vars))
	for k, v := range vs.vars {
		if v.Mode == ModeWriteOnly {
			result[k] = ""
		} else {
			result[k] = v.Value
		}
	}
	return result
}

func (vs *VarStore) String() string {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	return fmt.Sprintf("VarStore(%d vars)", len(vs.vars))
}