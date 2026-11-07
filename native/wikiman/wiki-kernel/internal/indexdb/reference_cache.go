package indexdb

import (
	"errors"
	"sync"
	"time"
)

// ReferenceCache preserves the original per-definition, per-referring-block
// grouping and sliding 30-minute expiry. Values never escape by reference;
// cleanup is on access, not an import-started janitor. Zero limit is legacy-only.
type ReferenceCache struct {
	mu      sync.Mutex
	entries map[string]referenceEntry
	closed  bool
	Limit   int
}
type referenceEntry struct {
	until time.Time
	refs  map[string]RefRecord
}

var ErrReferenceCache = errors.New("reference_cache_closed_or_full")

func (c *ReferenceCache) Get(id string) []RefRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	if !ok {
		return nil
	}
	if time.Now().After(e.until) {
		delete(c.entries, id)
		return nil
	}
	ret := make([]RefRecord, 0, len(e.refs))
	for _, r := range e.refs {
		ret = append(ret, r)
	}
	return ret
}
func (c *ReferenceCache) Put(ref RefRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrReferenceCache
	}
	if c.entries == nil {
		c.entries = map[string]referenceEntry{}
	}
	now := time.Now()
	total := 0
	for id, e := range c.entries {
		if now.After(e.until) {
			delete(c.entries, id)
		} else {
			total += len(e.refs)
		}
	}
	e, ok := c.entries[ref.DefBlockID]
	if !ok {
		if c.Limit > 0 && len(c.entries) >= c.Limit {
			return ErrReferenceCache
		}
		e.refs = map[string]RefRecord{}
	}
	if c.Limit > 0 && total >= c.Limit {
		if _, ok := e.refs[ref.BlockID]; !ok {
			return ErrReferenceCache
		}
	}
	e.refs[ref.BlockID] = ref
	e.until = now.Add(30 * time.Minute)
	c.entries[ref.DefBlockID] = e
	return nil
}
func (c *ReferenceCache) Delete(id string) { c.mu.Lock(); defer c.mu.Unlock(); delete(c.entries, id) }
func (c *ReferenceCache) Clear()           { c.mu.Lock(); defer c.mu.Unlock(); clear(c.entries) }
func (c *ReferenceCache) Close()           { c.mu.Lock(); defer c.mu.Unlock(); c.closed = true; clear(c.entries) }
