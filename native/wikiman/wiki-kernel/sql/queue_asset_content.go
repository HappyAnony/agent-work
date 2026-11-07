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
	stdsql "database/sql"
	"errors"
	"sync"

	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

var legacyAssetQueue indexdb.AssetQueue
var legacyAssetEventOwners = struct {
	sync.Mutex
	byDB map[*stdsql.DB]interface{}
}{byDB: map[*stdsql.DB]interface{}{}}

type LegacyAssetQueue struct {
	DB         *stdsql.DB
	queue      *indexdb.AssetQueue
	eventOwner interface{}
}

var ErrAssetQueueOwner = errors.New("legacy_asset_queue_owner_changed")

func CaptureLegacyAssetQueue() LegacyAssetQueue {
	return LegacyAssetQueue{DB: assetContentDB, queue: &legacyAssetQueue, eventOwner: captureLegacySQLEventOwner()}
}

func (q LegacyAssetQueue) EventContext(values map[string]interface{}) map[string]interface{} {
	legacyAssetEventOwners.Lock()
	owner := legacyAssetEventOwners.byDB[q.DB]
	legacyAssetEventOwners.Unlock()
	if owner == nil {
		owner = q.eventOwner
	}
	return LegacySQLEventContext(owner, values)
}

func (q LegacyAssetQueue) admitEventOwner() {
	if q.DB == nil || q.eventOwner == nil {
		return
	}
	legacyAssetEventOwners.Lock()
	legacyAssetEventOwners.byDB[q.DB] = q.eventOwner
	legacyAssetEventOwners.Unlock()
}

func (q LegacyAssetQueue) guard(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.DB == nil || q.DB != assetContentDB || q.queue == nil {
		return ErrAssetQueueOwner
	}
	return nil
}

func (q LegacyAssetQueue) Index(ctx context.Context, items []indexdb.AssetContentEntry) error {
	if err := q.guard(ctx); err != nil {
		return err
	}
	q.admitEventOwner()
	return q.queue.Index(items)
}

func (q LegacyAssetQueue) DeletePath(ctx context.Context, path string) error {
	if err := q.guard(ctx); err != nil {
		return err
	}
	q.admitEventOwner()
	return q.queue.DeletePath(path)
}

func (q LegacyAssetQueue) Flush(ctx context.Context, notify func(indexdb.AssetProgress)) error {
	if err := q.guard(ctx); err != nil {
		return err
	}
	err := q.queue.Flush(ctx, q.DB, notify)
	if err == nil {
		legacyAssetEventOwners.Lock()
		delete(legacyAssetEventOwners.byDB, q.DB)
		legacyAssetEventOwners.Unlock()
	}
	return err
}

func FlushAssetContentTxJob() {
	task.AppendTask(task.AssetContentDatabaseIndexCommit, FlushAssetContentQueue)
}
func FlushAssetContentQueue() {
	if util.IsExiting.Load() {
		return
	}
	queue := CaptureLegacyAssetQueue()
	err := queue.Flush(context.Background(), func(p indexdb.AssetProgress) {
		eventbus.Publish(eventbus.EvtSQLInsertAssetContent, queue.EventContext(map[string]interface{}{eventbus.CtxPushMsg: eventbus.CtxPushMsgToStatusBar, "current": p.Current, "total": p.Total}))
	})
	if err != nil {
		logging.LogErrorf("asset queue failed: %s", err)
		eventbus.Publish(util.EvtSQLAssetContentRebuild, queue.EventContext(nil))
	}
}
func DeleteAssetContentsByPathQueue(path string) {
	if err := CaptureLegacyAssetQueue().DeletePath(context.Background(), path); err != nil {
		logging.LogErrorf("asset enqueue failed: %s", err)
	}
}
func IndexAssetContentsQueue(assets []*AssetContent) {
	items := make([]indexdb.AssetContentEntry, 0, len(assets))
	for _, asset := range assets {
		items = append(items, *asset)
	}
	if err := CaptureLegacyAssetQueue().Index(context.Background(), items); err != nil {
		logging.LogErrorf("asset enqueue failed: %s", err)
	}
}
