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

type HistoryEntry struct {
	ID                                string
	Type                              int
	Op, Title, Content, Created, Path string
}

type HistoryProgress struct {
	Action         string
	Current, Total int
}
type historyOperation struct {
	action    string
	histories []HistoryEntry
	before    int64
	directory string
}

// HistoryQueue retains the original FIFO operations, per-operation transactions
// and 512-row insertion batches. It has no worker, global DB or event bus.
// Flush serialization precedes dequeue, preventing concurrent flush reordering.
type HistoryQueue struct {
	mu       sync.Mutex
	gateOnce sync.Once
	gate     chan struct{}
	pending  []historyOperation
	closed   bool
}

var ErrHistoryQueueClosed = errors.New("history_queue_closed")

func (q *HistoryQueue) Index(items []HistoryEntry) error {
	if len(items) == 0 {
		return nil
	}
	return q.append(historyOperation{action: "index", histories: append([]HistoryEntry(nil), items...)})
}

// ReplaceDirectory keeps directory reindex/retry idempotent in the derived DB.
// Delete and original batched inserts share one transaction; other paths survive.
func (q *HistoryQueue) ReplaceDirectory(directory string, items []HistoryEntry) error {
	if directory == "" || directory == "." || directory == ".." || strings.ContainsAny(directory, "/\\") {
		return errors.New("invalid_history_directory")
	}
	return q.append(historyOperation{action: "index", directory: directory, histories: append([]HistoryEntry(nil), items...)})
}

func (q *HistoryQueue) DeleteBefore(before int64) error {
	return q.append(historyOperation{action: "deleteOutdated", before: before})
}

func (q *HistoryQueue) Clear() error { return q.append(historyOperation{action: "clear"}) }

func (q *HistoryQueue) append(op historyOperation) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrHistoryQueueClosed
	}
	q.pending = append(q.pending, op)
	return nil
}

// Failed/unstarted operations are restored ahead of newly admitted work. A
// completed SQL commit is never replayed merely because a later operation fails.
func (q *HistoryQueue) Flush(ctx context.Context, db *sql.DB, notify func(HistoryProgress)) error {
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
		progress := HistoryProgress{op.action, current[op.action], total[op.action]}
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
				if op.action == "clear" {
					_, err := tx.ExecContext(ctx, "DELETE FROM histories_fts_case_insensitive")
					return err
				}
				if op.action == "deleteOutdated" {
					_, err := tx.ExecContext(ctx, "DELETE FROM histories_fts_case_insensitive WHERE CAST(created AS INTEGER) < ?", op.before)
					return err
				}
				if op.directory != "" {
					prefix := op.directory + "/"
					if _, e := tx.ExecContext(ctx, "DELETE FROM histories_fts_case_insensitive WHERE path=? OR substr(path,1,length(?))=?", op.directory, prefix, prefix); e != nil {
						return e
					}
				}
				for start := 0; start < len(op.histories); start += 512 {
					end := min(start+512, len(op.histories))
					args := make([]interface{}, 0, (end-start)*7)
					for _, h := range op.histories[start:end] {
						args = append(args, h.ID, h.Type, h.Op, h.Title, h.Content, h.Path, h.Created)
					}
					statement := "INSERT INTO histories_fts_case_insensitive (id,type,op,title,content,path,created) VALUES " + strings.TrimSuffix(strings.Repeat("(?,?,?,?,?,?,?),", end-start), ",")
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

func (q *HistoryQueue) Close(ctx context.Context, db *sql.DB, notify func(HistoryProgress)) error {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	return q.Flush(ctx, db, notify)
}
