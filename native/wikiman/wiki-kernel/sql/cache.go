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

package sql

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"sync"
	"wiki-kernel/internal/indexdb"
)

// Compatibility owner only. No package initializer opens worker-backed caches.
// The original query algorithms still need R03 Context receivers.
var legacyCaches struct {
	sync.Mutex
	blocks  *indexdb.BlockCache
	refs    *indexdb.ReferenceCache
	enabled bool
}

func OpenLegacyCaches() error {
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	if legacyCaches.blocks != nil {
		return nil
	}
	blocks := &indexdb.BlockCache{Limit: 10240}
	legacyCaches.blocks = blocks
	// Expiry is checked by Get; no process-global janitor survives shutdown.
	legacyCaches.refs = &indexdb.ReferenceCache{}
	return nil
}
func CloseLegacyCaches() {
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	if legacyCaches.blocks != nil {
		legacyCaches.blocks.Close()
		legacyCaches.blocks = nil
	}
	if legacyCaches.refs != nil {
		legacyCaches.refs.Clear()
		legacyCaches.refs = nil
	}
	legacyCaches.enabled = false
}
func enableCache()  { legacyCaches.Lock(); defer legacyCaches.Unlock(); legacyCaches.enabled = true }
func disableCache() { legacyCaches.Lock(); defer legacyCaches.Unlock(); legacyCaches.enabled = false }
func ClearCache() {
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	if legacyCaches.blocks != nil {
		legacyCaches.blocks.Clear()
	}
}

func legacyCachedBlocks() indexdb.CachedBlocks {
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	reader := indexdb.CachedBlocks{DB: db}
	if legacyCaches.enabled {
		reader.Cache = legacyCaches.blocks
	}
	return reader
}
func clearAllCaches() {
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	if legacyCaches.blocks != nil {
		legacyCaches.blocks.Clear()
	}
	if legacyCaches.refs != nil {
		legacyCaches.refs.Clear()
	}
}
func putBlockCache(block *Block) {
	if block == nil {
		return
	}
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	if legacyCaches.enabled && legacyCaches.blocks != nil {
		legacyCaches.blocks.Put(indexdb.BlockRecord(*block))
	}
}
func getBlockCache(id string) *Block {
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	if !legacyCaches.enabled || legacyCaches.blocks == nil {
		return nil
	}
	row, ok := legacyCaches.blocks.Get(id)
	if !ok {
		return nil
	}
	copy := Block(row)
	return &copy
}
func removeBlockCache(id string) {
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	if legacyCaches.blocks != nil {
		legacyCaches.blocks.Delete(id)
	}
	if legacyCaches.refs != nil {
		legacyCaches.refs.Delete(id)
	}
}
func GetRefsCacheByDefID(defID string) (ret []*Ref) {
	legacyCaches.Lock()
	if legacyCaches.refs != nil {
		for _, v := range legacyCaches.refs.Get(defID) {
			v := v
			ret = append(ret, &v)
		}
	}
	legacyCaches.Unlock()
	if len(ret) == 0 {
		ret = QueryRefsByDefID(defID, false)
		for _, r := range ret {
			putRefCache(r)
		}
	}
	return
}
func CacheRef(tree *parse.Tree, refNode *ast.Node) { putRefCache(buildRef(tree, refNode)) }

// Capture at FIFO admission; never publish a late reference into a replacement
// compatibility cache. The caller projects using its own parser and registry.
func CaptureReferencePublisher() func(context.Context, indexdb.RefRecord) error {
	legacyCaches.Lock()
	c := legacyCaches.refs
	legacyCaches.Unlock()
	return func(ctx context.Context, ref indexdb.RefRecord) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		legacyCaches.Lock()
		defer legacyCaches.Unlock()
		if c == nil || c != legacyCaches.refs {
			return errors.New("reference_cache_owner_changed")
		}
		return c.Put(ref)
	}
}
func putRefCache(ref *Ref) {
	if ref == nil {
		return
	}
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	if legacyCaches.refs != nil {
		_ = legacyCaches.refs.Put(*ref)
	}
}
func removeRefCacheByDefID(defID string) {
	legacyCaches.Lock()
	defer legacyCaches.Unlock()
	if legacyCaches.refs != nil {
		legacyCaches.refs.Delete(defID)
	}
}

// CaptureCommitReferences pins the compatibility cache and DB before FIFO
// admission. Errors from durable lookups must reach commit, not disappear in a
// void legacy query wrapper. Cached unflushed CacheRef candidates retain priority.
func CaptureCommitReferences() func(context.Context, string) ([]indexdb.RefRecord, error) {
	legacyCaches.Lock()
	c, database := legacyCaches.refs, db
	legacyCaches.Unlock()
	return func(ctx context.Context, id string) ([]indexdb.RefRecord, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if c != nil {
			if rows := c.Get(id); len(rows) > 0 {
				return rows, nil
			}
		}
		rows, err := (indexdb.References{DB: database}).ByDefinition(ctx, id, false)
		if err != nil {
			return nil, err
		}
		if c != nil {
			for _, r := range rows {
				if err = c.Put(r); err != nil {
					return nil, err
				}
			}
		}
		return rows, nil
	}
}
