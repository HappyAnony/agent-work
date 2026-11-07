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

package job

import (
	"context"
	"sync"
	"time"

	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/model"
	"wiki-kernel/sql"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

// The compat runtime owns one scheduler. Repeated starts do not leak workers,
// and a stopped owner cannot be replaced while its callbacks are still live.
var cronCtxMu sync.Mutex
var cronOwner *taskgroup.Group

func StartCron() {
	cronCtxMu.Lock()
	defer cronCtxMu.Unlock()
	if cronOwner != nil {
		select {
		case <-cronOwner.Done():
		default:
			return
		}
	}
	cronOwner = taskgroup.Open()
	owner := cronOwner
	owner.EveryContext(100*time.Millisecond, 100*time.Millisecond, task.ExecTaskJobContext)
	owner.EveryContext(100*time.Millisecond, 100*time.Millisecond, task.ExecAsyncTaskJobContext)
	owner.EveryContext(7*time.Second, 100*time.Millisecond, task.StatusJobContext)
	scheduleWorkspace(owner, 5*time.Second, model.SyncDataJobContext)
	if statistics, err := model.CaptureStatJob(context.Background()); err != nil {
		logging.LogErrorf("bind workspace statistics: %s", err)
	} else {
		owner.EveryContext(2*time.Hour, 100*time.Millisecond, func(ctx context.Context) {
			if e := statistics(ctx); e != nil && ctx.Err() == nil {
				logging.LogErrorf("workspace statistics: %s", e)
			}
		})
	}
	owner.EveryContext(6*time.Hour, 100*time.Millisecond, func(ctx context.Context) {
		defer logging.Recover()
		util.RefreshRhyResultJobContext(ctx)
	})
	scheduleWorkspace(owner, 2*time.Hour, model.RefreshCheckJobContext)
	scheduleWorkspace(owner, 3*time.Second, model.FlushUpdateRefTextRenameDocJobContext)
	schedule(owner, util.SQLFlushInterval, sql.FlushTxJob)
	schedule(owner, util.SQLFlushInterval, sql.FlushHistoryTxJob)
	schedule(owner, util.SQLFlushInterval, sql.FlushAssetContentTxJob)
	scheduleWorkspace(owner, 10*time.Minute, model.IndexEmbedBlockJobContext)
	scheduleWorkspace(owner, 10*time.Minute, model.CacheVirtualBlockRefJobContext)
	scheduleWorkspace(owner, 30*time.Second, func(ctx context.Context) error { model.OCRAssetsJobContext(ctx); return ctx.Err() })
	scheduleWorkspace(owner, 24*time.Hour, func(ctx context.Context) error { model.AutoPurgeRepoJobContext(ctx); return ctx.Err() })
	scheduleWorkspace(owner, 30*time.Minute, model.AutoCheckMicrosoftDefenderJobContext)

}

func StopCron() {
	cronCtxMu.Lock()
	owner := cronOwner
	cronCtxMu.Unlock()
	if owner != nil {
		owner.Stop()
	}
}

// DrainCron never declares completion while callbacks can still access DBs.
func DrainCron() { _ = DrainCronContext(context.Background()) }
func DrainCronContext(ctx context.Context) error {
	cronCtxMu.Lock()
	owner := cronOwner
	cronCtxMu.Unlock()
	if owner == nil {
		return nil
	}
	return owner.Close(ctx)
}

func schedule(owner *taskgroup.Group, interval time.Duration, fn func(), name ...string) {
	owner.EveryContext(interval, 100*time.Millisecond, func(ctx context.Context) {
		defer logging.Recover()
		if ctx.Err() != nil {
			return
		}
		fn()
		if len(name) > 0 && ctx.Err() == nil {
			logging.LogInfof("cron job [%s] executed", name[0])
		}
	})
}

func scheduleWorkspace(owner *taskgroup.Group, interval time.Duration, fn func(context.Context) error) {
	bound, err := model.CaptureWorkspaceJob(context.Background(), fn)
	if err != nil {
		logging.LogErrorf("bind periodic workspace task: %s", err)
		return
	}
	owner.EveryContext(interval, 100*time.Millisecond, func(ctx context.Context) {
		defer logging.Recover()
		if err := bound(ctx); err != nil && ctx.Err() == nil {
			logging.LogErrorf("periodic workspace task: %s", err)
		}
	})
}
