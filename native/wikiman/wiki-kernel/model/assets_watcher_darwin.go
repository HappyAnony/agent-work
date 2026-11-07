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

//go:build darwin

package model

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/radovskyb/watcher"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/pollwatch"
)

var assetsWatcherMu sync.Mutex
var assetsWatcher *pollwatch.Watch
var openAssetsWatcher = pollwatch.Open

func WatchAssets() {
	if err := WatchAssetsContext(context.Background()); err != nil {
		logging.LogErrorf("open assets watcher failed: %s", err)
	}
}

func WatchAssetsContext(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	watchOwner, err := captureWorkspaceWatcherOwner(ctx)
	if err != nil {
		return err
	}
	assetsWatcherMu.Lock()
	defer assetsWatcherMu.Unlock()
	if assetsWatcher != nil {
		if err := assetsWatcher.Close(ctx); err != nil {
			return err
		}
		assetsWatcher = nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	folder := filepath.Join(watchOwner.data, "assets")
	assetOwner, err := captureAssetWatchContext(ctx)
	if err != nil {
		return err
	}
	owner, err := openAssetsWatcher(ctx, folder, 10*time.Second, func(ownerCtx context.Context, event watcher.Event) {
		if err := watchOwner.guard(ownerCtx); err != nil {
			return
		}
		if err := assetOwner.dispatch(ownerCtx, assetflow.Event{Path: event.Path, Removed: watcher.Remove == event.Op, Written: watcher.Write == event.Op}); err != nil && ownerCtx.Err() == nil {
			logging.LogErrorf("asset callback failed: %s", err)
		}
	}, func(err error) { logging.LogErrorf("watch assets failed: %s", err) })
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		_ = owner.Close(context.Background())
		return err
	}
	if !watchOwner.ownWorkspaceWatcher(owner) {
		_ = owner.Close(context.Background())
		return errors.New("assets_watcher_owner_closed")
	}
	assetsWatcher = owner
	return nil
}

func StopWatchAssets() {
	assetsWatcherMu.Lock()
	defer assetsWatcherMu.Unlock()
	if assetsWatcher != nil {
		assetsWatcher.Stop()
	}
}

func CloseWatchAssets() {
	if err := CloseWatchAssetsContext(context.Background()); err != nil {
		logging.LogErrorf("close assets watcher failed: %s", err)
	}
}

func CloseWatchAssetsContext(ctx context.Context) error {
	assetsWatcherMu.Lock()
	defer assetsWatcherMu.Unlock()
	if assetsWatcher != nil {
		if err := assetsWatcher.Close(ctx); err != nil {
			return err
		}
		assetsWatcher = nil
	}
	return nil
}
