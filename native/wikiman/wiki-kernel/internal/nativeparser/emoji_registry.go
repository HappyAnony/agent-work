package nativeparser

import (
	"maps"
	"sync"
)

// EmojiRegistry belongs to one workspace (or the explicit compatibility owner).
// No package initializer, filesystem repair or worker is involved.
type EmojiRegistry struct {
	mu     sync.Mutex
	values map[string]string
	closed bool
}

func (r *EmojiRegistry) Put(name, src string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[name] = src
}
func (r *EmojiRegistry) Replace(values map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.values = maps.Clone(values)
	}
}
func (r *EmojiRegistry) Snapshot() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Clone(r.values)
}
func (r *EmojiRegistry) Clear() { r.mu.Lock(); defer r.mu.Unlock(); clear(r.values) }
func (r *EmojiRegistry) Close() { r.mu.Lock(); defer r.mu.Unlock(); clear(r.values); r.closed = true }
