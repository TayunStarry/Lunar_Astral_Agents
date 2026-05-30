package engine

import (
	"sync"
)

type PointerEntry struct {
	Type      PointerType
	Lines     []string
	SnapData  map[string]VarEntry
	TickStamp int
}

type PointerRegistry struct {
	mu       sync.RWMutex
	pointers map[string]PointerEntry
}

func NewPointerRegistry() *PointerRegistry {
	return &PointerRegistry{
		pointers: make(map[string]PointerEntry),
	}
}

func (pr *PointerRegistry) Define(name string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.pointers[name]; !exists {
		pr.pointers[name] = PointerEntry{
			Type:  PointerClass,
			Lines: nil,
		}
	}
}

func (pr *PointerRegistry) BuildClass(name string, lines []string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pr.pointers[name] = PointerEntry{
		Type:  PointerClass,
		Lines: copyLines(lines),
	}
}

func (pr *PointerRegistry) BuildSnapshot(name string, snap map[string]VarEntry, tick int) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pr.pointers[name] = PointerEntry{
		Type:      PointerSnapshot,
		SnapData:  snap,
		TickStamp: tick,
	}
}

func (pr *PointerRegistry) RegisterLazy(name string, lines []string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pr.pointers[name] = PointerEntry{
		Type:  PointerClass,
		Lines: copyLines(lines),
	}
}

func (pr *PointerRegistry) Get(name string) (PointerEntry, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	entry, exists := pr.pointers[name]
	return entry, exists
}

func (pr *PointerRegistry) Exists(name string) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	_, exists := pr.pointers[name]
	return exists
}

func (pr *PointerRegistry) IsClass(name string) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	entry, exists := pr.pointers[name]
	return exists && entry.Type == PointerClass
}

func (pr *PointerRegistry) IsSnapshot(name string) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	entry, exists := pr.pointers[name]
	return exists && entry.Type == PointerSnapshot
}

func (pr *PointerRegistry) Remove(name string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	delete(pr.pointers, name)
}

func (pr *PointerRegistry) GetAllClassNames() []string {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	var names []string
	for name, entry := range pr.pointers {
		if entry.Type == PointerClass {
			names = append(names, name)
		}
	}
	return names
}

func copyLines(lines []string) []string {
	result := make([]string, len(lines))
	copy(result, lines)
	return result
}