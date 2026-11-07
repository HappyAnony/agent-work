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
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

// BlockRecord preserves the original complete SQL row (including all 21 fields).
type BlockRecord struct {
	ID, ParentID, RootID, Hash, Box, Path, HPath, Name, Alias, Memo, Tag, Content, FContent, Markdown string
	Length                                                                                            int
	Type, SubType, IAL                                                                                string
	Sort                                                                                              int
	Created, Updated                                                                                  string
}

func EmptyEmbedBlocks(ctx context.Context, db *sql.DB) (ret []BlockRecord, err error) {
	rows, err := Query(ctx, db, "SELECT * FROM blocks WHERE type = 'query_embed' AND content = ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scanErrors error
	for rows.Next() {
		var b BlockRecord
		if err = rows.Scan(&b.ID, &b.ParentID, &b.RootID, &b.Hash, &b.Box, &b.Path, &b.HPath, &b.Name, &b.Alias, &b.Memo, &b.Tag, &b.Content, &b.FContent, &b.Markdown, &b.Length, &b.Type, &b.SubType, &b.IAL, &b.Sort, &b.Created, &b.Updated); err != nil {
			scanErrors = errors.Join(scanErrors, err)
			continue
		}
		ret = append(ret, b)
	}
	return ret, errors.Join(scanErrors, rows.Err())
}

// UpdateBlockContent is the original main-index business algorithm. Caches and
// lifecycle/error policy belong to the owner, never an implicit global facade.
func UpdateBlockContent(ctx context.Context, tx *sql.Tx, id, content string, caseSensitive bool) error {
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		if table == "blocks_fts_case_insensitive" && caseSensitive {
			continue
		}
		if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET content = ? WHERE id = ?", content, id); err != nil {
			return err
		}
	}
	return nil
}

type contentUpdate struct {
	id, content string
	mutation    *MainMutation
}

func (o contentUpdate) key() (struct{ action, id, box, path string }, bool) {
	if o.mutation != nil {
		return o.mutation.key()
	}
	return struct{ action, id, box, path string }{action: "update_block_content", id: o.id}, true
}

// ContentQueue is the contextualized update_block_content family of the
// original main queue. Equal IDs coalesce in place; different IDs retain FIFO.
// Eleven additional audited families share this FIFO, including owned reference
// and content projections, plus the full tree index/upsert families.
type ContentQueue struct {
	mu      sync.Mutex
	once    sync.Once
	gate    chan struct{}
	pending []contentUpdate
	closed  bool
}

var ErrContentQueueClosed = errors.New("content_queue_closed")

func (q *ContentQueue) Pending() int { q.mu.Lock(); defer q.mu.Unlock(); return len(q.pending) }

func (q *ContentQueue) Update(id, content string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrContentQueueClosed
	}
	for i, op := range q.pending {
		if op.mutation == nil && op.id == id {
			q.pending[i] = contentUpdate{id: id, content: content}
			return nil
		}
	}
	q.pending = append(q.pending, contentUpdate{id: id, content: content})
	return nil
}
func (q *ContentQueue) Flush(ctx context.Context, db *sql.DB, caseSensitive bool) error {
	return q.FlushMain(ctx, db, caseSensitive, nil)
}

type MainProgress struct {
	Action, Detail             string
	TreeBatch                  bool
	Current, Total, BlockCount int
}

func (q *ContentQueue) FlushMain(ctx context.Context, db *sql.DB, caseSensitive bool, committed func(string)) error {
	return q.FlushMainProgress(ctx, db, caseSensitive, func(p MainProgress) {
		if committed != nil {
			committed(p.Action)
		}
	})
}

func (q *ContentQueue) FlushMainProgress(ctx context.Context, db *sql.DB, caseSensitive bool, committed func(MainProgress)) error {
	q.once.Do(func() { q.gate = make(chan struct{}, 1); q.gate <- struct{}{} })
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
	current, total := map[string]int{}, map[string]int{}
	for _, op := range ops {
		action := op.action()
		total[action]++
	}
	for i, op := range ops {
		var treeBatches []BlockBatch
		err := func() (err error) {
			defer func() {
				if value := recover(); value != nil {
					q.restore(ops[i:])
					panic(value)
				}
			}()
			return Transaction(ctx, db, func(tx *sql.Tx) error {
				if op.mutation != nil {
					if op.mutation.Tree != nil && (op.mutation.Action == "index" || op.mutation.Action == "upsert") {
						var err error
						treeBatches, err = WriteTree(ctx, tx, *op.mutation.Tree, op.mutation.Action == "upsert", caseSensitive)
						return err
					}
					return ApplyMainMutation(ctx, tx, *op.mutation, caseSensitive)
				}
				return UpdateBlockContent(ctx, tx, op.id, op.content, caseSensitive)
			})
		}()
		if err != nil {
			q.restore(ops[i:])
			return err
		}
		if committed != nil {
			func() {
				defer func() {
					if value := recover(); value != nil {
						q.restore(ops[i+1:])
						panic(value)
					}
				}()
				action := op.action()
				current[action]++
				progress := MainProgress{Action: action, Current: current[action], Total: total[action]}
				if m := op.mutation; m != nil {
					switch action {
					case "delete_id":
						progress.Detail = m.ID
					case "delete_ids":
						progress.Detail = fmt.Sprint(len(m.IDs))
					case "rename", "rename_sub_tree":
						progress.BlockCount = 1
						progress.Detail = fmt.Sprintf("%x", sha256.Sum256([]byte(m.ID)))[:7]
					}
				}
				for _, batch := range treeBatches {
					p := progress
					p.TreeBatch = true
					p.BlockCount = batch.Count
					p.Detail = batch.Hash
					committed(p)
				}
				committed(progress)
			}()
		}
	}
	return ctx.Err()
}

// Work admitted during a failed flush is newer than restored work. Coalesce
// those equal IDs before a later Update can otherwise replace the wrong slot.
func (q *ContentQueue) restore(ops []contentUpdate) {
	q.mu.Lock()
	defer q.mu.Unlock()
	all := append(ops, q.pending...)
	next := make([]contentUpdate, 0, len(all))
	positions := map[struct{ action, id, box, path string }]int{}
	for _, op := range all {
		key, coalesce := op.key()
		if i, ok := positions[key]; ok && coalesce {
			next[i] = op
		} else {
			if coalesce {
				positions[key] = len(next)
			}
			next = append(next, op)
		}
	}
	q.pending = next
}
func (q *ContentQueue) Close(ctx context.Context, db *sql.DB, caseSensitive bool) error {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	return q.Flush(ctx, db, caseSensitive)
}

// Enqueue copies values before returning; equal single-target operations replace
// their original FIFO slot, while batch deletes retain every admission.
func (q *ContentQueue) Enqueue(m MainMutation) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.Tree != nil {
		copy := m.Tree.Clone()
		m.Tree = &copy
	}
	m.IDs = append([]string(nil), m.IDs...)
	m.Hashes = append([]string(nil), m.Hashes...)
	m.Refs = append([]RefRecord(nil), m.Refs...)
	m.Annotations = append([]AnnotationRecord(nil), m.Annotations...)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrContentQueueClosed
	}
	key, coalesce := m.key()
	if coalesce {
		for i, op := range q.pending {
			old, ok := op.key()
			if ok && old == key {
				q.pending[i] = contentUpdate{mutation: &m}
				return nil
			}
		}
	}
	q.pending = append(q.pending, contentUpdate{mutation: &m})
	return nil
}

func (o contentUpdate) action() string {
	if o.mutation != nil {
		return o.mutation.Action
	}
	return "update_block_content"
}
