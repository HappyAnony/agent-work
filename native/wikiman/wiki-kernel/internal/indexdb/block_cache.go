// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"wiki-kernel/search"
)

// BlockCache is an explicitly owned value cache, not a worker-backed process
// singleton. It retains original capacity/mark stripping and returns copies.
type BlockCache struct {
	mu     sync.Mutex
	rows   map[string]BlockRecord
	Limit  int
	closed bool
}

func (c *BlockCache) Get(id string) (BlockRecord, bool) {
	if c == nil {
		return BlockRecord{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return BlockRecord{}, false
	}
	row, ok := c.rows[id]
	return row, ok
}
func (c *BlockCache) Put(row BlockRecord) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	limit := c.Limit
	if limit <= 0 {
		limit = 10240
	}
	if c.rows == nil {
		c.rows = map[string]BlockRecord{}
	}
	if _, ok := c.rows[row.ID]; !ok && len(c.rows) >= limit {
		clear(c.rows)
	}
	row.Content = strings.ReplaceAll(row.Content, search.SearchMarkLeft, "")
	row.Content = strings.ReplaceAll(row.Content, search.SearchMarkRight, "")
	c.rows[row.ID] = row
}
func (c *BlockCache) Delete(id string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.rows, id)
}
func (c *BlockCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.rows)
}
func (c *BlockCache) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.rows)
	c.closed = true
}
func (c *BlockCache) isClosed() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

type CachedBlocks struct {
	DB    *sql.DB
	Cache *BlockCache
}

func (q CachedBlocks) Get(ctx context.Context, id string) (BlockRecord, bool, error) {
	rows, e := q.GetMany(ctx, []string{id})
	if e != nil {
		return BlockRecord{}, false, e
	}
	if rows[0] == nil {
		return BlockRecord{}, false, nil
	}
	return *rows[0], true, nil
}

// Missing rows retain nil placeholders and input order, including duplicates.
// A failed/cancelled batch never publishes a partly populated cache.
func (q CachedBlocks) GetMany(ctx context.Context, ids []string) ([]*BlockRecord, error) {
	if ctx == nil || q.DB == nil {
		return nil, ErrNoDatabase
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if q.Cache.isClosed() {
		return nil, ErrClosed
	}
	found := map[string]BlockRecord{}
	missing := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if row, ok := q.Cache.Get(id); ok {
			found[id] = row
		} else {
			missing = append(missing, id)
		}
	}
	loaded := []BlockRecord{}
	for start := 0; start < len(missing); start += 512 {
		batch := missing[start:min(start+512, len(missing))]
		args := make([]interface{}, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		rows, e := q.DB.QueryContext(ctx, "SELECT * FROM blocks WHERE id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")+")", args...)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			row, e := scanBlock(rows)
			if e != nil {
				rows.Close()
				return nil, e
			}
			loaded = append(loaded, row)
			found[row.ID] = row
		}
		e = errors.Join(rows.Err(), rows.Close())
		if e != nil {
			return nil, e
		}
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if q.Cache.isClosed() {
		return nil, ErrClosed
	}
	for _, row := range loaded {
		q.Cache.Put(row)
	}
	out := make([]*BlockRecord, len(ids))
	for i, id := range ids {
		if row, ok := found[id]; ok {
			copy := row
			out[i] = &copy
		}
	}
	return out, nil
}
