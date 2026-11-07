// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import (
	"context"
	stdsql "database/sql"
	"errors"
	"sync"
	"wiki-kernel/internal/indexdb"
)

var historyQueues = struct {
	sync.Mutex
	byDB        map[*stdsql.DB]*indexdb.HistoryQueue
	eventOwners map[*stdsql.DB]interface{}
}{byDB: map[*stdsql.DB]*indexdb.HistoryQueue{}, eventOwners: map[*stdsql.DB]interface{}{}}

type LegacyHistoryQueue struct {
	DB         *stdsql.DB
	queue      *indexdb.HistoryQueue
	eventOwner interface{}
}

func CaptureLegacyHistoryQueue() LegacyHistoryQueue {
	eventOwner := captureLegacySQLEventOwner()
	historyQueues.Lock()
	defer historyQueues.Unlock()
	q := historyQueues.byDB[historyDB]
	if q == nil {
		q = &indexdb.HistoryQueue{}
		historyQueues.byDB[historyDB] = q
	}
	return LegacyHistoryQueue{DB: historyDB, queue: q, eventOwner: eventOwner}
}

func (q LegacyHistoryQueue) EventContext(values map[string]interface{}) map[string]interface{} {
	historyQueues.Lock()
	owner := historyQueues.eventOwners[q.DB]
	historyQueues.Unlock()
	if owner == nil {
		owner = q.eventOwner
	}
	return LegacySQLEventContext(owner, values)
}

func (q LegacyHistoryQueue) admitEventOwner() {
	if q.DB == nil || q.eventOwner == nil {
		return
	}
	historyQueues.Lock()
	historyQueues.eventOwners[q.DB] = q.eventOwner
	historyQueues.Unlock()
}
func (q LegacyHistoryQueue) guard(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	historyQueues.Lock()
	current := historyDB
	historyQueues.Unlock()
	if q.DB == nil || q.DB != current || q.queue == nil {
		return errors.New("history_queue_owner_changed")
	}
	return nil
}
func (q LegacyHistoryQueue) Index(ctx context.Context, items []indexdb.HistoryEntry) error {
	if e := q.guard(ctx); e != nil {
		return e
	}
	q.admitEventOwner()
	return q.queue.Index(items)
}

func (q LegacyHistoryQueue) Clear(ctx context.Context) error {
	if e := q.guard(ctx); e != nil {
		return e
	}
	q.admitEventOwner()
	return q.queue.Clear()
}
func (q LegacyHistoryQueue) Flush(ctx context.Context, notify func(indexdb.HistoryProgress)) error {
	if e := q.guard(ctx); e != nil {
		return e
	}
	err := q.queue.Flush(ctx, q.DB, notify)
	if err == nil {
		historyQueues.Lock()
		delete(historyQueues.eventOwners, q.DB)
		historyQueues.Unlock()
	}
	return err
}
func (q LegacyHistoryQueue) DeleteBefore(ctx context.Context, before int64) error {
	if e := q.guard(ctx); e != nil {
		return e
	}
	q.admitEventOwner()
	return q.queue.DeleteBefore(before)
}
func closeLegacyHistoryQueue(ctx context.Context) error {
	q := CaptureLegacyHistoryQueue()
	if q.DB == nil {
		return nil
	}
	return q.queue.Close(ctx, q.DB, nil)
}
func forgetLegacyHistoryQueue(db *stdsql.DB) {
	historyQueues.Lock()
	delete(historyQueues.byDB, db)
	delete(historyQueues.eventOwners, db)
	historyQueues.Unlock()
}

func (q LegacyHistoryQueue) ReplaceDirectory(ctx context.Context, name string, items []indexdb.HistoryEntry) error {
	if e := q.guard(ctx); e != nil {
		return e
	}
	q.admitEventOwner()
	return q.queue.ReplaceDirectory(name, items)
}
