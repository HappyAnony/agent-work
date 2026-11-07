// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Original document/block IAL caching extracted from cache/ial.go. The original
// third-party copyright and AGPL-3.0-or-later license remain applicable.
package ialcache

import (
	"errors"
	"strings"
	"sync"

	"github.com/88250/lute/editor"
	"github.com/dgraph-io/ristretto"
)

type Cache struct {
	mu           sync.RWMutex
	docs, blocks *ristretto.Cache
	closed       bool
}

// Open starts only this owner's Ristretto workers. A zero budget retains the
// legacy limits; Context callers can bound the number of 128-cost IAL entries.
func Open(entries int64) (*Cache, error) {
	if entries < 0 || entries > 1<<20 {
		return nil, errors.New("invalid_ial_cache_budget")
	}
	docCounters, blockCounters, cost := int64(1024*100), int64(1024*1000), int64(1024*1024*200)
	if entries > 0 {
		docCounters = entries * 10
		blockCounters = entries * 10
		cost = entries * 128
	}
	docs, err := ristretto.NewCache(&ristretto.Config{NumCounters: docCounters, MaxCost: cost, BufferItems: 64})
	if err != nil {
		return nil, err
	}
	blocks, err := ristretto.NewCache(&ristretto.Config{NumCounters: blockCounters, MaxCost: cost, BufferItems: 64})
	if err != nil {
		docs.Close()
		return nil, err
	}
	return &Cache{docs: docs, blocks: blocks}, nil
}

func copyIAL(ial map[string]string, unescape bool) map[string]string {
	if ial == nil {
		return nil
	}
	result := make(map[string]string, len(ial))
	for k, v := range ial {
		if unescape {
			v = strings.ReplaceAll(v, editor.IALValEscNewLine, "\n")
		}
		result[k] = v
	}
	return result
}
func (c *Cache) PutDoc(key string, ial map[string]string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.closed {
		c.docs.Set(key, copyIAL(ial, false), 128)
	}
}
func (c *Cache) PutBlock(key string, ial map[string]string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.closed {
		c.blocks.Set(key, copyIAL(ial, false), 128)
	}
}

// PublishBlock is a publication barrier for callers that notify immediately.
// Drain older buffered admissions BEFORE Set: an unadmitted older Set could
// otherwise overwrite the new store value even after a trailing Wait.
// Ordinary cache warming remains buffered through PutBlock.
func (c *Cache) PublishBlock(key string, ial map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.blocks.Wait()
	c.blocks.Set(key, copyIAL(ial, false), 128)
	c.blocks.Wait()
}
func (c *Cache) GetDoc(key string) map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil
	}
	v, ok := c.docs.Get(key)
	if !ok {
		return nil
	}
	return copyIAL(v.(map[string]string), true)
}
func (c *Cache) GetBlock(key string) map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil
	}
	v, ok := c.blocks.Get(key)
	if !ok {
		return nil
	}
	return copyIAL(v.(map[string]string), false)
}
func (c *Cache) RemoveDoc(key string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.closed {
		c.docs.Del(key)
	}
}
func (c *Cache) RemoveBlock(key string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.closed {
		c.blocks.Del(key)
	}
}
func (c *Cache) ClearDocs() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.docs.Clear()
	}
}
func (c *Cache) ClearBlocks() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.blocks.Clear()
	}
}
func (c *Cache) Wait() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.closed {
		c.docs.Wait()
		c.blocks.Wait()
	}
}
func (c *Cache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		c.docs.Close()
		c.blocks.Close()
	}
}

// InvalidateTree is a publication barrier for index rebuild completion. Finish
// older buffered admissions and all deletions while excluding concurrent warming.
// Batch by tree rather than waiting separately for every block in a document.
func (c *Cache) InvalidateTree(doc string, ids []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.docs.Wait()
	c.blocks.Wait()
	for _, id := range ids {
		c.blocks.Del(id)
	}
	c.docs.Del(doc)
	c.docs.Wait()
	c.blocks.Wait()
}
