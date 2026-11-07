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

package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
)

type AssetContentEntry struct {
	ID, Name, Ext, Path string
	Size, Updated       int64
	Content             string
}

type AssetProgress struct {
	Action         string
	Current, Total int
}
type assetOperation struct {
	action string
	assets []AssetContentEntry
	path   string
}

// AssetQueue retains the original FIFO operations, per-operation transactions
// and 512-row insertion batches. It has no worker, global DB or event bus.
// Flush serialization precedes dequeue, preventing concurrent flush reordering.
type AssetQueue struct {
	mu       sync.Mutex
	gateOnce sync.Once
	gate     chan struct{}
	pending  []assetOperation
	closed   bool
}

var ErrAssetQueueClosed = errors.New("asset_queue_closed")

func (q *AssetQueue) Index(items []AssetContentEntry) error {
	if len(items) == 0 {
		return nil
	}
	return q.append(assetOperation{action: "index", assets: append([]AssetContentEntry(nil), items...)})
}

func (q *AssetQueue) DeletePath(path string) error {
	return q.append(assetOperation{action: "deletePath", path: path})
}

func (q *AssetQueue) append(op assetOperation) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrAssetQueueClosed
	}
	q.pending = append(q.pending, op)
	return nil
}

// Failed/unstarted operations are restored ahead of newly admitted work. A
// completed SQL commit is never replayed merely because a later operation fails.
func (q *AssetQueue) Flush(ctx context.Context, db *sql.DB, notify func(AssetProgress)) error {
	q.gateOnce.Do(func() { q.gate = make(chan struct{}, 1); q.gate <- struct{}{} })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-q.gate:
	}
	defer func() { q.gate <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	ops := q.pending
	q.pending = nil
	q.mu.Unlock()
	total, current := map[string]int{}, map[string]int{}
	for _, op := range ops {
		total[op.action]++
	}
	for i, op := range ops {
		current[op.action]++
		progress := AssetProgress{op.action, current[op.action], total[op.action]}
		// Preserve the uncommitted operation even if an observer panics. SQL
		// rollback runs before restoration; the panic still reaches the owner.
		err := func() (err error) {
			defer func() {
				if value := recover(); value != nil {
					q.mu.Lock()
					q.pending = append(ops[i:], q.pending...)
					q.mu.Unlock()
					panic(value)
				}
			}()
			return Transaction(ctx, db, func(tx *sql.Tx) error {
				if op.action == "deletePath" {
					_, err := tx.ExecContext(ctx, "DELETE FROM asset_contents_fts_case_insensitive WHERE path = ?", op.path)
					return err
				}
				for start := 0; start < len(op.assets); start += 512 {
					end := min(start+512, len(op.assets))
					args := make([]interface{}, 0, (end-start)*7)
					for _, h := range op.assets[start:end] {
						args = append(args, h.ID, h.Name, h.Ext, h.Path, h.Size, h.Updated, h.Content)
					}
					statement := "INSERT INTO asset_contents_fts_case_insensitive (id,name,ext,path,size,updated,content) VALUES " + strings.TrimSuffix(strings.Repeat("(?,?,?,?,?,?,?),", end-start), ",")
					if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
						return err
					}
					if notify != nil {
						notify(progress)
					}
				}
				return nil
			})
		}()
		if err != nil {
			q.mu.Lock()
			q.pending = append(ops[i:], q.pending...)
			q.mu.Unlock()
			return err
		}
	}
	return ctx.Err()
}

func (q *AssetQueue) Close(ctx context.Context, db *sql.DB, notify func(AssetProgress)) error {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	return q.Flush(ctx, db, notify)
}
