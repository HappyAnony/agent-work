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
	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

// Compatibility owns this queue only. Each WorkspaceContext owns another queue.

func FlushHistoryTxJob() { task.AppendTask(task.HistoryDatabaseIndexCommit, FlushHistoryQueue) }

func FlushHistoryQueue() {
	if util.IsExiting.Load() {
		return
	}
	queue := CaptureLegacyHistoryQueue()
	err := queue.Flush(context.Background(), func(p indexdb.HistoryProgress) {
		eventbus.Publish(eventbus.EvtSQLInsertHistory, queue.EventContext(map[string]interface{}{eventbus.CtxPushMsg: eventbus.CtxPushMsgToStatusBar, "current": p.Current, "total": p.Total}))
	})
	if err != nil {
		logging.LogErrorf("history queue failed: %s", err)
		eventbus.Publish(util.EvtSQLHistoryRebuild, queue.EventContext(nil))
	}
}

func DeleteOutdatedHistories(before int64) {
	if err := CaptureLegacyHistoryQueue().DeleteBefore(context.Background(), before); err != nil {
		logging.LogErrorf("history enqueue failed: %s", err)
	}
}

func IndexHistoriesQueue(histories []*History) {
	items := make([]indexdb.HistoryEntry, 0, len(histories))
	for _, h := range histories {
		items = append(items, *h)
	}
	if err := CaptureLegacyHistoryQueue().Index(context.Background(), items); err != nil {
		logging.LogErrorf("history enqueue failed: %s", err)
	}
}
