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

package cache

import (
	"sync"
	"wiki-kernel/internal/ialcache"
)

// This slot is only the transitional compat owner, never an active-page switch.
// Importing cache does not open IAL caches or start their workers.
var legacyIAL struct {
	sync.RWMutex
	cache *ialcache.Cache
}

func OpenLegacyIAL() error {
	legacyIAL.Lock()
	defer legacyIAL.Unlock()
	if legacyIAL.cache != nil {
		return nil
	}
	c, err := ialcache.Open(0)
	if err != nil {
		return err
	}
	legacyIAL.cache = c
	return nil
}
func CloseLegacyIAL() {
	legacyIAL.Lock()
	defer legacyIAL.Unlock()
	if legacyIAL.cache != nil {
		legacyIAL.cache.Close()
		legacyIAL.cache = nil
	}
}
func PutDocIAL(p string, ial map[string]string) {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache != nil {
		legacyIAL.cache.PutDoc(p, ial)
	}
}
func GetDocIAL(p string) map[string]string {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache == nil {
		return nil
	}
	return legacyIAL.cache.GetDoc(p)
}
func RemoveDocIAL(p string) {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache != nil {
		legacyIAL.cache.RemoveDoc(p)
	}
}
func ClearDocsIAL() {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache != nil {
		legacyIAL.cache.ClearDocs()
	}
}
func PutBlockIAL(id string, ial map[string]string) {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache != nil {
		legacyIAL.cache.PutBlock(id, ial)
	}
}

// PublishBlockIAL completes cache publication before an external notification.
func PublishBlockIAL(id string, ial map[string]string) {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache != nil {
		legacyIAL.cache.PublishBlock(id, ial)
	}
}
func GetBlockIAL(id string) map[string]string {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache == nil {
		return nil
	}
	return legacyIAL.cache.GetBlock(id)
}
func RemoveBlockIAL(id string) {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache != nil {
		legacyIAL.cache.RemoveBlock(id)
	}
}
func ClearBlocksIAL() {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache != nil {
		legacyIAL.cache.ClearBlocks()
	}
}

// InvalidateTreeIAL completes this compatibility owner's buffered invalidations
// before its synchronous index operation may report completion.
func InvalidateTreeIAL(doc string, ids []string) {
	legacyIAL.RLock()
	defer legacyIAL.RUnlock()
	if legacyIAL.cache != nil {
		legacyIAL.cache.InvalidateTree(doc, ids)
	}
}

// CaptureCommitIAL never follows a later compatibility cache selection.
func CaptureCommitIAL() func(string, []string, map[string]string) {
	legacyIAL.RLock()
	c := legacyIAL.cache
	legacyIAL.RUnlock()
	return func(doc string, ids []string, ial map[string]string) {
		if c == nil {
			return
		}
		c.InvalidateTree(doc, ids)
		c.PutDoc(doc, ial)
		c.Wait()
	}
}

// CaptureBlockIALPublisher pins the compatibility cache used before synchronous
// attribute broadcasts. It must not follow a later global cache selection.
func CaptureBlockIALPublisher() func(string, map[string]string) {
	legacyIAL.RLock()
	c := legacyIAL.cache
	legacyIAL.RUnlock()
	return func(id string, attrs map[string]string) {
		if c != nil {
			c.PublishBlock(id, attrs)
		}
	}
}
func CaptureBlockIALEvicter() func(string) {
	legacyIAL.RLock()
	c := legacyIAL.cache
	legacyIAL.RUnlock()
	return func(id string) {
		if c != nil {
			c.RemoveBlock(id)
		}
	}
}
