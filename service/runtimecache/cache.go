package runtimecache

import "sync"

type entry struct {
	invalidate func()
	clear      func()
}

var registry struct {
	mu      sync.RWMutex
	entries map[string]entry
}

func Register(name string, invalidate func(), clear func()) {
	if name == "" || (invalidate == nil && clear == nil) {
		return
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.entries == nil {
		registry.entries = make(map[string]entry)
	}
	registry.entries[name] = entry{
		invalidate: invalidate,
		clear:      clear,
	}
}

func Invalidate() {
	for _, item := range snapshot() {
		if item.invalidate != nil {
			item.invalidate()
		}
	}
}

func Clear() {
	for _, item := range snapshot() {
		if item.clear != nil {
			item.clear()
		}
	}
}

func snapshot() []entry {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	items := make([]entry, 0, len(registry.entries))
	for _, item := range registry.entries {
		items = append(items, item)
	}
	return items
}
